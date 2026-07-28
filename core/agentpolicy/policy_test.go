package agentpolicy

import "testing"

func TestMedicalDecisionPolicy(t *testing.T) {
	tests := []struct {
		message string
		refuse  bool
	}{
		{message: "When should I take melatonin?", refuse: true},
		{message: "How much medication should I take?", refuse: true},
		{message: "Does this drug interact with another one?", refuse: true},
		{message: "Can you diagnose Non-24 from my chart?", refuse: true},
		{message: "What is my exact DLMO?", refuse: true},
		{message: "Tell me about my medication", refuse: true},
		{message: "Show when I logged doses relative to wake", refuse: false},
		{message: "List my recorded medication history", refuse: false},
		{message: "What is the next scheduled dose time?", refuse: false},
		{message: "Show medication collision facts", refuse: false},
		{message: "List my light therapy markers", refuse: false},
		{message: "Help with my medication log history", refuse: false},
		{message: "Find 45 minutes for my call", refuse: false},
	}
	for _, test := range tests {
		t.Run(test.message, func(t *testing.T) {
			if got := IsMedicalDecisionPrompt(test.message); got != test.refuse {
				t.Fatalf("IsMedicalDecisionPrompt(%q) = %v, want %v", test.message, got, test.refuse)
			}
		})
	}
}

func TestUnsafeMedicalAnswerPolicy(t *testing.T) {
	tests := []struct {
		answer string
		unsafe bool
	}{
		{answer: "Your last dose was logged 2 h after recorded wake.", unsafe: false},
		{answer: "The next scheduled medication time is 8:00 PM UTC.", unsafe: false},
		{answer: "You should take 5 mg at 8 PM.", unsafe: true},
		{answer: "I recommend that you increase the dose.", unsafe: true},
		{answer: "The best time for light therapy is 7 AM.", unsafe: true},
	}
	for _, test := range tests {
		t.Run(test.answer, func(t *testing.T) {
			if got := IsUnsafeMedicalAnswer(test.answer); got != test.unsafe {
				t.Fatalf("IsUnsafeMedicalAnswer(%q) = %v, want %v", test.answer, got, test.unsafe)
			}
		})
	}
}

func TestMarkerSubjectPolicy(t *testing.T) {
	for _, message := range []string{
		"List my travel markers",
		"Show the forced schedule context",
		"Was an illness marker recorded?",
	} {
		if !ContainsMarkerSubject(message) {
			t.Fatalf("ContainsMarkerSubject(%q) = false", message)
		}
	}
	if ContainsMarkerSubject("Find a window for my tax task") {
		t.Fatal("ordinary planning prompt was classified as a marker question")
	}
}

func TestMedicationFactSubjectPolicy(t *testing.T) {
	for _, message := range []string{
		"Show medication collision facts",
		"List my logged melatonin doses",
	} {
		if !ContainsMedicationFactSubject(message) {
			t.Fatalf("ContainsMedicationFactSubject(%q) = false", message)
		}
	}
	if ContainsMedicationFactSubject("List my light therapy markers") {
		t.Fatal("marker-only question was classified as a medication fact question")
	}
}

func TestMedicalRefusalIsStable(t *testing.T) {
	const expected = "I can't help with medical decisions like medication or dosing. I can show when you logged doses relative to your rhythm, or help you plan around appointments."
	if MedicalRefusal != expected {
		t.Fatalf("medical refusal changed: %q", MedicalRefusal)
	}
}

// Regression: an unknown brand name must still fail closed on a decision
// question, because the substance vocabulary can never be exhaustive.
func TestUnknownMedicationNameStillFailsClosedOnDecisionShape(t *testing.T) {
	decisions := []string{
		"how much Hetlioz should I take tonight?",
		"when should I take my Xyzzyzine?",
		"is it safe to take 3 mg of that before my shift?",
		"can I take my evening capsule earlier?",
		"how often should I take it?",
	}
	for _, prompt := range decisions {
		if !IsMedicalDecisionPrompt(prompt) {
			t.Fatalf("decision-shaped medication prompt was not refused: %q", prompt)
		}
	}
}

// The shape rule must not swallow ordinary scheduling language.
func TestSchedulingPromptsAreNotTreatedAsMedicalDecisions(t *testing.T) {
	planning := []string{
		"when should I take my lunch break?",
		"what time should I start the tax paperwork?",
		"should I move my call to tomorrow?",
		"when am I likely awake tomorrow?",
		"find 90 minutes for deep work before Friday",
	}
	for _, prompt := range planning {
		if IsMedicalDecisionPrompt(prompt) {
			t.Fatalf("ordinary planning prompt was misclassified as medical: %q", prompt)
		}
	}
}

// The unconditional output tier must catch dosing directives, and must not
// fire on ordinary scheduling answers.
func TestMedicationDirectiveScreeningIsPreciseAndUnconditional(t *testing.T) {
	unsafe := []string{
		"Start taking it two hours before bed.",
		"Increase your dose to 20 mg nightly.",
		"Your dosage should move earlier.",
		"Take it with food.",
	}
	for _, answer := range unsafe {
		if !ContainsMedicationDirective(answer) {
			t.Fatalf("medication directive not caught: %q", answer)
		}
	}
	safe := []string{
		"You should have a good window from 2 PM to 8 PM.",
		"Take the 2 PM slot; it avoids your predicted sleep.",
		"The best time for that task looks like Thursday afternoon.",
		"You logged that dose 3 hours after waking the last five times.",
	}
	for _, answer := range safe {
		if ContainsMedicationDirective(answer) {
			t.Fatalf("ordinary answer falsely flagged as a medication directive: %q", answer)
		}
	}
}

// The fallback shape rule stands down for activity objects, but a named
// substance must still refuse even when an activity word is present.
func TestKnownSubstanceRefusesEvenAlongsideActivityWords(t *testing.T) {
	if !IsMedicalDecisionPrompt("should I take my pill during my lunch break?") {
		t.Fatal("known substance with an activity word must still refuse")
	}
	if !IsMedicalDecisionPrompt("can I take melatonin before my meeting?") {
		t.Fatal("known substance with an activity word must still refuse")
	}
}
