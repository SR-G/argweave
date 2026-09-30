// Package docgen provides the static, AST-based struct introspection and
// file-injection helpers shared by argweave's `go generate` doc tools
// (cmd/weavedoc and cmd/weavehelp). It parses a Go source file (or
// directory) with go/ast to discover `arg`-tagged struct fields, without
// reflecting on or compiling the target package.
package docgen

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	argweave "github.com/SR-G/argweave"
)

// Marker tag pieces used to delimit generated content injected into an
// existing file (e.g. `<!-- weavedoc:start -->` ... `<!-- weavedoc:end -->`).
const (
	MarkerTagPrefix = "<!-- "
	MarkerTagSuffix = " -->"

	MarkerTagBegin = ":start"
	MarkerTagEnd   = ":end"

	MarkerTagLabelDefault = "weavedoc"
)

// BuildMarkerTag assembles a full marker comment (e.g. "<!-- weavedoc:start -->")
// from a label and a begin/end suffix.
func BuildMarkerTag(label, tag string) string {
	return MarkerTagPrefix + label + tag + MarkerTagSuffix
}

// DocField is the documentation-relevant information extracted for a
// single struct field.
type DocField struct {
	Spec     argweave.FieldSpec
	Name     string
	TypeName string
}

// ResolveFiles returns the list of *.go files (excluding _test.go) to
// parse from a comma-separated list of files and/or directories, so a
// struct embedding a sub-struct defined elsewhere (a sibling package, not
// just a sibling file) can be documented by listing its location too.
func ResolveFiles(fileOrDirList string) ([]string, error) {
	var files []string
	seen := map[string]bool{}
	for _, entry := range strings.Split(fileOrDirList, ",") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		entryFiles, err := resolveFileOrDir(entry)
		if err != nil {
			return nil, err
		}
		for _, f := range entryFiles {
			if !seen[f] {
				seen[f] = true
				files = append(files, f)
			}
		}
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("no .go files found in %s", fileOrDirList)
	}
	return files, nil
}

// resolveFileOrDir expands a single file or directory entry into its *.go
// files (excluding _test.go); a plain file is returned as-is.
func resolveFileOrDir(fileOrDir string) ([]string, error) {
	info, err := os.Stat(fileOrDir)
	if err != nil {
		return nil, fmt.Errorf("stat %s: %w", fileOrDir, err)
	}
	if !info.IsDir() {
		return []string{fileOrDir}, nil
	}
	matches, err := filepath.Glob(filepath.Join(fileOrDir, "*.go"))
	if err != nil {
		return nil, err
	}
	var files []string
	for _, m := range matches {
		if !strings.HasSuffix(m, "_test.go") {
			files = append(files, m)
		}
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("no .go files found in %s", fileOrDir)
	}
	return files, nil
}

// ExtractFields statically parses fileOrDir (a comma-separated list of Go
// source files and/or directories) and returns the documentation fields
// of typeName, flattening embedded (or plain named) struct fields that
// have no `arg` tag of their own - mirroring the runtime parser.
func ExtractFields(fileOrDir, typeName string) ([]DocField, error) {
	files, err := ResolveFiles(fileOrDir)
	if err != nil {
		return nil, err
	}

	fset := token.NewFileSet()
	structs := map[string]*ast.StructType{}
	var duplicateType string
	for _, f := range files {
		astFile, err := parser.ParseFile(fset, f, nil, parser.ParseComments)
		if err != nil {
			return nil, fmt.Errorf("parsing %s: %w", f, err)
		}
		ast.Inspect(astFile, func(n ast.Node) bool {
			ts, ok := n.(*ast.TypeSpec)
			if !ok {
				return true
			}
			if st, ok := ts.Type.(*ast.StructType); ok {
				if _, exists := structs[ts.Name.Name]; exists {
					duplicateType = ts.Name.Name
					return false
				}
				structs[ts.Name.Name] = st
			}
			return true
		})
		if duplicateType != "" {
			return nil, fmt.Errorf("duplicate struct type %q found while reading %s", duplicateType, fileOrDir)
		}
	}

	st, ok := structs[typeName]
	if !ok {
		return nil, fmt.Errorf("type %q not found (or is not a struct) in %s", typeName, fileOrDir)
	}
	return flattenStruct(st, structs, map[string]bool{})
}

// flattenStruct extracts documentation fields from st, recursing into
// struct-typed fields (embedded or plain named, value or pointer) that
// have no `arg` tag of their own - mirroring the runtime parser, so a
// nested config group (e.g. a DatabaseConfig field) is documented the
// same way whether it's embedded or referenced by name.
func flattenStruct(st *ast.StructType, structs map[string]*ast.StructType, active map[string]bool) ([]DocField, error) {
	// The struct pointer is not enough to produce a useful error, so the
	// caller tracks named types through this helper's active map.
	var currentName string
	for name, candidate := range structs {
		if candidate == st {
			currentName = name
			break
		}
	}
	if currentName != "" {
		if active[currentName] {
			return nil, fmt.Errorf("recursive struct flattening detected for type %q", currentName)
		}
		active[currentName] = true
		defer delete(active, currentName)
	}
	var fields []DocField
	for _, f := range st.Fields.List {
		if f.Tag == nil {
			if ident := structTypeIdent(f.Type); ident != nil {
				if embedded, found := structs[ident.Name]; found {
					sub, err := flattenStruct(embedded, structs, active)
					if err != nil {
						return nil, err
					}
					fields = append(fields, sub...)
				}
			}
			continue
		}
		if len(f.Names) == 0 {
			continue
		}
		tagValue, err := strconv.Unquote(f.Tag.Value)
		if err != nil {
			continue
		}
		argTag := reflectStructTagLookup(tagValue, argweave.TAG_KEY)
		if argTag == "" {
			continue
		}
		fieldName := f.Names[0].Name
		spec, err := argweave.ParseTag(argTag, fieldName)
		if err != nil {
			return nil, fmt.Errorf("field %s: %w", fieldName, err)
		}
		if spec.Hidden {
			continue
		}
		if spec.Help == "" {
			if doc := strings.TrimSpace(f.Doc.Text()); doc != "" {
				spec.Help = strings.TrimSuffix(doc, "\n")
			} else if cmt := strings.TrimSpace(f.Comment.Text()); cmt != "" {
				spec.Help = strings.TrimSuffix(cmt, "\n")
			}
		}
		fields = append(fields, DocField{Spec: spec, Name: fieldName, TypeName: formatType(f.Type)})
	}
	return fields, nil
}

func formatType(expr ast.Expr) string {
	var buf bytes.Buffer
	if err := format.Node(&buf, token.NewFileSet(), expr); err != nil {
		return "unknown"
	}
	return buf.String()
}

// structTypeIdent returns the identifier naming expr's struct type,
// unwrapping a single pointer indirection (*T), or nil if expr isn't a
// plain (possibly pointer) named type.
func structTypeIdent(expr ast.Expr) *ast.Ident {
	switch t := expr.(type) {
	case *ast.Ident:
		return t
	case *ast.StarExpr:
		return structTypeIdent(t.X)
	default:
		return nil
	}
}

// reflectStructTagLookup extracts the value for key from a raw struct tag
// string without requiring the reflect.StructTag type (which needs the
// tag to come from a compiled type).
func reflectStructTagLookup(tag, key string) string {
	return string(structTag(tag).lookup(key))
}

type structTag string

func (tag structTag) lookup(key string) string {
	t := string(tag)
	for t != "" {
		t = strings.TrimLeft(t, " \t")
		if t == "" {
			break
		}
		i := 0
		for i < len(t) && t[i] > ' ' && t[i] != ':' && t[i] != '"' && t[i] != 0x7f {
			i++
		}
		if i == 0 || i+1 >= len(t) || t[i] != ':' || t[i+1] != '"' {
			break
		}
		name := t[:i]
		t = t[i+1:]

		i = 1
		for i < len(t) && t[i] != '"' {
			if t[i] == '\\' {
				i++
			}
			i++
		}
		if i >= len(t) {
			break
		}
		qvalue := t[:i+1]
		t = t[i+1:]

		if name == key {
			value, err := strconv.Unquote(qvalue)
			if err != nil {
				return ""
			}
			return value
		}
	}
	return ""
}

// InjectIntoFile replaces the content between markerBlockStart and
// markerBlockEnd in path with content, preserving everything else.
func InjectIntoFile(path, content, markerBlockStart, markerBlockEnd string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("reading %s: %w", path, err)
	}
	fileContent := string(data)

	startIdx := strings.Index(fileContent, markerBlockStart)
	endIdx := strings.Index(fileContent, markerBlockEnd)
	if startIdx == -1 || endIdx == -1 || endIdx < startIdx {
		return fmt.Errorf("markers %q / %q not found (in this order) in %s", markerBlockStart, markerBlockEnd, path)
	}

	before := fileContent[:startIdx+len(markerBlockStart)]
	after := fileContent[endIdx:]
	newContent := before + "\n" + content + "\n" + after

	if err := os.WriteFile(path, []byte(newContent), 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	return nil
}
