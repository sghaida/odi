// odi/di2/main_test.go
package main

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

// -------------------------
// applyConfigDefaults
// -------------------------

func TestApplyConfigDefaults(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   *ConfigSpec
		want *ConfigSpec
	}{
		{name: "nil_noop", in: nil, want: nil},
		{
			name: "fills_all_defaults",
			in:   &ConfigSpec{},
			want: &ConfigSpec{Type: "config.Config", FieldName: "cfg", ParamName: "cfg"},
		},
		{
			name: "preserves_existing_values",
			in: &ConfigSpec{
				Enabled:   true,
				Import:    "github.com/acme/proj/config",
				Type:      "my.Config",
				FieldName: "c",
				ParamName: "cfg2",
			},
			want: &ConfigSpec{
				Enabled:   true,
				Import:    "github.com/acme/proj/config",
				Type:      "my.Config",
				FieldName: "c",
				ParamName: "cfg2",
			},
		},
		{
			name: "fills_only_missing",
			in:   &ConfigSpec{Type: "X"},
			want: &ConfigSpec{Type: "X", FieldName: "cfg", ParamName: "cfg"},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			applyConfigDefaults(tt.in)
			if !reflect.DeepEqual(tt.in, tt.want) {
				t.Fatalf("got %+v want %+v", tt.in, tt.want)
			}
		})
	}
}

// -------------------------
// validateServiceSpec / validateGraphSpec
// -------------------------

func TestValidateServiceSpec(t *testing.T) {
	t.Parallel()

	base := func() ServiceSpec {
		return ServiceSpec{
			Package:       "p",
			WrapperBase:   "W",
			VersionSuffix: "V2",
			ImplType:      "Impl",
			Constructor:   "NewImpl",
			Required: []RequiredDep{
				{Name: "A", Field: "a", Type: "*A", Nilable: true},
			},
			Optional: []OptionalDep{
				{
					Name:        "Opt",
					Type:        "*O",
					RegistryKey: "k",
					Apply:       OptionalApply{Kind: "field", Name: "opt"},
				},
			},
			Methods: []MethodSpec{{Name: "Do"}},
			InjectPolicy: InjectPolicy{
				OnOverwrite: "error",
			},
		}
	}

	tests := []struct {
		name      string
		mutate    func(*ServiceSpec)
		wantPanic string
	}{
		{name: "valid_ok", mutate: func(*ServiceSpec) {}, wantPanic: ""},
		{name: "missing_package", mutate: func(s *ServiceSpec) { s.Package = " " }, wantPanic: "spec missing: package"},
		{name: "missing_wrapperBase", mutate: func(s *ServiceSpec) { s.WrapperBase = "" }, wantPanic: "spec missing: wrapperBase"},
		{name: "missing_versionSuffix", mutate: func(s *ServiceSpec) { s.VersionSuffix = "" }, wantPanic: "spec missing: versionSuffix"},
		{name: "missing_implType", mutate: func(s *ServiceSpec) { s.ImplType = "" }, wantPanic: "spec missing: implType"},
		{name: "missing_constructor", mutate: func(s *ServiceSpec) { s.Constructor = "" }, wantPanic: "spec missing: constructor"},
		{name: "required_empty", mutate: func(s *ServiceSpec) { s.Required = nil }, wantPanic: "spec required must be non-empty"},
		{
			name:      "required_dep_missing_fields",
			mutate:    func(s *ServiceSpec) { s.Required = []RequiredDep{{Name: "A", Field: "", Type: "*A", Nilable: true}} },
			wantPanic: "required dep must have name/field/type",
		},
		{
			name:      "required_dep_nilable_must_be_true",
			mutate:    func(s *ServiceSpec) { s.Required = []RequiredDep{{Name: "A", Field: "a", Type: "*A", Nilable: false}} },
			wantPanic: "required dep must set nilable=true",
		},
		{
			name: "optional_dep_missing_fields",
			mutate: func(s *ServiceSpec) {
				s.Optional = []OptionalDep{{
					Name:        "",
					Type:        "*O",
					RegistryKey: "k",
					Apply:       OptionalApply{Kind: "field", Name: "opt"},
				}}
			},
			wantPanic: "optional dep must have name/type/registryKey/apply{kind,name}",
		},
		{
			name: "optional_dep_invalid_apply_kind",
			mutate: func(s *ServiceSpec) {
				s.Optional = []OptionalDep{{
					Name:        "Opt",
					Type:        "*O",
					RegistryKey: "k",
					Apply:       OptionalApply{Kind: "wat", Name: "opt"},
				}}
			},
			wantPanic: "optional.apply.kind must be 'setter' or 'field'",
		},
		{
			name:      "method_missing_name",
			mutate:    func(s *ServiceSpec) { s.Methods = []MethodSpec{{Name: ""}} },
			wantPanic: "method must have name",
		},
		{
			name:      "inject_policy_invalid",
			mutate:    func(s *ServiceSpec) { s.InjectPolicy.OnOverwrite = "nope" },
			wantPanic: "injectPolicy.onOverwrite must be one of: error|ignore|overwrite",
		},
		{name: "inject_policy_empty_is_allowed", mutate: func(s *ServiceSpec) { s.InjectPolicy.OnOverwrite = "" }, wantPanic: ""},
		{name: "inject_policy_ignore_ok", mutate: func(s *ServiceSpec) { s.InjectPolicy.OnOverwrite = "ignore" }, wantPanic: ""},
		{name: "inject_policy_overwrite_ok", mutate: func(s *ServiceSpec) { s.InjectPolicy.OnOverwrite = "overwrite" }, wantPanic: ""},
		{name: "inject_policy_error_ok", mutate: func(s *ServiceSpec) { s.InjectPolicy.OnOverwrite = "error" }, wantPanic: ""},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			s := base()
			tt.mutate(&s)
			if tt.wantPanic == "" {
				validateServiceSpec(&s)
				return
			}
			assertPanicContains(t, func() { validateServiceSpec(&s) }, tt.wantPanic)
		})
	}
}

func TestValidateGraphSpec(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		g         GraphSpec
		wantPanic string
	}{
		{
			name: "valid_ok",
			g: GraphSpec{
				Package: "p",
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
			},
			wantPanic: "",
		},
		{
			name: "missing_package",
			g: GraphSpec{
				Package: " ",
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
			},
			wantPanic: "graph spec missing package",
		},
		{
			name: "roots_empty",
			g: GraphSpec{
				Package: "p",
				Roots:   nil,
			},
			wantPanic: "graph spec roots must be non-empty",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if tt.wantPanic == "" {
				validateGraphSpec(&tt.g)
				return
			}
			assertPanicContains(t, func() { validateGraphSpec(&tt.g) }, tt.wantPanic)
		})
	}
}

// -------------------------
// go.mod helpers
// -------------------------

func TestFindModule(t *testing.T) {
	t.Parallel()

	t.Run("finds_nearest_go_mod", func(t *testing.T) {
		t.Parallel()
		p := newPkg(t)
		p.write("go.mod", "module example.com/root\n\ngo 1.22\n")
		p.write("a/b/c/x.txt", "x") // create dirs
		start := filepath.Join(p.dir, "a", "b", "c")

		modRoot, modPath, err := findModule(start)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if modRoot == "" || modPath == "" {
			t.Fatalf("empty result: modRoot=%q modPath=%q", modRoot, modPath)
		}
		if modPath != "example.com/root" {
			t.Fatalf("modPath=%q want %q", modPath, "example.com/root")
		}
	})

	t.Run("empty_module_directive_returns_error", func(t *testing.T) {
		t.Parallel()
		p := newPkg(t)
		p.write("go.mod", "module \n\ngo 1.22\n")

		_, _, err := findModule(p.dir)
		if err == nil || !strings.Contains(err.Error(), "go.mod") {
			t.Fatalf("expected error, got %v", err)
		}
		if !strings.Contains(err.Error(), "empty module path") && !strings.Contains(err.Error(), "missing module directive") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("missing_module_directive_returns_error", func(t *testing.T) {
		t.Parallel()
		p := newPkg(t)
		p.write("go.mod", "go 1.22\n")

		_, _, err := findModule(p.dir)
		if err == nil || !strings.Contains(err.Error(), "missing module directive") {
			t.Fatalf("err=%v want contains %q", err, "missing module directive")
		}
	})

	t.Run("no_go_mod_returns_error", func(t *testing.T) {
		t.Parallel()
		p := newPkg(t)
		_, _, err := findModule(p.dir)
		if err == nil || !strings.Contains(err.Error(), "could not find go.mod") {
			t.Fatalf("err=%v want contains %q", err, "could not find go.mod")
		}
	})

	t.Run("readFile_error_returns_raw_os_error", func(t *testing.T) {
		t.Parallel()
		p := newPkg(t)
		gomod := p.write("go.mod", "module example.com/root\n\ngo 1.22\n")
		chmodNoRead(t, gomod)

		_, _, err := findModule(p.dir)
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		if strings.Contains(err.Error(), "missing module directive") ||
			strings.Contains(err.Error(), "could not find go.mod") {
			t.Fatalf("expected raw read error, got: %v", err)
		}
	})
}

func TestModuleImportPathForDir(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		modRoot string
		modPath string
		dir     string
		want    string
		wantErr string
	}{
		{
			name:    "root_dir_is_module_path",
			modRoot: "/repo",
			modPath: "example.com/repo",
			dir:     "/repo",
			want:    "example.com/repo",
		},
		{
			name:    "subdir_appends_rel_path",
			modRoot: "/repo",
			modPath: "example.com/repo",
			dir:     "/repo/pkg/thing",
			want:    "example.com/repo/pkg/thing",
		},
		{
			name:    "outside_module_errors",
			modRoot: "/repo",
			modPath: "example.com/repo",
			dir:     "/other/place",
			wantErr: "directory is outside module root",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := moduleImportPathForDir(tt.modRoot, tt.modPath, tt.dir)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err=%v want contains %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if got != tt.want {
				t.Fatalf("got %q want %q", got, tt.want)
			}
		})
	}
}

// -------------------------
// import scanning/merging helpers
// -------------------------

func TestScanPackageImports_ExcludesGeneratedAndTests_PreservesAlias_DedupesAndSorts(t *testing.T) {
	t.Parallel()

	p := newPkg(t)

	p.write("a.go", `
package p

import (
	config "example.com/proj/config"
	di "example.com/proj/di"
	"fmt"
)
`)

	p.write("a_test.go", `
package p
import "example.com/should/not/appear"
`)

	p.write("z.gen.go", `package p; import "example.com/should/not/appear2"`)
	p.write("x.gen.extra.go", `package p; import "example.com/should/not/appear3"`)
	p.write("y_gen.go", `package p; import "example.com/should/not/appear4"`)

	p.write("b.go", `
package p
import (
	config "example.com/proj/config"
	di "example.com/proj/di"
	strings "strings"
)
`)

	imps := scanPackageImports(p.dir)

	for _, gi := range imps {
		if strings.Contains(gi.Path, "should/not/appear") {
			t.Fatalf("unexpected import leaked from excluded files: %+v", gi)
		}
	}

	want := []GoImport{
		{Name: "config", Path: "example.com/proj/config"},
		{Name: "di", Path: "example.com/proj/di"},
		{Name: "", Path: "fmt"},
		{Name: "strings", Path: "strings"},
	}
	if !reflect.DeepEqual(imps, want) {
		t.Fatalf("got %#v\nwant %#v", imps, want)
	}
}

func TestScanPackageImports_ReadDirError_ReturnsNil(t *testing.T) {
	t.Parallel()
	imps := scanPackageImports(filepath.Join(t.TempDir(), "does-not-exist"))
	if imps != nil {
		t.Fatalf("expected nil, got %#v", imps)
	}
}

func TestScanPackageImports_SkipsUnreadableAndBadParseFiles(t *testing.T) {
	t.Parallel()

	p := newPkg(t)

	unreadable := p.write("unreadable.go", "package p\nimport \"fmt\"\n")
	chmodNoRead(t, unreadable)

	p.write("bad.go", "package p\nimport (\n") // invalid

	p.write("ok.go", `
package p
import di "example.com/proj/di"
func _() { _ = di.Registry(nil) }
`)

	imps := scanPackageImports(p.dir)
	if len(imps) == 0 {
		t.Fatalf("expected some imports, got none")
	}
	found := false
	for _, gi := range imps {
		if gi.Path == "example.com/proj/di" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected to find example.com/proj/di in %v", imps)
	}
}

func TestFindImportByAliasOrSuffix(t *testing.T) {
	t.Parallel()

	imps := []GoImport{
		{Name: "cfg", Path: "example.com/proj/config"},
		{Name: "", Path: "example.com/other/di"},
		{Name: "di", Path: "example.com/proj/di"},
		{Name: "", Path: "strings"},
	}

	tests := []struct {
		name         string
		preferAlias  string
		preferSuffix string
		want         GoImport
		wantOK       bool
	}{
		{
			name:         "alias_match_wins",
			preferAlias:  "di",
			preferSuffix: "/di",
			want:         GoImport{Name: "di", Path: "example.com/proj/di"},
			wantOK:       true,
		},
		{
			name:         "suffix_match_used_when_no_alias",
			preferAlias:  "config",
			preferSuffix: "/di",
			want:         GoImport{Name: "", Path: "example.com/other/di"},
			wantOK:       true,
		},
		{
			name:         "no_match",
			preferAlias:  "zzz",
			preferSuffix: "/zzz",
			want:         GoImport{},
			wantOK:       false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, ok := findImportByAliasOrSuffix(imps, tt.preferAlias, tt.preferSuffix)
			if ok != tt.wantOK {
				t.Fatalf("ok=%v want %v (got=%+v)", ok, tt.wantOK, got)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("got %+v want %+v", got, tt.want)
			}
		})
	}
}

func TestDedupeAndSortImports(t *testing.T) {
	t.Parallel()

	imps := []GoImport{
		{Name: "b", Path: "p"},
		{Name: "a", Path: "p"},
		{Name: "a", Path: "p"},
		{Name: "", Path: "a"},
		{Name: "", Path: "z"},
		{Name: "", Path: "a"},
	}
	got := dedupeAndSortImports(imps)

	want := []GoImport{
		{Name: "", Path: "a"},
		{Name: "a", Path: "p"},
		{Name: "b", Path: "p"},
		{Name: "", Path: "z"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v want %#v", got, want)
	}
}

func TestReadImportsFromExistingOut(t *testing.T) {
	t.Parallel()

	t.Run("missing_returns_nil", func(t *testing.T) {
		t.Parallel()
		p := newPkg(t)
		if got := readImportsFromExistingOut(filepath.Join(p.dir, "missing.go")); got != nil {
			t.Fatalf("expected nil, got %#v", got)
		}
	})

	t.Run("empty_path_returns_nil", func(t *testing.T) {
		t.Parallel()
		if got := readImportsFromExistingOut(""); got != nil {
			t.Fatalf("expected nil, got %#v", got)
		}
	})

	t.Run("parse_error_returns_nil", func(t *testing.T) {
		t.Parallel()
		p := newPkg(t)
		out := p.write("bad.go", "package p\nimport (\n")
		if got := readImportsFromExistingOut(out); got != nil {
			t.Fatalf("expected nil, got %#v", got)
		}
	})

	t.Run("reads_imports", func(t *testing.T) {
		t.Parallel()
		p := newPkg(t)
		out := p.write("x.gen.go", `
package p

import (
	di "example.com/proj/di"
	"fmt"
)
`)
		got := readImportsFromExistingOut(out)
		want := []GoImport{
			{Name: "di", Path: "example.com/proj/di"},
			{Name: "", Path: "fmt"},
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("got %#v want %#v", got, want)
		}
	})
}

func TestMergeImports_DedupesAndSorts(t *testing.T) {
	t.Parallel()

	required := []GoImport{
		{Name: "", Path: "fmt"},
		{Name: "di", Path: "example.com/proj/di"},
	}
	preserved := []GoImport{
		{Name: "config", Path: "example.com/proj/config"},
		{Name: "", Path: "fmt"},
		{Name: "di", Path: "example.com/proj/di"},
		{Name: "di2", Path: "example.com/proj/di"},
		{Name: "", Path: "strings"},
	}

	got := mergeImports(required, preserved)
	want := []GoImport{
		{Name: "config", Path: "example.com/proj/config"},
		{Name: "di", Path: "example.com/proj/di"},
		{Name: "di2", Path: "example.com/proj/di"},
		{Name: "", Path: "fmt"},
		{Name: "", Path: "strings"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v want %#v", got, want)
	}
}

// -------------------------
// small pure helpers
// -------------------------

func TestExportName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		in   string
		want string
	}{
		{"", ""},
		{"a", "A"},
		{"order", "Order"},
		{"Voucher", "Voucher"},
		{"ß", strings.ToUpper("ß"[:1]) + "ß"[1:]},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.in, func(t *testing.T) {
			t.Parallel()
			if got := exportName(tt.in); got != tt.want {
				t.Fatalf("got %q want %q", got, tt.want)
			}
		})
	}
}

func TestMethodUsesPkgQualifier(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		methods []MethodSpec
		pkg     string
		want    bool
	}{
		{name: "no_methods_false", pkg: "context", want: false},
		{
			name: "param_uses_pkg_true",
			pkg:  "context",
			methods: []MethodSpec{
				{Name: "A", Params: []MethodParam{{Name: "ctx", Type: "context.Context"}}},
			},
			want: true,
		},
		{
			name: "return_uses_pkg_true",
			pkg:  "time",
			methods: []MethodSpec{
				{Name: "B", Returns: []MethodReturn{{Type: "time.Duration"}}},
			},
			want: true,
		},
		{
			name: "other_pkg_false",
			pkg:  "context",
			methods: []MethodSpec{
				{Name: "C", Params: []MethodParam{{Name: "x", Type: "foo.Context"}}},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := methodUsesPkgQualifier(tt.methods, tt.pkg); got != tt.want {
				t.Fatalf("got %v want %v", got, tt.want)
			}
		})
	}
}

func TestSha256Hex(t *testing.T) {
	t.Parallel()

	wantEmpty := "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	if got := sha256Hex([]byte("")); got != wantEmpty {
		t.Fatalf("sha256Hex(\"\") got %q want %q", got, wantEmpty)
	}
	if got := sha256Hex([]byte("x")); got == wantEmpty {
		t.Fatalf("sha256Hex(\"x\") unexpectedly equals empty hash")
	}
	if len(sha256Hex([]byte("x"))) != 64 {
		t.Fatalf("expected 64 hex chars")
	}
}

// -------------------------
// inferImportsForService / inferImportsForGraph (DEDUPED USING test_helpers.go)
// -------------------------

func TestInferImportsForService_Cases(t *testing.T) {
	t.Parallel()

	cases := []inferCase[ServiceSpec]{
		{
			name: "config_disabled_empties_config_import_and_reads_di_from_sources",
			setup: func(p *pkgHarness) (*ServiceSpec, string) {
				p.write("a.go", `package p
import di "example.com/proj/di"
func _() { _ = di.Registry(nil) }`)

				s := &ServiceSpec{
					Package: "p", WrapperBase: "W", VersionSuffix: "V2", ImplType: "Impl", Constructor: "NewImpl",
					Imports: Imports{Config: "should_be_cleared"},
					Config:  ConfigSpec{Enabled: false},
					Required: []RequiredDep{
						{Name: "A", Field: "a", Type: "*A", Nilable: true},
					},
				}
				return s, p.out("svc.gen.go")
			},
			call: inferImportsForService,
			assert: func(t *testing.T, s *ServiceSpec) {
				if s.Imports.Config != "" {
					t.Fatalf("Config import should be empty when disabled; got %q", s.Imports.Config)
				}
				if s.Imports.DI != "example.com/proj/di" {
					t.Fatalf("DI import: got %q want %q", s.Imports.DI, "example.com/proj/di")
				}
			},
		},
		{
			name: "config_enabled_no_project_go_mod_panics",
			setup: func(p *pkgHarness) (*ServiceSpec, string) {
				s := &ServiceSpec{
					Package: "p", WrapperBase: "W", VersionSuffix: "V2", ImplType: "Impl", Constructor: "NewImpl",
					Config: ConfigSpec{Enabled: true},
					Required: []RequiredDep{
						{Name: "A", Field: "a", Type: "*A", Nilable: true},
					},
				}
				return s, p.out("svc.gen.go")
			},
			call:      inferImportsForService,
			wantPanic: "cannot find project go.mod",
		},
		{
			name: "config_disabled_no_sources_uses_runtime_di_import",
			setup: func(p *pkgHarness) (*ServiceSpec, string) {
				s := &ServiceSpec{
					Package: "p", WrapperBase: "W", VersionSuffix: "V2", ImplType: "Impl", Constructor: "NewImpl",
					Config: ConfigSpec{Enabled: false},
					Required: []RequiredDep{
						{Name: "A", Field: "a", Type: "*A", Nilable: true},
					},
				}
				return s, p.out("svc.gen.go")
			},
			call: inferImportsForService,
			assert: func(t *testing.T, s *ServiceSpec) {
				if strings.TrimSpace(s.Imports.DI) == "" {
					t.Fatalf("expected DI import inferred from runtime, got empty")
				}
				if !strings.Contains(s.Imports.DI, "/di") {
					t.Fatalf("expected DI import to contain /di, got %q", s.Imports.DI)
				}
			},
		},
	}

	// matrix-driven config-enabled cases (from test_helpers.go)
	serviceMatrix := make([]cfgMatrixRow, 0, len(configMatrix))
	for _, r := range configMatrix {
		r2 := r
		if r2.wantPanic != "" {
			r2.wantPanic = "cannot infer imports.config (service)"
		}
		serviceMatrix = append(serviceMatrix, r2)
	}
	cases = addServiceConfigMatrixCases(cases, serviceMatrix)

	runInferCases(t, cases)
}

func TestInferImportsForGraph_Cases(t *testing.T) {
	t.Parallel()

	cases := []inferCase[GraphSpec]{
		{
			name: "config_disabled_empties_config_import_and_reads_di_from_sources",
			setup: func(p *pkgHarness) (*GraphSpec, string) {
				p.write("a.go", `package p
import di "example.com/proj/di"
func _() { _ = di.Registry(nil) }`)

				g := &GraphSpec{
					Package: "p",
					Imports: Imports{Config: "should_be_cleared"},
					Config:  ConfigSpec{Enabled: false},
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
				return g, p.out("graph.gen.go")
			},
			call: inferImportsForGraph,
			assert: func(t *testing.T, g *GraphSpec) {
				if g.Imports.Config != "" {
					t.Fatalf("Config import should be empty when disabled; got %q", g.Imports.Config)
				}
				if g.Imports.DI != "example.com/proj/di" {
					t.Fatalf("DI import: got %q want %q", g.Imports.DI, "example.com/proj/di")
				}
			},
		},
		{
			name: "config_enabled_no_project_go_mod_panics",
			setup: func(p *pkgHarness) (*GraphSpec, string) {
				g := &GraphSpec{
					Package: "p",
					Config:  ConfigSpec{Enabled: true},
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
				return g, p.out("graph.gen.go")
			},
			call:      inferImportsForGraph,
			wantPanic: "cannot find project go.mod",
		},
		{
			name: "config_disabled_no_sources_uses_runtime_di_import",
			setup: func(p *pkgHarness) (*GraphSpec, string) {
				g := &GraphSpec{
					Package: "p",
					Config:  ConfigSpec{Enabled: false},
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
				return g, p.out("graph.gen.go")
			},
			call: inferImportsForGraph,
			assert: func(t *testing.T, g *GraphSpec) {
				if strings.TrimSpace(g.Imports.DI) == "" {
					t.Fatalf("expected DI import to be inferred from runtime, got empty")
				}
				if !strings.Contains(g.Imports.DI, "/di") {
					t.Fatalf("expected DI import to contain /di, got %q", g.Imports.DI)
				}
			},
		},
	}

	graphMatrix := make([]cfgMatrixRow, 0, len(configMatrix))
	for _, r := range configMatrix {
		r2 := r
		if r2.wantPanic != "" {
			r2.wantPanic = "cannot infer graph imports.config"
		}
		graphMatrix = append(graphMatrix, r2)
	}
	cases = addGraphConfigMatrixCases(cases, graphMatrix)

	runInferCases(t, cases)
}

func TestDirExistsAndFileExists(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	file := filepath.Join(dir, "x.txt")
	mustWriteFile(t, file, "hi")

	tests := []struct {
		name  string
		path  string
		wantD bool
		wantF bool
	}{
		{name: "dir", path: dir, wantD: true, wantF: false},
		{name: "file", path: file, wantD: false, wantF: true},
		{name: "missing", path: filepath.Join(dir, "missing"), wantD: false, wantF: false},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := dirExists(tt.path); got != tt.wantD {
				t.Fatalf("dirExists(%q)=%v want %v", tt.path, got, tt.wantD)
			}
			if got := fileExists(tt.path); got != tt.wantF {
				t.Fatalf("fileExists(%q)=%v want %v", tt.path, got, tt.wantF)
			}
		})
	}
}

func TestInferDIRuntimeImportFromDI2Module_DefaultRelPathAndMissingDir(t *testing.T) {
	t.Parallel()

	got := inferDIRuntimeImportFromDI2Module("")
	if strings.TrimSpace(got) == "" || !strings.Contains(got, "/di") {
		t.Fatalf("expected inferred import to contain /di, got %q", got)
	}

	assertPanicContains(t, func() { inferDIRuntimeImportFromDI2Module("definitely-does-not-exist") }, "expected runtime package dir")
}

// Just a sanity check to ensure runtime.Caller works on this platform.
func TestRuntimeCallerWorks(t *testing.T) {
	t.Parallel()
	_, _, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatalf("runtime.Caller(0) unexpectedly failed")
	}
}

// -------------------------
// writeFormatted / must / run routing
// -------------------------

func TestWriteFormatted_FormatError_WritesRawAndDies(t *testing.T) {
	t.Parallel()

	out := filepath.Join(t.TempDir(), "x.gen.go")
	invalid := []byte("package p\n\nfunc {") // invalid Go => format fails

	assertPanicContains(t, func() { writeFormatted(out, invalid) }, "gofmt/format failed")

	got := mustReadString(t, out)
	if !strings.Contains(got, "func {") {
		t.Fatalf("expected raw src to be written; got:\n%s", got)
	}
}

func TestMust_PanicsOnError(t *testing.T) {
	t.Parallel()
	assertPanicContains(t, func() { must(errors.New("boom")) }, "boom")
}

func TestRun_Routing_ParseError(t *testing.T) {
	t.Parallel()
	err := run([]string{"-out", "x", "-wat"})
	if err == nil {
		t.Fatalf("expected parse error, got nil")
	}
}

func TestRun_Routing_Errors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{name: "missing_out", args: []string{"-spec", "x.json"}, wantErr: "missing -out"},
		{name: "both_spec_and_graph", args: []string{"-out", "x", "-spec", "a", "-graph", "b"}, wantErr: "use only one of -spec or -graph"},
		{name: "missing_spec_and_graph", args: []string{"-out", "x"}, wantErr: "missing -spec or -graph"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := run(tt.args)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("err=%v want contains %q", err, tt.wantErr)
			}
		})
	}
}

func TestRun_Routing_SpecAndGraphHappyPaths(t *testing.T) {
	t.Parallel()

	t.Run("spec_routes_to_genService_and_returns_nil", func(t *testing.T) {
		t.Parallel()
		p := newPkg(t)

		specPath := p.out("service.inject.json")
		outPath := p.out("svc.gen.go")

		spec := ServiceSpec{
			Package:       "p",
			WrapperBase:   "Foo",
			VersionSuffix: "V2",
			ImplType:      "FooImpl",
			Constructor:   "NewFooImpl",
			Config:        ConfigSpec{Enabled: false},
			Required: []RequiredDep{
				{Name: "A", Field: "a", Type: "*A", Nilable: true},
			},
		}
		raw, err := json.Marshal(spec)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		mustWriteFile(t, specPath, string(raw))

		err = run([]string{"-spec", specPath, "-out", outPath})
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if !fileExists(outPath) {
			t.Fatalf("expected generated file at %s", outPath)
		}
	})

	t.Run("graph_routes_to_genGraph_and_returns_nil", func(t *testing.T) {
		t.Parallel()
		p := newPkg(t)

		graphPath := p.out("graph.json")
		outPath := p.out("graph.gen.go")

		g := GraphSpec{
			Package: "p",
			Config:  ConfigSpec{Enabled: false},
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

		raw, err := json.Marshal(g)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		mustWriteFile(t, graphPath, string(raw))

		err = run([]string{"-graph", graphPath, "-out", outPath})
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if !fileExists(outPath) {
			t.Fatalf("expected generated file at %s", outPath)
		}
	})
}

// -------------------------
// genService / genGraph (unchanged; already good coverage)
// -------------------------

func TestGenService_CoversDefaultsSortingImportsPreserveAndStdlibAutoImports(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name          string
		configEnabled bool
		wantConfigImp bool
	}{
		{name: "config_disabled", configEnabled: false, wantConfigImp: false},
		{name: "config_enabled", configEnabled: true, wantConfigImp: true},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			p := newPkg(t)

			outPath := p.out("svc.gen.go")
			specPath := p.out("service.inject.json")

			p.write("a.go", `package p
import di "example.com/proj/di"
func _() { _ = di.Registry(nil) }`)

			if tc.configEnabled {
				p.write("cfg.go", `package p
import config "example.com/proj/config"
var _ = config.Config{}`)
			}

			p.write("svc.gen.go", `package p
import keep "example.com/keep/me"`)

			spec := ServiceSpec{
				Package:       "p",
				WrapperBase:   "Foo",
				VersionSuffix: "V2",
				ImplType:      "FooImpl",
				Constructor:   "NewFooImpl",

				FacadeName:            "",
				PublicConstructorName: "",
				InjectPolicy:          InjectPolicy{OnOverwrite: ""},

				Config: ConfigSpec{Enabled: tc.configEnabled},

				Required: []RequiredDep{
					{Name: "B", Field: "b", Type: "*B", Nilable: true},
					{Name: "A", Field: "a", Type: "*A", Nilable: true},
				},
				Optional: []OptionalDep{
					{Name: "Zed", Type: "*Z", RegistryKey: "zed-key", Apply: OptionalApply{Kind: "field", Name: "zed"}},
					{Name: "Alpha", Type: "*Alpha", RegistryKey: "alpha-key", Apply: OptionalApply{Kind: "setter", Name: "SetAlpha"}},
				},
				Methods: []MethodSpec{
					{
						Name:   "Zeta",
						Params: []MethodParam{{Name: "ctx", Type: "context.Context"}},
						Returns: []MethodReturn{
							{Type: "time.Duration"},
						},
						Requires: []string{"A"},
					},
					{
						Name:   "Alpha",
						Params: []MethodParam{{Name: "x", Type: "int"}},
						Returns: []MethodReturn{
							{Type: "error"},
						},
						Requires: []string{"B"},
					},
				},
			}

			raw, err := json.Marshal(spec)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			mustWriteFile(t, specPath, string(raw))

			genService(specPath, outPath)
			out := p.read("svc.gen.go")

			if !strings.Contains(out, "Spec: "+filepath.ToSlash(specPath)) {
				t.Fatalf("expected Spec path in header")
			}
			if !strings.Contains(out, "Spec-SHA256: "+sha256Hex(raw)) {
				t.Fatalf("expected Spec hash in header")
			}

			if !strings.Contains(out, `keep "example.com/keep/me"`) {
				t.Fatalf("expected preserved import to remain")
			}

			assertHasImport(t, out, "fmt")
			assertHasImport(t, out, "strings")
			assertHasImport(t, out, "context")
			assertHasImport(t, out, "time")
			if !strings.Contains(out, `di "example.com/proj/di"`) {
				t.Fatalf("expected di import inferred from sources")
			}

			if tc.wantConfigImp {
				if !strings.Contains(out, `config "example.com/proj/config"`) {
					t.Fatalf("expected config import when enabled")
				}
				if !strings.Contains(out, "func NewFooV2(cfg config.Config) *FooV2") {
					t.Fatalf("expected ctor signature with cfg when enabled")
				}
			} else {
				if strings.Contains(out, `config "example.com/proj/config"`) {
					t.Fatalf("did not expect config import when disabled")
				}
				if !strings.Contains(out, "func NewFooV2() *FooV2") {
					t.Fatalf("expected ctor signature without cfg when disabled")
				}
			}

			if !strings.Contains(out, `var FooV2InjectPolicyOnOverwrite = "error"`) {
				t.Fatalf("expected InjectPolicy default to error")
			}

			assertContainsInOrder(t, out, "TryInjectA", "TryInjectB")
			assertContainsInOrder(t, out, `= "alpha-key"`, `= "zed-key"`)
			assertContainsInOrder(t, out, "func (b *FooV2) Alpha(", "func (b *FooV2) Zeta(")

			if !strings.Contains(out, `"alpha-key"`) || !strings.Contains(out, `"zed-key"`) {
				t.Fatalf("expected to find optional keys in output")
			}
		})
	}
}

func TestGenGraph_CoversSortingImportsPreserveAndCfgBranch(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name          string
		configEnabled bool
		wantCfgSig    string
		wantCtorCall  string
	}{
		{
			name:          "config_disabled",
			configEnabled: false,
			wantCfgSig:    "func ARoot(reg di.Registry) (ARootResult, error)",
			wantCtorCall:  "xB := NewX()",
		},
		{
			name:          "config_enabled",
			configEnabled: true,
			wantCfgSig:    "func ARoot(cfg config.Config, reg di.Registry) (ARootResult, error)",
			wantCtorCall:  "xB := NewX(cfg)",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			p := newPkg(t)

			outPath := p.out("graph.gen.go")
			graphPath := p.out("graph.json")

			if tc.configEnabled {
				p.write("a.go", `package p
import (
	di "example.com/proj/di"
	config "example.com/proj/config"
)
func _() { _ = di.Registry(nil); _ = config.Config{} }`)
			} else {
				p.write("a.go", `package p
import di "example.com/proj/di"
func _() { _ = di.Registry(nil) }`)
			}

			p.write("graph.gen.go", `package p
import keep "example.com/keep/me"`)

			g := GraphSpec{
				Package: "p",
				Config:  ConfigSpec{Enabled: tc.configEnabled},
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
					{
						Name:              "ZRoot",
						BuildWithRegistry: false,
						Services: []struct {
							Var        string `json:"var"`
							FacadeCtor string `json:"facadeCtor"`
							FacadeType string `json:"facadeType"`
							ImplType   string `json:"implType"`
						}{
							{Var: "b", FacadeCtor: "NewB", FacadeType: "B", ImplType: "BImpl"},
							{Var: "a", FacadeCtor: "NewA", FacadeType: "A", ImplType: "AImpl"},
						},
						Wiring: []struct {
							To      string `json:"to"`
							Call    string `json:"call"`
							ArgFrom string `json:"argFrom"`
						}{
							{To: "b", Call: "InjectX", ArgFrom: "a"},
							{To: "a", Call: "InjectY", ArgFrom: "b"},
						},
					},
					{
						Name:              "ARoot",
						BuildWithRegistry: true,
						Services: []struct {
							Var        string `json:"var"`
							FacadeCtor string `json:"facadeCtor"`
							FacadeType string `json:"facadeType"`
							ImplType   string `json:"implType"`
						}{
							{Var: "x", FacadeCtor: "NewX", FacadeType: "X", ImplType: "XImpl"},
						},
					},
				},
			}

			raw, err := json.Marshal(g)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			mustWriteFile(t, graphPath, string(raw))

			genGraph(graphPath, outPath)
			out := p.read("graph.gen.go")

			if !strings.Contains(out, "Graph: "+filepath.ToSlash(graphPath)) {
				t.Fatalf("expected Graph path in header")
			}
			if !strings.Contains(out, "Graph-SHA256: "+sha256Hex(raw)) {
				t.Fatalf("expected Graph hash in header")
			}

			if !strings.Contains(out, `keep "example.com/keep/me"`) {
				t.Fatalf("expected preserved import to remain")
			}

			assertHasImport(t, out, "fmt")
			if !strings.Contains(out, `di "example.com/proj/di"`) {
				t.Fatalf("expected di import inferred from sources")
			}

			if tc.configEnabled {
				if !strings.Contains(out, `config "example.com/proj/config"`) {
					t.Fatalf("expected config import when enabled")
				}
			} else {
				if strings.Contains(out, `config "example.com/proj/config"`) {
					t.Fatalf("did not expect config import when disabled")
				}
			}

			assertContainsInOrder(t, out, "type ARootResult struct", "type ZRootResult struct")

			if !strings.Contains(out, tc.wantCfgSig) {
				t.Fatalf("expected root signature %q", tc.wantCfgSig)
			}
			if !strings.Contains(out, tc.wantCtorCall) {
				t.Fatalf("expected ctor call %q", tc.wantCtorCall)
			}
		})
	}
}
