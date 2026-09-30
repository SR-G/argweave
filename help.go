package argweave

import (
	"fmt"
	"sort"
	"strings"
)

// HelpRenderer renders the --help page for a Parser. Assign a custom
// implementation to AppConfig.HelpRenderer to replace the built-in
// clap-style layout produced by DefaultHelpRenderer.
type HelpRenderer interface {
	RenderHelp(p *Parser) string
}

// DefaultHelpRenderer returns the built-in HelpRenderer used when
// AppConfig.HelpRenderer is left unset.
func DefaultHelpRenderer() HelpRenderer {
	return &defaultHelpRenderer{}
}

// HelpEntry is the field-level information needed to render a --help
// page. It is the common ground between the runtime renderer (built from
// a live Parser) and static doc-generation tools such as cmd/weavedoc and
// cmd/weavehelp (built by statically parsing a struct's `arg` tags).
type HelpEntry struct {
	FieldSpec
	FieldName string
	IsBool    bool
	IsSlice   bool
}

// HelpMarkup selects how RenderHelpPage decorates section headers, group
// headers, and flag/argument names: plain text, ANSI bold escape codes
// (for a terminal), or Markdown bold syntax (for generated docs).
type HelpMarkup int

const (
	// HelpMarkupNone renders plain text, with no emphasis.
	HelpMarkupNone HelpMarkup = iota
	// HelpMarkupANSI wraps emphasized text in ANSI bold escape codes.
	HelpMarkupANSI
	// HelpMarkupMarkdown wraps emphasized text in Markdown `**bold**` syntax.
	HelpMarkupMarkdown
)

// bold wraps s according to markup, or returns it unchanged for
// HelpMarkupNone or an empty string.
func bold(s string, markup HelpMarkup) string {
	if s == "" {
		return s
	}
	switch markup {
	case HelpMarkupANSI:
		return "\x1b[1m" + s + "\x1b[0m"
	case HelpMarkupMarkdown:
		return "**" + s + "**"
	default:
		return s
	}
}

// HelpPageOptions holds everything RenderHelpPage needs to produce a
// clap-style --help page, independent of any live Parser.
type HelpPageOptions struct {
	Name        string
	Version     string
	Description string

	// Positional holds the positional entries, in declaration order.
	Positional []HelpEntry
	// Options holds every non-positional entry (hidden entries are
	// skipped by RenderHelpPage, so callers don't need to filter them).
	Options []HelpEntry

	DisableHelp         bool
	DisableVersion      bool
	FileProviderEnabled bool

	// Markup selects how section headers, group headers, and flag/argument
	// names are emphasized. Defaults to HelpMarkupNone (plain text).
	Markup HelpMarkup
}

// RenderHelpPage renders a clap-style --help page from opts. It is used
// both by the runtime Parser (via defaultHelpRenderer) and by static
// doc-generation tools that only have a struct's `arg` tags to work with.
func RenderHelpPage(opts HelpPageOptions) string {
	var b strings.Builder

	name := opts.Name
	if name == "" {
		name = "app"
	}

	header := name
	if opts.Version != "" {
		header += " " + opts.Version
	}
	b.WriteString(header)
	b.WriteString("\n")
	if opts.Description != "" {
		b.WriteString(opts.Description)
		b.WriteString("\n")
	}
	b.WriteString("\n")

	b.WriteString(bold("USAGE:", opts.Markup) + "\n")
	fmt.Fprintf(&b, "    %s [OPTIONS]%s\n\n", name, usagePositionalSuffix(opts.Positional))

	if len(opts.Positional) > 0 {
		b.WriteString(bold("ARGS:", opts.Markup) + "\n")
		var argRows []helpRow
		for _, e := range opts.Positional {
			if e.Hidden {
				continue
			}
			argRows = append(argRows, helpRow{rowType: HELP_ROW_ENTRY, left: positionalLeftColumn(e), help: helpText(e)})
		}
		writeRows(&b, argRows, opts.Markup)
		b.WriteString("\n")
	}

	var rows []helpRow

	entries := make([]HelpEntry, 0, len(opts.Options))
	helpShortTaken, versionShortTaken, configShortTaken := false, false, false
	for _, e := range opts.Options {
		if e.Short == "h" {
			helpShortTaken = true
		}
		if e.Short == "V" {
			versionShortTaken = true
		}
		if e.Short == "c" {
			configShortTaken = true
		}
		if !e.Hidden {
			entries = append(entries, e)
		}
	}
	sort.SliceStable(entries, func(i, j int) bool {
		if entries[i].Group != entries[j].Group {
			return entries[i].Group < entries[j].Group
		}
		return flagLabel(entries[i]) < flagLabel(entries[j])
	})

	currentGroup := ""
	for _, e := range entries {
		if e.Group != currentGroup {
			currentGroup = e.Group
			if currentGroup != "" {
				rows = append(rows, helpRow{rowType: HELP_ROW_GROUP, left: strings.ToUpper(currentGroup) + ":"})
			}
		}
		rows = append(rows, helpRow{rowType: HELP_ROW_ENTRY, left: helpLeftColumn(e.Short, e.Long, e.IsBool, placeholderName(e)), help: helpText(e)})
	}
	if currentGroup != "" {
		rows = append(rows, helpRow{rowType: HELP_ROW_BLANK})
	}
	if !opts.DisableHelp {
		if helpShortTaken {
			rows = append(rows, helpRow{rowType: HELP_ROW_ENTRY, left: helpLeftColumn("", "help", true, ""), help: "Print help information"})
		} else {
			rows = append(rows, helpRow{rowType: HELP_ROW_ENTRY, left: helpLeftColumn("h", "help", true, ""), help: "Print help information"})
		}
	}
	if !opts.DisableVersion {
		if versionShortTaken {
			rows = append(rows, helpRow{rowType: HELP_ROW_ENTRY, left: helpLeftColumn("", "version", true, ""), help: "Print version information"})
		} else {
			rows = append(rows, helpRow{rowType: HELP_ROW_ENTRY, left: helpLeftColumn("V", "version", true, ""), help: "Print version information"})
		}
	}
	if opts.FileProviderEnabled {
		configHelp := "Path to a JSON or TOML config file (auto-detected from the binary name if omitted)"
		if configShortTaken {
			rows = append(rows, helpRow{rowType: HELP_ROW_ENTRY, left: helpLeftColumn("", "config", false, "PATH"), help: configHelp})
		} else {
			rows = append(rows, helpRow{rowType: HELP_ROW_ENTRY, left: helpLeftColumn("c", "config", false, "PATH"), help: configHelp})
		}
	}
	rows = append(rows, helpRow{rowType: HELP_ROW_ENTRY, left: helpLeftColumn("", "print-config", true, ""), help: "Print the resolved configuration as JSON"})

	b.WriteString(bold("OPTIONS:", opts.Markup) + "\n")
	writeRows(&b, rows, opts.Markup)

	return strings.TrimRight(b.String(), "\n")
}

// defaultHelpRenderer renders a clap-style formatted --help page.
type defaultHelpRenderer struct{}

func (defaultHelpRenderer) RenderHelp(p *Parser) string {
	positional := make([]HelpEntry, 0, len(p.posArgs))
	for _, e := range p.posArgs {
		positional = append(positional, e.toHelpEntry())
	}
	options := make([]HelpEntry, 0, len(p.entries))
	for _, e := range p.entries {
		if !e.Positional {
			options = append(options, e.toHelpEntry())
		}
	}
	return RenderHelpPage(HelpPageOptions{
		Name:                p.app.Name,
		Version:             p.app.Version,
		Description:         p.app.Description,
		Positional:          positional,
		Options:             options,
		DisableHelp:         p.app.DisableHelp,
		DisableVersion:      p.app.DisableVersion,
		FileProviderEnabled: p.fileProviderEnabled,
		Markup:              helpMarkupFor(p.app.Color),
	})
}

// helpMarkupFor resolves an AppConfig.Color setting to the markup used by
// RenderHelpPage: ANSI bold escape codes when color is enabled, plain text
// otherwise.
func helpMarkupFor(mode ColorMode) HelpMarkup {
	if shouldColorize(mode) {
		return HelpMarkupANSI
	}
	return HelpMarkupNone
}

// toHelpEntry converts an internal, reflection-backed entry into the
// renderer-agnostic HelpEntry shared with static doc-generation tools.
func (e *entry) toHelpEntry() HelpEntry {
	return HelpEntry{FieldSpec: e.FieldSpec, FieldName: e.fieldName, IsBool: e.isBool, IsSlice: e.isSlice}
}

// helpRowType identifies how writeRows should render a helpRow.
type helpRowType int

const (
	// HELP_ROW_ENTRY is a regular "flag / help text" row.
	HELP_ROW_ENTRY helpRowType = iota
	// HELP_ROW_GROUP is a group header row (e.g. "SERVER:").
	HELP_ROW_GROUP
	// HELP_ROW_BLANK is a blank separator line.
	HELP_ROW_BLANK
)

type helpRow struct {
	rowType helpRowType
	left    string
	help    string
}

func writeRows(b *strings.Builder, rows []helpRow, markup HelpMarkup) {
	width := 0
	for _, r := range rows {
		if r.rowType != HELP_ROW_ENTRY {
			continue
		}
		if len(r.left) > width {
			width = len(r.left)
		}
	}
	for index, r := range rows {
		switch r.rowType {
		case HELP_ROW_BLANK:
			b.WriteString("\n")
		case HELP_ROW_GROUP:
			if index > 0 {
				b.WriteString("\n")
			}
			fmt.Fprintf(b, "    %s\n", bold(r.left, markup))
		default:
			pad := strings.Repeat(" ", width-len(r.left))
			fmt.Fprintf(b, "    %s%s  %s\n", bold(r.left, markup), pad, r.help)
		}
	}
}

func flagLabel(e HelpEntry) string {
	switch {
	case e.Positional:
		return "<" + positionalName(e) + ">"
	case e.Long != "" && e.Short != "":
		return fmt.Sprintf("-%s/--%s", e.Short, e.Long)
	case e.Long != "":
		return "--" + e.Long
	default:
		return "-" + e.Short
	}
}

func positionalName(e HelpEntry) string {
	if e.ValueNameInHelpDescription != "" {
		return strings.ToUpper(e.ValueNameInHelpDescription)
	}
	return strings.ToUpper(e.FieldName)
}

func usagePositionalSuffix(posArgs []HelpEntry) string {
	var b strings.Builder
	for _, e := range posArgs {
		name := positionalName(e)
		switch {
		case e.IsSlice:
			fmt.Fprintf(&b, " [%s]...", name)
		case e.Required:
			fmt.Fprintf(&b, " <%s>", name)
		default:
			fmt.Fprintf(&b, " [%s]", name)
		}
	}
	return b.String()
}

func positionalLeftColumn(e HelpEntry) string {
	name := positionalName(e)
	if e.IsSlice {
		return fmt.Sprintf("[%s]...", name)
	}
	if e.Required {
		return fmt.Sprintf("<%s>", name)
	}
	return fmt.Sprintf("[%s]", name)
}

func placeholderName(e HelpEntry) string {
	if e.ValueNameInHelpDescription != "" {
		return strings.ToUpper(e.ValueNameInHelpDescription)
	}
	name := e.Long
	if name == "" {
		name = e.FieldName
	}
	name = strings.ReplaceAll(name, "-", "_")
	return strings.ToUpper(name)
}

func helpLeftColumn(short, long string, isBool bool, placeholder string) string {
	var b strings.Builder
	switch {
	case short != "" && long != "":
		fmt.Fprintf(&b, "-%s, --%s", short, long)
	case short != "":
		fmt.Fprintf(&b, "-%s", short)
	default:
		fmt.Fprintf(&b, "    --%s", long)
	}
	if !isBool && placeholder != "" {
		fmt.Fprintf(&b, " <%s>", placeholder)
	}
	return b.String()
}

func helpText(e HelpEntry) string {
	var parts []string
	if e.Help != "" {
		parts = append(parts, e.Help)
	}
	var meta []string
	if len(e.Aliases) > 0 {
		meta = append(meta, "aliases: "+strings.Join(e.Aliases, ", "))
	}
	if e.Env != "" {
		meta = append(meta, "env: "+e.Env)
	}
	if e.HasDefault {
		defaultValue := e.Default
		if e.Secret {
			defaultValue = REDACTED_PLACEHOLDER
		}
		meta = append(meta, "default: "+defaultValue)
	}
	if e.Required {
		meta = append(meta, "required")
	}
	if e.Secret {
		meta = append(meta, "secret")
	}
	if e.FromFile {
		meta = append(meta, "file")
	}
	if len(meta) > 0 {
		parts = append(parts, "["+strings.Join(meta, "] [")+"]")
	}
	return strings.Join(parts, " ")
}
