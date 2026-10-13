// Package changelog reads CHANGELOG.md: one "## <version>" section per
// release, "## Unreleased" on top. The app embeds the file, and the
// release workflow takes a tag's section as its release notes.
package changelog

import "strings"

// Section is the text under the "## <version>" heading, up to the next
// "## " heading, trimmed. version is a tag such as v0.4.0, or Unreleased;
// the heading may go on after a space, e.g. "## v0.4.0 (2026-10-14)".
func Section(md, version string) (string, bool) {
	if version == "" {
		return "", false
	}
	var (
		out   []string
		found bool
	)
	for _, line := range strings.Split(md, "\n") {
		if strings.HasPrefix(line, "## ") {
			if found {
				break
			}
			head := strings.TrimPrefix(line, "## ")
			found = head == version || strings.HasPrefix(head, version+" ")
			continue
		}
		if found {
			out = append(out, line)
		}
	}
	return strings.TrimSpace(strings.Join(out, "\n")), found
}
