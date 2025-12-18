package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"go/format"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/template"
)

type Imports struct {
	DI     string `json:"di"`
	Config string `json:"config"`
}

type InjectPolicy struct {
	OnOverwrite string `json:"onOverwrite"` // "error" | "overwrite" | "ignore"
}

type RequiredDep struct {
	Name    string `json:"name"`
	Field   string `json:"field"`
	Type    string `json:"type"`
	Nilable bool   `json:"nilable"`
}

type OptionalApply struct {
	Kind string `json:"kind"` // "setter" | "field"
	Name string `json:"name"`
}

type OptionalDep struct {
	Name        string        `json:"name"`
	Type        string        `json:"type"`
	RegistryKey string        `json:"registryKey"`
	Apply       OptionalApply `json:"apply"`
}

type MethodParam struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

type MethodReturn struct {
	Type string `json:"type"`
}

type MethodSpec struct {
	Name     string         `json:"name"`
	Params   []MethodParam  `json:"params"`
	Returns  []MethodReturn `json:"returns"`
	Requires []string       `json:"requires"`
}

type ServiceSpec struct {
	Package       string  `json:"package"`
	WrapperBase   string  `json:"wrapperBase"`
	VersionSuffix string  `json:"versionSuffix"`
	ImplType      string  `json:"implType"`
	Constructor   string  `json:"constructor"`
	Imports       Imports `json:"imports"`

	FacadeName            string       `json:"facadeName"`
	PublicConstructorName string       `json:"publicConstructorName"`
	InjectPolicy          InjectPolicy `json:"injectPolicy"`

	// if true, spec indicates cycle wiring; we still generate UnsafeImpl() always
	Cyclic bool `json:"cyclic"`

	Required []RequiredDep `json:"required"`
	Optional []OptionalDep `json:"optional"`
	Methods  []MethodSpec  `json:"methods"`
}

type GraphSpec struct {
	Package string  `json:"package"`
	Imports Imports `json:"imports"`
	Roots   []struct {
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
	} `json:"roots"`
}

func main() {
	specPath := flag.String("spec", "", "path to service.inject.json")
	graphPath := flag.String("graph", "", "path to graph.json")
	outPath := flag.String("out", "", "output .gen.go file path")
	flag.Parse()

	if *outPath == "" {
		die("missing -out")
	}

	switch {
	case *specPath != "" && *graphPath != "":
		die("use only one of -spec or -graph")
	case *specPath != "":
		genService(*specPath, *outPath)
	case *graphPath != "":
		genGraph(*graphPath, *outPath)
	default:
		die("missing -spec or -graph")
	}
}

func genService(specPath, outPath string) {
	raw := mustRead(specPath)

	var spec ServiceSpec
	must(json.Unmarshal(raw, &spec))
	validateServiceSpec(&spec)

	if strings.TrimSpace(spec.FacadeName) == "" {
		spec.FacadeName = spec.WrapperBase + spec.VersionSuffix
	}
	if strings.TrimSpace(spec.PublicConstructorName) == "" {
		spec.PublicConstructorName = "New" + spec.WrapperBase + spec.VersionSuffix
	}
	if spec.InjectPolicy.OnOverwrite == "" {
		spec.InjectPolicy.OnOverwrite = "error"
	}

	specHash := sha256Hex(raw)

	// deterministic ordering (hygiene)
	sort.Slice(spec.Required, func(i, j int) bool { return spec.Required[i].Name < spec.Required[j].Name })
	sort.Slice(spec.Optional, func(i, j int) bool { return spec.Optional[i].Name < spec.Optional[j].Name })
	sort.Slice(spec.Methods, func(i, j int) bool { return spec.Methods[i].Name < spec.Methods[j].Name })

	data := map[string]any{
		"Spec":     spec,
		"SpecPath": filepath.ToSlash(specPath),
		"SpecHash": specHash,
	}

	src := mustExecTemplate(serviceTpl, data)
	writeFormatted(outPath, src)
}

func genGraph(graphPath, outPath string) {
	raw := mustRead(graphPath)

	var g GraphSpec
	must(json.Unmarshal(raw, &g))
	validateGraphSpec(&g)

	graphHash := sha256Hex(raw)

	for i := range g.Roots {
		sort.Slice(g.Roots[i].Services, func(a, b int) bool { return g.Roots[i].Services[a].Var < g.Roots[i].Services[b].Var })
		sort.Slice(g.Roots[i].Wiring, func(a, b int) bool {
			wa := g.Roots[i].Wiring[a]
			wb := g.Roots[i].Wiring[b]
			return wa.To+wa.Call+wa.ArgFrom < wb.To+wb.Call+wb.ArgFrom
		})
	}
	sort.Slice(g.Roots, func(i, j int) bool { return g.Roots[i].Name < g.Roots[j].Name })

	data := map[string]any{
		"G":         g,
		"GraphPath": filepath.ToSlash(graphPath),
		"GraphHash": graphHash,
	}

	src := mustExecTemplate(graphTpl, data)
	writeFormatted(outPath, src)
}

func validateServiceSpec(s *ServiceSpec) {
	req := func(name, v string) {
		if strings.TrimSpace(v) == "" {
			die("spec missing: " + name)
		}
	}
	req("package", s.Package)
	req("wrapperBase", s.WrapperBase)
	req("versionSuffix", s.VersionSuffix)
	req("implType", s.ImplType)
	req("constructor", s.Constructor)
	req("imports.config", s.Imports.Config)
	req("imports.di", s.Imports.DI)

	if len(s.Required) == 0 {
		die("spec required must be non-empty")
	}
	for _, d := range s.Required {
		if d.Name == "" || d.Field == "" || d.Type == "" {
			die("required dep must have name/field/type")
		}
		if !d.Nilable {
			die("required dep must set nilable=true (generator emits nil checks)")
		}
	}
	for _, o := range s.Optional {
		if o.Name == "" || o.Type == "" || o.RegistryKey == "" || o.Apply.Kind == "" || o.Apply.Name == "" {
			die("optional dep must have name/type/registryKey/apply{kind,name}")
		}
		if o.Apply.Kind != "setter" && o.Apply.Kind != "field" {
			die("optional.apply.kind must be 'setter' or 'field'")
		}
	}
	for _, m := range s.Methods {
		if m.Name == "" {
			die("method must have name")
		}
	}

	switch s.InjectPolicy.OnOverwrite {
	case "", "error", "ignore", "overwrite":
	default:
		die("injectPolicy.onOverwrite must be one of: error|ignore|overwrite")
	}
}

func validateGraphSpec(g *GraphSpec) {
	if g.Package == "" || g.Imports.Config == "" || g.Imports.DI == "" {
		die("graph spec missing package/imports")
	}
	if len(g.Roots) == 0 {
		die("graph spec roots must be non-empty")
	}
}

func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func mustRead(path string) []byte {
	b, err := os.ReadFile(path)
	must(err)
	return b
}

func mustExecTemplate(tpl *template.Template, data any) []byte {
	var sb strings.Builder
	must(tpl.Execute(&sb, data))
	return []byte(sb.String())
}

func writeFormatted(out string, src []byte) {
	fmtSrc, err := format.Source(src)
	if err != nil {
		_ = os.WriteFile(out, src, 0o644)
		die("gofmt/format failed: " + err.Error())
	}
	must(os.WriteFile(out, fmtSrc, 0o644))
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}

func die(msg string) {
	panic(msg)
}

// Export helper for graph result fields (Voucher -> Voucher, order -> Order)
func exportName(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

var serviceTpl = template.Must(
	template.New("service").
		Funcs(template.FuncMap{
			"isError": func(t string) bool { return t == "error" },
			"minus1":  func(n int) int { return n - 1 },
		}).
		Parse(`// Code generated by (di v2); DO NOT EDIT.
// Spec: {{.SpecPath}}
// Spec-SHA256: {{.SpecHash}}

package {{.Spec.Package}}

import (
	"fmt"
	config "{{.Spec.Imports.Config}}"
	di "{{.Spec.Imports.DI}}"
)

// {{.Spec.FacadeName}}InjectPolicyOnOverwrite controls behavior when a required dep is injected twice.
// NOTE: generated as a var to allow unit tests to cover all branches.
var {{.Spec.FacadeName}}InjectPolicyOnOverwrite = "{{.Spec.InjectPolicy.OnOverwrite}}"

type {{.Spec.FacadeName}} struct {
	cfg config.Config
	svc *{{.Spec.ImplType}}

	injected map[string]bool
}

func {{.Spec.PublicConstructorName}}(cfg config.Config) *{{.Spec.FacadeName}} {
	return &{{.Spec.FacadeName}}{
		cfg:      cfg,
		svc:      {{.Spec.Constructor}}(cfg),
		injected: map[string]bool{},
	}
}

// UnsafeImpl returns the underlying implementation pointer for composition root wiring.
// It must NOT be used to call business methods before Build()/MustBuild().
func (b *{{.Spec.FacadeName}}) UnsafeImpl() *{{.Spec.ImplType}} { return b.svc }

func (b *{{.Spec.FacadeName}}) Inject(fn func(*{{.Spec.ImplType}})) *{{.Spec.FacadeName}} {
	if fn != nil {
		fn(b.svc)
	}
	return b
}

{{ range .Spec.Required }}
func (b *{{ $.Spec.FacadeName }}) Inject{{ .Name }}(dep {{ .Type }}) *{{ $.Spec.FacadeName }} {
	switch {{ $.Spec.FacadeName }}InjectPolicyOnOverwrite {
	case "error":
		if b.injected["{{ .Name }}"] {
			panic(fmt.Errorf("{{ $.Spec.FacadeName }}: duplicate inject {{ .Name }}"))
		}
	case "ignore":
		if b.injected["{{ .Name }}"] {
			return b
		}
	case "overwrite":
		// allow overwriting
	default:
		panic(fmt.Errorf("{{ $.Spec.FacadeName }}: invalid injectPolicy.onOverwrite=%s", {{ $.Spec.FacadeName }}InjectPolicyOnOverwrite))
	}
	b.svc.{{ .Field }} = dep
	b.injected["{{ .Name }}"] = true
	return b
}
{{ end }}

func (b *{{.Spec.FacadeName}}) Build() (*{{.Spec.ImplType}}, error) {
	return b.buildScoped("Build", nil)
}

// NOTE: RegistryV2.Resolve must be (val any, ok bool, err error)
func (b *{{.Spec.FacadeName}}) BuildWith(reg di.RegistryV2) (*{{.Spec.ImplType}}, error) {
{{ if gt (len .Spec.Optional) 0 }}
	if reg != nil {
{{ range .Spec.Optional }}
		v, ok, err := reg.Resolve(b.cfg, "{{ .RegistryKey }}")
		if err != nil {
			return nil, fmt.Errorf("{{ $.Spec.FacadeName }}: optional dep {{ .Name }} resolve failed: %w", err)
		}
		if ok {
			casted, ok := v.({{ .Type }})
			if !ok {
				return nil, fmt.Errorf("{{ $.Spec.FacadeName }}: optional dep {{ .Name }} key={{ .RegistryKey }}: want {{ .Type }}, got %T", v)
			}
{{ if eq .Apply.Kind "setter" }}
			b.svc.{{ .Apply.Name }}(casted)
{{ else }}
			b.svc.{{ .Apply.Name }} = casted
{{ end }}
		}
{{ end }}
	}
{{ end }}
	return b.buildScoped("BuildWith", nil)
}

func (b *{{.Spec.FacadeName}}) MustBuild() *{{.Spec.ImplType}} {
	svc, err := b.Build()
	if err != nil {
		panic(err)
	}
	return svc
}

func (b *{{.Spec.FacadeName}}) buildScoped(ctx string, reqNames []string) (*{{.Spec.ImplType}}, error) {
	missing := []string{}

{{ range .Spec.Required }}
	isMissing{{ .Name }} := b.svc.{{ .Field }} == nil
{{ end }}

	check := func(name string, isMissing bool) {
		if isMissing {
			missing = append(missing, name)
		}
	}

	if reqNames == nil {
{{ range .Spec.Required }}
		check("{{ .Name }}", isMissing{{ .Name }})
{{ end }}
	} else {
		for _, n := range reqNames {
			switch n {
{{ range .Spec.Required }}
			case "{{ .Name }}":
				check("{{ .Name }}", isMissing{{ .Name }})
{{ end }}
			}
		}
	}

	if len(missing) > 0 {
		return nil, fmt.Errorf("%s: wiring incomplete (ctx=%s, missing=%v, spec=%s)",
			"{{ .Spec.FacadeName }}", ctx, missing, "{{ .SpecHash }}")
	}
	return b.svc, nil
}

{{ range .Spec.Methods }}
func (b *{{ $.Spec.FacadeName }}) {{ .Name }}(
{{- range .Params }}
	{{ .Name }} {{ .Type }},
{{- end }}
){{ if eq (len .Returns) 0 }}{{ else if eq (len .Returns) 1 }} {{ (index .Returns 0).Type }}{{ else }} ({{ range $i, $r := .Returns }}{{ if gt $i 0 }}, {{ end }}{{ $r.Type }}{{ end }}){{ end }} {
	{{- $m := . }}
	svc, err := b.buildScoped("{{ $m.Name }}", []string{
{{- range $m.Requires }}
		"{{ . }}",
{{- end }}
	})
	if err != nil {
{{- if eq (len $m.Returns) 0 }}
		return
{{- else if eq (len $m.Returns) 1 }}
{{- if isError (index $m.Returns 0).Type }}
		return err
{{- else }}
		var zero {{ (index $m.Returns 0).Type }}
		return zero
{{- end }}
{{- else }}
		{{- $last := index $m.Returns (minus1 (len $m.Returns)) }}
		{{- if not (isError $last.Type) }}
		panic(fmt.Errorf("di2: method {{ $m.Name }} last return must be error for safe codegen"))
		{{- end }}

{{- range $i, $r := $m.Returns }}
{{- if lt $i (minus1 (len $m.Returns)) }}
		var zero{{ $i }} {{ $r.Type }}
{{- end }}
{{- end }}

		// IMPORTANT: do NOT trim whitespace between 'return ' and the first expression
		return {{ range $i, $r := $m.Returns }}{{ if lt $i (minus1 (len $m.Returns)) }}zero{{ $i }}, {{ end }}{{ end }}err
{{- end }}
	}

	return svc.{{ $m.Name }}(
{{- range $m.Params }}
		{{ .Name }},
{{- end }}
	)
}
{{ end }}
`),
)

var graphTpl = template.Must(
	template.New("graph").
		Funcs(template.FuncMap{"export": exportName}).
		Parse(`// Code generated by (di v2); DO NOT EDIT.
// Graph: {{.GraphPath}}
// Graph-SHA256: {{.GraphHash}}

package {{.G.Package}}

import (
	"fmt"
	config "{{.G.Imports.Config}}"
	di "{{.G.Imports.DI}}"
)

{{- range .G.Roots}}
{{- $root := . }}

type {{.Name}}Result struct {
	{{- range .Services}}
	{{ export .Var }} *{{.ImplType}}
	{{- end}}
}

func {{.Name}}(cfg config.Config, reg di.RegistryV2) ({{.Name}}Result, error) {
	var res {{.Name}}Result

	{{- range .Services}}
	{{.Var}}B := {{.FacadeCtor}}(cfg)
	{{- end}}

	{{- range .Wiring}}
	{{.To}}B.{{.Call}}({{.ArgFrom}}B.UnsafeImpl())
	{{- end}}

	{{- range .Services}}
	{{- if $root.BuildWithRegistry}}
	{{.Var}}Svc, err := {{.Var}}B.BuildWith(reg)
	{{- else}}
	{{.Var}}Svc, err := {{.Var}}B.Build()
	{{- end}}
	if err != nil {
		return res, fmt.Errorf("{{ $root.Name }}: build {{.Var}} failed: %w", err)
	}
	res.{{ export .Var }} = {{.Var}}Svc
	{{- end}}

	return res, nil
}

{{- end}}
`),
)
