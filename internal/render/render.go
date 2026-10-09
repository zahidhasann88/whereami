// Package render formats text and JSON output.
package render

import (
	"fmt"
	"strings"
	"time"

	"github.com/zahidhasann88/whereami/internal/sections"
)

// SchemaVersion changes when the JSON schema breaks compatibility.
const SchemaVersion = 1

// Report is everything a renderer needs.
type Report struct {
	Version     string
	Path        string
	GeneratedAt time.Time
	Results     []sections.Result
}

// Style controls decoration of text output.
type Style struct {
	// Color enables ANSI colours.
	Color bool
	// Emoji enables Unicode symbols; otherwise plain ASCII is used.
	Emoji bool
}

type symbols struct {
	ok, warn, unknown, arrow, dot, dash string
}

func (s Style) symbols() symbols {
	if s.Emoji {
		return symbols{ok: "✓", warn: "⚠", unknown: "·", arrow: "→", dot: "·", dash: "—"}
	}
	return symbols{ok: "[ok]", warn: "[warn]", unknown: "[?]", arrow: "->", dot: "-", dash: "-"}
}

// paint wraps s in an ANSI code when colour is enabled.
func (s Style) paint(code, text string) string {
	if !s.Color || text == "" {
		return text
	}
	return "\x1b[" + code + "m" + text + "\x1b[0m"
}

func (s Style) bold(t string) string   { return s.paint("1", t) }
func (s Style) dim(t string) string    { return s.paint("2", t) }
func (s Style) green(t string) string  { return s.paint("32", t) }
func (s Style) yellow(t string) string { return s.paint("33", t) }

const labelWidth = 11

func line(b *strings.Builder, label, value string) {
	fmt.Fprintf(b, "  %-*s %s\n", labelWidth, label, value)
}

func joinOrDash(items []string) string {
	if len(items) == 0 {
		return "-"
	}
	return strings.Join(items, ", ")
}

func plural(n int, word string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, word)
	}
	return fmt.Sprintf("%d %ss", n, word)
}
