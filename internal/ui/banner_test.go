package ui

import (
	"strings"
	"testing"
	"unicode"
)

// Every banner line between the corners must have identical display width:
// single-width runes only, padded programmatically. This is the test the
// original hand-spaced banner failed.
func TestBannerPerfectlyAligned(t *testing.T) {
	for _, tc := range []struct{ version, url, data string }{
		{"dev", "http://127.0.0.1:7777", "/home/very-long-username/.pleumcloud"},
		{"v1.2.3-rc.1+githash", "https://cloud.pleum.ai", "/short/d"},
	} {
		b := Banner(tc.version, tc.url, tc.data)
		lines := strings.Split(strings.Trim(b, "\n"), "\n")
		if len(lines) < 5 {
			t.Fatalf("banner too short:\n%s", b)
		}
		if !strings.HasPrefix(lines[0], "┌") || !strings.HasSuffix(lines[0], "┐") {
			t.Fatalf("bad top corner: %q", lines[0])
		}
		if !strings.HasPrefix(lines[len(lines)-1], "└") || !strings.HasSuffix(lines[len(lines)-1], "┘") {
			t.Fatalf("bad bottom corner: %q", lines[len(lines)-1])
		}
		width := -1
		for i, l := range lines {
			runes := []rune(l)
			if width == -1 {
				width = len(runes)
			}
			if len(runes) != width {
				t.Fatalf("line %d width %d != %d:\n%s", i, len(runes), width, b)
			}
			if i > 0 && i < len(lines)-1 {
				if !strings.HasPrefix(l, "│ ") || !strings.HasSuffix(l, " │") {
					t.Fatalf("line %d missing side borders: %q", i, l)
				}
				for _, r := range string(runes[2 : len(runes)-2]) {
					if r > unicode.MaxASCII {
						t.Fatalf("line %d contains non-ASCII %q inside box (width-ambiguous): %q", i, r, l)
					}
				}
			}
		}
		if !strings.Contains(b, tc.version) || !strings.Contains(b, tc.url) || !strings.Contains(b, tc.data) {
			t.Fatalf("banner missing facts:\n%s", b)
		}
	}
}

// The banner is our identity: cloud art + tagline must be present.
func TestBannerHasArtAndTagline(t *testing.T) {
	b := Banner("dev", "http://127.0.0.1:7777", "/d")
	if !strings.Contains(b, "PleumCloud") || !strings.Contains(b, "one drive for all your free clouds") {
		t.Fatalf("missing branding:\n%s", b)
	}
	if !strings.Contains(b, ".--.") { // the cloud
		t.Fatalf("missing cloud art:\n%s", b)
	}
}

func TestBannerIsStableForSameInput(t *testing.T) {
	a := Banner("dev", "http://x", "/d")
	c := Banner("dev", "http://x", "/d")
	if a != c {
		t.Fatalf("banner not deterministic")
	}
}
