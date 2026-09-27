package mcp

import (
	"testing"

	"non24.app/core/agentactions"
)

// The server's connector offers exactly the registry's server proposals, under
// the registry's names, and nothing else that changes anything.
func TestToolListOffersExactlyTheRegisteredServerProposals(t *testing.T) {
	listed := map[string]toolDefinition{}
	for _, tool := range toolDefinitions() {
		listed[tool.Name] = tool
		if _, isRead := readToolPath(tool.Name); isRead == isProposeTool(tool.Name) {
			t.Errorf("tool %s must be a read or a registered proposal, and not both", tool.Name)
		}
	}
	for _, action := range agentactions.All() {
		tool, offered := listed[action.ID]
		if want := action.On(agentactions.ServerMCP); offered != want {
			t.Errorf("%s offered %v, want %v", action.ID, offered, want)
		}
		if offered && (tool.Title != action.Title || tool.Description != action.Description) {
			t.Errorf("%s is listed as %q / %q, not the registry's", action.ID, tool.Title, tool.Description)
		}
	}
}
