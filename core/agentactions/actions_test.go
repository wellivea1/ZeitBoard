package agentactions

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestEveryActionIsCompleteAndProposalsCannotApplyThemselves(t *testing.T) {
	seen := map[string]bool{}
	for _, action := range All() {
		if seen[action.ID] || !identifier.MatchString(action.ID) || action.Title == "" || action.Description == "" || action.Surfaces == 0 {
			t.Fatalf("incomplete or duplicate action %+v", action)
		}
		seen[action.ID] = true
		switch action.Kind {
		case Proposal:
			if !strings.HasPrefix(action.ID, "propose_") || action.CardTitle == "" || !strings.Contains(action.Description, "must approve") {
				t.Fatalf("proposal %s must be named, carded and say a human approves it", action.ID)
			}
		case Direct:
			// Direct actions are local and reversible (ADR-0021): never on a
			// model's or the server's surface.
			if action.On(ChatAssistant|ServerMCP) || strings.HasPrefix(action.ID, "propose_") {
				t.Fatalf("direct action %s is offered beyond the local endpoint", action.ID)
			}
		default:
			t.Fatalf("action %s has no kind", action.ID)
		}
		// No agent can approve or apply anything (ADR-0012).
		for _, forbidden := range []string{"approve", "apply", "accept", "decide"} {
			if strings.Contains(action.ID, forbidden) {
				t.Fatalf("action %s would let an agent decide", action.ID)
			}
		}
	}
	if CardTitle("propose_place_task") != "Place task" || CardTitle("propose_retired") != "Schedule change" {
		t.Fatal("card titles do not come from the registry")
	}
	if !IsProposal("propose_move_task", LocalMCP) || !IsProposal("propose_move_task", AnyMCP) ||
		IsProposal("set_appearance", LocalMCP) || IsProposal("propose_retired", AnyMCP) {
		t.Fatal("IsProposal disagrees with the registry")
	}
}

func TestTargetRules(t *testing.T) {
	start := time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)
	end := start.Add(time.Hour)
	wake := 60
	if err := (Target{TaskID: "task_synthetic", EarliestStartAt: &start, LatestFinishAt: &end, DurationMinutes: 30, PreferredAfterWakeMinutes: &wake}).Validate(); err != nil {
		t.Fatal(err)
	}
	late := 2000
	for name, target := range map[string]Target{
		"no task":             {},
		"bad reminder":        {TaskID: "task_synthetic", ReminderID: "Bad Reminder"},
		"negative duration":   {TaskID: "task_synthetic", DurationMinutes: -1},
		"long duration":       {TaskID: "task_synthetic", DurationMinutes: 1441},
		"late wake offset":    {TaskID: "task_synthetic", PreferredAfterWakeMinutes: &late},
		"empty bound":         {TaskID: "task_synthetic", EarliestStartAt: &time.Time{}},
		"finish before start": {TaskID: "task_synthetic", EarliestStartAt: &end, LatestFinishAt: &start},
	} {
		if target.Validate() == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

// The contracts list the proposals too; they must list exactly the registry's.
func TestContractsListTheRegisteredProposals(t *testing.T) {
	chat := union(IDs(ChatAssistant, Proposal))
	agents := union(IDs(ServerMCP, Proposal), IDs(LocalMCP, Proposal))
	stored := union(chat, agents)

	action := readContract(t, "assistant-action.schema.json")
	for _, check := range []struct {
		schema map[string]any
		path   string
		want   []string
	}{
		{action, "properties/recommended_action", union(chat, []string{"answer_only"})},
		{action, "allOf/1/if/properties/recommended_action", chat},
		{readContract(t, "direct-proposal-request.schema.json"), "properties/recommended_action", agents},
		{readContract(t, "proposal-response.schema.json"), "properties/action", stored},
		{readContract(t, "proposal-response.schema.json"), "$defs/storedProposalPayload/properties/action_id", stored},
	} {
		if got := enumAt(t, check.schema, check.path); !reflect.DeepEqual(got, check.want) {
			t.Errorf("%s = %v, registry says %v", check.path, got, check.want)
		}
	}
}

func readContract(t *testing.T, name string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "contracts", "v1", name))
	if err != nil {
		t.Fatal(err)
	}
	var schema map[string]any
	if err := json.Unmarshal(data, &schema); err != nil {
		t.Fatal(err)
	}
	return schema
}

// enumAt reads the enum at a slash-separated path, sorted.
func enumAt(t *testing.T, node any, path string) []string {
	t.Helper()
	for _, key := range strings.Split(path, "/") {
		switch value := node.(type) {
		case map[string]any:
			node = value[key]
		case []any:
			index, err := strconv.Atoi(key)
			if err != nil || index >= len(value) {
				t.Fatalf("%s: no element %s", path, key)
			}
			node = value[index]
		}
	}
	object, _ := node.(map[string]any)
	values, _ := object["enum"].([]any)
	var ids []string
	for _, value := range values {
		ids = append(ids, value.(string))
	}
	sort.Strings(ids)
	return ids
}

func union(lists ...[]string) []string {
	set := map[string]bool{}
	for _, list := range lists {
		for _, id := range list {
			set[id] = true
		}
	}
	ids := make([]string, 0, len(set))
	for id := range set {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}
