// test_helpers.go
package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

//
// -----------------------------------------------------------------------------
// Shared fixtures
// -----------------------------------------------------------------------------

// minimalSpecJSON returns a minimal inject spec JSON that passes validateSpec
// and allows run() to generate output.
//
// It includes imports.config as a fallback so generation can still succeed
// even when owner-file import discovery fails (by design in some tests).
func minimalSpecJSON() []byte {
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
// Small helpers
// -----------------------------------------------------------------------------

func boolPtr(v bool) *bool { return &v }
func intPtr(v int) *int    { return &v }

// writeTempFile writes a file under dir/name and returns its full path.
func writeTempFile(t *testing.T, dir, name, content string, perm os.FileMode) string {
	t.Helper()
	p := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(p, []byte(content), perm))
	return p
}

// readFileString reads a file and returns its contents as string (fatal on error).
func readFileString(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	require.NoError(t, err)
	return string(b)
}

// makeUnreadableGoFile tries to create a path that causes os.ReadFile to error.
// Prefers a broken symlink; falls back to chmod(000) on a real file.
func makeUnreadableGoFile(t *testing.T, dir, name string) string {
	t.Helper()

	p := filepath.Join(dir, name)

	// Prefer broken symlink.
	if err := os.Symlink(filepath.Join(dir, "does-not-exist-target"), p); err == nil {
		return p
	}

	// Fallback: real file with no read perms.
	require.NoError(t, os.WriteFile(p, []byte("package svc\n"), 0o644))
	require.NoError(t, os.Chmod(p, 0o000))
	t.Cleanup(func() { _ = os.Chmod(p, 0o644) })
	return p
}

// mustPanicContains asserts fn panics and the panic message contains wantSub.
func mustPanicContains(t *testing.T, wantSub string, fn func()) {
	t.Helper()

	defer func() {
		r := recover()
		require.NotNil(t, r)

		var msg string
		switch v := r.(type) {
		case error:
			msg = v.Error()
		case string:
			msg = v
		default:
			msg = fmt.Sprintf("%v", v)
		}
		require.Contains(t, msg, wantSub)
	}()

	fn()
}

//
// -----------------------------------------------------------------------------
// writeFileAtomic() seam helpers
// -----------------------------------------------------------------------------

// fakeTempFile is a controllable file-like object for writeFileAtomic tests.
// It lets tests force errors on Write and Close without touching real files.
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

func (f *fakeTempFile) Close() error { return f.closeErr }

// snapWriteSeams captures the current global file seams so tests can restore them.
// writeFileAtomic uses these seams for testability.
func snapWriteSeams(t *testing.T) (
	origCreate func(string, string) (tempFile, error),
	origRemove func(string) error,
	origChmod func(string, os.FileMode) error,
	origRename func(string, string) error,
) {
	t.Helper()
	return createTempFile, removeFile, chmodFile, renameFile
}

// setWriteSeams overrides the global seams used by writeFileAtomic.
// Pass nil for any seam you don't want to override.
func setWriteSeams(
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

// Keep errors imported even if we shuffle tests later.
var _ = errors.New
