package syncmodel

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

func TestPushValidatorAndSchemaDriftGuard(t *testing.T) {
	schema := loadSyncBatchSchema(t)
	cases := []struct {
		name  string
		body  string
		valid bool
	}{
		{
			name:  "valid observation push",
			body:  `{"schema_version":"v1","records":[{"recordId":"obs_sleep_01","kind":"observation","createdAt":"2026-03-05T12:40:00Z","payload":` + driftObservationPayload("obs_sleep_01") + `}]}`,
			valid: true,
		},
		{
			name:  "invalid action kind",
			body:  `{"schema_version":"v1","records":[{"recordId":"obs_sleep_01","kind":"delete","createdAt":"2026-03-05T12:40:00Z","payload":` + driftObservationPayload("obs_sleep_01") + `}]}`,
			valid: false,
		},
		{
			name:  "extra payload field",
			body:  `{"schema_version":"v1","records":[{"recordId":"obs_sleep_01","kind":"observation","createdAt":"2026-03-05T12:40:00Z","payload":` + string(bytes.ReplaceAll([]byte(driftObservationPayload("obs_sleep_01")), []byte(`"provenance":`), []byte(`"unexpected":"nope","provenance":`))) + `}]}`,
			valid: false,
		},
		{
			name:  "valid task revision push",
			body:  `{"schema_version":"v1","records":[{"recordId":"task_paperwork_01_r2","kind":"task","createdAt":"2026-03-05T12:40:00Z","payload":{"task_id":"task_paperwork_01","title":"File paperwork","duration_minutes":45,"status":"open","created_at":"2026-03-05T12:00:00Z","revision":2,"updated_at":"2026-03-05T12:02:00Z"}}]}`,
			valid: true,
		},
		{
			name:  "task without revision",
			body:  `{"schema_version":"v1","records":[{"recordId":"task_paperwork_01_r1","kind":"task","createdAt":"2026-03-05T12:40:00Z","payload":{"task_id":"task_paperwork_01","title":"File paperwork","duration_minutes":45,"status":"open","created_at":"2026-03-05T12:00:00Z","updated_at":"2026-03-05T12:02:00Z"}}]}`,
			valid: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			schemaErr := validateSchemaBytes(schema, []byte(tc.body))
			validatorErr := validatePushBytes([]byte(tc.body))
			if (schemaErr == nil) != tc.valid {
				t.Fatalf("schema valid=%v err=%v, want %v", schemaErr == nil, schemaErr, tc.valid)
			}
			if (validatorErr == nil) != tc.valid {
				t.Fatalf("validator valid=%v err=%v, want %v", validatorErr == nil, validatorErr, tc.valid)
			}
		})
	}
}

func loadSyncBatchSchema(t *testing.T) *jsonschema.Schema {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", "..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	compiler := jsonschema.NewCompiler()
	compiler.AssertFormat()
	for _, name := range []string{
		"common.schema.json",
		"observation-set.schema.json",
		"correction-set.schema.json",
		"task-set.schema.json",
		"sync-batch.schema.json",
	} {
		data, err := os.ReadFile(filepath.Join(root, "contracts", "v1", name))
		if err != nil {
			t.Fatal(err)
		}
		doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(data))
		if err != nil {
			t.Fatal(err)
		}
		if err := compiler.AddResource("mem:///contracts/v1/"+name, doc); err != nil {
			t.Fatal(err)
		}
	}
	schema, err := compiler.Compile("mem:///contracts/v1/sync-batch.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	return schema
}

func validateSchemaBytes(schema *jsonschema.Schema, data []byte) error {
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(data))
	if err != nil {
		return err
	}
	return schema.Validate(doc)
}

func validatePushBytes(data []byte) error {
	var req PushRequest
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		return err
	}
	return ValidatePushRequest(&req)
}

func driftObservationPayload(id string) string {
	return `{"observation_id":"` + id + `","kind":"sleep_episode","start_at":"2026-03-05T04:30:00Z","end_at":"2026-03-05T12:30:00Z","zone_id":"America/New_York","sleep":{"classification":"principal"},"provenance":{"acquisition_method":"synthetic","evidence_status":"directly_observed","recorded_at":"2026-03-05T12:35:00Z","source_record_id":"synthetic-source"}}`
}
