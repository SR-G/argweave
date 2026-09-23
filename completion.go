package argweave

import (
	"fmt"
	"strings"
)

// completionFlag is a flattened, shell-agnostic view of one flag, used to
// render bash/zsh/fish completion scripts.
type completionFlag struct {
	Short string
	Long  string
	Help  string
}

// completionFlags lists every non-hidden, non-positional flag (including
// aliases and the enabled built-ins), in a shell-agnostic form.
func (p *Parser) completionFlags() []completionFlag {
	var flags []completionFlag
	for _, e := range p.entries {
		if e.Positional || e.Hidden {
			continue
		}
		flags = append(flags, completionFlag{Short: e.Short, Long: e.Long, Help: e.Help})
		for _, alias := range e.Aliases {
			flags = append(flags, completionFlag{Long: alias, Help: e.Help})
		}
	}
	if !p.app.DisableHelp {
		short := ""
		if !p.helpShortTaken {
			short = "h"
		}
		flags = append(flags, completionFlag{Short: short, Long: "help", Help: "Print help information"})
	}
	if !p.app.DisableVersion {
		short := ""
		if !p.versionShortTaken {
			short = "V"
		}
		flags = append(flags, completionFlag{Short: short, Long: "version", Help: "Print version information"})
	}
	if p.fileProviderEnabled {
		short := ""
		if !p.configShortTaken {
			short = "c"
		}
		flags = append(flags, completionFlag{Short: short, Long: "config", Help: "Path to a JSON or TOML config file"})
	}
	flags = append(flags, completionFlag{Long: "print-config", Help: "Print the resolved configuration as JSON"})
	return flags
}

// GenerateCompletion renders a shell completion script for shell, which
// must be one of "bash", "zsh", "fish" or "powershell". Like GenerateHelp, this only
// returns the script text; the library never writes it anywhere itself.
func (p *Parser) GenerateCompletion(shell string) (string, error) {
	name := p.app.Name
	if name == "" {
		name = "app"
	}
	flags := p.completionFlags()

	switch shell {
	case "bash":
		return bashCompletion(name, flags), nil
	case "zsh":
		return zshCompletion(name, flags), nil
	case "fish":
		return fishCompletion(name, flags), nil
	case "powershell":
		return powershellCompletion(name, flags), nil
	default:
		return "", fmt.Errorf("argweave: unsupported shell %q (expected \"bash\", \"zsh\", \"fish\" or \"powershell\")", shell)
	}
}

// completionFuncName turns a program name into a valid bash/zsh function
// name fragment (letters, digits and underscores only).
func completionFuncName(name string) string {
	var b strings.Builder
	for _, r := range name {
		if r == '-' || r == ' ' || r == '.' {
			b.WriteRune('_')
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func bashCompletion(name string, flags []completionFlag) string {
	var opts []string
	for _, f := range flags {
		if f.Long != "" {
			opts = append(opts, "--"+f.Long)
		}
		if f.Short != "" {
			opts = append(opts, "-"+f.Short)
		}
	}

	fn := completionFuncName(name)
	var b strings.Builder
	fmt.Fprintf(&b, "_%s_completions() {\n", fn)
	b.WriteString("    local cur\n")
	b.WriteString("    cur=\"${COMP_WORDS[COMP_CWORD]}\"\n")
	fmt.Fprintf(&b, "    COMPREPLY=( $(compgen -W \"%s\" -- \"${cur}\") )\n", strings.Join(opts, " "))
	b.WriteString("    return 0\n")
	b.WriteString("}\n")
	fmt.Fprintf(&b, "complete -F _%s_completions %s\n", fn, name)
	return b.String()
}

func zshCompletion(name string, flags []completionFlag) string {
	var b strings.Builder
	fmt.Fprintf(&b, "#compdef %s\n\n_arguments \\\n", name)
	for i, f := range flags {
		spec := "--" + f.Long
		switch {
		case f.Short != "" && f.Long != "":
			spec = fmt.Sprintf("{-%s,--%s}", f.Short, f.Long)
		case f.Long == "":
			spec = "-" + f.Short
		}
		help := strings.ReplaceAll(f.Help, "'", `'\''`)
		fmt.Fprintf(&b, "  '%s[%s]'", spec, help)
		if i < len(flags)-1 {
			b.WriteString(" \\")
		}
		b.WriteString("\n")
	}
	return b.String()
}

func fishCompletion(name string, flags []completionFlag) string {
	var b strings.Builder
	for _, f := range flags {
		fmt.Fprintf(&b, "complete -c %s", name)
		if f.Short != "" {
			fmt.Fprintf(&b, " -s %s", f.Short)
		}
		if f.Long != "" {
			fmt.Fprintf(&b, " -l %s", f.Long)
		}
		if f.Help != "" {
			fmt.Fprintf(&b, " -d %q", f.Help)
		}
		b.WriteString("\n")
	}
	return b.String()
}

func powershellCompletion(name string, flags []completionFlag) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Register-ArgumentCompleter -Native -CommandName '%s' -ScriptBlock {\n", strings.ReplaceAll(name, "'", "''"))
	b.WriteString("    param($wordToComplete, $commandAst, $cursorPosition)\n")
	b.WriteString("    $options = @(\n")
	for _, f := range flags {
		if f.Long != "" {
			fmt.Fprintf(&b, "        @{ Name = '--%s'; Help = '%s' }\n", f.Long, strings.ReplaceAll(f.Help, "'", "''"))
		}
		if f.Short != "" {
			fmt.Fprintf(&b, "        @{ Name = '-%s'; Help = '%s' }\n", f.Short, strings.ReplaceAll(f.Help, "'", "''"))
		}
	}
	b.WriteString("    )\n")
	b.WriteString("    $options | Where-Object { $_.Name -like \"$wordToComplete*\" } | ForEach-Object {\n")
	b.WriteString("        [System.Management.Automation.CompletionResult]::new($_.Name, $_.Name, 'ParameterName', $_.Help)\n")
	b.WriteString("    }\n")
	b.WriteString("}\n")
	return b.String()
}
