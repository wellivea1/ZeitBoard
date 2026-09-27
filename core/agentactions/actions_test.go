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
			switch action.Subject {
			case TaskSubject:
			case DoseSubject:
				// The chat model sees no medication, and the server resolves
				// only tasks: a dose waits on the owner's computer.
				if action.On(ChatAssistant | ServerMCP) {
					t.Fatalf("dose proposal %s is offered beyond the local endpoint", action.ID)
				}
			default:
				t.Fatalf("proposal %s has no subject", action.ID)
			}
		case Direct:
			// Direct actions are local and reversible (ADR-0021): never on a
			// model's or the server's surface.
			if action.On(ChatAssistant|ServerMCP) || strings.HasPrefix(action.ID, "propose_") || action.Subject != "" {
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
	if !IsProposal("propose_log_dose", LocalMCP) || IsTaskProposal("propose_log_dose", AnyMCP) || !IsTaskProposal("propose_place_task", AnyMCP) {
		t.Fatal("IsTaskProposal disagrees with the registry")
	}
}

func TestTargetRules(t *testing.T) {
	start := time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)
	end := start.Add(time.Hour)
	wake := 60
	if err := (TaskTarget{TaskID: "task_synthetic", EarliestStartAt: &start, LatestFinishAt: &end, DurationMinutes: 30, PreferredAfterWakeMinutes: &wake}).Validate(); err != nil {
		t.Fatal(err)
	}
	late := 2000
	for name, target := range map[string]TaskTarget{
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

func TestDoseTargetRules(t *testing.T) {
	now := time.Date(2026, 9, 27, 21, 0, 0, 0, time.UTC)
	at := func(offset time.Duration) *time.Time { value := now.Add(offset); return &value }
	for name, target := range map[string]DoseTarget{
		"now":                   {MedicationID: "med_synthetic", Status: "taken"},
		"an hour ago, skipped":  {MedicationID: "med_synthetic", Status: "skipped", DoseAt: at(-time.Hour)},
		"within the clock skew": {MedicationID: "med_synthetic", Status: "taken", DoseAt: at(DoseClockSkew)},
		"a week ago":            {MedicationID: "med_synthetic", Status: "taken", DoseAt: at(-MaxProposedDoseAge)},
	} {
		if err := target.Validate(now); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
	for name, target := range map[string]DoseTarget{
		"no medication":     {Status: "taken"},
		"a label as the id": {MedicationID: "Evening Tablet", Status: "taken"},
		"no status":         {MedicationID: "med_synthetic"},
		"a dose amount":     {MedicationID: "med_synthetic", Status: "2 tablets"},
		"empty time":        {MedicationID: "med_synthetic", Status: "taken", DoseAt: &time.Time{}},
		"in the future":     {MedicationID: "med_synthetic", Status: "taken", DoseAt: at(DoseClockSkew + time.Second)},
		"over a week ago":   {MedicationID: "med_synthetic", Status: "taken", DoseAt: at(-MaxProposedDoseAge - time.Second)},
	} {
		if target.Validate(now) == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

// The contracts list the task proposals the server resolves; they must list
// exactly the registry's.
func TestContractsListTheRegisteredTaskProposals(t *testing.T) {
	taskIDs := func(surfaces Surface) []string {
		var ids []string
		for _, action := range All() {
			if IsTaskProposal(action.ID, surfaces) {
				ids = append(ids, action.ID)
			}
		}
		return union(ids)
	}
	chat := taskIDs(ChatAssistant)
	agents := taskIDs(AnyMCP)
	stored := taskIDs(ChatAssistant | AnyMCP)

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
