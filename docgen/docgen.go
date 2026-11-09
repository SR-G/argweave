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
	"reflect"
	"strconv"
	"strings"

	argweave "github.com/SR-G/argweave"
)

// Marker tag pieces used to delimit generated content injected into an
// existing file (e.g. `<!-- weavedoc:start -->` ... `<!-- weavedoc:end -->`).
const (
	MARKER_TAG_PREFIX = "<!-- "
	MARKER_TAG_SUFFIX = " -->"
	MARKER_TAG_BEGIN  = ":start"
	MARKER_TAG_END    = ":end"
)

// DocField is the documentation-relevant information extracted for a
// single struct field: the shared HelpEntry plus the Go type as written in
// source (e.g. "[]time.Duration"), which documentation generators need for
// their type columns and schemas.
type DocField struct {
	argweave.HelpEntry
	TypeName string
}

// RequireSource checks the -type and -file flags shared by the doc generators.
func RequireSource(typeName, file string) error {
	if typeName == "" {
		return fmt.Errorf("-type is required")
	}
	if file == "" {
		return fmt.Errorf("-file is required (or run via go:generate so $GOFILE is set)")
	}
	return nil
}

// RequireOneOf fails when every value is empty, naming the flags in the error
// message in the order given.
func RequireOneOf(names []string, values ...string) error {
	for _, value := range values {
		if value != "" {
			return nil
		}
	}
	last := len(names) - 1
	return fmt.Errorf("one of %s or %s is required", strings.Join(names[:last], ", "), names[last])
}

// resolveFiles returns the list of *.go files (excluding _test.go) to
// parse from a comma-separated list of files and/or directories, so a
// struct embedding a sub-struct defined elsewhere (a sibling package, not
// just a sibling file) can be documented by listing its location too.
func resolveFiles(fileOrDirList string) ([]string, error) {
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
	files, err := resolveFiles(fileOrDir)
	if err != nil {
		return nil, err
	}

	fset := token.NewFileSet()
	structs := map[string]*ast.StructType{}
	var astFiles []*ast.File
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
		astFiles = append(astFiles, astFile)
	}

	st, ok := structs[typeName]
	if !ok {
		return nil, fmt.Errorf("type %q not found (or is not a struct) in %s", typeName, fileOrDir)
	}
	return flattenStruct(typeName, st, structs, extractEnumValues(astFiles), map[string]bool{})
}

// flattenStruct extracts documentation fields from st, recursing into
// struct-typed fields (embedded or plain named, value or pointer) that
// have no `arg` tag of their own - mirroring the runtime parser, so a
// nested config group (e.g. a DatabaseConfig field) is documented the
// same way whether it's embedded or referenced by name.
func flattenStruct(typeName string, st *ast.StructType, structs map[string]*ast.StructType, enumValues map[string][]string, active map[string]bool) ([]DocField, error) {
	if active[typeName] {
		return nil, fmt.Errorf("recursive struct flattening detected for type %q", typeName)
	}
	active[typeName] = true
	defer delete(active, typeName)
	var fields []DocField
	for _, f := range st.Fields.List {
		if f.Tag == nil {
			if ident := structTypeIdent(f.Type); ident != nil {
				if embedded, found := structs[ident.Name]; found {
					sub, err := flattenStruct(ident.Name, embedded, structs, enumValues, active)
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
		argTag := reflect.StructTag(tagValue).Get(argweave.TAG_KEY)
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
		var values []string
		if ident := fieldTypeIdent(f.Type); ident != nil {
			values = enumValues[ident.Name]
		}
		typeText := formatType(f.Type)
		fields = append(fields, DocField{
			HelpEntry: argweave.HelpEntry{
				FieldSpec:  spec,
				FieldName:  fieldName,
				IsBool:     typeText == "bool",
				IsSlice:    strings.HasPrefix(typeText, "[]"),
				EnumValues: values,
			},
			TypeName: typeText,
		})
	}
	return fields, nil
}

func extractEnumValues(files []*ast.File) map[string][]string {
	constants := map[string]ast.Expr{}
	methods := map[string]*ast.FuncDecl{}
	for _, file := range files {
		for _, decl := range file.Decls {
			switch decl := decl.(type) {
			case *ast.GenDecl:
				if decl.Tok != token.CONST {
					continue
				}
				var previousValues []ast.Expr
				for _, spec := range decl.Specs {
					valueSpec := spec.(*ast.ValueSpec)
					if len(valueSpec.Values) > 0 {
						previousValues = valueSpec.Values
					}
					for index, name := range valueSpec.Names {
						switch {
						case len(previousValues) == len(valueSpec.Names):
							constants[name.Name] = previousValues[index]
						case len(previousValues) == 1:
							constants[name.Name] = previousValues[0]
						}
					}
				}
			case *ast.FuncDecl:
				if decl.Name.Name != "EnumValues" || decl.Recv == nil || len(decl.Recv.List) == 0 {
					continue
				}
				if receiver := fieldTypeIdent(decl.Recv.List[0].Type); receiver != nil {
					methods[receiver.Name] = decl
				}
			}
		}
	}

	result := make(map[string][]string, len(methods))
	for typeName, method := range methods {
		if values, ok := enumValuesFromMethod(method, constants); ok {
			result[typeName] = values
		}
	}
	return result
}

func enumValuesFromMethod(method *ast.FuncDecl, constants map[string]ast.Expr) ([]string, bool) {
	if method.Body == nil {
		return nil, false
	}
	for _, statement := range method.Body.List {
		ret, ok := statement.(*ast.ReturnStmt)
		if !ok || len(ret.Results) != 1 {
			continue
		}
		values, ok := ret.Results[0].(*ast.CompositeLit)
		if !ok {
			continue
		}
		array, ok := values.Type.(*ast.ArrayType)
		if !ok || array.Len != nil {
			continue
		}
		elementType, ok := array.Elt.(*ast.Ident)
		if !ok || elementType.Name != "string" {
			continue
		}
		result := make([]string, 0, len(values.Elts))
		for _, element := range values.Elts {
			value, ok := resolveStringConstant(element, constants, map[string]bool{})
			if !ok {
				return nil, false
			}
			result = append(result, value)
		}
		return result, true
	}
	return nil, false
}

func resolveStringConstant(expr ast.Expr, constants map[string]ast.Expr, active map[string]bool) (string, bool) {
	switch expr := expr.(type) {
	case *ast.BasicLit:
		if expr.Kind != token.STRING {
			return "", false
		}
		value, err := strconv.Unquote(expr.Value)
		return value, err == nil
	case *ast.Ident:
		constant, ok := constants[expr.Name]
		if !ok || active[expr.Name] {
			return "", false
		}
		active[expr.Name] = true
		value, ok := resolveStringConstant(constant, constants, active)
		delete(active, expr.Name)
		return value, ok
	case *ast.ParenExpr:
		return resolveStringConstant(expr.X, constants, active)
	case *ast.CallExpr:
		name, ok := expr.Fun.(*ast.Ident)
		if !ok || name.Name != "string" || len(expr.Args) != 1 {
			return "", false
		}
		return resolveStringConstant(expr.Args[0], constants, active)
	case *ast.BinaryExpr:
		if expr.Op != token.ADD {
			return "", false
		}
		left, ok := resolveStringConstant(expr.X, constants, active)
		if !ok {
			return "", false
		}
		right, ok := resolveStringConstant(expr.Y, constants, active)
		return left + right, ok
	default:
		return "", false
	}
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

func fieldTypeIdent(expr ast.Expr) *ast.Ident {
	switch t := expr.(type) {
	case *ast.Ident:
		return t
	case *ast.StarExpr:
		return fieldTypeIdent(t.X)
	case *ast.ArrayType:
		return fieldTypeIdent(t.Elt)
	default:
		return nil
	}
}

// WriteFile writes content to path with 0o644 permissions, wrapping any
// error with the path for context.
func WriteFile(path, content string) error {
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	return nil
}

// InjectIntoFile replaces the content between the `<!-- label:start -->` and
// `<!-- label:end -->` markers in path with content, preserving everything else.
func InjectIntoFile(path, content, label string) error {
	markerBlockStart := buildMarkerTag(label, MARKER_TAG_BEGIN)
	markerBlockEnd := buildMarkerTag(label, MARKER_TAG_END)

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
	return WriteFile(path, before+"\n"+content+"\n"+after)
}

func buildMarkerTag(label, tag string) string {
	return MARKER_TAG_PREFIX + label + tag + MARKER_TAG_SUFFIX
}
