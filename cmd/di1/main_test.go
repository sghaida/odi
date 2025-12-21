package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

//
// -----------------------------------------------------------------------------
// Fixtures
// -----------------------------------------------------------------------------

// minimalValidSpecJSON returns a minimal inject spec that passes validateSpec
// and allows run() to generate output.
//
// We include imports.config as a fallback to ensure generation still succeeds
// even when owner-file import discovery fails (by design in some tests).
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

//
// -----------------------------------------------------------------------------
// Helpers
// -----------------------------------------------------------------------------

func ptrBool(v bool) *bool { return &v }

// requirePanicContains asserts fn panics and the panic message contains wantSub.
func requirePanicContains(t *testing.T, wantSub string, fn func()) {
	t.Helper()

	defer func() {
		recovered := recover()
		require.NotNil(t, recovered)

		var message string
		switch v := recovered.(type) {
		case error:
			message = v.Error()
		case string:
			message = v
		default:
			message = fmt.Sprintf("%v", v)
		}
		require.Contains(t, message, wantSub)
	}()

	fn()
}

//
// -----------------------------------------------------------------------------
// writeFileAtomic() seam helpers
// -----------------------------------------------------------------------------

// fakeTempFile is a controllable file-like object for writeFileAtomic tests.
// It lets us force errors on Write and Close without using a real file.
type fakeTempFile struct {
	fileName string
	writeErr error
	closeErr error
}

func (f *fakeTempFile) Name() string { return f.fileName }

func (f *fakeTempFile) Write(p []byte) (int, error) {
	if f.writeErr != nil {
		return 0, f.writeErr
	}
	return len(p), nil
}

func (f *fakeTempFile) Close() error {
	return f.closeErr
}

// snapshotWriteFileSeams captures the current global file seams so tests can restore them.
// writeFileAtomic uses these seams for testability.
func snapshotWriteFileSeams(t *testing.T) (
	origCreate func(string, string) (tempFile, error),
	origRemove func(string) error,
	origChmod func(string, os.FileMode) error,
	origRename func(string, string) error,
) {
	t.Helper()
	return createTempFile, removeFile, chmodFile, renameFile
}

// setWriteFileSeams overrides the global seams used by writeFileAtomic.
// Pass nil for any seam you don't want to override.
func setWriteFileSeams(
	t *testing.T,
	createFn func(string, string) (tempFile, error),
	removeFn func(path string) error,
	chmodFn func(path string, mode os.FileMode) error,
	renameFn func(oldpath, newpath string) error,
) {
	t.Helper()

	if createFn != nil {
		createTempFile = createFn
	}
	if removeFn != nil {
		removeFile = removeFn
	}
	if chmodFn != nil {
		chmodFile = chmodFn
	}
	if renameFn != nil {
		renameFile = renameFn
	}
}

//
// -----------------------------------------------------------------------------
// must()
// -----------------------------------------------------------------------------

// Covers:
// func must(err error) { if err != nil { panic(err) } }
func TestMust_PanicsOnError(t *testing.T) {
	t.Parallel()

	require.NotPanics(t, func() { must(nil) })
	require.PanicsWithError(t, "boom", func() { must(errors.New("boom")) })
}

//
// -----------------------------------------------------------------------------
// writeFileAtomic()
// -----------------------------------------------------------------------------

// Covers every writeFileAtomic error branch, including deferred cleanup:
// - createTempFile failure
// - Write failure triggers Close + deferred remove
// - Close failure triggers deferred remove
// - chmod failure triggers deferred remove
// - rename failure triggers deferred remove
func TestWriteFileAtomic_AllErrorBranches(t *testing.T) {
	// NOT parallel: mutates global seams.

	type seamOverrides struct {
		createTemp func(dir, pattern string) (tempFile, error)
		removeTmp  func(path string) error
		chmodTmp   func(path string, mode os.FileMode) error
		renameTmp  func(oldpath, newpath string) error
	}

	testCases := []struct {
		name                 string
		seams                seamOverrides
		expectedErrSubstring string
		expectedRemoveCount  int
	}{
		{
			name: "create temp error",
			seams: seamOverrides{
				createTemp: func(dir, pattern string) (tempFile, error) {
					return nil, errors.New("create temp failed")
				},
			},
			expectedErrSubstring: "create temp failed",
			expectedRemoveCount:  0,
		},
		{
			name: "write error closes and removes temp via deferred cleanup",
			seams: seamOverrides{
				createTemp: func(dir, pattern string) (tempFile, error) {
					return &fakeTempFile{
						fileName: filepath.Join(dir, "tmpfile"),
						writeErr: errors.New("write failed"),
					}, nil
				},
				removeTmp: func(path string) error { return nil },
			},
			expectedErrSubstring: "write failed",
			expectedRemoveCount:  1,
		},
		{
			name: "close error removes temp via deferred cleanup",
			seams: seamOverrides{
				createTemp: func(dir, pattern string) (tempFile, error) {
					return &fakeTempFile{
						fileName: filepath.Join(dir, "tmpfile"),
						closeErr: errors.New("close failed"),
					}, nil
				},
				removeTmp: func(path string) error { return nil },
			},
			expectedErrSubstring: "close failed",
			expectedRemoveCount:  1,
		},
		{
			name: "chmod error removes temp via deferred cleanup",
			seams: seamOverrides{
				createTemp: func(dir, pattern string) (tempFile, error) {
					return &fakeTempFile{fileName: filepath.Join(dir, "tmpfile")}, nil
				},
				chmodTmp:  func(path string, mode os.FileMode) error { return errors.New("chmod failed") },
				removeTmp: func(path string) error { return nil },
			},
			expectedErrSubstring: "chmod failed",
			expectedRemoveCount:  1,
		},
		{
			name: "rename error removes temp via deferred cleanup",
			seams: seamOverrides{
				createTemp: func(dir, pattern string) (tempFile, error) {
					return &fakeTempFile{fileName: filepath.Join(dir, "tmpfile")}, nil
				},
				chmodTmp:  func(path string, mode os.FileMode) error { return nil },
				renameTmp: func(oldpath, newpath string) error { return errors.New("rename failed") },
				removeTmp: func(path string) error { return nil },
			},
			expectedErrSubstring: "rename failed",
			expectedRemoveCount:  1,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			originalCreate, originalRemove, originalChmod, originalRename := snapshotWriteFileSeams(t)
			t.Cleanup(func() {
				createTempFile = originalCreate
				removeFile = originalRemove
				chmodFile = originalChmod
				renameFile = originalRename
			})

			var removedTempPaths []string

			setWriteFileSeams(
				t,
				tc.seams.createTemp,
				func(path string) error {
					removedTempPaths = append(removedTempPaths, path)
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
			assert.Contains(t, err.Error(), tc.expectedErrSubstring)
			assert.Len(t, removedTempPaths, tc.expectedRemoveCount)
		})
	}
}

// Covers the success path of writeFileAtomic:
// - createTempFile ok
// - Write ok
// - Close ok
// - chmod ok
// - rename ok
func TestWriteFileAtomic_Success(t *testing.T) {
	// NOT parallel: uses real filesystem but does not mutate seams.
	tempDir := t.TempDir()
	outputPath := filepath.Join(tempDir, "final.go")

	require.NoError(t, writeFileAtomic(outputPath, []byte("hello"), 0o644))

	contents, err := os.ReadFile(outputPath)
	require.NoError(t, err)
	assert.Equal(t, "hello", string(contents))
}

//
// -----------------------------------------------------------------------------
// validateSpec()
// -----------------------------------------------------------------------------

// Covers validateSpec behavior including:
// - missing required fields collection
// - required deps empty
// - dep validation (missing name/field/type)
// - duplicates across required + optional
// - optional deps loop
func TestValidateSpec_AllBranches(t *testing.T) {
	t.Parallel()

	baseSpec := func() Spec {
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

	testCases := []struct {
		name        string
		mutate      func(s *Spec)
		expectPanic bool
	}{
		{
			name:        "ok does not panic (includes optional deps loop)",
			mutate:      func(s *Spec) {},
			expectPanic: false,
		},
		{
			name: "missing required fields collected",
			mutate: func(s *Spec) {
				s.Package = "   "
				s.Constructor = " "
				s.Required = nil
			},
			expectPanic: true,
		},
		{
			name: "dep missing field triggers panic",
			mutate: func(s *Spec) {
				s.Required = []Dep{{Name: "DB", Field: "", Type: "*sql.DB"}}
			},
			expectPanic: true,
		},
		{
			name: "duplicate dep name across required+optional triggers panic",
			mutate: func(s *Spec) {
				s.Optional = []Dep{{Name: "DB", Field: "db2", Type: "*sql.DB"}}
			},
			expectPanic: true,
		},
		{
			name: "duplicate dep field across required+optional triggers panic",
			mutate: func(s *Spec) {
				s.Optional = []Dep{{Name: "Cache", Field: "db", Type: "any"}}
			},
			expectPanic: true,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			spec := baseSpec()
			tc.mutate(&spec)

			if tc.expectPanic {
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

// Covers:
// - parser.ParseFile error path
// - parsing imports incl. aliases
func TestReadImportsFromFile_SuccessAndParseError(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name          string
		source        string
		expectErr     bool
		assertImports func(t *testing.T, imports []ImportSpec)
	}{
		{
			name:      "parse error returned",
			source:    "package", // invalid
			expectErr: true,
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
			expectErr: false,
			assertImports: func(t *testing.T, imports []ImportSpec) {
				assert.True(t, containsPath(imports, "fmt"))
				assert.True(t, containsPath(imports, "example.com/project/autowire/config"))
				assert.True(t, containsAlias(imports, "config"))
				assert.True(t, containsAlias(imports, "_"))
			},
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			tempDir := t.TempDir()
			goFilePath := filepath.Join(tempDir, "file.go")
			require.NoError(t, os.WriteFile(goFilePath, []byte(tc.source), 0o644))

			imports, err := readImportsFromFile(goFilePath)
			if tc.expectErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			if tc.assertImports != nil {
				tc.assertImports(t, imports)
			}
		})
	}
}

// Covers:
// - ensureImport early return when path already exists
func TestEnsureImport_DoesNotDuplicateByPath(t *testing.T) {
	t.Parallel()

	var imports []ImportSpec
	ensureImport(&imports, ImportSpec{Path: "fmt"})
	ensureImport(&imports, ImportSpec{Path: "fmt"}) // should no-op

	require.Len(t, imports, 1)
	assert.Equal(t, "fmt", imports[0].Path)
}

// Covers:
// - containsAlias requires alias != ""
// - containsPath match/no-match
func TestContainsAliasAndContainsPath_Branches(t *testing.T) {
	t.Parallel()

	imports := []ImportSpec{
		{Alias: "", Path: "fmt"},
		{Alias: "config", Path: "example.com/project/autowire/config"},
	}

	assert.True(t, containsPath(imports, "fmt"))
	assert.False(t, containsPath(imports, "nope"))

	assert.True(t, containsAlias(imports, "config"))
	assert.False(t, containsAlias(imports, ""))        // alias must be non-empty
	assert.False(t, containsAlias(imports, "missing")) // absent
}

//
// -----------------------------------------------------------------------------
// resolveImports()
// -----------------------------------------------------------------------------

// Covers resolveImports branches:
// - owner imports parse error fallback to empty
// - !constructorNeedsConfig early return
// - containsAlias("config") early return
// - missing spec.Imports.Config error
// - (updated behavior) config path imported without explicit alias is OK when default ident is 'config'
func TestResolveImports_AllBranches(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name          string
		setup         func(t *testing.T) (ownerFilePath string, spec *Spec, constructorNeedsConfig bool)
		expectErrSub  string
		assertImports func(t *testing.T, imports []ImportSpec)
	}{
		{
			name: "owner import parse error falls back to empty and uses spec config import",
			setup: func(t *testing.T) (string, *Spec, bool) {
				tempDir := t.TempDir()
				ownerFile := filepath.Join(tempDir, "bad.go")
				require.NoError(t, os.WriteFile(ownerFile, []byte("package"), 0o644))
				return ownerFile, &Spec{
					Constructor: "NewService",
					Imports:     Imports{Config: "example.com/project/autowire/config"},
				}, true
			},
			assertImports: func(t *testing.T, imports []ImportSpec) {
				assert.True(t, containsPath(imports, "fmt"))
				assert.True(t, containsAlias(imports, "config"))
				assert.True(t, containsPath(imports, "example.com/project/autowire/config"))
			},
		},
		{
			name: "constructor does not need config returns early with fmt ensured",
			setup: func(t *testing.T) (string, *Spec, bool) {
				return "", &Spec{}, false
			},
			assertImports: func(t *testing.T, imports []ImportSpec) {
				assert.True(t, containsPath(imports, "fmt"))
				assert.False(t, containsAlias(imports, "config"))
			},
		},
		{
			name: "owner already has alias config returns early without using spec config path",
			setup: func(t *testing.T) (string, *Spec, bool) {
				tempDir := t.TempDir()
				ownerFile := filepath.Join(tempDir, "owner.go")
				source := `package svc

import (
	config "example.com/owner/config"
)
`
				require.NoError(t, os.WriteFile(ownerFile, []byte(source), 0o644))
				return ownerFile, &Spec{
					Constructor: "NewService",
					Imports:     Imports{Config: "example.com/spec/config"},
				}, true
			},
			assertImports: func(t *testing.T, imports []ImportSpec) {
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
			expectErrSub: "spec.imports.config is empty",
		},
		{
			name: "config path already imported without explicit alias is ok when default ident is 'config'",
			setup: func(t *testing.T) (string, *Spec, bool) {
				tempDir := t.TempDir()
				ownerFile := filepath.Join(tempDir, "owner.go")
				source := `package svc

import (
	"example.com/project/autowire/config"
)
`
				require.NoError(t, os.WriteFile(ownerFile, []byte(source), 0o644))

				return ownerFile, &Spec{
					Constructor: "NewService",
					Imports:     Imports{Config: "example.com/project/autowire/config"},
				}, true
			},
			expectErrSub: "",
			assertImports: func(t *testing.T, imports []ImportSpec) {
				assert.True(t, containsPath(imports, "fmt"))
				assert.True(t, containsPath(imports, "example.com/project/autowire/config"))
			},
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			ownerFilePath, spec, constructorNeedsConfig := tc.setup(t)

			imports, err := resolveImports(ownerFilePath, spec, constructorNeedsConfig)
			if tc.expectErrSub != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.expectErrSub)
				return
			}

			require.NoError(t, err)
			if tc.assertImports != nil {
				tc.assertImports(t, imports)
			}
		})
	}
}

//
// -----------------------------------------------------------------------------
// determineConstructorNeedsConfig()
// -----------------------------------------------------------------------------

// Covers determineConstructorNeedsConfig branches:
// - explicit override (nil vs non-nil)
// - ReadDir error default true
// - entry.IsDir() skip
// - parser.ParseFile error skip
// - non-func decl skip
// - method receiver skip
// - name mismatch skip
// - no params => false
// - one param but not *ast.SelectorExpr => true
// - selector matches config.Config => true
// - selector is Ident but not "config" => unrecognized signature => true
// - constructor not found => default true
func TestDetermineConstructorNeedsConfig_AllBranches(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name              string
		override          *bool
		files             map[string]string
		expectNeedsConfig bool
		useMissingDir     bool
	}{
		{
			name:              "explicit override wins (true)",
			override:          ptrBool(true),
			files:             map[string]string{},
			expectNeedsConfig: true,
		},
		{
			name:              "explicit override wins (false)",
			override:          ptrBool(false),
			files:             map[string]string{},
			expectNeedsConfig: false,
		},
		{
			name:              "ReadDir error defaults true",
			override:          nil,
			useMissingDir:     true,
			expectNeedsConfig: true,
		},
		{
			name:     "skips dirs/parse errors/non-func/method/name mismatch and detects config.Config",
			override: nil,
			files: map[string]string{
				"bad.go": "package", // parse error -> skipped
				"svc.go": `package svc

var x = 1 // non-func decl ensures funcDecl type assertion fails for at least one decl

type T struct{}
func (t *T) NewService() {} // method: should be skipped
func Other() {}            // name mismatch: skipped

func NewService(cfg config.Config) {} // target constructor with config.Config
`,
			},
			expectNeedsConfig: true,
		},
		{
			name:     "constructor with no params returns false",
			override: nil,
			files: map[string]string{
				"svc.go": `package svc
func NewService() {}
`,
			},
			expectNeedsConfig: false,
		},
		{
			name:     "constructor with one param but not selector defaults true",
			override: nil,
			files: map[string]string{
				"svc.go": `package svc
func NewService(x int) {}
`,
			},
			expectNeedsConfig: true,
		},
		{
			name:     "selector ident but not 'config' defaults true (other.Config)",
			override: nil,
			files: map[string]string{
				"svc.go": `package svc
func NewService(cfg other.Config) {}
`,
			},
			expectNeedsConfig: true,
		},
		{
			name:     "constructor not found defaults true",
			override: nil,
			files: map[string]string{
				"svc.go": "package svc\n",
			},
			expectNeedsConfig: true,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			constructorSpec := &Spec{Constructor: "NewService", ConstructorTakesConfig: tc.override}

			if tc.useMissingDir {
				missingDir := filepath.Join(t.TempDir(), "does-not-exist")
				assert.Equal(t, tc.expectNeedsConfig, determineConstructorNeedsConfig(constructorSpec, missingDir))
				return
			}

			sourceDir := t.TempDir()

			// Create a directory entry to cover entry.IsDir() skip behavior.
			require.NoError(t, os.Mkdir(filepath.Join(sourceDir, "subdir"), 0o755))

			for fileName, contents := range tc.files {
				require.NoError(t, os.WriteFile(filepath.Join(sourceDir, fileName), []byte(contents), 0o644))
			}

			assert.Equal(t, tc.expectNeedsConfig, determineConstructorNeedsConfig(constructorSpec, sourceDir))
		})
	}
}

//
// -----------------------------------------------------------------------------
// findOwnerGoGenerateFile()
// -----------------------------------------------------------------------------

// Covers findOwnerGoGenerateFile branches:
// - os.ReadDir error
// - entry.IsDir() skip
// - suffix filters for non-go and _test.go
// - os.ReadFile error skip
// - match found
// - no match found
func TestFindOwnerGoGenerateFile_AllBranches(t *testing.T) {
	// NOT parallel: uses symlink (may skip).

	testCases := []struct {
		name               string
		setup              func(t *testing.T) (packageDir string)
		expectErr          bool
		expectPathEndsWith string
	}{
		{
			name: "ReadDir error returned",
			setup: func(t *testing.T) string {
				return filepath.Join(t.TempDir(), "does-not-exist")
			},
			expectErr: true,
		},
		{
			name: "skips dir + suffix filters + readfile error and finds matching owner file",
			setup: func(t *testing.T) string {
				packageDir := t.TempDir()

				// entry.IsDir() skip
				require.NoError(t, os.Mkdir(filepath.Join(packageDir, "00_dir"), 0o755))

				// suffix filters
				require.NoError(t, os.WriteFile(filepath.Join(packageDir, "01_readme.md"), []byte("ignore"), 0o644))
				require.NoError(t, os.WriteFile(filepath.Join(packageDir, "02_owner_test.go"), []byte("package svc\n"), 0o644))

				// os.ReadFile error skip: broken symlink with .go suffix
				brokenSymlinkPath := filepath.Join(packageDir, "03_broken.go")
				symlinkErr := os.Symlink(filepath.Join(packageDir, "does-not-exist-target"), brokenSymlinkPath)
				if symlinkErr != nil {
					// Some environments disallow symlinks; fall back to chmod-based unreadable file.
					unreadableGo := filepath.Join(packageDir, "03_unreadable.go")
					require.NoError(t, os.WriteFile(unreadableGo, []byte("package svc\n"), 0o644))
					require.NoError(t, os.Chmod(unreadableGo, 0o000))
					t.Cleanup(func() { _ = os.Chmod(unreadableGo, 0o644) })
				}

				// Non-matching go file
				require.NoError(t, os.WriteFile(filepath.Join(packageDir, "04_other.go"), []byte("package svc\n"), 0o644))

				// Matching owner file (sorted last)
				ownerPath := filepath.Join(packageDir, "zz_owner.go")
				ownerSource := `package svc

//go:generate go run ../../cmd/di1 -spec ./specs/x.inject.json -out ./x.gen.go
`
				require.NoError(t, os.WriteFile(ownerPath, []byte(ownerSource), 0o644))

				return packageDir
			},
			expectErr:          false,
			expectPathEndsWith: "zz_owner.go",
		},
		{
			name: "no matching file returns error",
			setup: func(t *testing.T) string {
				packageDir := t.TempDir()
				require.NoError(t, os.WriteFile(filepath.Join(packageDir, "a.go"), []byte("package svc\n"), 0o644))
				return packageDir
			},
			expectErr: true,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			packageDir := tc.setup(t)

			foundOwnerPath, err := findOwnerGoGenerateFile(packageDir)
			if tc.expectErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.True(t, strings.HasSuffix(foundOwnerPath, tc.expectPathEndsWith))
		})
	}
}

//
// -----------------------------------------------------------------------------
// Template rendering (smoke)
// -----------------------------------------------------------------------------

// A quick sanity check that the template still renders expected output with templateData.
// This is not exhaustive; run() tests already validate generated output too.
func TestGenTemplate_Smoke(t *testing.T) {
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

	var rendered strings.Builder
	require.NoError(t, genTemplate.Execute(&rendered, data))

	out := rendered.String()
	assert.Contains(t, out, "type UserV1 struct")
	assert.Contains(t, out, "func NewUserV1")
	assert.Contains(t, out, "InjectDB")
}

//
// -----------------------------------------------------------------------------
// run(): relative out path cleaning
// -----------------------------------------------------------------------------

// NOT parallel:
// - uses run() which calls writeFileAtomic (global seams)
// - mutates working directory (process-global state)
func TestRun_RelativeOutPath_IsCleaned(t *testing.T) {
	tempDir := t.TempDir()

	oldWD, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.Chdir(oldWD) })

	require.NoError(t, os.Chdir(tempDir))

	specPath := filepath.Join(tempDir, "service.inject.json")
	require.NoError(t, os.WriteFile(specPath, minimalValidSpecJSON(), 0o644))

	relativeOutputPath := filepath.Join(".", "subdir", "..", "gen", "out.gen.go")
	cleanedOutputPath := filepath.Clean(relativeOutputPath)

	require.NoError(t, os.MkdirAll(filepath.Dir(cleanedOutputPath), 0o755))

	var stderr bytes.Buffer
	exitCode := run([]string{"-spec", specPath, "-out", relativeOutputPath}, &stderr)
	require.Equal(t, 0, exitCode)

	contents, err := os.ReadFile(cleanedOutputPath)
	require.NoError(t, err)
	assert.Contains(t, string(contents), "type UserV1 struct")
}

//
// -----------------------------------------------------------------------------
// run(): cover flag parse error, missing flags usage, and resolveImports panic
// -----------------------------------------------------------------------------

func TestRun_ErrorBranches(t *testing.T) {
	// NOT parallel: interacts with filesystem and run() generation.

	testCases := []struct {
		name       string
		setupArgs  func(t *testing.T) []string
		wantCode   *int
		wantStderr string
		wantPanic  string
	}{
		{
			name: "flag parse error returns 2",
			setupArgs: func(t *testing.T) []string {
				return []string{"-nope"}
			},
			wantCode: ptrInt(2),
		},
		{
			name: "missing flags prints usage and returns 2",
			setupArgs: func(t *testing.T) []string {
				return []string{} // no -spec/-out
			},
			wantCode:   ptrInt(2),
			wantStderr: "usage: di1 -spec",
		},
		{
			name: "resolveImports error panics (needs config but spec.imports.config empty)",
			setupArgs: func(t *testing.T) []string {
				tempDir := t.TempDir()

				// Owner file so findOwnerGoGenerateFile can succeed.
				ownerFile := filepath.Join(tempDir, "zz_owner.go")
				ownerSource := `package svc

//go:generate go run ../../cmd/di1 -spec ./service.inject.json -out ./out.gen.go
`
				require.NoError(t, os.WriteFile(ownerFile, []byte(ownerSource), 0o644))

				// Spec that forces NeedsConfig=true but provides no fallback import path.
				specPath := filepath.Join(tempDir, "service.inject.json")
				specJSON := `{
  "package": "svc",
  "wrapperBase": "User",
  "versionSuffix": "V1",
  "implType": "Service",
  "constructor": "NewService",
  "imports": { "config": "" },
  "required": [
    { "name": "DB", "field": "db", "type": "*sql.DB" }
  ]
}`
				require.NoError(t, os.WriteFile(specPath, []byte(specJSON), 0o644))

				// Ensure determineConstructorNeedsConfig returns true by having the constructor take config.Config.
				sourceFile := filepath.Join(tempDir, "svc.go")
				source := `package svc

import config "example.com/project/autowire/config"

func NewService(cfg config.Config) {}
`
				require.NoError(t, os.WriteFile(sourceFile, []byte(source), 0o644))

				outPath := filepath.Join(tempDir, "out.gen.go")
				return []string{"-spec", specPath, "-out", outPath}
			},
			wantPanic: "spec.imports.config is empty",
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			args := tc.setupArgs(t)

			var stderr bytes.Buffer

			if tc.wantPanic != "" {
				requirePanicContains(t, tc.wantPanic, func() {
					_ = run(args, &stderr)
				})
				return
			}

			code := run(args, &stderr)
			require.NotNil(t, tc.wantCode)
			require.Equal(t, *tc.wantCode, code)

			if tc.wantStderr != "" {
				assert.Contains(t, stderr.String(), tc.wantStderr)
			}
		})
	}
}

func ptrInt(v int) *int { return &v }

//
// -----------------------------------------------------------------------------
// Coverage-focused: findOwnerGoGenerateFile IsDir + ReadFile error continue
// -----------------------------------------------------------------------------

func TestFindOwnerGoGenerateFile_CoversIsDirAndReadFileErrorContinue(t *testing.T) {
	// NOT parallel: relies on filesystem entries.
	packageDir := t.TempDir()

	// Force: entry.IsDir() == true branch (sorted early)
	require.NoError(t, os.Mkdir(filepath.Join(packageDir, "00_dir"), 0o755))

	// Force: os.ReadFile(filePath) error branch (sorted early).
	// Prefer a broken symlink (deterministic) over chmod (platform-dependent).
	brokenSymlinkPath := filepath.Join(packageDir, "01_broken.go")
	symlinkErr := os.Symlink(filepath.Join(packageDir, "does-not-exist-target"), brokenSymlinkPath)
	if symlinkErr != nil {
		// If symlinks aren't supported in the environment, fall back to chmod.
		unreadableGo := filepath.Join(packageDir, "01_unreadable.go")
		require.NoError(t, os.WriteFile(unreadableGo, []byte("package svc\n"), 0o644))
		require.NoError(t, os.Chmod(unreadableGo, 0o000))
		t.Cleanup(func() { _ = os.Chmod(unreadableGo, 0o644) })
	}

	// Add a couple of files that should be skipped by suffix rules
	require.NoError(t, os.WriteFile(filepath.Join(packageDir, "02_readme.md"), []byte("ignore"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(packageDir, "03_owner_test.go"), []byte("package svc\n"), 0o644))

	// Matching owner file must sort LAST so we don't return early.
	expectedOwnerPath := filepath.Join(packageDir, "zz_owner.go")
	ownerSource := `package svc

//go:generate go run ../../cmd/di1 -spec ./specs/x.inject.json -out ./x.gen.go
`
	require.NoError(t, os.WriteFile(expectedOwnerPath, []byte(ownerSource), 0o644))

	found, err := findOwnerGoGenerateFile(packageDir)
	require.NoError(t, err)
	assert.Equal(t, expectedOwnerPath, found)
}

//
// -----------------------------------------------------------------------------
// Coverage-focused: determineConstructorNeedsConfig suffix continues
// -----------------------------------------------------------------------------

func TestDetermineConstructorNeedsConfig_CoversSuffixFilterContinues(t *testing.T) {
	// NOT parallel: filesystem order sensitive for coverage.
	sourceDir := t.TempDir()

	// These must hit:
	// if !strings.HasSuffix(".go") || HasSuffix("_test.go") || HasSuffix(".gen.go") { continue }

	require.NoError(t, os.WriteFile(filepath.Join(sourceDir, "00_notes.txt"), []byte("ignore"), 0o644))          // not .go
	require.NoError(t, os.WriteFile(filepath.Join(sourceDir, "01_svc_test.go"), []byte("package svc\n"), 0o644)) // _test.go
	require.NoError(t, os.WriteFile(filepath.Join(sourceDir, "02_svc.gen.go"), []byte("package svc\n"), 0o644))  // .gen.go
	require.NoError(t, os.WriteFile(filepath.Join(sourceDir, "zz_svc.go"), []byte(`package svc
func NewService(cfg config.Config) {}
`), 0o644)) // real constructor

	spec := &Spec{Constructor: "NewService"}
	assert.True(t, determineConstructorNeedsConfig(spec, sourceDir))
}
