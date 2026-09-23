package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	argweave "github.com/SR-G/argweave"
)

func TestRenderTableIncludesMetadataAndEscapesMarkdown(t *testing.T) {
	fields, err := extractFields(writeSource(t, `package sample

type Config struct {
    TLS bool `+"`arg:\"long=tls,group=Security,help=TLS | transport\"`"+`
    Cert string `+"`arg:\"long=cert,requires=tls,conflicts=insecure,secret,file,help=Certificate\\npath\"`"+`
    Delays []time.Duration `+"`arg:\"long=delay,default=1s,help=Retry | delay\"`"+`
}
`), "Config")
	if err != nil {
		t.Fatalf("extractFields: %v", err)
	}
	table := renderTable(fields)
	for _, want := range []string{"Group", "Type", "Requires", "Conflicts", "Behavior", "Security", "`tls`", "`insecure`", "secret, file-backed", "\\|"} {
		if !strings.Contains(table, want) {
			t.Fatalf("table missing %q:\n%s", want, table)
		}
	}
}

func TestSchemaGeneration(t *testing.T) {
	source := writeSource(t, `package sample

type Config struct {
    Port uint16 `+"`arg:\"long=port,required,default=8080\"`"+`
    Tags []string `+"`arg:\"long=tags,secret\"`"+`
}
`)
	output := filepath.Join(t.TempDir(), "schema.json")
	if err := run("Config", source, "", "", output, "", false, "", "", "", "Schema", MARKER_TAG_LABEL_DEFAULT); err != nil {
		t.Fatalf("run schema: %v", err)
	}
	data, err := os.ReadFile(output)
	if err != nil {
		t.Fatalf("read schema: %v", err)
	}
	text := string(data)
	for _, want := range []string{"$schema", "\"port\"", "\"integer\"", "x-argweave-secret"} {
		if !strings.Contains(text, want) {
			t.Fatalf("schema missing %q: %s", want, text)
		}
	}
}

func TestStrictSchemaDisallowsUnknownProperties(t *testing.T) {
	source := writeSource(t, "package sample\ntype Config struct { Port int `arg:\"long=port,required\"` }\n")
	output := filepath.Join(t.TempDir(), "schema.json")
	if err := run("Config", source, "", "", output, "", true, "", "", "", "Schema", MARKER_TAG_LABEL_DEFAULT); err != nil {
		t.Fatalf("run strict schema: %v", err)
	}
	data, err := os.ReadFile(output)
	if err != nil {
		t.Fatalf("read schema: %v", err)
	}
	if !strings.Contains(string(data), `"additionalProperties": false`) || !strings.Contains(string(data), `"required"`) {
		t.Fatalf("strict schema missing constraints: %s", data)
	}
}

func TestStaticCompletionGeneration(t *testing.T) {
	source := writeSource(t, "package sample\ntype Config struct { Port int `arg:\"short=p,long=port\"` }\n")
	fields, err := extractFields(source, "Config")
	if err != nil {
		t.Fatalf("extractFields: %v", err)
	}
	completion, err := renderStaticCompletion("powershell", "sample", fields)
	if err != nil || !strings.Contains(completion, "Register-ArgumentCompleter") || !strings.Contains(completion, "--port") {
		t.Fatalf("unexpected completion=%q err=%v", completion, err)
	}
}

func TestSelectColumnsPreservesRequestedOrder(t *testing.T) {
	columns, err := selectColumns(" required, long, required ")
	if err != nil {
		t.Fatalf("selectColumns: %v", err)
	}
	if len(columns) != 2 || columns[0] != "required" || columns[1] != "long" {
		t.Fatalf("unexpected selected columns: %v", columns)
	}

	fields := []docField{{name: "Port", typeName: "int", spec: argweave.FieldSpec{Long: "port", Required: true}}}
	table := renderTable(fields, columns)
	if !strings.Contains(table, "| Required | Long |") || !strings.Contains(table, "| Yes | `--port` |") {
		t.Fatalf("unexpected filtered table:\n%s", table)
	}
}

func TestSelectColumnsRejectsUnknownNames(t *testing.T) {
	if _, err := selectColumns("long,unknown"); err == nil || !strings.Contains(err.Error(), `column "unknown" is not supported`) {
		t.Fatalf("expected unknown column error, got %v", err)
	}
}

func TestExtractFieldsRejectsDuplicateTypesAndCycles(t *testing.T) {
	duplicateDir := t.TempDir()
	writeFile(t, filepath.Join(duplicateDir, "a.go"), "package sample\ntype Config struct{}\n")
	writeFile(t, filepath.Join(duplicateDir, "b.go"), "package sample\ntype Config struct{}\n")
	if _, err := extractFields(duplicateDir, "Config"); err == nil || !strings.Contains(err.Error(), "duplicate struct type") {
		t.Fatalf("expected duplicate type error, got %v", err)
	}

	cycle := writeSource(t, `package sample

type Config struct { Node Node }
type Node struct { Next *Node }
`)
	if _, err := extractFields(cycle, "Config"); err == nil || !strings.Contains(err.Error(), "recursive struct flattening") {
		t.Fatalf("expected recursive type error, got %v", err)
	}
}

func writeSource(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.go")
	writeFile(t, path, content)
	return path
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
