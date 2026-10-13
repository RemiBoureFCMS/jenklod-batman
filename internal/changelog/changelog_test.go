package changelog

import (
	"os"
	"strings"
	"testing"
)

const md = `# Changelog

Intro.

## Unreleased

- Next thing.

## v0.10.0 (2026-10-01)

### Builds
- Ten.

## v0.1.0 (2026-09-25)

- One.
`

func TestSection(t *testing.T) {
	for v, want := range map[string]string{
		"Unreleased": "- Next thing.",
		"v0.10.0":    "### Builds\n- Ten.",
		"v0.1.0":     "- One.",
	} {
		got, ok := Section(md, v)
		if !ok || got != want {
			t.Errorf("Section(%s) = %q, %v; want %q", v, got, ok, want)
		}
	}
	// v0.1 must not match v0.10.0, nor v0.1.0 match a prefix of it.
	for _, v := range []string{"v0.1", "v0.2.0", "", "Changelog"} {
		if got, ok := Section(md, v); ok {
			t.Errorf("Section(%q) = %q, want none", v, got)
		}
	}
}

// The real file must have a section for every release so far.
func TestRepoChangelog(t *testing.T) {
	b, err := os.ReadFile("../../CHANGELOG.md")
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range []string{"Unreleased", "v0.1.0", "v0.2.0", "v0.3.0"} {
		if s, ok := Section(string(b), v); !ok || strings.TrimSpace(s) == "" {
			t.Errorf("CHANGELOG.md has no %s section", v)
		}
	}
}
