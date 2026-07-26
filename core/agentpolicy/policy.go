package agentpolicy

import "strings"

// MedicalRefusal is the one response used by every ZeitBoard prompt surface.
// Keep this text byte-identical across chat, MCP, and future agent adapters.
const MedicalRefusal = "I can't help with medical decisions like medication or dosing. I can show when you logged doses relative to your rhythm, or help you plan around appointments."

// IsMedicalDecisionPrompt distinguishes permitted record/timing-fact questions
// from medical decisions. Ambiguous medication questions fail closed.
func IsMedicalDecisionPrompt(message string) bool {
	lower := strings.ToLower(strings.TrimSpace(message))
	if lower == "" {
		return false
	}

	if containsAny(lower,
		"diagnose", "diagnosis", "do i have", "dlmo", "exact phase",
		"prescribe", "prescription advice", "treatment advice",
	) {
		return true
	}
	if !containsMedicalSubject(lower) {
		return false
	}

	if containsAny(lower,
		"should i", "should we", "when should", "what time should", "how much",
		"what dose", "which dose", "change my dose", "increase", "decrease",
		"recommend", "best time", "safe to", "is it safe", "interaction",
		"interact", "side effect", "use to treat", "take for",
	) {
		return true
	}

	// The assistant may report user-entered evidence and neutral schedule facts.
	// Requiring an explicit fact intent prevents an ambiguous medication query
	// from reaching a model as though it were a planning question.
	return !containsAny(lower,
		"show", "list", "logged", "log history", "recorded", "record history",
		"did i log", "when did", "scheduled", "schedule facts", "next scheduled",
		"collision", "relative to", "timing fact", "how many", "marker",
	)
}

// ContainsMedicalSubject reports whether text is about treatment or medication.
// Provider-backed surfaces use it to apply stricter output handling after a
// neutral fact question is allowed through the input policy.
func ContainsMedicalSubject(message string) bool {
	return containsMedicalSubject(strings.ToLower(strings.TrimSpace(message)))
}

// ContainsMedicationFactSubject reports whether text asks about medication or
// dosing records. It deliberately excludes broader treatment terms so marker
// questions do not cause unrelated medication facts to cross a boundary.
func ContainsMedicationFactSubject(message string) bool {
	return containsMedicationFactSubject(strings.ToLower(strings.TrimSpace(message)))
}

// ContainsMarkerSubject reports whether text asks about the optional context
// markers that ZeitBoard treats as non-causal self-reports.
func ContainsMarkerSubject(message string) bool {
	lower := strings.ToLower(strings.TrimSpace(message))
	return containsAny(lower,
		"rhythm marker", "marker", "travel", "illness", "disruption", "forced schedule",
	)
}

// IsUnsafeMedicalAnswer catches decision language in a model answer to an
// otherwise permitted fact question. False positives fail closed to the stable
// refusal instead of letting generated dosing or treatment advice through.
func IsUnsafeMedicalAnswer(answer string) bool {
	lower := strings.ToLower(strings.TrimSpace(answer))
	if lower == "" {
		return false
	}
	return containsAny(lower,
		"you should", "i recommend", "recommend that", "try taking", "take ",
		"start taking", "stop taking", "continue taking", "avoid taking",
		"increase ", "decrease ", "change your dose", "best time", "is safe",
		"safe for you", "dosage", " milligram", " mg", " microgram", " mcg",
		"light therapy at",
	)
}

func containsMedicalSubject(lower string) bool {
	return containsMedicationFactSubject(lower) || containsAny(lower,
		"light therapy", "treatment",
	)
}

func containsMedicationFactSubject(lower string) bool {
	return containsAny(lower,
		"medication", "medicine", "drug", "dose", "dosing", "melatonin",
		"stimulant", "hypnotic",
	)
}

func containsAny(value string, terms ...string) bool {
	for _, term := range terms {
		if strings.Contains(value, term) {
			return true
		}
	}
	return false
}
