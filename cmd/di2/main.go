// odi/di2/main.go
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"go/format"
	"go/parser"
	"go/token"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"text/template"
)

type Imports struct {
	DI     string `json:"di"`
	Config string `json:"config"`
}

// ConfigSpec makes config truly optional.
// If Enabled=false (default), generator will NOT:
// - import config
// - store cfg on the builder
// - require cfg in builder ctor
// - pass cfg to service constructor
type ConfigSpec struct {
	Enabled bool `json:"enabled"`

	// Optional: override inferred import path (e.g. "github.com/acme/proj/config")
	Import string `json:"import"`

	// Optional: override the type used in builder ctor & field (default "config.Config")
	Type string `json:"type"`

	// Optional: override the field name in builder (default "cfg")
	FieldName string `json:"fieldName"`

	// Optional: override the parameter name in builder constructor (default "cfg")
	ParamName string `json:"paramName"`
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

	// Optional: if set, generator emits this expression when registry lookup misses (ok=false).
	// Example: "NoopTracer{}" or "&NoopMetrics{}"
	DefaultExpr string `json:"defaultExpr"`
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
	Package       string `json:"package"`
	WrapperBase   string `json:"wrapperBase"`
	VersionSuffix string `json:"versionSuffix"`
	ImplType      string `json:"implType"`

	// Constructor is a symbol name (in the same package) for the service constructor.
	// It will be called as:
	// - Constructor(cfg) if Config.Enabled=true
	// - Constructor()    if Config.Enabled=false
	Constructor string `json:"constructor"`

	Imports Imports    `json:"imports"`
	Config  ConfigSpec `json:"config"`

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
	Package string `json:"package"`

	Imports Imports    `json:"imports"`
	Config  ConfigSpec `json:"config"`

	Roots []struct {
		Name              string `json:"name"`
		BuildWithRegistry bool   `json:"buildWithRegistry"`
		Services          []struct {
			Var        string `json:"var"`
			FacadeCtor string `json:"facadeCtor"` // symbol name, called with cfg if Config.Enabled=true
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

func run(args []string) error {
	fs := flag.NewFlagSet("di2", flag.ContinueOnError)
	fs.SetOutput(io.Discard) // or os.Stderr if you want CLI output

	specPath := fs.String("spec", "", "path to service.inject.json")
	graphPath := fs.String("graph", "", "path to graph.json")
	outPath := fs.String("out", "", "output .gen.go file path")

	if err := fs.Parse(args); err != nil {
		return err
	}

	if strings.TrimSpace(*outPath) == "" {
		return fmt.Errorf("missing -out")
	}

	switch {
	case *specPath != "" && *graphPath != "":
		return fmt.Errorf("use only one of -spec or -graph")
	case *specPath != "":
		genService(*specPath, *outPath)
		return nil
	case *graphPath != "":
		genGraph(*graphPath, *outPath)
		return nil
	default:
		return fmt.Errorf("missing -spec or -graph")
	}
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		// keep current behavior: fail hard
		panic(err) // or die(err.Error())
	}
}

func genService(specPath, outPath string) {
	raw := mustRead(specPath)

	var spec ServiceSpec
	must(json.Unmarshal(raw, &spec))

	applyConfigDefaults(&spec.Config)
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

	// imports are optional:
	// - config import inferred only if spec.Config.Enabled
	// - di import always needed (BuildWith uses di.Registry)
	inferImportsForService(&spec, outPath)

	specHash := sha256Hex(raw)

	// deterministic ordering (hygiene)
	sort.Slice(spec.Required, func(i, j int) bool { return spec.Required[i].Name < spec.Required[j].Name })
	sort.Slice(spec.Optional, func(i, j int) bool { return spec.Optional[i].Name < spec.Optional[j].Name })
	sort.Slice(spec.Methods, func(i, j int) bool { return spec.Methods[i].Name < spec.Methods[j].Name })

	// Preserve imports from existing generated file (keeps manually added imports)
	preserved := readImportsFromExistingOut(outPath)

	// Required imports for this template
	required := []GoImport{
		{Path: "fmt"},
		{Path: "strings"},
		{Name: "di", Path: spec.Imports.DI}, // always needed because BuildWith(reg di.Registry) exists
	}
	if spec.Config.Enabled {
		required = append(required, GoImport{Name: "config", Path: spec.Imports.Config})
	}

	// auto-import stdlib packages referenced by types in method signatures
	if methodUsesPkgQualifier(spec.Methods, "context") {
		required = append(required, GoImport{Path: "context"})
	}
	if methodUsesPkgQualifier(spec.Methods, "time") {
		required = append(required, GoImport{Path: "time"})
	}

	mergedImports := mergeImports(required, preserved)

	data := map[string]any{
		"Spec":     spec,
		"SpecPath": filepath.ToSlash(specPath),
		"SpecHash": specHash,
		"Imports":  mergedImports,
	}

	src := mustExecTemplate(serviceTpl, data)
	writeFormatted(outPath, src)
}

func genGraph(graphPath, outPath string) {
	raw := mustRead(graphPath)

	var g GraphSpec
	must(json.Unmarshal(raw, &g))

	applyConfigDefaults(&g.Config)
	validateGraphSpec(&g)

	// imports optional:
	// - config import inferred only if g.Config.Enabled
	// - di import always needed (reg di.Registry)
	inferImportsForGraph(&g, outPath)

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

	preserved := readImportsFromExistingOut(outPath)

	required := []GoImport{
		{Path: "fmt"},
		{Name: "di", Path: g.Imports.DI},
	}
	if g.Config.Enabled {
		required = append(required, GoImport{Name: "config", Path: g.Imports.Config})
	}

	mergedImports := mergeImports(required, preserved)

	data := map[string]any{
		"G":         g,
		"GraphPath": filepath.ToSlash(graphPath),
		"GraphHash": graphHash,
		"Imports":   mergedImports,
	}

	src := mustExecTemplate(graphTpl, data)
	writeFormatted(outPath, src)
}

func applyConfigDefaults(c *ConfigSpec) {
	if c == nil {
		return
	}
	if c.Type == "" {
		c.Type = "config.Config"
	}
	if c.FieldName == "" {
		c.FieldName = "cfg"
	}
	if c.ParamName == "" {
		c.ParamName = "cfg"
	}
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
	if strings.TrimSpace(g.Package) == "" {
		die("graph spec missing package")
	}
	if len(g.Roots) == 0 {
		die("graph spec roots must be non-empty")
	}
}

// -------------------------
// Import inference
// -------------------------
//
// Rules implemented:
//
// (1) Config is optional:
//     - Only infer config import if Config.Enabled=true.
// (2) Read needed imports from the original non-generated .go files in the target package dir.
// (3) DI runtime path is from the DI library's own go.mod (the module containing di2),
//     BUT project imports are from the project go.mod (nearest go.mod above outPath dir).
//
// Notes:
// - For config: prefer local-package import (since config is part of the project).
// - For di runtime: prefer local-package import if present (lets a project override/fork),
//   otherwise compute from di2 module via runtime.Caller + findModule.

func inferImportsForService(s *ServiceSpec, outPath string) {
	pkgDir := filepath.Dir(outPath)

	// Scan "original" source files in the target package directory
	scanned := scanPackageImports(pkgDir)

	// --- CONFIG (optional) ---
	if s.Config.Enabled {
		// If user forced config import, honor it.
		if strings.TrimSpace(s.Config.Import) != "" {
			s.Imports.Config = strings.TrimSpace(s.Config.Import)
		} else if strings.TrimSpace(s.Imports.Config) == "" {
			// Prefer whatever the project already uses in source files
			if gi, ok := findImportByAliasOrSuffix(scanned, "config", "/config"); ok {
				s.Imports.Config = gi.Path
			}
		}

		// Fallback: use project go.mod if still missing
		if strings.TrimSpace(s.Imports.Config) == "" {
			modRoot, modPath, err := findModule(pkgDir)
			if err != nil {
				die("cannot infer imports.config (service): config enabled, but no config import in sources and cannot find project go.mod: " + err.Error())
			}
			pkgImport, perr := moduleImportPathForDir(modRoot, modPath, pkgDir)
			if perr != nil || strings.TrimSpace(pkgImport) == "" {
				msg := "cannot infer imports.config (service): cannot compute project pkg import for " + filepath.ToSlash(pkgDir)
				if perr != nil {
					msg += ": " + perr.Error()
				}
				die(msg)
			}
			if !dirExists(filepath.Join(pkgDir, "config")) {
				die("cannot infer imports.config (service): config enabled but ./config directory not found in " + filepath.ToSlash(pkgDir) + " (and not imported in sources)")
			}
			s.Imports.Config = pkgImport + "/config"
		}
	} else {
		// config disabled: ensure empty so template doesn't import it
		s.Imports.Config = ""
	}

	// --- DI (always needed because BuildWith(reg di.Registry) exists) ---
	if strings.TrimSpace(s.Imports.DI) == "" {
		// Prefer what project already imports in package sources (allows override/fork)
		if gi, ok := findImportByAliasOrSuffix(scanned, "di", "/di"); ok {
			s.Imports.DI = gi.Path
		} else {
			// Otherwise: infer DI runtime import from the DI library module (module containing di2)
			s.Imports.DI = inferDIRuntimeImportFromDI2Module("di")
		}
	}
}

func inferImportsForGraph(g *GraphSpec, outPath string) {
	pkgDir := filepath.Dir(outPath)
	scanned := scanPackageImports(pkgDir)

	// CONFIG (optional)
	if g.Config.Enabled {
		if strings.TrimSpace(g.Config.Import) != "" {
			g.Imports.Config = strings.TrimSpace(g.Config.Import)
		} else if strings.TrimSpace(g.Imports.Config) == "" {
			if gi, ok := findImportByAliasOrSuffix(scanned, "config", "/config"); ok {
				g.Imports.Config = gi.Path
			}
		}

		if strings.TrimSpace(g.Imports.Config) == "" {
			modRoot, modPath, err := findModule(pkgDir)
			if err != nil {
				die("cannot infer graph imports.config: config enabled but not imported in sources and cannot find project go.mod: " + err.Error())
			}
			pkgImport, perr := moduleImportPathForDir(modRoot, modPath, pkgDir)
			if perr != nil || strings.TrimSpace(pkgImport) == "" {
				msg := "cannot infer graph imports.config: cannot compute project pkg import for " + filepath.ToSlash(pkgDir)
				if perr != nil {
					msg += ": " + perr.Error()
				}
				die(msg)
			}
			if !dirExists(filepath.Join(pkgDir, "config")) {
				die("cannot infer graph imports.config: config enabled but ./config directory not found in " + filepath.ToSlash(pkgDir) + " (and not imported in sources)")
			}
			g.Imports.Config = pkgImport + "/config"
		}
	} else {
		g.Imports.Config = ""
	}

	// DI (always needed because reg di.Registry exists in graph signature)
	if strings.TrimSpace(g.Imports.DI) == "" {
		if gi, ok := findImportByAliasOrSuffix(scanned, "di", "/di"); ok {
			g.Imports.DI = gi.Path
		} else {
			g.Imports.DI = inferDIRuntimeImportFromDI2Module("di")
		}
	}
}

// inferDIRuntimeImportFromDI2Module computes the import path for the DI runtime package
// based on the go.mod of the module that contains di2 (this generator).
func inferDIRuntimeImportFromDI2Module(runtimePkgRel string) string {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		die("cannot infer di runtime import: runtime.Caller failed")
	}
	genDir := filepath.Dir(thisFile)

	modRoot, modPath, err := findModule(genDir)
	if err != nil {
		die("cannot infer di runtime import: cannot find go.mod for generator module: " + err.Error())
	}

	if strings.TrimSpace(runtimePkgRel) == "" {
		runtimePkgRel = "di"
	}

	runtimeAbs := filepath.Join(modRoot, filepath.FromSlash(runtimePkgRel))
	if !dirExists(runtimeAbs) {
		die("cannot infer di runtime import: expected runtime package dir at " + filepath.ToSlash(runtimeAbs))
	}

	return modPath + "/" + filepath.ToSlash(runtimePkgRel)
}

// -------------------------
// go.mod helpers
// -------------------------

type cmdError struct{ msg string }

func (e *cmdError) Error() string { return e.msg }

func findModule(startDir string) (modRoot string, modPath string, err error) {
	dir := startDir
	for {
		gomod := filepath.Join(dir, "go.mod")
		if fileExists(gomod) {
			b, rerr := os.ReadFile(gomod)
			if rerr != nil {
				return "", "", rerr
			}
			lines := strings.Split(string(b), "\n")
			for _, ln := range lines {
				ln = strings.TrimSpace(ln)
				if strings.HasPrefix(ln, "module ") {
					mod := strings.TrimSpace(strings.TrimPrefix(ln, "module "))
					if mod == "" {
						return "", "", &cmdError{msg: "go.mod has empty module path at " + filepath.ToSlash(gomod)}
					}
					return dir, mod, nil
				}
			}
			return "", "", &cmdError{msg: "go.mod missing module directive at " + filepath.ToSlash(gomod)}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", "", &cmdError{msg: "could not find go.mod starting from " + filepath.ToSlash(startDir)}
}

func moduleImportPathForDir(modRoot, modPath, dir string) (string, error) {
	rel, err := filepath.Rel(modRoot, dir)
	if err != nil {
		return "", err
	}
	rel = filepath.ToSlash(rel)

	if rel == "." {
		return modPath, nil
	}
	if strings.HasPrefix(rel, "../") || rel == ".." {
		return "", &cmdError{msg: "directory is outside module root: dir=" + filepath.ToSlash(dir) + " modRoot=" + filepath.ToSlash(modRoot)}
	}
	return modPath + "/" + rel, nil
}

func dirExists(path string) bool {
	st, err := os.Stat(path)
	return err == nil && st.IsDir()
}

func fileExists(path string) bool {
	st, err := os.Stat(path)
	return err == nil && !st.IsDir()
}

// -------------------------
// Scan "original" files imports in a package dir
// -------------------------

type GoImport struct {
	Name string // optional alias, e.g. "config"
	Path string // import path or stdlib package, e.g. "context"
}

// scanPackageImports reads imports from all non-generated .go files in pkgDir
// (excluding *_test.go and *.gen.go) and returns them as GoImport entries.
// It preserves aliases from source files (e.g. `config "..."`).
func scanPackageImports(pkgDir string) []GoImport {
	entries, err := os.ReadDir(pkgDir)
	if err != nil {
		return nil
	}

	var out []GoImport
	fset := token.NewFileSet()

	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, ".go") {
			continue
		}
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		// avoid feeding generated outputs back into inference
		if strings.HasSuffix(name, ".gen.go") || strings.Contains(name, ".gen.") || strings.HasSuffix(name, "_gen.go") {
			continue
		}

		full := filepath.Join(pkgDir, name)
		src, rerr := os.ReadFile(full)
		if rerr != nil {
			continue
		}

		f, perr := parser.ParseFile(fset, full, src, parser.ImportsOnly)
		if perr != nil {
			continue
		}

		for _, imp := range f.Imports {
			path := strings.Trim(imp.Path.Value, `"`)
			alias := ""
			if imp.Name != nil {
				alias = imp.Name.Name
			}
			out = append(out, GoImport{Name: alias, Path: path})
		}
	}

	return dedupeAndSortImports(out)
}

// findImportByAliasOrSuffix picks an import from scanned imports.
// Prefer alias match first, then suffix match.
func findImportByAliasOrSuffix(imports []GoImport, preferAlias, preferSuffix string) (GoImport, bool) {
	if preferAlias != "" {
		for _, gi := range imports {
			if gi.Name == preferAlias {
				return gi, true
			}
		}
	}
	if preferSuffix != "" {
		for _, gi := range imports {
			if strings.HasSuffix(gi.Path, preferSuffix) {
				return gi, true
			}
		}
	}
	return GoImport{}, false
}

func dedupeAndSortImports(imps []GoImport) []GoImport {
	type key struct {
		path string
		name string
	}
	seen := map[key]bool{}
	out := make([]GoImport, 0, len(imps))
	for _, gi := range imps {
		k := key{path: gi.Path, name: gi.Name}
		if seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, gi)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Path == out[j].Path {
			return out[i].Name < out[j].Name
		}
		return out[i].Path < out[j].Path
	})
	return out
}

// -------------------------
// Import preservation from existing generated file
// -------------------------

func readImportsFromExistingOut(outPath string) []GoImport {
	if strings.TrimSpace(outPath) == "" {
		return nil
	}
	if _, err := os.Stat(outPath); err != nil {
		return nil
	}
	src, err := os.ReadFile(outPath)
	if err != nil {
		return nil
	}

	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, outPath, src, parser.ImportsOnly)
	if err != nil {
		return nil
	}

	out := make([]GoImport, 0, len(f.Imports))
	for _, imp := range f.Imports {
		path := strings.Trim(imp.Path.Value, `"`)
		name := ""
		if imp.Name != nil {
			name = imp.Name.Name
		}
		out = append(out, GoImport{Name: name, Path: path})
	}
	return out
}

func mergeImports(required []GoImport, preserved []GoImport) []GoImport {
	type key struct {
		path string
		name string
	}
	seen := map[key]GoImport{}
	add := func(gi GoImport) {
		k := key{path: gi.Path, name: gi.Name}
		if _, ok := seen[k]; ok {
			return
		}
		seen[k] = gi
	}

	for _, gi := range required {
		add(gi)
	}
	for _, gi := range preserved {
		add(gi)
	}

	out := make([]GoImport, 0, len(seen))
	for _, gi := range seen {
		out = append(out, gi)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Path == out[j].Path {
			return out[i].Name < out[j].Name
		}
		return out[i].Path < out[j].Path
	})
	return out
}

// -------------------------
// Misc helpers
// -------------------------

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

// methodUsesPkgQualifier returns true if any method param/return contains "pkg."
func methodUsesPkgQualifier(methods []MethodSpec, pkg string) bool {
	needle := pkg + "."
	for _, m := range methods {
		for _, p := range m.Params {
			if strings.Contains(p.Type, needle) {
				return true
			}
		}
		for _, r := range m.Returns {
			if strings.Contains(r.Type, needle) {
				return true
			}
		}
	}
	return false
}

// -------------------------
// Templates
// -------------------------

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
{{- range .Imports }}
	{{- if .Name }}
	{{ .Name }} "{{ .Path }}"
	{{- else }}
	"{{ .Path }}"
	{{- end }}
{{- end }}
)

// {{.Spec.FacadeName}}InjectPolicyOnOverwrite controls behavior when a required dep is injected twice.
// NOTE: generated as a var to allow unit tests to cover all branches.
var {{.Spec.FacadeName}}InjectPolicyOnOverwrite = "{{.Spec.InjectPolicy.OnOverwrite}}"

{{- if gt (len .Spec.Optional) 0 }}

// Optional registry keys for {{.Spec.FacadeName}}.
const (
{{- range .Spec.Optional }}
	{{ $.Spec.FacadeName }}Optional{{ .Name }}Key = "{{ .RegistryKey }}"
{{- end }}
)

{{- end }}

type {{.Spec.FacadeName}} struct {
{{- if .Spec.Config.Enabled }}
	{{ .Spec.Config.FieldName }} {{ .Spec.Config.Type }}
{{- end }}
	svc *{{.Spec.ImplType}}

	injected map[string]bool

	// Optional wiring diagnostics (best-effort)
	optionalResolved map[string]string
	optionalMissing  map[string]string
}

// {{.Spec.PublicConstructorName}} creates a new builder/facade.
// You must call Build()/BuildWith()/MustBuild() before calling business methods.
{{- if .Spec.Config.Enabled }}
func {{.Spec.PublicConstructorName}}({{ .Spec.Config.ParamName }} {{ .Spec.Config.Type }}) *{{.Spec.FacadeName}} {
	return &{{.Spec.FacadeName}}{
		{{ .Spec.Config.FieldName }}: {{ .Spec.Config.ParamName }},
		svc:              {{.Spec.Constructor}}({{ .Spec.Config.ParamName }}),
		injected:         map[string]bool{},
		optionalResolved: map[string]string{},
		optionalMissing:  map[string]string{},
	}
}
{{- else }}
func {{.Spec.PublicConstructorName}}() *{{.Spec.FacadeName}} {
	return &{{.Spec.FacadeName}}{
		svc:              {{.Spec.Constructor}}(),
		injected:         map[string]bool{},
		optionalResolved: map[string]string{},
		optionalMissing:  map[string]string{},
	}
}
{{- end }}

// Clone copies the builder with the current injected state.
// Useful for tests and branching wiring paths.
func (b *{{.Spec.FacadeName}}) Clone() *{{.Spec.FacadeName}} {
	nb := &{{.Spec.FacadeName}}{
{{- if .Spec.Config.Enabled }}
		{{ .Spec.Config.FieldName }}: b.{{ .Spec.Config.FieldName }},
{{- end }}
		svc:              b.svc,
		injected:         map[string]bool{},
		optionalResolved: map[string]string{},
		optionalMissing:  map[string]string{},
	}
	for k, v := range b.injected {
		nb.injected[k] = v
	}
	for k, v := range b.optionalResolved {
		nb.optionalResolved[k] = v
	}
	for k, v := range b.optionalMissing {
		nb.optionalMissing[k] = v
	}
	return nb
}

// Reset discards injected bookkeeping and recreates the underlying implementation.
func (b *{{.Spec.FacadeName}}) Reset() *{{.Spec.FacadeName}} {
{{- if .Spec.Config.Enabled }}
	b.svc = {{.Spec.Constructor}}(b.{{ .Spec.Config.FieldName }})
{{- else }}
	b.svc = {{.Spec.Constructor}}()
{{- end }}
	b.injected = map[string]bool{}
	b.optionalResolved = map[string]string{}
	b.optionalMissing = map[string]string{}
	return b
}

// UnsafeImpl returns the underlying implementation pointer for composition root wiring.
// It must NOT be used to call business methods before Build()/MustBuild().
func (b *{{.Spec.FacadeName}}) UnsafeImpl() *{{.Spec.ImplType}} { return b.svc }

// Inject allows custom wiring for advanced usage.
// Prefer InjectX methods for required deps.
func (b *{{.Spec.FacadeName}}) Inject(fn func(*{{.Spec.ImplType}})) *{{.Spec.FacadeName}} {
	if fn != nil {
		fn(b.svc)
	}
	return b
}

{{ range .Spec.Required }}

// TryInject{{ .Name }} injects the required dependency {{ .Name }}.
// Unlike Inject{{ .Name }}, it returns an error instead of panicking.
func (b *{{ $.Spec.FacadeName }}) TryInject{{ .Name }}(dep {{ .Type }}) (*{{ $.Spec.FacadeName }}, error) {
	switch {{ $.Spec.FacadeName }}InjectPolicyOnOverwrite {
	case "error":
		if b.injected["{{ .Name }}"] {
			return nil, fmt.Errorf("{{ $.Spec.FacadeName }}: duplicate inject {{ .Name }}")
		}
	case "ignore":
		if b.injected["{{ .Name }}"] {
			return b, nil
		}
	case "overwrite":
		// allow overwriting
	default:
		return nil, fmt.Errorf("{{ $.Spec.FacadeName }}: invalid injectPolicy.onOverwrite=%s", {{ $.Spec.FacadeName }}InjectPolicyOnOverwrite)
	}
	b.svc.{{ .Field }} = dep
	b.injected["{{ .Name }}"] = true
	return b, nil
}

// Inject{{ .Name }} injects the required dependency {{ .Name }} and panics on policy violations.
// Prefer TryInject{{ .Name }} for safer wiring in tests.
func (b *{{ $.Spec.FacadeName }}) Inject{{ .Name }}(dep {{ .Type }}) *{{ $.Spec.FacadeName }} {
	nb, err := b.TryInject{{ .Name }}(dep)
	if err != nil {
		panic(err)
	}
	return nb
}
{{ end }}

// Missing returns the list of missing required dependency names at this moment.
// This is useful for debug UX before calling Build().
func (b *{{.Spec.FacadeName}}) Missing() []string {
	missing := []string{}
{{- range .Spec.Required }}
	if b.svc.{{ .Field }} == nil {
		missing = append(missing, "{{ .Name }}")
	}
{{- end }}
	return missing
}

// Explain returns a human-friendly summary of the wiring state.
func (b *{{.Spec.FacadeName}}) Explain() string {
	var sb strings.Builder
	m := b.Missing()
	if len(m) == 0 {
		sb.WriteString("required: complete\n")
	} else {
		sb.WriteString(fmt.Sprintf("required: missing=%v\n", m))
	}
{{- if gt (len .Spec.Optional) 0 }}
	if len(b.optionalResolved) > 0 {
		sb.WriteString("optional: resolved\n")
		for k, v := range b.optionalResolved {
			sb.WriteString(fmt.Sprintf("  - %s => %s\n", k, v))
		}
	}
	if len(b.optionalMissing) > 0 {
		sb.WriteString("optional: missing\n")
		for k, v := range b.optionalMissing {
			sb.WriteString(fmt.Sprintf("  - %s => %s\n", k, v))
		}
	}
{{- end }}
	return sb.String()
}

func (b *{{.Spec.FacadeName}}) Build() (*{{.Spec.ImplType}}, error) {
	return b.buildScoped("Build", nil)
}

// NOTE: Registry.Resolve must be (val any, ok bool, err error)
func (b *{{.Spec.FacadeName}}) BuildWith(reg di.Registry) (*{{.Spec.ImplType}}, error) {
{{ if gt (len .Spec.Optional) 0 }}
	if reg != nil {
		// IMPORTANT: declare once; reuse for each optional dep to avoid ":=" redeclare errors.
		var (
			v   any
			ok  bool
			err error
		)

{{ range .Spec.Optional }}
		v, ok, err = reg.Resolve({{ if $.Spec.Config.Enabled }}b.{{ $.Spec.Config.FieldName }}{{ else }}nil{{ end }}, "{{ .RegistryKey }}")
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
			b.optionalResolved["{{ .RegistryKey }}"] = fmt.Sprintf("%T", v)
		} else {
{{- if ne (print .DefaultExpr) "" }}
			def := {{ .DefaultExpr }}
{{- if eq .Apply.Kind "setter" }}
			b.svc.{{ .Apply.Name }}(def)
{{- else }}
			b.svc.{{ .Apply.Name }} = def
{{- end }}
			b.optionalMissing["{{ .RegistryKey }}"] = "used defaultExpr"
{{- else }}
			b.optionalMissing["{{ .RegistryKey }}"] = "not provided"
{{- end }}
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
{{- range .Imports }}
	{{- if .Name }}
	{{ .Name }} "{{ .Path }}"
	{{- else }}
	"{{ .Path }}"
	{{- end }}
{{- end }}
)

{{- range .G.Roots}}
{{- $root := . }}

type {{.Name}}Result struct {
	{{- range .Services}}
	{{ export .Var }} *{{.ImplType}}
	{{- end}}
}

{{- if $.G.Config.Enabled }}
func {{.Name}}({{ $.G.Config.ParamName }} {{ $.G.Config.Type }}, reg di.Registry) ({{.Name}}Result, error) {
{{- else }}
func {{.Name}}(reg di.Registry) ({{.Name}}Result, error) {
{{- end }}
	var res {{.Name}}Result

	{{- range .Services}}
	{{.Var}}B := {{.FacadeCtor}}({{ if $.G.Config.Enabled }}{{ $.G.Config.ParamName }}{{ end }})
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
