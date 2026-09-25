package activity

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"non24.app/core/domain"
	"non24.app/core/ingest"
)

func TestGapRecordsCarryInferredProvenanceAndUnknownConfidence(t *testing.T) {
	sink := &ingest.MemorySink{}
	collector := SafeCollector{ZoneID: "UTC", Now: func() time.Time { return base }}
	var sequence uint64
	if err := collector.append(context.Background(), sink, []Transition{{At: base, State: StateSuspended}, {At: base, State: StateResumed}}, "synthetic", &sequence); err != nil {
		t.Fatal(err)
	}
	for _, row := range sink.Snapshot() {
		if row.Evidence.Status != domain.StatusInferred || row.Evidence.Algorithm != CollectorVersion || row.Evidence.RecordedAt.IsZero() {
			t.Fatalf("gap presented as observed: %#v", row.Evidence)
		}
		var value payload
		if err := json.Unmarshal(row.Payload, &value); err != nil {
			t.Fatal(err)
		}
		if value.Confidence == nil || value.Confidence.Level != domain.ConfidenceUnknown {
			t.Fatal("gap has no uncertainty")
		}
	}
}
