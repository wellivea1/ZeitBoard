package localagent

import (
	"strings"
	"testing"

	"non24.app/core/agentactions"
)

// The local endpoint offers exactly the registry's local actions, under the
// registry's names, and proposals only while a backend can take them.
func TestToolListOffersExactlyTheRegisteredLocalActions(t *testing.T) {
	for _, proposalsAvailable := range []bool{false, true} {
		listed := map[string]ToolDefinition{}
		for _, tool := range ToolDefinitions(proposalsAvailable) {
			listed[tool.Name] = tool
			_, registered := agentactions.Lookup(tool.Name)
			isRead := strings.HasPrefix(tool.Name, "get_") || strings.HasPrefix(tool.Name, "list_") || strings.HasPrefix(tool.Name, "ask_")
			if registered == isRead {
				t.Errorf("tool %s: registered %v, read %v; an action must be registered and a read must not", tool.Name, registered, isRead)
			}
		}
		for _, action := range agentactions.All() {
			tool, offered := listed[action.ID]
			want := action.On(agentactions.LocalMCP) && (action.Kind == agentactions.Direct || proposalsAvailable)
			if offered != want {
				t.Errorf("proposals available %v: %s offered %v, want %v", proposalsAvailable, action.ID, offered, want)
			}
			if offered && (tool.Title != action.Title || tool.Description != action.Description) {
				t.Errorf("%s is listed as %q / %q, not the registry's", action.ID, tool.Title, tool.Description)
			}
			if offered && (action.Kind == agentactions.Proposal) != IsProposeTool(action.ID) {
				t.Errorf("%s: the propose budget disagrees with the registry", action.ID)
			}
		}
	}
}
