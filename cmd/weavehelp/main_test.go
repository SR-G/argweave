package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	argweave "github.com/SR-G/argweave"
)

func TestRunGeneratesFullHelpPage(t *testing.T) {
	source := writeSource(t, `package sample

type Config struct {
    Port int `+"`arg:\"short=p,long=port,env=PORT,default=8080,group=Server,help=Port to listen on\"`"+`
    Name string `+"`arg:\"positional,required,help=Name of the resource\"`"+`
}
`)
	output := filepath.Join(t.TempDir(), "help.txt")
	if err := run("Config", source, output, "", "", "myapp", "1.0.0", "My app", false, false, false, false, false, argweave.HELP_MAX_WIDTH_NONE); err != nil {
		t.Fatalf("run: %v", err)
	}
	data, err := os.ReadFile(output)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	text := string(data)
	for _, want := range []string{
		"myapp 1.0.0", "My app", "USAGE:", "ARGS:", "<NAME>", "OPTIONS:",
		"SERVER:", "-p, --port", "env: PORT", "default: 8080", "-h, --help", "-V, --version", "-c, --config",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("help text missing %q:\n%s", want, text)
		}
	}
}

func TestRunDisplaysStaticEnumValues(t *testing.T) {
	source := writeSource(t, `package sample

type Mode string

const (
    MODE_FAST Mode = "fast"
    MODE_SAFE Mode = "safe"
)

func (*Mode) EnumValues() []string {
    return []string{string(MODE_FAST), string(MODE_SAFE)}
}

type Config struct {
    Mode Mode `+"`arg:\"long=mode,help=Execution mode\"`"+`
}
`)
	output := filepath.Join(t.TempDir(), "help.txt")
	if err := run("Config", source, output, "", "", "myapp", "", "", false, false, false, false, false, argweave.HELP_MAX_WIDTH_NONE); err != nil {
		t.Fatalf("run: %v", err)
	}
	data, err := os.ReadFile(output)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	if !strings.Contains(string(data), "values: fast, safe") {
		t.Fatalf("help output missing static enum values:\n%s", data)
	}
}
func TestRunInjectsBetweenMarkers(t *testing.T) {
	source := writeSource(t, "package sample\ntype Config struct { Port int `arg:\"short=p,long=port\"` }\n")
	readme := filepath.Join(t.TempDir(), "README.md")
	if err := os.WriteFile(readme, []byte("before\n<!-- weavehelp:start -->\nold\n<!-- weavehelp:end -->\nafter\n"), 0o600); err != nil {
		t.Fatalf("write readme: %v", err)
	}
	if err := run("Config", source, "", readme, "", "myapp", "", "", false, false, true, true, false, argweave.HELP_MAX_WIDTH_NONE); err != nil {
		t.Fatalf("run: %v", err)
	}
	data, err := os.ReadFile(readme)
	if err != nil {
		t.Fatalf("read readme: %v", err)
	}
	text := string(data)
	if !strings.Contains(text, "before") || !strings.Contains(text, "after") {
		t.Fatalf("markers surroundings lost: %s", text)
	}
	if strings.Contains(text, "old") {
		t.Fatalf("old content not replaced: %s", text)
	}
	if !strings.Contains(text, "```") || !strings.Contains(text, "--port") {
		t.Fatalf("expected fenced help block with --port: %s", text)
	}
	if strings.Contains(text, "-c, --config") {
		t.Fatalf("expected -c/--config to be omitted with -no-config-flag: %s", text)
	}
}

func TestRunRequiresTypeAndDestination(t *testing.T) {
	if err := run("", "file.go", "out.txt", "", "", "", "", "", false, false, false, false, false, argweave.HELP_MAX_WIDTH_NONE); err == nil {
		t.Fatalf("expected error when -type is missing")
	}
	if err := run("Config", "file.go", "", "", "", "", "", "", false, false, false, false, false, argweave.HELP_MAX_WIDTH_NONE); err == nil {
		t.Fatalf("expected error when neither -out nor -edit is set")
	}
}

func TestRunMarkdownBoldDisablesCodeBlock(t *testing.T) {
	source := writeSource(t, `package sample

type Config struct {
    Port int `+"`arg:\"short=p,long=port,group=Server,help=Port to listen on\"`"+`
}
`)
	output := filepath.Join(t.TempDir(), "help.md")
	if err := run("Config", source, output, "", "", "myapp", "", "", false, false, false, true, true, argweave.HELP_MAX_WIDTH_NONE); err != nil {
		t.Fatalf("run: %v", err)
	}
	data, err := os.ReadFile(output)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	text := string(data)
	if strings.Contains(text, "```") {
		t.Fatalf("expected -markdown-bold to disable the code fence: %s", text)
	}
	for _, want := range []string{"**USAGE:**", "**OPTIONS:**", "**SERVER:**", "**-p, --port <PORT>**"} {
		if !strings.Contains(text, want) {
			t.Fatalf("help text missing %q:\n%s", want, text)
		}
	}
}

func TestRunWrapsAtMaxWidth(t *testing.T) {
	source := writeSource(t, `package sample

type Config struct {
    Port int `+"`arg:\"short=p,long=port,help=Port to listen on for incoming connections from every client\"`"+`
}
`)
	output := filepath.Join(t.TempDir(), "help.txt")
	if err := run("Config", source, output, "", "", "myapp", "", "", false, false, false, false, false, 60); err != nil {
		t.Fatalf("run: %v", err)
	}
	data, err := os.ReadFile(output)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	for _, line := range strings.Split(strings.TrimRight(string(data), "\n"), "\n") {
		if len(line) > 60 {
			t.Fatalf("line exceeds -max-width=60 (%d): %q\n%s", len(line), line, data)
		}
	}
}

func writeSource(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.go")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	return path
}
