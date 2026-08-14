// Package ui renders terminal chrome. Banner content stays pure-ASCII:
// padding is computed per line, and width-ambiguous runes (em-dashes,
// emoji, CJK) would break alignment in some terminal fonts.
package ui

import (
	"strings"
	"unicode/utf8"
)

// cloud is the PleumCloud mark: a small cloud with a silver lining.
var cloud = []string{
	"     .--.    ",
	"   .-(    ). ",
	"  (___.__)__)",
}

// Banner renders the startup box. Each row is padded to the computed box
// width, so the frame can never misalign regardless of version or paths.
func Banner(version, url, dataDir string) string {
	const indent = "  "
	rows := []string{
		cloud[0],
		cloud[1] + "  PleumCloud",
		cloud[2],
		"",
		"one drive for all your free clouds",
		"",
		"version " + version,
		"listening " + url,
		"data " + dataDir,
	}

	width := 0
	for _, r := range rows {
		if n := len(indent) + len(r); n > width {
			width = n
		}
	}
	if width < 44 {
		width = 44 // keep the box from hugging short configs
	}

	var b strings.Builder
	b.WriteString("┌" + strings.Repeat("─", width+2) + "┐\n")
	for _, r := range rows {
		b.WriteString("│ " + pad(indent+r, width) + " │\n")
	}
	b.WriteString("└" + strings.Repeat("─", width+2) + "┘\n")
	return b.String()
}

func pad(s string, w int) string {
	if n := w - len(s); n > 0 {
		return s + strings.Repeat(" ", n)
	}
	return s
}

// Width returns the display width of s (runes; banner content is ASCII).
func Width(s string) int { return utf8.RuneCountInString(s) }
