// main_test.go
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

//
// -----------------------------------------------------------------------------
// must()
// -----------------------------------------------------------------------------

func TestMust_PanicsOnError(t *testing.T) {
	t.Parallel()

	require.NotPanics(t, func() { must(nil) })
	require.PanicsWithError(t, "boom", func() { must(errors.New("boom")) })
}

//
// -----------------------------------------------------------------------------
// writeFileAtomic()
// -----------------------------------------------------------------------------

func TestWriteFileAtomic_ErrorBranches(t *testing.T) {
	// NOT parallel: mutates global seams.

	type seams struct {
		createTemp func(dir, pattern string) (tempFile, error)
		removeTmp  func(path string) error
		chmodTmp   func(path string, mode os.FileMode) error
		renameTmp  func(oldpath, newpath string) error
	}

	tests := []struct {
		name        string
		seams       seams
		wantErrSub  string
		wantRemoves int
	}{
		{
			name: "create temp error",
			seams: seams{
				createTemp: func(dir, pattern string) (tempFile, error) {
					return nil, errors.New("create temp failed")
				},
			},
			wantErrSub:  "create temp failed",
			wantRemoves: 0,
		},
		{
			name: "write error removes temp via deferred cleanup",
			seams: seams{
				createTemp: func(dir, pattern string) (tempFile, error) {
					return &fakeTempFile{
						fileName: filepath.Join(dir, "tmpfile"),
						writeErr: errors.New("write failed"),
					}, nil
				},
				removeTmp: func(path string) error { return nil },
			},
			wantErrSub:  "write failed",
			wantRemoves: 1,
		},
		{
			name: "close error removes temp via deferred cleanup",
			seams: seams{
				createTemp: func(dir, pattern string) (tempFile, error) {
					return &fakeTempFile{
						fileName: filepath.Join(dir, "tmpfile"),
						closeErr: errors.New("close failed"),
					}, nil
				},
				removeTmp: func(path string) error { return nil },
			},
			wantErrSub:  "close failed",
			wantRemoves: 1,
		},
		{
			name: "chmod error removes temp via deferred cleanup",
			seams: seams{
				createTemp: func(dir, pattern string) (tempFile, error) {
					return &fakeTempFile{fileName: filepath.Join(dir, "tmpfile")}, nil
				},
				chmodTmp:  func(path string, mode os.FileMode) error { return errors.New("chmod failed") },
				removeTmp: func(path string) error { return nil },
			},
			wantErrSub:  "chmod failed",
			wantRemoves: 1,
		},
		{
			name: "rename error removes temp via deferred cleanup",
			seams: seams{
				createTemp: func(dir, pattern string) (tempFile, error) {
					return &fakeTempFile{fileName: filepath.Join(dir, "tmpfile")}, nil
				},
				chmodTmp:  func(path string, mode os.FileMode) error { return nil },
				renameTmp: func(oldpath, newpath string) error { return errors.New("rename failed") },
				removeTmp: func(path string) error { return nil },
			},
			wantErrSub:  "rename failed",
			wantRemoves: 1,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			origCreate, origRemove, origChmod, origRename := snapWriteSeams(t)
			t.Cleanup(func() {
				createTempFile = origCreate
				removeFile = origRemove
				chmodFile = origChmod
				renameFile = origRename
			})

			var removed []string

			setWriteSeams(
				t,
				tc.seams.createTemp,
				func(path string) error {
					removed = append(removed, path)
					if tc.seams.removeTmp != nil {
						return tc.seams.removeTmp(path)
					}
					return nil
				},
				func(path string, mode os.FileMode) error {
					if tc.seams.chmodTmp != nil {
						return tc.seams.chmodTmp(path, mode)
					}
					return nil
				},
				func(oldpath, newpath string) error {
					if tc.seams.renameTmp != nil {
						return tc.seams.renameTmp(oldpath, newpath)
					}
					return nil
				},
			)

			err := writeFileAtomic(filepath.Join(t.TempDir(), "out.go"), []byte("x"), 0o644)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantErrSub)
			assert.Len(t, removed, tc.wantRemoves)
		})
	}
}

func TestWriteFileAtomic_Success(t *testing.T) {
	// NOT parallel: uses real filesystem but does not mutate seams.
	tempDir := t.TempDir()
	out := filepath.Join(tempDir, "final.go")

	require.NoError(t, writeFileAtomic(out, []byte("hello"), 0o644))
	assert.Equal(t, "hello", readFileString(t, out))
}

//
// -----------------------------------------------------------------------------
// validateSpec()
// -----------------------------------------------------------------------------

func TestValidateSpec_Branches(t *testing.T) {
	t.Parallel()

	base := func() Spec {
		return Spec{
			Package:       "svc",
			WrapperBase:   "User",
			VersionSuffix: "V1",
			ImplType:      "Service",
			Constructor:   "NewService",
			Required: []Dep{
				{Name: "DB", Field: "db", Type: "*sql.DB"},
			},
			Optional: []Dep{
				{Name: "Logger", Field: "logger", Type: "Logger"},
			},
		}
	}

	tests := []struct {
		name      string
		mutate    func(*Spec)
		wantPanic bool
	}{
		{
			name:      "ok",
			mutate:    func(*Spec) {},
			wantPanic: false,
		},
		{
			name: "missing required fields collected",
			mutate: func(s *Spec) {
				s.Package = "   "
				s.Constructor = " "
				s.Required = nil
			},
			wantPanic: true,
		},
		{
			name: "dep missing field panics",
			mutate: func(s *Spec) {
				s.Required = []Dep{{Name: "DB", Field: "", Type: "*sql.DB"}}
			},
			wantPanic: true,
		},
		{
			name: "duplicate dep name across required+optional panics",
			mutate: func(s *Spec) {
				s.Optional = []Dep{{Name: "DB", Field: "db2", Type: "*sql.DB"}}
			},
			wantPanic: true,
		},
		{
			name: "duplicate dep field across required+optional panics",
			mutate: func(s *Spec) {
				s.Optional = []Dep{{Name: "Cache", Field: "db", Type: "any"}}
			},
			wantPanic: true,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			spec := base()
			tc.mutate(&spec)

			if tc.wantPanic {
				require.Panics(t, func() { validateSpec(&spec) })
				return
			}
			require.NotPanics(t, func() { validateSpec(&spec) })
		})
	}
}

//
// -----------------------------------------------------------------------------
// readImportsFromFile / ensureImport / containsAlias / containsPath
// -----------------------------------------------------------------------------

func TestReadImportsFromFile(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		source  string
		wantErr bool
		check   func(t *testing.T, imports []ImportSpec)
	}{
		{
			name:    "parse error",
			source:  "package", // invalid
			wantErr: true,
		},
		{
			name: "parses imports and aliases",
			source: `package svc

import (
	"fmt"
	config "example.com/project/autowire/config"
	_ "net/http"
)
`,
			wantErr: false,
			check: func(t *testing.T, imports []ImportSpec) {
				assert.True(t, containsPath(imports, "fmt"))
				assert.True(t, containsPath(imports, "example.com/project/autowire/config"))
				assert.True(t, containsAlias(imports, "config"))
				assert.True(t, containsAlias(imports, "_"))
			},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			p := writeTempFile(t, dir, "file.go", tc.source, 0o644)

			imps, err := readImportsFromFile(p)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			if tc.check != nil {
				tc.check(t, imps)
			}
		})
	}
}

func TestEnsureImport_NoDupByPath(t *testing.T) {
	t.Parallel()

	var imps []ImportSpec
	ensureImport(&imps, ImportSpec{Path: "fmt"})
	ensureImport(&imps, ImportSpec{Path: "fmt"}) // no-op

	require.Len(t, imps, 1)
	assert.Equal(t, "fmt", imps[0].Path)
}

func TestContainsAliasPath(t *testing.T) {
	t.Parallel()

	imps := []ImportSpec{
		{Alias: "", Path: "fmt"},
		{Alias: "config", Path: "example.com/project/autowire/config"},
	}

	assert.True(t, containsPath(imps, "fmt"))
	assert.False(t, containsPath(imps, "nope"))

	assert.True(t, containsAlias(imps, "config"))
	assert.False(t, containsAlias(imps, ""))        // alias must be non-empty
	assert.False(t, containsAlias(imps, "missing")) // absent
}

//
// -----------------------------------------------------------------------------
// resolveImports()
// -----------------------------------------------------------------------------

func TestResolveImports_Branches(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		setup      func(t *testing.T) (ownerFile string, spec *Spec, needsConfig bool)
		wantErrSub string
		check      func(t *testing.T, imports []ImportSpec)
	}{
		{
			name: "owner parse error falls back; uses spec config import",
			setup: func(t *testing.T) (string, *Spec, bool) {
				dir := t.TempDir()
				owner := filepath.Join(dir, "bad.go")
				require.NoError(t, os.WriteFile(owner, []byte("package"), 0o644)) // invalid

				return owner, &Spec{
					Constructor: "NewService",
					Imports:     Imports{Config: "example.com/project/autowire/config"},
				}, true
			},
			check: func(t *testing.T, imports []ImportSpec) {
				assert.True(t, containsPath(imports, "fmt"))
				assert.True(t, containsAlias(imports, "config"))
				assert.True(t, containsPath(imports, "example.com/project/autowire/config"))
			},
		},
		{
			name: "does not need config returns early (fmt ensured)",
			setup: func(t *testing.T) (string, *Spec, bool) {
				return "", &Spec{}, false
			},
			check: func(t *testing.T, imports []ImportSpec) {
				assert.True(t, containsPath(imports, "fmt"))
				assert.False(t, containsAlias(imports, "config"))
			},
		},
		{
			name: "owner already has alias config returns early",
			setup: func(t *testing.T) (string, *Spec, bool) {
				dir := t.TempDir()
				owner := filepath.Join(dir, "owner.go")
				src := `package svc

import (
	config "example.com/owner/config"
)
`
				require.NoError(t, os.WriteFile(owner, []byte(src), 0o644))

				return owner, &Spec{
					Constructor: "NewService",
					Imports:     Imports{Config: "example.com/spec/config"},
				}, true
			},
			check: func(t *testing.T, imports []ImportSpec) {
				assert.True(t, containsAlias(imports, "config"))
				assert.True(t, containsPath(imports, "example.com/owner/config"))
				assert.True(t, containsPath(imports, "fmt"))
				assert.False(t, containsPath(imports, "example.com/spec/config"))
			},
		},
		{
			name: "needs config but spec imports.config empty returns error",
			setup: func(t *testing.T) (string, *Spec, bool) {
				return "", &Spec{
					Constructor: "NewService",
					Imports:     Imports{Config: ""},
				}, true
			},
			wantErrSub: "spec.imports.config is empty",
		},
		{
			name: "config path already imported without alias is ok",
			setup: func(t *testing.T) (string, *Spec, bool) {
				dir := t.TempDir()
				owner := filepath.Join(dir, "owner.go")
				src := `package svc

import (
	"example.com/project/autowire/config"
)
`
				require.NoError(t, os.WriteFile(owner, []byte(src), 0o644))

				return owner, &Spec{
					Constructor: "NewService",
					Imports:     Imports{Config: "example.com/project/autowire/config"},
				}, true
			},
			check: func(t *testing.T, imports []ImportSpec) {
				assert.True(t, containsPath(imports, "fmt"))
				assert.True(t, containsPath(imports, "example.com/project/autowire/config"))
			},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			owner, spec, needs := tc.setup(t)

			imps, err := resolveImports(owner, spec, needs)
			if tc.wantErrSub != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErrSub)
				return
			}

			require.NoError(t, err)
			if tc.check != nil {
				tc.check(t, imps)
			}
		})
	}
}

//
// -----------------------------------------------------------------------------
// determineConstructorNeedsConfig()
// -----------------------------------------------------------------------------

func TestCtorNeedsConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		override *bool
		files    map[string]string
		want     bool
		missing  bool
	}{
		{
			name:     "override true",
			override: boolPtr(true),
			want:     true,
		},
		{
			name:     "override false",
			override: boolPtr(false),
			want:     false,
		},
		{
			name:    "ReadDir error defaults true",
			missing: true,
			want:    true,
		},
		{
			name: "skips misc decls and finds config.Config",
			files: map[string]string{
				"bad.go": "package", // parse error -> skipped
				"svc.go": `package svc

var x = 1

type T struct{}
func (t *T) NewService() {}
func Other() {}

func NewService(cfg config.Config) {}
`,
			},
			want: true,
		},
		{
			name: "no params => false",
			files: map[string]string{
				"svc.go": `package svc
func NewService() {}
`,
			},
			want: false,
		},
		{
			name: "one param but not selector => true",
			files: map[string]string{
				"svc.go": `package svc
func NewService(x int) {}
`,
			},
			want: true,
		},
		{
			name: "other.Config => true",
			files: map[string]string{
				"svc.go": `package svc
func NewService(cfg other.Config) {}
`,
			},
			want: true,
		},
		{
			name: "constructor not found => true",
			files: map[string]string{
				"svc.go": "package svc\n",
			},
			want: true,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			spec := &Spec{Constructor: "NewService", ConstructorTakesConfig: tc.override}

			if tc.missing {
				dir := filepath.Join(t.TempDir(), "does-not-exist")
				assert.Equal(t, tc.want, determineConstructorNeedsConfig(spec, dir))
				return
			}

			dir := t.TempDir()

			// covers entry.IsDir() skip
			require.NoError(t, os.Mkdir(filepath.Join(dir, "subdir"), 0o755))

			for name, src := range tc.files {
				writeTempFile(t, dir, name, src, 0o644)
			}

			assert.Equal(t, tc.want, determineConstructorNeedsConfig(spec, dir))
		})
	}
}

//
// -----------------------------------------------------------------------------
// findOwnerGoGenerateFile()
// -----------------------------------------------------------------------------

func TestFindOwnerFile(t *testing.T) {
	// NOT parallel: uses symlink (may be skipped).

	tests := []struct {
		name    string
		setup   func(t *testing.T) string
		wantErr bool
		wantSfx string
	}{
		{
			name: "ReadDir error",
			setup: func(t *testing.T) string {
				return filepath.Join(t.TempDir(), "does-not-exist")
			},
			wantErr: true,
		},
		{
			name: "skips junk and finds owner",
			setup: func(t *testing.T) string {
				dir := t.TempDir()

				// IsDir skip
				require.NoError(t, os.Mkdir(filepath.Join(dir, "00_dir"), 0o755))

				// suffix filters
				writeTempFile(t, dir, "01_readme.md", "ignore", 0o644)
				writeTempFile(t, dir, "02_owner_test.go", "package svc\n", 0o644)

				// ReadFile error skip
				_ = makeUnreadableGoFile(t, dir, "03_broken.go")

				// Non-matching go file
				writeTempFile(t, dir, "04_other.go", "package svc\n", 0o644)

				// Matching owner file (sorted last)
				owner := filepath.Join(dir, "zz_owner.go")
				src := `package svc

//go:generate go run ../../cmd/di1 -spec ./specs/x.inject.json -out ./x.gen.go
`
				require.NoError(t, os.WriteFile(owner, []byte(src), 0o644))
				return dir
			},
			wantErr: false,
			wantSfx: "zz_owner.go",
		},
		{
			name: "no match",
			setup: func(t *testing.T) string {
				dir := t.TempDir()
				writeTempFile(t, dir, "a.go", "package svc\n", 0o644)
				return dir
			},
			wantErr: true,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			dir := tc.setup(t)

			found, err := findOwnerGoGenerateFile(dir)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.True(t, strings.HasSuffix(found, tc.wantSfx))
		})
	}
}

func TestFindOwnerFile_SkipsDirAndReadError(t *testing.T) {
	// NOT parallel: relies on filesystem entries.
	dir := t.TempDir()

	// IsDir == true branch
	require.NoError(t, os.Mkdir(filepath.Join(dir, "00_dir"), 0o755))

	// ReadFile error branch
	_ = makeUnreadableGoFile(t, dir, "01_broken.go")

	// suffix skip files
	writeTempFile(t, dir, "02_readme.md", "ignore", 0o644)
	writeTempFile(t, dir, "03_owner_test.go", "package svc\n", 0o644)

	// matching owner file must sort last
	want := filepath.Join(dir, "zz_owner.go")
	require.NoError(t, os.WriteFile(want, []byte(`package svc

//go:generate go run ../../cmd/di1 -spec ./specs/x.inject.json -out ./x.gen.go
`), 0o644))

	found, err := findOwnerGoGenerateFile(dir)
	require.NoError(t, err)
	assert.Equal(t, want, found)
}

//
// -----------------------------------------------------------------------------
// Template rendering (smoke)
// -----------------------------------------------------------------------------

func TestTemplateSmoke(t *testing.T) {
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

	data := templateData{
		Spec:        spec,
		NeedsConfig: true,
		ConfigAlias: "config",
		ImportsList: []ImportSpec{
			{Path: "fmt"},
			{Alias: "config", Path: spec.Imports.Config},
		},
	}

	var b strings.Builder
	require.NoError(t, genTemplate.Execute(&b, data))

	out := b.String()
	assert.Contains(t, out, "type UserV1 struct")
	assert.Contains(t, out, "func NewUserV1")
	assert.Contains(t, out, "InjectDB")
}

//
// -----------------------------------------------------------------------------
// run(): relative out path cleaning
// -----------------------------------------------------------------------------

func TestRun_CleansRelativeOutPath(t *testing.T) {
	// NOT parallel:
	// - uses run() which calls writeFileAtomic
	// - changes process CWD

	tmp := t.TempDir()

	oldWD, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.Chdir(oldWD) })

	require.NoError(t, os.Chdir(tmp))

	specPath := filepath.Join(tmp, "service.inject.json")
	require.NoError(t, os.WriteFile(specPath, minimalSpecJSON(), 0o644))

	relOut := filepath.Join(".", "subdir", "..", "gen", "out.gen.go")
	cleanOut := filepath.Clean(relOut)

	require.NoError(t, os.MkdirAll(filepath.Dir(cleanOut), 0o755))

	var stderr bytes.Buffer
	code := run([]string{"-spec", specPath, "-out", relOut}, &stderr)
	require.Equal(t, 0, code)

	assert.Contains(t, readFileString(t, cleanOut), "type UserV1 struct")
}

//
// -----------------------------------------------------------------------------
// run(): error branches
// -----------------------------------------------------------------------------

func TestRun_Errors(t *testing.T) {
	// NOT parallel: filesystem + generation

	tests := []struct {
		name      string
		args      func(t *testing.T) []string
		wantCode  *int
		wantErr   string
		wantPanic string
	}{
		{
			name: "flag parse error => 2",
			args: func(t *testing.T) []string {
				return []string{"-nope"}
			},
			wantCode: intPtr(2),
		},
		{
			name: "missing flags => usage + 2",
			args: func(t *testing.T) []string {
				return []string{}
			},
			wantCode: intPtr(2),
			wantErr:  "usage: di1 -spec",
		},
		{
			name: "resolveImports error panics (needs config but empty spec.imports.config)",
			args: func(t *testing.T) []string {
				dir := t.TempDir()

				// Owner file so findOwnerGoGenerateFile succeeds
				owner := filepath.Join(dir, "zz_owner.go")
				require.NoError(t, os.WriteFile(owner, []byte(`package svc

//go:generate go run ../../cmd/di1 -spec ./service.inject.json -out ./out.gen.go
`), 0o644))

				// Spec forces NeedsConfig=true but provides no fallback import
				specPath := filepath.Join(dir, "service.inject.json")
				require.NoError(t, os.WriteFile(specPath, []byte(`{
  "package": "svc",
  "wrapperBase": "User",
  "versionSuffix": "V1",
  "implType": "Service",
  "constructor": "NewService",
  "imports": { "config": "" },
  "required": [
    { "name": "DB", "field": "db", "type": "*sql.DB" }
  ]
}`), 0o644))

				// Make determineConstructorNeedsConfig return true
				require.NoError(t, os.WriteFile(filepath.Join(dir, "svc.go"), []byte(`package svc

import config "example.com/project/autowire/config"

func NewService(cfg config.Config) {}
`), 0o644))

				out := filepath.Join(dir, "out.gen.go")
				return []string{"-spec", specPath, "-out", out}
			},
			wantPanic: "spec.imports.config is empty",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			args := tc.args(t)
			var stderr bytes.Buffer

			if tc.wantPanic != "" {
				mustPanicContains(t, tc.wantPanic, func() {
					_ = run(args, &stderr)
				})
				return
			}

			code := run(args, &stderr)
			require.NotNil(t, tc.wantCode)
			require.Equal(t, *tc.wantCode, code)

			if tc.wantErr != "" {
				assert.Contains(t, stderr.String(), tc.wantErr)
			}
		})
	}
}

//
// -----------------------------------------------------------------------------
// Coverage-focused: determineConstructorNeedsConfig suffix continues
// -----------------------------------------------------------------------------

func TestCtorNeedsConfig_SkipsSuffixes(t *testing.T) {
	// NOT parallel: filesystem order sensitive for coverage.
	dir := t.TempDir()

	// Hits:
	// - not .go
	// - _test.go
	// - .gen.go
	writeTempFile(t, dir, "00_notes.txt", "ignore", 0o644)
	writeTempFile(t, dir, "01_svc_test.go", "package svc\n", 0o644)
	writeTempFile(t, dir, "02_svc.gen.go", "package svc\n", 0o644)

	// real constructor
	writeTempFile(t, dir, "zz_svc.go", `package svc
func NewService(cfg config.Config) {}
`, 0o644)

	spec := &Spec{Constructor: "NewService"}
	assert.True(t, determineConstructorNeedsConfig(spec, dir))
}
