package main

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// usageCommands pulls the `hpctl <cmd>` entries out of the usage text, so the
// docs check below cannot drift from what the binary actually implements.
func usageCommands() map[string]bool {
	set := map[string]bool{}
	for _, line := range strings.Split(usage, "\n") {
		f := strings.Fields(strings.TrimSpace(line))
		if len(f) >= 2 && f[0] == "hpctl" && !strings.HasPrefix(f[1], "-") {
			set[f[1]] = true
		}
	}
	return set
}

// A README or SKILL.md that teaches a command hpctl does not have wastes an
// agent's whole turn, so a stale mention fails the build.
func TestDocsMentionOnlyRealCommands(t *testing.T) {
	known := usageCommands()
	if len(known) < 10 {
		t.Fatalf("parsed only %d commands from the usage text: %v", len(known), known)
	}
	re := regexp.MustCompile(`\bhpctl ([a-z][a-z-]*)`)
	for _, path := range []string{"README.md", "SKILL.md"} {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		for _, m := range re.FindAllStringSubmatch(string(raw), -1) {
			if cmd := m[1]; !known[cmd] {
				t.Errorf("%s mentions `hpctl %s`, which the usage text does not list (known: %v)", path, cmd, sortedKeys(known))
			}
		}
	}
}

// SKILL.md is loaded verbatim into other agents, so a broken frontmatter block
// silently keeps it out of their index.
func TestSkillFrontmatter(t *testing.T) {
	raw, err := os.ReadFile("SKILL.md")
	if err != nil {
		t.Fatalf("read SKILL.md: %v", err)
	}
	lines := strings.Split(string(raw), "\n")
	if len(lines) < 4 || strings.TrimSpace(lines[0]) != "---" {
		t.Fatal("SKILL.md must start with a --- frontmatter block")
	}
	var name, desc string
	closed := false
	for _, l := range lines[1:] {
		if strings.TrimSpace(l) == "---" {
			closed = true
			break
		}
		if v, ok := strings.CutPrefix(l, "name:"); ok {
			name = strings.TrimSpace(v)
		}
		if v, ok := strings.CutPrefix(l, "description:"); ok {
			desc = strings.TrimSpace(v)
		}
	}
	if !closed {
		t.Fatal("SKILL.md frontmatter is not closed with ---")
	}
	if name == "" || desc == "" {
		t.Fatalf("SKILL.md frontmatter needs name and description (got name=%q desc=%q)", name, desc)
	}
	if len(desc) > 1024 {
		t.Fatalf("description is %d chars, agents cap it at 1024", len(desc))
	}
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j] < out[j-1]; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}
