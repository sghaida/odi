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

type cfgMatrixRow struct {
	name      string
	force     string
	initial   string
	want      string
	wantPanic string
}

var configMatrix = []cfgMatrixRow{
	{name: "forced_import_wins", force: "example.com/forced/config", initial: "", want: "example.com/forced/config"},
	{name: "scanned_used_when_no_force_and_empty", force: "", initial: "", want: "example.com/proj/config"},
	{name: "keeps_existing_import_if_already_set", force: "", initial: "example.com/already/config", want: "example.com/already/config"},
	{name: "panics_if_enabled_and_cannot_infer_and_no_config_dir", force: "", initial: "", wantPanic: "cannot infer"},
}

func writeDISource(p *pkgHarness) {
	p.write("di.go", `package p
import di "example.com/proj/di"
func _() { _ = di.Registry(nil) }`)
}

func writeConfigSource(p *pkgHarness) {
	p.write("cfg.go", `package p
import config "example.com/proj/config"
var _ = config.Config{}`)
}

func writeGoMod(p *pkgHarness) {
	p.write("go.mod", "module example.com/proj\n\ngo 1.22\n")
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

func addServiceConfigMatrixCases(cases []inferCase[ServiceSpec], matrix []cfgMatrixRow) []inferCase[ServiceSpec] {
	for _, row := range matrix {
		row := row
		cases = append(cases, inferCase[ServiceSpec]{
			name: "config_enabled_" + row.name,
			setup: func(p *pkgHarness) (*ServiceSpec, string) {
				outPath := p.out("svc.gen.go")

				writeDISource(p)

				if row.wantPanic == "" {
					writeConfigSource(p)
				} else {
					writeGoMod(p)
					// no ./config dir and no config import in sources
				}

				s := &ServiceSpec{
					Package: "p", WrapperBase: "W", VersionSuffix: "V2", ImplType: "Impl", Constructor: "NewImpl",
					Imports:  Imports{DI: "", Config: row.initial},
					Config:   ConfigSpec{Enabled: true, Import: row.force},
					Required: []RequiredDep{{Name: "A", Field: "a", Type: "*A", Nilable: true}},
				}
				return s, outPath
			},
			call:      inferImportsForService,
			wantPanic: row.wantPanic,
			assert: func(t *testing.T, s *ServiceSpec) {
				// tighten the panic string to service’s message
				if row.wantPanic != "" {
					// The caller already checks wantPanic; no assert needed here.
					return
				}
				if s.Imports.Config != row.want {
					t.Fatalf("Config import: got %q want %q", s.Imports.Config, row.want)
				}
				if s.Imports.DI != "example.com/proj/di" {
					t.Fatalf("DI import: got %q want %q", s.Imports.DI, "example.com/proj/di")
				}
			},
		})
	}
	return cases
}

func addGraphConfigMatrixCases(cases []inferCase[GraphSpec], matrix []cfgMatrixRow) []inferCase[GraphSpec] {
	for _, row := range matrix {
		row := row
		cases = append(cases, inferCase[GraphSpec]{
			name: "config_enabled_" + row.name,
			setup: func(p *pkgHarness) (*GraphSpec, string) {
				outPath := p.out("graph.gen.go")

				writeDISource(p)

				if row.wantPanic == "" {
					writeConfigSource(p)
				} else {
					writeGoMod(p)
				}

				g := &GraphSpec{
					Package: "p",
					Imports: Imports{DI: "", Config: row.initial},
					Config:  ConfigSpec{Enabled: true, Import: row.force},
					Roots: []struct {
						Name              string `json:"name"`
						BuildWithRegistry bool   `json:"buildWithRegistry"`
						Services          []struct {
							Var        string `json:"var"`
							FacadeCtor string `json:"facadeCtor"`
							FacadeType string `json:"facadeType"`
							ImplType   string `json:"implType"`
						} `json:"services"`
						Wiring []struct {
							To      string `json:"to"`
							Call    string `json:"call"`
							ArgFrom string `json:"argFrom"`
						} `json:"wiring"`
					}{
						{Name: "Root"},
					},
				}
				return g, outPath
			},
			call:      inferImportsForGraph,
			wantPanic: row.wantPanic,
			assert: func(t *testing.T, g *GraphSpec) {
				if row.wantPanic != "" {
					return
				}
				if g.Imports.Config != row.want {
					t.Fatalf("Config import: got %q want %q", g.Imports.Config, row.want)
				}
				if g.Imports.DI != "example.com/proj/di" {
					t.Fatalf("DI import: got %q want %q", g.Imports.DI, "example.com/proj/di")
				}
			},
		})
	}
	return cases
}
