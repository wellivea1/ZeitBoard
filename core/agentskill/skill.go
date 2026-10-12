// Package agentskill holds the skill that tells an assistant how to use
// ZeitBoard's tools (ADR-0061): read first, speak in ranges, refuse medical
// decisions, propose rather than change, and keep health detail to what a
// question needs. It is guidance only. Every guard it describes is enforced
// by ZeitBoard itself, so a model that ignores it can do no more harm.
package agentskill

import (
	_ "embed"
	"regexp"
)

// Name is the skill's name, as its folder and its frontmatter give it.
const Name = "zeitboard"

// Markdown is the skill's SKILL.md, frontmatter included.
//
//go:embed SKILL.md
var Markdown []byte

var toolMention = regexp.MustCompile("`((?:get|list|propose|set|ask)_[a-z_]+)`")

// Tools lists the tool names the skill mentions, once each, in order.
func Tools() []string {
	var names []string
	seen := map[string]bool{}
	for _, match := range toolMention.FindAllSubmatch(Markdown, -1) {
		name := string(match[1])
		if !seen[name] {
			seen[name] = true
			names = append(names, name)
		}
	}
	return names
}
