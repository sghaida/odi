package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func minimalValidSpecJSON() []byte {
	return []byte(`{
  "package": "svc",
  "wrapperBase": "User",
  "versionSuffix": "V1",
  "implType": "Service",
  "constructor": "NewService",
  "imports": { "config": "example.com/project/autowire/config" },
  "required": [
    { "name": "DB", "field": "db", "type": "*sql.DB" }
  ]
}`)
}

type fakeTemp struct {
	name     string
	writeErr error
	closeErr error
}

func (f *fakeTemp) Name() string { return f.name }
func (f *fakeTemp) Write(p []byte) (int, error) {
	if f.writeErr != nil {
		return 0, f.writeErr
	}
	return len(p), nil
}
func (f *fakeTemp) Close() error { return f.closeErr }

func restoreWriteFileSeams(t *testing.T) (origCreate func(string, string) (tempFile, error), origRemove func(string) error, origChmod func(string, os.FileMode) error, origRename func(string, string) error) {
	t.Helper()
	return createTemp, removeFn, chmodFn, renameFn
}

func setWriteFileSeams(t *testing.T,
	newCreate func(string, string) (tempFile, error),
	newRemove func(string) error,
	newChmod func(string, os.FileMode) error,
	newRename func(string, string) error,
) {
	t.Helper()
	if newCreate != nil {
		createTemp = newCreate
	}
	if newRemove != nil {
		removeFn = newRemove
	}
	if newChmod != nil {
		chmodFn = newChmod
	}
	if newRename != nil {
		renameFn = newRename
	}
}

// NOT parallel:
// - Uses run() which calls writeFileAtomic()
// - writeFileAtomic uses global seam variables (createTemp/removeFn/chmodFn/renameFn)
// - other tests mutate those seams, so running in parallel causes flakiness.
func TestRun(t *testing.T) {
	cases := []struct {
		name          string
		setup         func(t *testing.T) (args []string, outPath string)
		wantCode      int
		wantStderrSub string
		wantOutSub    string
	}{
		{
			name: "missing flags prints usage",
			setup: func(t *testing.T) ([]string, string) {
				return []string{}, ""
			},
			wantCode:      2,
			wantStderrSub: "usage: di1 -spec",
		},
		{
			name: "flag parse error returns 2",
			setup: func(t *testing.T) ([]string, string) {
				return []string{"-nope"}, ""
			},
			wantCode: 2,
		},
		{
			name: "success generates file and defaults facade name",
			setup: func(t *testing.T) ([]string, string) {
				dir := t.TempDir()
				specPath := filepath.Join(dir, "service.inject.json")
				outPath := filepath.Join(dir, "out.gen.go")

				require.NoError(t, os.WriteFile(specPath, minimalValidSpecJSON(), 0o644))
				return []string{"-spec", specPath, "-out", outPath}, outPath
			},
			wantCode:   0,
			wantOutSub: "type UserV1 struct",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			args, outPath := tc.setup(t)

			var stderr bytes.Buffer
			code := run(args, &stderr)

			require.Equal(t, tc.wantCode, code)

			if tc.wantStderrSub != "" {
				assert.Contains(t, stderr.String(), tc.wantStderrSub)
			}

			if tc.wantOutSub != "" {
				b, err := os.ReadFile(outPath)
				require.NoError(t, err)
				assert.Contains(t, string(b), tc.wantOutSub)
				assert.Contains(t, string(b), "func NewUserV1")
			}
		})
	}
}

// NOT parallel:
// - Mutates global seam variables (createTemp/removeFn/chmodFn/renameFn)
// - Must not overlap with any other test touching writeFileAtomic or run().
func TestWriteFileAtomic_Errors(t *testing.T) {
	type stubs struct {
		create func(dir, pattern string) (tempFile, error)
		remove func(path string) error
		chmod  func(path string, mode os.FileMode) error
		rename func(oldpath, newpath string) error
	}

	cases := []struct {
		name           string
		stubs          stubs
		wantErrSub     string
		wantRemovedCnt int
	}{
		{
			name: "create temp error",
			stubs: stubs{
				create: func(dir, pattern string) (tempFile, error) { return nil, errors.New("create temp failed") },
			},
			wantErrSub:     "create temp failed",
			wantRemovedCnt: 0,
		},
		{
			name: "write error removes temp",
			stubs: stubs{
				create: func(dir, pattern string) (tempFile, error) {
					return &fakeTemp{name: filepath.Join(dir, "tmpfile"), writeErr: errors.New("write failed")}, nil
				},
				remove: func(path string) error { return nil },
			},
			wantErrSub:     "write failed",
			wantRemovedCnt: 1,
		},
		{
			name: "close error removes temp",
			stubs: stubs{
				create: func(dir, pattern string) (tempFile, error) {
					return &fakeTemp{name: filepath.Join(dir, "tmpfile"), closeErr: errors.New("close failed")}, nil
				},
				remove: func(path string) error { return nil },
			},
			wantErrSub:     "close failed",
			wantRemovedCnt: 1,
		},
		{
			name: "chmod error removes temp",
			stubs: stubs{
				create: func(dir, pattern string) (tempFile, error) {
					return &fakeTemp{name: filepath.Join(dir, "tmpfile")}, nil
				},
				chmod:  func(path string, mode os.FileMode) error { return errors.New("chmod failed") },
				remove: func(path string) error { return nil },
			},
			wantErrSub:     "chmod failed",
			wantRemovedCnt: 1,
		},
		{
			name: "rename error propagates",
			stubs: stubs{
				create: func(dir, pattern string) (tempFile, error) {
					return &fakeTemp{name: filepath.Join(dir, "tmpfile")}, nil
				},
				chmod:  func(path string, mode os.FileMode) error { return nil },
				rename: func(oldpath, newpath string) error { return errors.New("rename failed") },
			},
			wantErrSub:     "rename failed",
			wantRemovedCnt: 1,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			origCreate, origRemove, origChmod, origRename := restoreWriteFileSeams(t)
			t.Cleanup(func() {
				createTemp = origCreate
				removeFn = origRemove
				chmodFn = origChmod
				renameFn = origRename
			})

			var removed []string

			setWriteFileSeams(t,
				tc.stubs.create,
				func(path string) error {
					removed = append(removed, path)
					if tc.stubs.remove != nil {
						return tc.stubs.remove(path)
					}
					return nil
				},
				func(path string, mode os.FileMode) error {
					if tc.stubs.chmod != nil {
						return tc.stubs.chmod(path, mode)
					}
					return nil
				},
				func(oldpath, newpath string) error {
					if tc.stubs.rename != nil {
						return tc.stubs.rename(oldpath, newpath)
					}
					return nil
				},
			)

			err := writeFileAtomic(filepath.Join(t.TempDir(), "out.go"), []byte("x"), 0o644)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantErrSub)
			assert.Len(t, removed, tc.wantRemovedCnt)
		})
	}
}

// NOT parallel:
// - Calls writeFileAtomic (uses global seam variables).
// - Must not overlap with tests that stub seams.
func TestWriteFileAtomic_Success_WritesAndRenames(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "final.go")

	require.NoError(t, writeFileAtomic(out, []byte("hello"), 0o644))

	b, err := os.ReadFile(out)
	require.NoError(t, err)
	assert.Equal(t, "hello", string(b))
}

// Can run in parallel:
// - Does not mutate global seams
// - Does not call run() / writeFileAtomic()
func TestGenTemplate_RendersExpectedBits(t *testing.T) {
	t.Parallel()

	spec := Spec{
		Package:       "svc",
		WrapperBase:   "User",
		VersionSuffix: "V1",
		ImplType:      "Service",
		Constructor:   "NewService",
		FacadeName:    "UserV1",
		Imports:       Imports{Config: "example.com/project/autowire/config"},
		Required: []Dep{
			{Name: "DB", Field: "db", Type: "*sql.DB"},
		},
	}

	var b strings.Builder
	require.NoError(t, genTemplate.Execute(&b, spec))

	out := b.String()
	assert.Contains(t, out, "type UserV1 struct")
	assert.Contains(t, out, "func NewUserV1")
	assert.Contains(t, out, "func (b *UserV1) InjectDB")
	assert.Contains(t, out, `return nil, fmt.Errorf("UserV1 not wired: missing required dep DB")`)
}

// Can run in parallel:
// - Does not mutate seams
// - Only tests panic behavior of must()
func TestMust(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name         string
		err          error
		wantPanic    bool
		wantPanicMsg string
	}{
		{name: "nil does not panic", err: nil, wantPanic: false},
		{name: "non-nil panics", err: errors.New("boom"), wantPanic: true, wantPanicMsg: "boom"},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			if tc.wantPanic {
				require.PanicsWithError(t, tc.wantPanicMsg, func() { must(tc.err) })
				return
			}
			require.NotPanics(t, func() { must(tc.err) })
		})
	}
}

// Can run in parallel:
// - Does not mutate seams
// - Only calls validateSpec (pure function over input struct)
func TestValidateSpec(t *testing.T) {
	t.Parallel()

	base := func() Spec {
		return Spec{
			Package:       "svc",
			WrapperBase:   "User",
			VersionSuffix: "V1",
			ImplType:      "Service",
			Constructor:   "NewService",
			Imports:       Imports{Config: "example.com/project/autowire/config"},
			Required: []Dep{
				{Name: "DB", Field: "db", Type: "*sql.DB"},
			},
		}
	}

	cases := []struct {
		name         string
		mutate       func(s *Spec)
		wantPanic    bool
		wantPanicSub string
	}{
		{name: "ok", mutate: func(s *Spec) {}, wantPanic: false},
		{name: "missing package (trimspace)", mutate: func(s *Spec) { s.Package = "   " }, wantPanic: true, wantPanicSub: "spec missing required fields: [package]"},
		{name: "missing wrapperBase (trimspace)", mutate: func(s *Spec) { s.WrapperBase = "" }, wantPanic: true, wantPanicSub: "spec missing required fields: [wrapperBase]"},
		{name: "missing versionSuffix (trimspace)", mutate: func(s *Spec) { s.VersionSuffix = " " }, wantPanic: true, wantPanicSub: "spec missing required fields: [versionSuffix]"},
		{name: "missing implType (trimspace)", mutate: func(s *Spec) { s.ImplType = " " }, wantPanic: true, wantPanicSub: "spec missing required fields: [implType]"},
		{name: "missing constructor (trimspace)", mutate: func(s *Spec) { s.Constructor = "" }, wantPanic: true, wantPanicSub: "spec missing required fields: [constructor]"},
		{name: "missing imports.config (trimspace)", mutate: func(s *Spec) { s.Imports.Config = "   " }, wantPanic: true, wantPanicSub: "spec missing required fields: [imports.config]"},
		{name: "required empty triggers missing required message", mutate: func(s *Spec) { s.Required = nil }, wantPanic: true, wantPanicSub: "spec missing required fields: [required (must have at least 1)]"},
		{name: "dep missing field triggers dep validation panic", mutate: func(s *Spec) { s.Required = []Dep{{Name: "DB", Field: "", Type: "*sql.DB"}} }, wantPanic: true, wantPanicSub: "each dep must have name/field/type"},
		{name: "duplicate dep name across required+optional", mutate: func(s *Spec) { s.Optional = []Dep{{Name: "DB", Field: "db2", Type: "*sql.DB"}} }, wantPanic: true, wantPanicSub: "duplicate dep name: DB"},
		{name: "duplicate dep field across required+optional", mutate: func(s *Spec) { s.Optional = []Dep{{Name: "Cache", Field: "db", Type: "any"}} }, wantPanic: true, wantPanicSub: "duplicate dep field: db"},
		{name: "multiple missing fields collected together", mutate: func(s *Spec) { s.Package, s.Constructor, s.Imports.Config = " ", " ", " " }, wantPanic: true, wantPanicSub: "spec missing required fields: [package constructor imports.config]"},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			s := base()
			tc.mutate(&s)

			if tc.wantPanic {
				func() {
					defer func() {
						r := recover()
						require.NotNil(t, r)

						msg := ""
						switch v := r.(type) {
						case error:
							msg = v.Error()
						case string:
							msg = v
						}
						require.NotEmpty(t, msg)
						assert.Contains(t, msg, tc.wantPanicSub)
					}()
					validateSpec(&s)
				}()
				return
			}

			require.NotPanics(t, func() { validateSpec(&s) })
		})
	}
}

// NOT parallel:
// - Uses run() which calls writeFileAtomic (global seams).
// - Also changes process working directory via os.Chdir (global process state).
func TestRun_RelativeOutPath_IsCleaned(t *testing.T) {
	dir := t.TempDir()

	oldWD, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.Chdir(oldWD) })
	require.NoError(t, os.Chdir(dir))

	specPath := filepath.Join(dir, "service.inject.json")
	require.NoError(t, os.WriteFile(specPath, minimalValidSpecJSON(), 0o644))

	relOut := filepath.Join(".", "subdir", "..", "gen", "out.gen.go")
	cleanOut := filepath.Clean(relOut)

	require.NoError(t, os.MkdirAll(filepath.Dir(cleanOut), 0o755))

	var stderr bytes.Buffer
	code := run([]string{"-spec", specPath, "-out", relOut}, &stderr)
	require.Equal(t, 0, code)

	b, err := os.ReadFile(cleanOut)
	require.NoError(t, err)
	assert.Contains(t, string(b), "type UserV1 struct")
}
