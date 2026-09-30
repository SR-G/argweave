// Command weavedoc generates Markdown documentation for a struct annotated
// with `arg` tags, by statically parsing the Go source with go/ast (no
// reflection, no compilation of the target package is required). It is
// meant to be invoked through `go generate`. Embedded (anonymous) struct
// fields are flattened automatically, mirroring the runtime parser.
//
// Full page mode:
//
//	//go:generate go run argweave/cmd/weavedoc -type=Config -file=config.go -out=docs/CONFIG.md
//
// Edit mode: injects the generated table into an existing file between the
// markers `<!-- weavedoc:start -->` and `<!-- weavedoc:end -->`:
//
//	//go:generate go run argweave/cmd/weavedoc -type=Config -file=config.go -edit=README.md
//
// -file may also point at a directory, in which case every *.go file in it
// (excluding _test.go files) is parsed, which is useful when the struct
// embeds a sub-struct defined in a sibling file of the same package.
// -file accepts a comma-separated list of files and/or directories, so a
// sub-struct defined elsewhere (e.g. a shared config package in another
// directory) can be included too.
// -schema writes a JSON Schema document containing field types, required
// fields, defaults, descriptions, and argweave-specific metadata extensions.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	argweave "github.com/SR-G/argweave"
	"github.com/SR-G/argweave/docgen"
)

const (
	MARKER_TAG_PREFIX = docgen.MarkerTagPrefix
	MARKER_TAG_SUFFIX = docgen.MarkerTagSuffix

	MARKER_TAG_BEGIN = docgen.MarkerTagBegin
	MARKER_TAG_END   = docgen.MarkerTagEnd

	MARKER_TAG_LABEL_DEFAULT = docgen.MarkerTagLabelDefault
)

func main() {
	var (
		typeName      = flag.String("type", "", "name of the struct type to document (required)")
		file          = flag.String("file", os.Getenv("GOFILE"), "comma-separated Go source files and/or directories containing the struct (defaults to $GOFILE)")
		out           = flag.String("out", "", "write a full standalone Markdown page to this path")
		edit          = flag.String("edit", "", "inject the generated table into this file, between weavedoc markers")
		schema        = flag.String("schema", "", "write a JSON Schema configuration document to this path")
		fields        = flag.String("fields", "", "comma-separated Markdown columns to include (defaults to all columns)")
		strict        = flag.Bool("strict-schema", false, "set additionalProperties=false in generated JSON Schema")
		completion    = flag.String("completion", "", "generate static completion for bash, zsh, fish or powershell")
		completionOut = flag.String("completion-out", "", "write static completion to this path instead of stdout")
		commandName   = flag.String("name", "", "command name used in generated completion (defaults to -type)")
		title         = flag.String("title", "Configuration Reference", "title used in full page mode")
		marker        = flag.String("marker", "Name of the label", "key used in the HTML marker in edit mode, default is 'weavedoc'")
	)
	flag.Parse()

	if err := run(*typeName, *file, *out, *edit, *schema, *fields, *strict, *completion, *completionOut, *commandName, *title, *marker); err != nil {
		fmt.Fprintln(os.Stderr, "weavedoc:", err)
		os.Exit(1)
	}
}

func buildMakerTag(label string, tag string) string {
	return docgen.BuildMarkerTag(label, tag)
}

func run(typeName, file, out, edit, schema, fieldNames string, strictSchema bool, completion, completionOut, commandName, title, marker string) error {
	if typeName == "" {
		return fmt.Errorf("-type is required")
	}
	if file == "" {
		return fmt.Errorf("-file is required (or run via go:generate so $GOFILE is set)")
	}
	if out == "" && edit == "" && schema == "" && completion == "" {
		return fmt.Errorf("one of -out, -edit, -schema or -completion is required")
	}

	fields, err := extractFields(file, typeName)
	if err != nil {
		return err
	}
	columns := markdownColumns
	if out != "" || edit != "" || completion != "" {
		columns, err = selectColumns(fieldNames)
		if err != nil {
			return err
		}
	}

	table := renderTable(fields, columns)

	if out != "" {
		content := fmt.Sprintf("# %s\n\n%s\n", title, table)
		if err := os.WriteFile(out, []byte(content), 0o644); err != nil {
			return fmt.Errorf("writing %s: %w", out, err)
		}
	}

	if edit != "" {
		// MARKER_BLOCK_START = MARKER_TAG_PREFIX + MARKER_TAG_LABEL_DEFAULT + MARKER_TAG_BEGIN + MARKER_TAG_SUFFIX
		// MARKER_BLOCK_END   = MARKER_TAG_PREFIX + MARKER_TAG_LABEL_DEFAULT + MARKER_TAG_END + MARKER_TAG_SUFFIX
		markerBlockStart := buildMakerTag(MARKER_TAG_LABEL_DEFAULT, MARKER_TAG_BEGIN)
		markerBlockEnd := buildMakerTag(MARKER_TAG_LABEL_DEFAULT, MARKER_TAG_END)
		if marker != "" {
			markerBlockStart = buildMakerTag(marker, MARKER_TAG_BEGIN)
			markerBlockEnd = buildMakerTag(marker, MARKER_TAG_END)
		}
		if err := docgen.InjectIntoFile(edit, table, markerBlockStart, markerBlockEnd); err != nil {
			return err
		}
	}
	if schema != "" {
		if err := writeSchema(schema, fields, strictSchema); err != nil {
			return err
		}
	}
	if completion != "" {
		if commandName == "" {
			commandName = strings.ToLower(typeName)
		}
		content, err := renderStaticCompletion(completion, commandName, fields)
		if err != nil {
			return err
		}
		if completionOut == "" {
			fmt.Print(content)
		} else if err := os.WriteFile(completionOut, []byte(content), 0o644); err != nil {
			return fmt.Errorf("writing %s: %w", completionOut, err)
		}
	}

	return nil
}

var markdownColumns = []string{
	"group", "type", "long", "short", "aliases", "positional", "env",
	"required", "default", "requires", "conflicts", "behavior", "description",
}

func selectColumns(columnNames string) ([]string, error) {
	if strings.TrimSpace(columnNames) == "" {
		return markdownColumns, nil
	}
	known := make(map[string]bool, len(markdownColumns))
	for _, column := range markdownColumns {
		known[column] = true
	}
	selected := make([]string, 0)
	seen := make(map[string]bool)
	for _, rawName := range strings.Split(columnNames, ",") {
		name := strings.ToLower(strings.TrimSpace(rawName))
		if name == "" {
			continue
		}
		if !known[name] {
			return nil, fmt.Errorf("column %q is not supported (choose from: %s)", name, strings.Join(markdownColumns, ", "))
		}
		if !seen[name] {
			selected = append(selected, name)
			seen[name] = true
		}
	}
	if len(selected) == 0 {
		return nil, fmt.Errorf("-fields must contain at least one column name")
	}
	return selected, nil
}

// docField aliases the shared docgen.DocField type so the rest of this
// file (and its tests) can keep using the shorter, weavedoc-local name.
type docField = docgen.DocField

// extractFields statically parses fileOrDir and returns typeName's
// documentation fields; delegates to the shared docgen package.
func extractFields(fileOrDir, typeName string) ([]docField, error) {
	return docgen.ExtractFields(fileOrDir, typeName)
}

func renderTable(fields []docField, columns ...[]string) string {
	selectedColumns := markdownColumns
	if len(columns) > 0 && len(columns[0]) > 0 {
		selectedColumns = columns[0]
	}
	var b strings.Builder
	headers := make([]string, len(selectedColumns))
	for i, column := range selectedColumns {
		headers[i] = columnHeader(column)
	}
	b.WriteString("| ")
	b.WriteString(strings.Join(headers, " | "))
	b.WriteString(" |\n")
	separators := make([]string, len(selectedColumns))
	for i := range separators {
		separators[i] = "---"
	}
	b.WriteString("| ")
	b.WriteString(strings.Join(separators, " | "))
	b.WriteString(" |\n")
	for _, f := range fields {
		cells := make([]string, len(selectedColumns))
		for i, column := range selectedColumns {
			cells[i] = columnValue(f, column)
		}
		for i := range cells {
			cells[i] = markdownCell(cells[i])
		}
		fmt.Fprintf(&b, "| %s |\n", strings.Join(cells, " | "))
	}
	return strings.TrimRight(b.String(), "\n")
}

func columnHeader(column string) string {
	return map[string]string{
		"group": "Group", "type": "Type", "long": "Long", "short": "Short",
		"aliases": "Aliases", "positional": "Positional", "env": "Env",
		"required": "Required", "default": "Default", "requires": "Requires",
		"conflicts": "Conflicts", "behavior": "Behavior", "description": "Description",
	}[column]
}

func columnValue(field docField, column string) string {
	s := field.Spec
	switch column {
	case "group":
		return emptyAsDash(s.Group)
	case "type":
		return emptyAsDash(field.TypeName)
	case "long":
		if s.Positional {
			name := strings.ToUpper(field.Name)
			if s.ValueNameInHelpDescription != "" {
				name = strings.ToUpper(s.ValueNameInHelpDescription)
			}
			return "`<" + name + ">`"
		}
		if s.Long == "" {
			return "-"
		}
		return "`--" + s.Long + "`"
	case "short":
		if s.Short == "" {
			return "-"
		}
		return "`-" + s.Short + "`"
	case "aliases":
		if len(s.Aliases) == 0 {
			return "-"
		}
		quoted := make([]string, len(s.Aliases))
		for i, alias := range s.Aliases {
			quoted[i] = "`--" + alias + "`"
		}
		return strings.Join(quoted, ", ")
	case "positional":
		if s.Positional {
			return "Yes"
		}
		return "No"
	case "env":
		return emptyAsDash(markdownCode(s.Env))
	case "required":
		return yesNo(s.Required)
	case "default":
		if !s.HasDefault {
			return "-"
		}
		if s.Secret {
			return markdownCode(argweave.REDACTED_PLACEHOLDER)
		}
		return markdownCode(s.Default)
	case "requires":
		if len(s.Requires) == 0 {
			return "-"
		}
		return formatReferences(s.Requires)
	case "conflicts":
		if len(s.Conflicts) == 0 {
			return "-"
		}
		return formatReferences(s.Conflicts)
	case "behavior":
		var parts []string
		if s.Secret {
			parts = append(parts, "secret")
		}
		if s.FromFile {
			parts = append(parts, "file-backed")
		}
		return emptyAsDash(strings.Join(parts, ", "))
	case "description":
		return emptyAsDash(s.Help)
	default:
		return "-"
	}
}

func emptyAsDash(value string) string {
	if value == "" {
		return "-"
	}
	return value
}

func markdownCode(value string) string {
	if value == "" {
		return ""
	}
	return "`" + value + "`"
}

func yesNo(value bool) string {
	if value {
		return "Yes"
	}
	return "No"
}

func formatReferences(references []string) string {
	formatted := make([]string, len(references))
	for i, reference := range references {
		formatted[i] = "`" + reference + "`"
	}
	return strings.Join(formatted, ", ")
}

func markdownCell(value string) string {
	value = strings.ReplaceAll(value, "\\", "\\\\")
	value = strings.ReplaceAll(value, "|", "\\|")
	value = strings.ReplaceAll(value, "\r\n", "<br>")
	value = strings.ReplaceAll(value, "\n", "<br>")
	return value
}

func writeSchema(path string, fields []docField, strict bool) error {
	properties := make(map[string]interface{})
	var required []string
	for _, field := range fields {
		key := field.Spec.Long
		if key == "" {
			key = strings.ToLower(field.Name)
		}
		property := schemaType(field.TypeName)
		property["description"] = field.Spec.Help
		if field.Spec.HasDefault {
			if field.Spec.Secret {
				property["default"] = argweave.REDACTED_PLACEHOLDER
			} else {
				property["default"] = field.Spec.Default
			}
		}
		if field.Spec.Env != "" {
			property["x-argweave-env"] = field.Spec.Env
		}
		if field.Spec.Secret {
			property["x-argweave-secret"] = true
		}
		if field.Spec.FromFile {
			property["x-argweave-file"] = true
		}
		if len(field.Spec.Requires) > 0 {
			property["x-argweave-requires"] = field.Spec.Requires
		}
		if len(field.Spec.Conflicts) > 0 {
			property["x-argweave-conflicts"] = field.Spec.Conflicts
		}
		properties[key] = property
		if field.Spec.Required && !field.Spec.HasDefault {
			required = append(required, key)
		}
	}
	schema := map[string]interface{}{
		"$schema":              "https://json-schema.org/draft/2020-12/schema",
		"title":                "Configuration",
		"type":                 "object",
		"properties":           properties,
		"additionalProperties": !strict,
	}
	if len(required) > 0 {
		schema["required"] = required
	}
	data, err := json.MarshalIndent(schema, "", "    ")
	if err != nil {
		return fmt.Errorf("encoding schema: %w", err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	return nil
}

func schemaType(typeName string) map[string]interface{} {
	if strings.HasPrefix(typeName, "[]") {
		return map[string]interface{}{"type": "array", "items": schemaType(strings.TrimPrefix(typeName, "[]"))}
	}
	switch typeName {
	case "bool":
		return map[string]interface{}{"type": "boolean"}
	case "string":
		return map[string]interface{}{"type": "string"}
	case "time.Duration":
		return map[string]interface{}{"type": "string", "format": "duration"}
	case "float32", "float64":
		return map[string]interface{}{"type": "number"}
	case "int", "int8", "int16", "int32", "int64", "uint", "uint8", "uint16", "uint32", "uint64":
		return map[string]interface{}{"type": "integer"}
	default:
		return map[string]interface{}{"type": "string", "x-argweave-go-type": typeName}
	}
}

func renderStaticCompletion(shell, name string, fields []docField) (string, error) {
	var options []string
	for _, field := range fields {
		if field.Spec.Long != "" {
			options = append(options, "--"+field.Spec.Long)
		}
		if field.Spec.Short != "" {
			options = append(options, "-"+field.Spec.Short)
		}
		for _, alias := range field.Spec.Aliases {
			options = append(options, "--"+alias)
		}
	}
	options = append(options, "--help", "--version", "--print-config")
	values := strings.Join(options, " ")
	switch strings.ToLower(shell) {
	case "bash":
		functionName := strings.NewReplacer("-", "_", ".", "_", " ", "_").Replace(name)
		return fmt.Sprintf("_%s_completions() {\n    local cur=\"${COMP_WORDS[COMP_CWORD]}\"\n    COMPREPLY=( $(compgen -W \"%s\" -- \"${cur}\") )\n}\ncomplete -F _%s_completions %s\n", functionName, values, functionName, name), nil
	case "zsh":
		var b strings.Builder
		fmt.Fprintf(&b, "#compdef %s\n_arguments \\\n", name)
		for i, option := range options {
			fmt.Fprintf(&b, "  '%s'", option)
			if i < len(options)-1 {
				b.WriteString(" \\")
			}
			b.WriteString("\n")
		}
		return b.String(), nil
	case "fish":
		var b strings.Builder
		for _, option := range options {
			if strings.HasPrefix(option, "--") {
				fmt.Fprintf(&b, "complete -c %s -l %s\n", name, strings.TrimPrefix(option, "--"))
			} else {
				fmt.Fprintf(&b, "complete -c %s -s %s\n", name, strings.TrimPrefix(option, "-"))
			}
		}
		return b.String(), nil
	case "powershell":
		var b strings.Builder
		fmt.Fprintf(&b, "Register-ArgumentCompleter -Native -CommandName '%s' -ScriptBlock {\n", strings.ReplaceAll(name, "'", "''"))
		b.WriteString("    param($wordToComplete, $commandAst, $cursorPosition)\n")
		b.WriteString("    @(")
		for i, option := range options {
			if i > 0 {
				b.WriteString(", ")
			}
			fmt.Fprintf(&b, "'%s'", strings.ReplaceAll(option, "'", "''"))
		}
		b.WriteString(") | Where-Object { $_ -like \"$wordToComplete*\" } | ForEach-Object {\n")
		b.WriteString("        [System.Management.Automation.CompletionResult]::new($_, $_, 'ParameterName', $_)\n    }\n}\n")
		return b.String(), nil
	default:
		return "", fmt.Errorf("unsupported completion shell %q (expected bash, zsh, fish or powershell)", shell)
	}
}
