package agentskill

import (
	"bytes"
	"regexp"
	"strings"
	"testing"

	"non24.app/core/agentactions"
	"non24.app/core/agentpolicy"
)

// frontmatter returns the skill's name and description.
func frontmatter(t *testing.T) (string, string) {
	t.Helper()
	text := string(bytes.ReplaceAll(Markdown, []byte("\r\n"), []byte("\n")))
	if !strings.HasPrefix(text, "---\n") {
		t.Fatal("SKILL.md does not open with frontmatter")
	}
	end := strings.Index(text[4:], "\n---\n")
	if end < 0 {
		t.Fatal("SKILL.md's frontmatter is not closed")
	}
	fields := map[string]string{}
	for _, line := range strings.Split(text[4:4+end], "\n") {
		key, value, found := strings.Cut(line, ": ")
		if !found {
			t.Fatalf("frontmatter line %q is not a field", line)
		}
		fields[key] = value
	}
	if len(fields) != 2 {
		t.Fatalf("frontmatter has %d fields, want name and description", len(fields))
	}
	return fields["name"], fields["description"]
}

// The frontmatter keeps to what Claude accepts for a skill.
func TestTheSkillIsOneClaudeAccepts(t *testing.T) {
	name, description := frontmatter(t)
	if name != Name || len(name) > 64 || !regexp.MustCompile(`^[a-z0-9-]+$`).MatchString(name) ||
		strings.Contains(name, "anthropic") || strings.Contains(name, "claude") {
		t.Errorf("name %q", name)
	}
	if description == "" || len(description) > 1024 || strings.ContainsAny(description, "<>") {
		t.Errorf("description of %d characters: %q", len(description), description)
	}
	// Claude reads the body when the skill is used; it is meant to stay
	// well under the 5,000 tokens a skill's instructions should take.
	if words := len(strings.Fields(string(Markdown))); words > 1500 {
		t.Errorf("SKILL.md is %d words", words)
	}
}

// The medical answer is ZeitBoard's own, word for word.
func TestTheSkillGivesTheCanonicalMedicalAnswer(t *testing.T) {
	if !bytes.Contains(Markdown, []byte(agentpolicy.MedicalRefusal)) {
		t.Fatal("SKILL.md does not quote agentpolicy.MedicalRefusal")
	}
}

// Every action an assistant may propose is explained, and nothing that
// approves or applies a change is named as if it existed.
func TestTheSkillNamesEveryActionAndNoApproval(t *testing.T) {
	mentioned := map[string]bool{}
	for _, tool := range Tools() {
		mentioned[tool] = true
		if strings.Contains(tool, "approve") || strings.Contains(tool, "apply") {
			t.Errorf("SKILL.md names %s", tool)
		}
	}
	for _, action := range agentactions.All() {
		if !mentioned[action.ID] {
			t.Errorf("SKILL.md does not explain %s", action.ID)
		}
	}
}
