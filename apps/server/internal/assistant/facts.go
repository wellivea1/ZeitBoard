package assistant

import (
	"fmt"
	"strings"

	"non24.app/core/agentpolicy"
)

// factualHealthAnswer is deliberately template-based. Medication and marker
// facts never need a provider-generated sentence, so this path cannot turn
// neutral records into dosing advice or causal claims.
func factualHealthAnswer(message string, input PlanningContext) string {
	sections := make([]string, 0, 2)
	if agentpolicy.ContainsMedicationFactSubject(message) {
		sections = append(sections, medicationFactAnswer(input.MedicationFacts))
	}
	if agentpolicy.ContainsMarkerSubject(message) {
		sections = append(sections, markerFactAnswer(input.Markers))
	}
	return strings.Join(sections, " ")
}

func medicationFactAnswer(input []MedicationFactContext) string {
	facts := sanitizedMedicationFacts(input, maxContextMedications)
	if len(facts) == 0 {
		return "No reviewed medication timing facts are available. ZeitBoard does not infer missed doses from absent logs."
	}
	items := make([]string, 0, min(len(facts), 3))
	for index, fact := range facts {
		if index == 3 {
			break
		}
		parts := []string{fmt.Sprintf("Medication %d has a %s schedule and %d explicit logged events", index+1, strings.ReplaceAll(fact.ScheduleKind, "_", "-"), fact.LoggedEventCount)}
		if fact.NextScheduled != "" {
			parts = append(parts, "the next user-scheduled time is "+fact.NextScheduled)
		}
		if fact.ScheduledOccurrenceCount > 0 {
			parts = append(parts, fmt.Sprintf("%d of %d covered forecast occurrences overlap predicted sleep", fact.CollisionCount, fact.ScheduledOccurrenceCount))
		}
		if fact.LastLoggedStatus != "" {
			latest := "the latest explicit status is " + fact.LastLoggedStatus
			if fact.LastWakeRelation != "" {
				latest += ", " + fact.LastWakeRelation
			}
			if fact.LastSleepRelation != "" {
				latest += ", " + fact.LastSleepRelation
			}
			parts = append(parts, latest)
		}
		items = append(items, strings.Join(parts, "; ")+".")
	}
	truncated := ""
	if len(facts) > len(items) {
		truncated = fmt.Sprintf(" %d additional reviewed medication records are omitted from this concise answer.", len(facts)-len(items))
	}
	return "Medication timing facts: " + strings.Join(items, " ") + truncated + " These are schedule and log facts, not dosing or treatment advice."
}

func markerFactAnswer(input []RhythmMarkerFactContext) string {
	facts := sanitizedMarkerFacts(input, maxContextMarkers)
	if len(facts) == 0 {
		return "No reviewed rhythm markers are available. Markers are optional self-reports and do not establish cause."
	}
	items := make([]string, 0, min(len(facts), 4))
	for index, fact := range facts {
		if index == 4 {
			break
		}
		dateRange := fact.CivilStartDate
		if fact.CivilEndDate != "" && fact.CivilEndDate != fact.CivilStartDate {
			dateRange += " through " + fact.CivilEndDate
		}
		items = append(items, strings.ReplaceAll(fact.Kind, "_", " ")+" on "+dateRange)
	}
	truncated := ""
	if len(facts) > len(items) {
		truncated = fmt.Sprintf(" %d additional reviewed markers are omitted from this concise answer.", len(facts)-len(items))
	}
	return "Rhythm marker facts: " + strings.Join(items, "; ") + "." + truncated + " Markers are optional self-reports; they do not change the estimate or establish cause."
}
