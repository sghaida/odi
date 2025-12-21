package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type TB interface {
	Helper()
	Fatalf(format string, args ...any)
	Cleanup(func())
}


type pkgHarness struct {
	t   *testing.T
	dir string
}

func newPkg(t *testing.T) *pkgHarness {
	t.Helper()
	return &pkgHarness{t: t, dir: t.TempDir()}
}

func (p *pkgHarness) write(rel, content string) string {
	p.t.Helper()
	path := filepath.Join(p.dir, rel)
	mustWriteFile(p.t, path, content)
	return path
}

func (p *pkgHarness) out(rel string) string {
	return filepath.Join(p.dir, rel)
}

func (p *pkgHarness) read(rel string) string {
	p.t.Helper()
	return mustReadString(p.t, filepath.Join(p.dir, rel))
}

func chmodNoRead(t TB, path string) {
	t.Helper()
	if err := os.Chmod(path, 0o000); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(path, 0o644) })
}

func assertHasImport(t TB, out, imp string) {
	t.Helper()
	// Handles both stdlib and aliased imports.
	if !strings.Contains(out, `"`+imp+`"`) {
		t.Fatalf("expected import %q", imp)
	}
}

func assertNotHasImport(t TB, out, imp string) {
	t.Helper()
	if strings.Contains(out, `"`+imp+`"`) {
		t.Fatalf("did not expect import %q", imp)
	}
}

func assertPanicContains(t TB, fn func(), wantSubstr string) {
	t.Helper()
	defer func() {
		r := recover()
		if r == nil {
			t.Fatalf("expected panic containing %q, got none", wantSubstr)
		}
		msg := toString(r)
		if !strings.Contains(msg, wantSubstr) {
			t.Fatalf("panic=%q want contains %q", msg, wantSubstr)
		}
	}()
	fn()
}

func mustWriteFile(t TB, path, content string) {
	t.Helper()
	mustMkdirAll(t, filepath.Dir(path))
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func mustMkdirAll(t TB, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
}

func mustReadString(t TB, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(b)
}

func assertContainsInOrder(t TB, s string, parts ...string) {
	t.Helper()
	pos := 0
	for _, p := range parts {
		i := strings.Index(s[pos:], p)
		if i < 0 {
			t.Fatalf("expected to find %q after pos=%d", p, pos)
		}
		pos += i + len(p)
	}
}

func toString(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case error:
		return x.Error()
	default:
		return fmt.Sprintf("%v", v)
	}
}

type inferCase[T any] struct {
	name      string
	setup     func(p *pkgHarness) (spec *T, outPath string)
	call      func(spec *T, outPath string)
	assert    func(t *testing.T, spec *T)
	wantPanic string
}


func runInferCases[T any](t *testing.T, cases []inferCase[T]) {
	t.Helper()
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			p := newPkg(t)

			spec, outPath := tc.setup(p)

			if tc.wantPanic != "" {
				assertPanicContains(t, func() { tc.call(spec, outPath) }, tc.wantPanic)
				return
			}
			tc.call(spec, outPath)
			tc.assert(t, spec)
		})
	}
}

type fatalTB struct {
	testing.TB
}

func (f fatalTB) Helper() {}

func (f fatalTB) Cleanup(fn func()) {
	// no-op for fake
}

func (f fatalTB) Fatalf(format string, args ...any) {
	panic(fmt.Sprintf(format, args...))
}

