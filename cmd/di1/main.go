// cmd/di1/main.go
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"text/template"
)

// This binary is a code-generation tool.
//
// It reads a JSON specification describing a concrete service implementation and its dependencies,
// then generates a facade / builder that enforces explicit dependency injection and validation at build time.
//
// Key behaviors:
// - Reads spec JSON: package, implType, constructor, required/optional deps
// - Locates the "owner" Go file (the file containing the go:generate for cmd/di1) in the same directory
// - Reads imports from the owner file and reuses them in the generated file (so generated code matches local style)
// - Ensures fmt is imported (Build() returns errors)
// - If the constructor needs config.Config, ensures an import usable as identifier `config` exists
// - Writes output atomically (temp file + rename) to avoid partial writes

// Dep describes a single dependency to be injected into a service.
// Each required dependency results in a generated Inject<Name> method and a build-time check.
type Dep struct {
	// Name is used for method naming (Inject<Name>).
	Name string `json:"name"`

	// Field is the field on the concrete service that receives the dependency.
	Field string `json:"field"`

	// Type is the Go type of the dependency.
	Type string `json:"type"`
}

// Imports defines external packages required by the generated code.
//
// Config is optional now: we prefer reading imports from the owner file.
// It is still supported as a fallback when owner imports do not provide a usable config import.
type Imports struct {
	// Deprecated, kept for backward compatibility with older specs.
	DI string `json:"di"`

	// Optional fallback import path for the config package.
	// Used only when constructor needs config.Config and owner file doesn't provide a usable import.
	Config string `json:"config"`
}

// Spec is the full input schema consumed by the generator.
type Spec struct {
	Package string `json:"package"`

	WrapperBase   string `json:"wrapperBase"`
	VersionSuffix string `json:"versionSuffix"`

	ImplType    string `json:"implType"`
	Constructor string `json:"constructor"`
	FacadeName  string `json:"facadeName"`

	Imports  Imports `json:"imports"`
	Required []Dep   `json:"required"`
	Optional []Dep   `json:"optional"`

	// ConstructorTakesConfig is optional:
	// - nil: auto-detect by parsing the constructor signature
	// - true/false: explicit override
	ConstructorTakesConfig *bool `json:"constructorTakesConfig"`
}

// ImportSpec models one Go import: optional alias and full import path.
type ImportSpec struct {
	Alias string
	Path  string
}

// templateData is the input passed to the Go template.
type templateData struct {
	Spec        Spec
	ImportsList []ImportSpec
	NeedsConfig bool
	ConfigAlias string
}

// run executes the generator logic and returns an exit code.
// It exists separately from main to allow unit testing without os.Exit.
func run(args []string, stderr io.Writer) int {
	flags := flag.NewFlagSet("di1", flag.ContinueOnError)
	flags.SetOutput(stderr)

	specPath := flags.String("spec", "", "path to service.inject.json")
	outPath := flags.String("out", "", "output .gen.go file path")

	if err := flags.Parse(args); err != nil {
		return 2
	}

	if strings.TrimSpace(*specPath) == "" || strings.TrimSpace(*outPath) == "" {
		_, _ = fmt.Fprintln(stderr, "usage: di1 -spec <file.inject.json> -out <file.gen.go>")
		return 2
	}

	specBytes, err := os.ReadFile(*specPath)
	must(err)

	var spec Spec
	must(json.Unmarshal(specBytes, &spec))

	validateSpec(&spec)

	if strings.TrimSpace(spec.FacadeName) == "" {
		spec.FacadeName = spec.WrapperBase + spec.VersionSuffix
	}

	generatedFilePath := filepath.Clean(*outPath)
	packageDir := filepath.Dir(generatedFilePath)

	ownerGoFilePath, err := findOwnerGoGenerateFile(packageDir)
	if err != nil {
		// If we can’t find the owner file, we can still generate.
		// resolveImports will fall back to spec.imports.config when needed.
		ownerGoFilePath = ""
	}

	constructorNeedsConfig := determineConstructorNeedsConfig(&spec, packageDir)

	importsList, err := resolveImports(ownerGoFilePath, &spec, constructorNeedsConfig)
	if err != nil {
		// This is user-actionable: it means we can’t produce valid imports for config.Config.
		panic(err)
	}

	data := templateData{
		Spec:        spec,
		ImportsList: importsList,
		NeedsConfig: constructorNeedsConfig,
		// Generated code always references config.Config when NeedsConfig == true.
		ConfigAlias: "config",
	}

	var out strings.Builder
	must(genTemplate.Execute(&out, data))

	must(writeFileAtomic(generatedFilePath, []byte(out.String()), 0o644))
	return 0
}

func main() {
	os.Exit(run(os.Args[1:], os.Stderr))
}

// validateSpec validates semantic correctness of the input specification.
func validateSpec(spec *Spec) {
	var missingFields []string

	requireNonEmpty := func(fieldName, value string) {
		if strings.TrimSpace(value) == "" {
			missingFields = append(missingFields, fieldName)
		}
	}

	requireNonEmpty("package", spec.Package)
	requireNonEmpty("wrapperBase", spec.WrapperBase)
	requireNonEmpty("versionSuffix", spec.VersionSuffix)
	requireNonEmpty("implType", spec.ImplType)
	requireNonEmpty("constructor", spec.Constructor)

	if len(spec.Required) == 0 {
		missingFields = append(missingFields, "required (must have at least 1)")
	}

	if len(missingFields) > 0 {
		panic(fmt.Errorf("spec missing required fields: %v", missingFields))
	}

	totalDeps := len(spec.Required) + len(spec.Optional)
	seenNames := make(map[string]struct{}, totalDeps)
	seenFields := make(map[string]struct{}, totalDeps)

	validateDep := func(dep Dep) {
		if dep.Name == "" || dep.Field == "" || dep.Type == "" {
			panic(fmt.Errorf("each dep must have name/field/type; got: %+v", dep))
		}
		if _, ok := seenNames[dep.Name]; ok {
			panic(fmt.Errorf("duplicate dep name: %s", dep.Name))
		}
		if _, ok := seenFields[dep.Field]; ok {
			panic(fmt.Errorf("duplicate dep field: %s", dep.Field))
		}
		seenNames[dep.Name] = struct{}{}
		seenFields[dep.Field] = struct{}{}
	}

	for _, dep := range spec.Required {
		validateDep(dep)
	}
	for _, dep := range spec.Optional {
		validateDep(dep)
	}
}

// findOwnerGoGenerateFile finds the Go source file in packageDir that contains a go:generate
// directive invoking cmd/di1.
//
// This is used to discover the owner file’s imports so generated code matches local style.
func findOwnerGoGenerateFile(packageDir string) (string, error) {
	dirEntries, err := os.ReadDir(packageDir)
	if err != nil {
		return "", err
	}

	for _, entry := range dirEntries {
		if entry.IsDir() {
			continue
		}

		fileName := entry.Name()
		if !strings.HasSuffix(fileName, ".go") ||
			strings.HasSuffix(fileName, "_test.go") ||
			strings.HasSuffix(fileName, ".gen.go") {
			continue
		}

		filePath := filepath.Join(packageDir, fileName)
		fileBytes, err := os.ReadFile(filePath)
		if err != nil {
			// Best-effort: unreadable file shouldn’t break generation.
			continue
		}

		if bytes.Contains(fileBytes, []byte("go:generate")) && bytes.Contains(fileBytes, []byte("cmd/di1")) {
			return filePath, nil
		}
	}

	return "", fmt.Errorf("could not find owner file with go:generate invoking cmd/di1 in %s", packageDir)
}

// readImportsFromFile parses imports from a Go file.
func readImportsFromFile(goFilePath string) ([]ImportSpec, error) {
	fileSet := token.NewFileSet()
	parsedFile, err := parser.ParseFile(fileSet, goFilePath, nil, parser.ImportsOnly)
	if err != nil {
		return nil, err
	}

	var imports []ImportSpec
	for _, importDecl := range parsedFile.Imports {
		importPath := strings.Trim(importDecl.Path.Value, `"`)
		importAlias := ""
		if importDecl.Name != nil {
			importAlias = importDecl.Name.Name
		}
		imports = append(imports, ImportSpec{Alias: importAlias, Path: importPath})
	}

	return imports, nil
}

func ensureImport(imports *[]ImportSpec, required ImportSpec) {
	for _, existing := range *imports {
		if existing.Path == required.Path {
			// Don’t duplicate the path; keep existing alias as-is.
			return
		}
	}
	*imports = append(*imports, required)
}

func containsAlias(imports []ImportSpec, alias string) bool {
	for _, existing := range imports {
		if existing.Alias == alias && alias != "" {
			return true
		}
	}
	return false
}

func containsPath(imports []ImportSpec, importPath string) bool {
	for _, existing := range imports {
		if existing.Path == importPath {
			return true
		}
	}
	return false
}

func importDefaultIdent(importPath string) string {
	// Import paths always use forward slashes, even on Windows.
	return path.Base(strings.TrimSpace(importPath))
}

// hasUsableConfigIdent returns true if generated code can refer to `config.Config`
// with the imports currently present.
func hasUsableConfigIdent(imports []ImportSpec) bool {
	// Explicit alias config "..."
	if containsAlias(imports, "config") {
		return true
	}
	// Default identifier is the base of the import path if Alias == "".
	for _, imp := range imports {
		if imp.Alias == "" && importDefaultIdent(imp.Path) == "config" {
			return true
		}
	}
	return false
}

// resolveImports builds the final imports list for the generated file.
//
// Rules:
// - Always ensure fmt is present (Build() uses fmt.Errorf)
// - Prefer imports from owner file, if available
// - If constructor does NOT need config.Config, do not force any config import
// - If constructor needs config.Config, guarantee a usable `config` identifier:
//   - Explicit alias `config "..."`, OR
//   - default import name is `config` (import path base == "config"), OR
//   - fall back to spec.imports.config and import it as `config "..."`.
func resolveImports(ownerFilePath string, spec *Spec, constructorNeedsConfig bool) ([]ImportSpec, error) {
	// Start with owner imports, best-effort.
	var importsFromOwner []ImportSpec
	if strings.TrimSpace(ownerFilePath) != "" {
		parsedOwnerImports, err := readImportsFromFile(ownerFilePath)
		if err == nil {
			importsFromOwner = parsedOwnerImports
		}
		// If parsing fails, fall back to empty and rely on spec fallback behavior.
	}

	finalImports := make([]ImportSpec, 0, len(importsFromOwner)+2)
	finalImports = append(finalImports, importsFromOwner...)

	// fmt is always required by generated Build().
	ensureImport(&finalImports, ImportSpec{Path: "fmt"})

	if !constructorNeedsConfig {
		return finalImports, nil
	}

	// If owner already provides a usable identifier `config`, we’re done.
	if hasUsableConfigIdent(finalImports) {
		return finalImports, nil
	}

	// Otherwise we must add a fallback config import from the spec.
	if strings.TrimSpace(spec.Imports.Config) == "" {
		return nil, fmt.Errorf(
			"constructor %q appears to require config.Config, but no import usable as identifier `config` was found in the owner file and spec.imports.config is empty",
			spec.Constructor,
		)
	}

	// Add an explicit alias import so generated code can reference config.Config.
	ensureImport(&finalImports, ImportSpec{Alias: "config", Path: spec.Imports.Config})
	return finalImports, nil
}

// determineConstructorNeedsConfig decides whether the service constructor takes config.Config.
//
// Behavior:
// - If spec.ConstructorTakesConfig != nil, return it (explicit override).
// - Otherwise, parse files in sourceDir and find a free function named spec.Constructor.
// - If found:
//   - No params -> false
//   - Exactly one param and it’s `config.Config` -> true
//   - Unrecognized signature -> true (backward-compatible default)
//
// - If not found or we cannot read/parse reliably -> true (backward-compatible default)
func determineConstructorNeedsConfig(spec *Spec, sourceDir string) bool {
	if spec.ConstructorTakesConfig != nil {
		return *spec.ConstructorTakesConfig
	}

	dirEntries, err := os.ReadDir(sourceDir)
	if err != nil {
		// Backward-compatible default: assume config.
		return true
	}

	fileSet := token.NewFileSet()

	for _, entry := range dirEntries {
		if entry.IsDir() {
			continue
		}

		fileName := entry.Name()
		if !strings.HasSuffix(fileName, ".go") ||
			strings.HasSuffix(fileName, "_test.go") ||
			strings.HasSuffix(fileName, ".gen.go") {
			continue
		}

		filePath := filepath.Join(sourceDir, fileName)

		// Parse with AllErrors so we can still get partial ASTs when possible.
		parsedFile, parseErr := parser.ParseFile(fileSet, filePath, nil, parser.AllErrors)
		if parsedFile == nil {
			_ = parseErr
			continue
		}

		for _, declaration := range parsedFile.Decls {
			funcDecl, ok := declaration.(*ast.FuncDecl)
			if !ok {
				continue
			}

			// Ignore methods; only free functions are constructors.
			if funcDecl.Recv != nil {
				continue
			}

			if funcDecl.Name == nil || funcDecl.Name.Name != spec.Constructor {
				continue
			}

			paramList := funcDecl.Type.Params
			if paramList == nil || len(paramList.List) == 0 {
				return false
			}

			// Exactly one param: detect config.Config
			if len(paramList.List) == 1 {
				paramType := paramList.List[0].Type

				selectorExpr, ok := paramType.(*ast.SelectorExpr)
				if !ok {
					// One param but not a selector expr: safest default is "needs config".
					return true
				}

				pkgIdent, ok := selectorExpr.X.(*ast.Ident)
				if !ok {
					// Defensive: can happen with partial AST from invalid code; safest default is "needs config".
					return true
				}

				if pkgIdent.Name == "config" && selectorExpr.Sel != nil && selectorExpr.Sel.Name == "Config" {
					return true
				}
			}

			// Unrecognized signature => safest backward-compatible default.
			return true
		}
	}

	// Constructor not found => safest backward-compatible default.
	return true
}

// genTemplate is the Go source template used to generate the facade code.
var genTemplate = template.Must(
	template.New("di1").Parse(`// Code generated by di1; DO NOT EDIT.

package {{.Spec.Package}}

import (
{{range .ImportsList}}
	{{if .Alias}}{{.Alias}} {{end}}"{{.Path}}"
{{end}}
)

// {{.Spec.FacadeName}} is a public facade/builder.
type {{.Spec.FacadeName}} struct {
	svc *{{.Spec.ImplType}}
	{{- range .Spec.Required}}
	has{{.Name}} bool
	{{- end}}
}

{{- if .NeedsConfig}}
func New{{.Spec.FacadeName}}(cfg {{.ConfigAlias}}.Config) *{{.Spec.FacadeName}} {
	return &{{.Spec.FacadeName}}{
		svc: {{.Spec.Constructor}}(cfg),
	}
}
{{- else}}
func New{{.Spec.FacadeName}}() *{{.Spec.FacadeName}} {
	return &{{.Spec.FacadeName}}{
		svc: {{.Spec.Constructor}}(),
	}
}
{{- end}}

{{- range .Spec.Required}}

func (b *{{$.Spec.FacadeName}}) Inject{{.Name}}(dep {{.Type}}) *{{$.Spec.FacadeName}} {
	b.svc.{{.Field}} = dep
	b.has{{.Name}} = true
	return b
}
{{- end}}

func (b *{{.Spec.FacadeName}}) Inject(fn func(*{{.Spec.ImplType}})) *{{.Spec.FacadeName}} {
	if fn != nil {
		fn(b.svc)
	}
	return b
}

func (b *{{.Spec.FacadeName}}) Build() (*{{.Spec.ImplType}}, error) {
	{{- range .Spec.Required}}
	if !b.has{{.Name}} {
		return nil, fmt.Errorf("{{$.Spec.FacadeName}} not wired: missing required dep {{.Name}}")
	}
	{{- end}}
	return b.svc, nil
}

func (b *{{.Spec.FacadeName}}) MustBuild() *{{.Spec.ImplType}} {
	svc, err := b.Build()
	if err != nil {
		panic(err)
	}
	return svc
}
`),
)

// tempFile abstracts an os.File for testability.
type tempFile interface {
	Name() string
	Write([]byte) (int, error)
	Close() error
}

// File operation hooks, overridden in tests.
var (
	createTempFile = func(dir, pattern string) (tempFile, error) { return os.CreateTemp(dir, pattern) }
	chmodFile      = os.Chmod
	renameFile     = os.Rename
	removeFile     = os.Remove
)

// writeFileAtomic writes a file atomically.
//
// It writes to a temporary file in the same directory and then renames it
// over the target path, ensuring readers never observe partial writes.
func writeFileAtomic(targetPath string, data []byte, perm os.FileMode) (err error) {
	targetDir := filepath.Dir(targetPath)

	tmpFile, err := createTempFile(targetDir, filepath.Base(targetPath)+".tmp-*")
	if err != nil {
		return err
	}
	tmpPath := tmpFile.Name()

	defer func() {
		if err != nil {
			_ = removeFile(tmpPath)
		}
	}()

	if _, err = tmpFile.Write(data); err != nil {
		_ = tmpFile.Close()
		return err
	}
	if err = tmpFile.Close(); err != nil {
		return err
	}
	if err = chmodFile(tmpPath, perm); err != nil {
		return err
	}
	if err = renameFile(tmpPath, targetPath); err != nil {
		return err
	}
	return nil
}

// must panics if err is non-nil.
func must(err error) {
	if err != nil {
		panic(err)
	}
}
