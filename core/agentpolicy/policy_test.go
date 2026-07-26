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
