package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"text/template"
)

// This binary is a code-generation tool.
//
// It reads a JSON specification describing a concrete service implementation
// and its dependencies, and generates a facade / builder that enforces
// explicit dependency injection and validation at build time.

// Dep describes a single dependency to be injected into a service.
//
// Each dependency results in a generated Inject<Name> method on the facade.
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
// Only Config is currently used by the facade generator.
// DI is kept for backward compatibility with older specifications.
type Imports struct {
	DI     string `json:"di"`
	Config string `json:"config"`
}

// Spec is the full input schema consumed by the generator.
//
// It is loaded from a JSON file passed via the -spec CLI flag.
type Spec struct {
	// Package is the target Go package for the generated file.
	Package string `json:"package"`

	// WrapperBase and VersionSuffix are combined to form the default facade name.
	// Example: User + V3 => UserV3.
	WrapperBase   string `json:"wrapperBase"`
	VersionSuffix string `json:"versionSuffix"`

	// ImplType is the concrete service type being wrapped.
	ImplType string `json:"implType"`

	// Constructor is the function used to construct the service.
	Constructor string `json:"constructor"`

	// Deprecated fields retained for backward compatibility.
	DepsStructName string `json:"depsStructName"`
	DepsFieldName  string `json:"depsFieldName"`

	// Imports contains external package references used by generated code.
	Imports Imports `json:"imports"`

	// Required lists dependencies that must be injected before Build().
	Required []Dep `json:"required"`

	// Optional lists dependencies that are validated for uniqueness only.
	Optional []Dep `json:"optional"`

	// FacadeName optionally overrides the generated facade name.
	// If empty, WrapperBase + VersionSuffix is used.
	FacadeName string `json:"facadeName"`
}

// run executes the generator logic and returns an exit code.
//
// It exists separately from main to allow unit testing without os.Exit.
func run(args []string, stderr io.Writer) int {
	fs := flag.NewFlagSet("di1", flag.ContinueOnError)
	fs.SetOutput(stderr)

	specPath := fs.String("spec", "", "path to service.inject.json")
	outPath := fs.String("out", "", "output .gen.go file path")

	if err := fs.Parse(args); err != nil {
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

	finalOut := filepath.Clean(*outPath)

	var b strings.Builder
	must(genTemplate.Execute(&b, spec))

	must(writeFileAtomic(finalOut, []byte(b.String()), 0o644))
	return 0
}

// main delegates execution to run and exits with the returned status code.
func main() {
	os.Exit(run(os.Args[1:], os.Stderr))
}

// validateSpec validates the semantic correctness of the input specification.
//
// It ensures required fields are present, required dependencies exist,
// and dependency names and fields are unique.
func validateSpec(s *Spec) {
	var missing []string

	req := func(name, v string) {
		if strings.TrimSpace(v) == "" {
			missing = append(missing, name)
		}
	}

	req("package", s.Package)
	req("wrapperBase", s.WrapperBase)
	req("versionSuffix", s.VersionSuffix)
	req("implType", s.ImplType)
	req("constructor", s.Constructor)
	req("imports.config", s.Imports.Config)

	if len(s.Required) == 0 {
		missing = append(missing, "required (must have at least 1)")
	}

	if len(missing) > 0 {
		panic(fmt.Errorf("spec missing required fields: %v", missing))
	}

	total := len(s.Required) + len(s.Optional)
	seenName := make(map[string]struct{}, total)
	seenField := make(map[string]struct{}, total)

	check := func(d Dep) {
		if d.Name == "" || d.Field == "" || d.Type == "" {
			panic(fmt.Errorf("each dep must have name/field/type; got: %+v", d))
		}
		if _, ok := seenName[d.Name]; ok {
			panic(fmt.Errorf("duplicate dep name: %s", d.Name))
		}
		if _, ok := seenField[d.Field]; ok {
			panic(fmt.Errorf("duplicate dep field: %s", d.Field))
		}
		seenName[d.Name] = struct{}{}
		seenField[d.Field] = struct{}{}
	}

	for _, d := range s.Required {
		check(d)
	}
	for _, d := range s.Optional {
		check(d)
	}
}

// genTemplate is the Go source template used to generate the facade code.
var genTemplate = template.Must(
	template.New("").Parse(`// Code generated by (di v1 wrap); DO NOT EDIT.

package {{.Package}}

import (
	"fmt"
	config "{{.Imports.Config}}"
)

// {{.FacadeName}} is a public facade/builder.
type {{.FacadeName}} struct {
	svc *{{.ImplType}}
	{{- range .Required}}
	has{{.Name}} bool
	{{- end}}
}

func New{{.FacadeName}}(cfg config.Config) *{{.FacadeName}} {
	return &{{.FacadeName}}{
		svc: {{.Constructor}}(cfg),
	}
}

{{- range .Required}}

func (b *{{$.FacadeName}}) Inject{{.Name}}(dep {{.Type}}) *{{$.FacadeName}} {
	b.svc.{{.Field}} = dep
	b.has{{.Name}} = true
	return b
}
{{- end}}

func (b *{{.FacadeName}}) Inject(fn func(*{{.ImplType}})) *{{.FacadeName}} {
	if fn != nil {
		fn(b.svc)
	}
	return b
}

func (b *{{.FacadeName}}) Build() (*{{.ImplType}}, error) {
	{{- range .Required}}
	if !b.has{{.Name}} {
		return nil, fmt.Errorf("{{$.FacadeName}} not wired: missing required dep {{.Name}}")
	}
	{{- end}}
	return b.svc, nil
}

func (b *{{.FacadeName}}) MustBuild() *{{.ImplType}} {
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
	createTemp = func(dir, pattern string) (tempFile, error) { return os.CreateTemp(dir, pattern) }
	chmodFn    = os.Chmod
	renameFn   = os.Rename
	removeFn   = os.Remove
)

// writeFileAtomic writes a file atomically.
//
// It writes to a temporary file in the same directory and then renames it
// over the target path, ensuring readers never observe partial writes.
func writeFileAtomic(path string, data []byte, perm os.FileMode) (err error) {
	dir := filepath.Dir(path)
	tmp, err := createTemp(dir, filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()

	defer func() {
		if err != nil {
			_ = removeFn(tmpName)
		}
	}()

	if _, err = tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err = tmp.Close(); err != nil {
		return err
	}
	if err = chmodFn(tmpName, perm); err != nil {
		return err
	}
	if err = renameFn(tmpName, path); err != nil {
		return err
	}
	return nil
}

// must panics if err is non-nil.
//
// It is used to keep the generator code readable by avoiding
// repetitive error handling on the happy path.
func must(err error) {
	if err != nil {
		panic(err)
	}
}
