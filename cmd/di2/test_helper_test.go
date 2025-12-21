// odi/di2/test_helpers_coverage_test.go
package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestHelpers_CoverFatalBranches(t *testing.T) {
	t.Parallel()

	tb := fatalTB{}

	t.Run("assertNotHasImport_success_path_is_used", func(t *testing.T) {
		t.Parallel()
		assertNotHasImport(t, `import "fmt"`, "strings") // success (covers helper usage)
	})

	t.Run("assertHasImport_failure_branch", func(t *testing.T) {
		t.Parallel()
		assertPanicContains(t, func() {
			assertHasImport(tb, `import "fmt"`, "strings")
		}, `expected import "strings"`)
	})

	t.Run("assertNotHasImport_failure_branch", func(t *testing.T) {
		t.Parallel()
		assertPanicContains(t, func() {
			assertNotHasImport(tb, `import "fmt"`, "fmt")
		}, `did not expect import "fmt"`)
	})

	t.Run("chmodNoRead_failure_branch", func(t *testing.T) {
		t.Parallel()
		assertPanicContains(t, func() {
			chmodNoRead(tb, filepath.Join(t.TempDir(), "does-not-exist"))
		}, "chmod:")
	})

	t.Run("assertPanicContains_r_nil_branch", func(t *testing.T) {
		t.Parallel()
		// fn does NOT panic => helper hits r==nil Fatalf branch
		assertPanicContains(t, func() {
			assertPanicContains(tb, func() {}, "boom")
		}, `expected panic containing "boom"`)
	})

	t.Run("assertPanicContains_msg_mismatch_branch", func(t *testing.T) {
		t.Parallel()
		// fn panics but does not contain substring => helper hits msg mismatch Fatalf branch
		assertPanicContains(t, func() {
			assertPanicContains(tb, func() { panic("boom") }, "zzz")
		}, `want contains "zzz"`)
	})

	t.Run("mustReadString_error_branch", func(t *testing.T) {
		t.Parallel()
		assertPanicContains(t, func() {
			_ = mustReadString(tb, filepath.Join(t.TempDir(), "missing.txt"))
		}, "read ")
	})

	t.Run("mustMkdirAll_error_branch", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		// Create a file "x" then try MkdirAll("x/y") => should error (x is file)
		x := filepath.Join(dir, "x")
		if err := os.WriteFile(x, []byte("nope"), 0o644); err != nil {
			t.Fatalf("setup: %v", err)
		}

		assertPanicContains(t, func() {
			mustMkdirAll(tb, filepath.Join(x, "y"))
		}, "mkdir ")
	})

	t.Run("mustWriteFile_error_branch", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		locked := filepath.Join(dir, "locked")
		if err := os.MkdirAll(locked, 0o755); err != nil {
			t.Fatalf("setup: %v", err)
		}
		// Remove all permissions so write fails.
		if err := os.Chmod(locked, 0o000); err != nil {
			t.Fatalf("setup chmod: %v", err)
		}
		t.Cleanup(func() { _ = os.Chmod(locked, 0o755) })

		assertPanicContains(t, func() {
			mustWriteFile(tb, filepath.Join(locked, "x.txt"), "hi")
		}, "write ")
	})

	t.Run("assertContainsInOrder_error_branch", func(t *testing.T) {
		t.Parallel()
		assertPanicContains(t, func() {
			assertContainsInOrder(tb, "a b c", "a", "zzz")
		}, "expected to find")
	})
}
