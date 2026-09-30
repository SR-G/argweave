// Command weavehelp generates the full --help page text for a struct
// annotated with `arg` tags, by statically parsing the Go source with
// go/ast (no reflection, no compilation of the target package is
// required) and rendering it with the exact same layout engine used by
// the runtime --help output (argweave.RenderHelpPage). It is meant to be
// invoked through `go generate`, so a README or man page can embed an
// always up-to-date --help listing without running the binary.
//
// It shares its struct introspection and marker-injection logic with
// cmd/weavedoc through the argweave/docgen package.
//
// Full page mode:
//
//	//go:generate go run argweave/cmd/weavehelp -type=Config -file=config.go -name=myapp -out=docs/HELP.txt
//
// Edit mode: injects the generated text into an existing file between the
// markers `<!-- weavehelp:start -->` and `<!-- weavehelp:end -->`:
//
//	//go:generate go run argweave/cmd/weavehelp -type=Config -file=config.go -name=myapp -edit=README.md
//
// -file accepts the same comma-separated file/directory list as weavedoc,
// and every -disable-help/-disable-version/-no-config-flag flag mirrors
// the corresponding argweave.AppConfig option, so the generated text
// matches what the real binary prints for --help.
//
// -markdown-bold renders section headers, group headers, and flag/argument
// names with Markdown `**bold**` syntax (mirroring the bold ANSI output of
// a real terminal --help) and implies -code-block=false, since bold
// markers have no effect inside a fenced code block.
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	argweave "github.com/SR-G/argweave"
	"github.com/SR-G/argweave/docgen"
)

const markerTagLabelDefault = "weavehelp"

func main() {
	var (
		typeName       = flag.String("type", "", "name of the struct type to document (required)")
		file           = flag.String("file", os.Getenv("GOFILE"), "comma-separated Go source files and/or directories containing the struct (defaults to $GOFILE)")
		out            = flag.String("out", "", "write the full --help text to this path")
		edit           = flag.String("edit", "", "inject the generated --help text into this file, between weavehelp markers")
		marker         = flag.String("marker", "", "key used in the HTML marker in edit mode, defaults to 'weavehelp'")
		name           = flag.String("name", "", "program name shown in the help header (defaults to -type, lowercased)")
		version        = flag.String("app-version", "", "program version shown in the help header")
		description    = flag.String("description", "", "program description shown in the help header")
		disableHelp    = flag.Bool("disable-help", false, "omit the built-in -h/--help flag, mirroring AppConfig.DisableHelp")
		disableVersion = flag.Bool("disable-version", false, "omit the built-in -V/--version flag, mirroring AppConfig.DisableVersion")
		noConfigFlag   = flag.Bool("no-config-flag", false, "omit the -c/--config flag, set this when ProviderFile is disabled in AppConfig.Providers")
		codeBlock      = flag.Bool("code-block", true, "wrap the generated text in a fenced ``` code block")
		markdownBold   = flag.Bool("markdown-bold", false, "render headers and flag/argument names in Markdown **bold**; implies -code-block=false")
	)
	flag.Parse()

	if err := run(*typeName, *file, *out, *edit, *marker, *name, *version, *description, *disableHelp, *disableVersion, *noConfigFlag, *codeBlock, *markdownBold); err != nil {
		fmt.Fprintln(os.Stderr, "weavehelp:", err)
		os.Exit(1)
	}
}

func run(typeName, file, out, edit, marker, name, version, description string, disableHelp, disableVersion, noConfigFlag, codeBlock, markdownBold bool) error {
	if typeName == "" {
		return fmt.Errorf("-type is required")
	}
	if file == "" {
		return fmt.Errorf("-file is required (or run via go:generate so $GOFILE is set)")
	}
	if out == "" && edit == "" {
		return fmt.Errorf("one of -out or -edit is required")
	}
	if name == "" {
		name = strings.ToLower(typeName)
	}

	fields, err := docgen.ExtractFields(file, typeName)
	if err != nil {
		return err
	}

	var positional, options []argweave.HelpEntry
	for _, field := range fields {
		entry := argweave.HelpEntry{
			FieldSpec: field.Spec,
			FieldName: field.Name,
			IsBool:    field.TypeName == "bool",
			IsSlice:   strings.HasPrefix(field.TypeName, "[]"),
		}
		if field.Spec.Positional {
			positional = append(positional, entry)
		} else {
			options = append(options, entry)
		}
	}

	markup := argweave.HelpMarkupNone
	if markdownBold {
		markup = argweave.HelpMarkupMarkdown
		codeBlock = false
	}

	text := argweave.RenderHelpPage(argweave.HelpPageOptions{
		Name:                name,
		Version:             version,
		Description:         description,
		Positional:          positional,
		Options:             options,
		DisableHelp:         disableHelp,
		DisableVersion:      disableVersion,
		FileProviderEnabled: !noConfigFlag,
		Markup:              markup,
	})
	content := text
	if codeBlock {
		content = "```\n" + text + "\n```"
	}

	if out != "" {
		if err := os.WriteFile(out, []byte(content+"\n"), 0o644); err != nil {
			return fmt.Errorf("writing %s: %w", out, err)
		}
	}

	if edit != "" {
		label := markerTagLabelDefault
		if marker != "" {
			label = marker
		}
		markerBlockStart := docgen.BuildMarkerTag(label, docgen.MarkerTagBegin)
		markerBlockEnd := docgen.BuildMarkerTag(label, docgen.MarkerTagEnd)
		if err := docgen.InjectIntoFile(edit, content, markerBlockStart, markerBlockEnd); err != nil {
			return err
		}
	}

	return nil
}
