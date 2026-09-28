-- The schema an earlier zeitboardd build created, by its own store.
CREATE TABLE schema_migrations (
			version INTEGER PRIMARY KEY,
			applied_at TEXT NOT NULL
		);
CREATE TABLE devices (
			id TEXT PRIMARY KEY,
			label TEXT NOT NULL,
			token_hash BLOB NOT NULL UNIQUE,
			created_at TEXT NOT NULL,
			revoked_at TEXT NOT NULL DEFAULT ''
		);
CREATE TABLE sync_records (
			seq INTEGER PRIMARY KEY AUTOINCREMENT,
			record_id TEXT NOT NULL UNIQUE,
			kind TEXT NOT NULL,
			device_id TEXT NOT NULL REFERENCES devices(id),
			created_at TEXT NOT NULL,
			nonce BLOB NOT NULL,
			ciphertext BLOB NOT NULL,
			payload_hash BLOB NOT NULL
		);
CREATE TABLE sync_tombstones (
			record_id TEXT PRIMARY KEY,
			device_id TEXT NOT NULL,
			erased_at TEXT NOT NULL
		);
CREATE TABLE sync_task_tombstones (
			task_id TEXT PRIMARY KEY,
			device_id TEXT NOT NULL,
			erased_at TEXT NOT NULL
		);
CREATE TABLE sync_correction_targets (
			record_id TEXT PRIMARY KEY REFERENCES sync_records(record_id) ON DELETE CASCADE,
			observation_id TEXT NOT NULL
		);
CREATE TABLE proposals (
			id TEXT PRIMARY KEY,
			action_id TEXT NOT NULL,
			device_id TEXT NOT NULL REFERENCES devices(id),
			status TEXT NOT NULL,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			expires_at TEXT NOT NULL,
			nonce BLOB NOT NULL,
			ciphertext BLOB NOT NULL
		);
CREATE TABLE approval_nonces (
			nonce TEXT PRIMARY KEY,
			proposal_id TEXT NOT NULL REFERENCES proposals(id),
			expires_at TEXT NOT NULL,
			used_at TEXT NOT NULL DEFAULT ''
		);
CREATE TABLE audit_events (
			seq INTEGER PRIMARY KEY AUTOINCREMENT,
			event_type TEXT NOT NULL,
			proposal_id TEXT NOT NULL DEFAULT '',
			device_id TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL,
			nonce BLOB NOT NULL,
			ciphertext BLOB NOT NULL
		);
CREATE TABLE portal_profile_labels (
			profile_id TEXT PRIMARY KEY,
			created_at TEXT NOT NULL,
			nonce BLOB NOT NULL,
			ciphertext BLOB NOT NULL
		);
CREATE TABLE portal_request_proposals (
			portal_request_id TEXT PRIMARY KEY,
			proposal_id TEXT NOT NULL UNIQUE REFERENCES proposals(id),
			profile_id TEXT NOT NULL,
			created_at TEXT NOT NULL
		);
CREATE TABLE portal_status_outbox (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			portal_request_id TEXT NOT NULL,
			status TEXT NOT NULL,
			decided_start TEXT NOT NULL DEFAULT '',
			decided_end TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL
		);
CREATE TABLE recompute_runs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			reason TEXT NOT NULL,
			state TEXT NOT NULL,
			started_at TEXT NOT NULL,
			completed_at TEXT NOT NULL DEFAULT '',
			content_changed_at TEXT NOT NULL DEFAULT '',
			valid_until TEXT NOT NULL DEFAULT '',
			nonce BLOB NOT NULL,
			ciphertext BLOB NOT NULL
		);
CREATE INDEX idx_sync_records_kind_seq
			ON sync_records(kind, seq);
CREATE INDEX idx_sync_correction_observation ON sync_correction_targets(observation_id);
CREATE UNIQUE INDEX idx_approval_nonces_proposal_unused
			ON approval_nonces(proposal_id)
			WHERE used_at = '';
CREATE INDEX idx_recompute_runs_state_id
			ON recompute_runs(state, id);
