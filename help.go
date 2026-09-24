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

// defaultHelpRenderer renders a clap-style formatted --help page.
type defaultHelpRenderer struct{}

func (defaultHelpRenderer) RenderHelp(p *Parser) string {
	var b strings.Builder

	name := p.app.Name
	if name == "" {
		name = "app"
	}

	header := name
	if p.app.Version != "" {
		header += " " + p.app.Version
	}
	b.WriteString(header)
	b.WriteString("\n")
	if p.app.Description != "" {
		b.WriteString(p.app.Description)
		b.WriteString("\n")
	}
	b.WriteString("\n")

	b.WriteString("USAGE:\n")
	fmt.Fprintf(&b, "    %s [OPTIONS]%s\n\n", name, usagePositionalSuffix(p.posArgs))

	if len(p.posArgs) > 0 {
		b.WriteString("ARGS:\n")
		var argRows []helpRow
		for _, e := range p.posArgs {
			if e.Hidden {
				continue
			}
			argRows = append(argRows, helpRow{rowType: HELP_ROW_ENTRY, left: positionalLeftColumn(e), help: helpText(e)})
		}
		writeRows(&b, argRows)
		b.WriteString("\n")
	}

	var rows []helpRow

	entries := make([]*entry, 0, len(p.entries))
	for _, e := range p.entries {
		if !e.Positional && !e.Hidden {
			entries = append(entries, e)
		}
	}
	sort.SliceStable(entries, func(i, j int) bool {
		if entries[i].Group != entries[j].Group {
			return entries[i].Group < entries[j].Group
		}
		return entries[i].flagLabel() < entries[j].flagLabel()
	})

	currentGroup := ""
	for _, e := range entries {
		if e.Group != currentGroup {
			currentGroup = e.Group
			if currentGroup != "" {
				rows = append(rows, helpRow{rowType: HELP_ROW_GROUP, left: strings.ToUpper(currentGroup) + ":"})
			}
		}
		rows = append(rows, helpRow{rowType: HELP_ROW_ENTRY, left: helpLeftColumn(e.Short, e.Long, e.isBool, placeholderName(e)), help: helpText(e)})
	}
	if currentGroup != "" {
		rows = append(rows, helpRow{rowType: HELP_ROW_BLANK})
	}
	if !p.app.DisableHelp {
		if p.helpShortTaken {
			rows = append(rows, helpRow{rowType: HELP_ROW_ENTRY, left: helpLeftColumn("", "help", true, ""), help: "Print help information"})
		} else {
			rows = append(rows, helpRow{rowType: HELP_ROW_ENTRY, left: helpLeftColumn("h", "help", true, ""), help: "Print help information"})
		}
	}
	if !p.app.DisableVersion {
		if p.versionShortTaken {
			rows = append(rows, helpRow{rowType: HELP_ROW_ENTRY, left: helpLeftColumn("", "version", true, ""), help: "Print version information"})
		} else {
			rows = append(rows, helpRow{rowType: HELP_ROW_ENTRY, left: helpLeftColumn("V", "version", true, ""), help: "Print version information"})
		}
	}
	if p.fileProviderEnabled {
		configHelp := "Path to a JSON or TOML config file (auto-detected from the binary name if omitted)"
		if p.configShortTaken {
			rows = append(rows, helpRow{rowType: HELP_ROW_ENTRY, left: helpLeftColumn("", "config", false, "PATH"), help: configHelp})
		} else {
			rows = append(rows, helpRow{rowType: HELP_ROW_ENTRY, left: helpLeftColumn("c", "config", false, "PATH"), help: configHelp})
		}
	}
	rows = append(rows, helpRow{rowType: HELP_ROW_ENTRY, left: helpLeftColumn("", "print-config", true, ""), help: "Print the resolved configuration as JSON"})

	b.WriteString("OPTIONS:\n")
	writeRows(&b, rows)

	return strings.TrimRight(b.String(), "\n")
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

func writeRows(b *strings.Builder, rows []helpRow) {
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
			fmt.Fprintf(b, "    %s\n", r.left)
		default:
			fmt.Fprintf(b, "    %-*s  %s\n", width, r.left, r.help)
		}
	}
}

func usagePositionalSuffix(posArgs []*entry) string {
	var b strings.Builder
	for _, e := range posArgs {
		name := e.positionalName()
		switch {
		case e.isSlice:
			fmt.Fprintf(&b, " [%s]...", name)
		case e.Required:
			fmt.Fprintf(&b, " <%s>", name)
		default:
			fmt.Fprintf(&b, " [%s]", name)
		}
	}
	return b.String()
}

func positionalLeftColumn(e *entry) string {
	name := e.positionalName()
	if e.isSlice {
		return fmt.Sprintf("[%s]...", name)
	}
	if e.Required {
		return fmt.Sprintf("<%s>", name)
	}
	return fmt.Sprintf("[%s]", name)
}

func placeholderName(e *entry) string {
	if e.ValueNameInHelpDescription != "" {
		return strings.ToUpper(e.ValueNameInHelpDescription)
	}
	name := e.Long
	if name == "" {
		name = e.fieldName
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

func helpText(e *entry) string {
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
