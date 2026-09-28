-- The schema an earlier zeitboardd build created, by its own store.
CREATE TABLE portal_profiles (
			profile_id TEXT PRIMARY KEY,
			token_hash BLOB NOT NULL UNIQUE,
			grant_windows INTEGER NOT NULL,
			grant_requests INTEGER NOT NULL,
			grant_messages INTEGER NOT NULL,
			created_at TEXT NOT NULL,
			expires_at TEXT NOT NULL,
			revoked_at TEXT NOT NULL DEFAULT '',
			passcode_hash BLOB NOT NULL,
			passcode_salt BLOB NOT NULL,
			passcode_time INTEGER NOT NULL,
			passcode_memory INTEGER NOT NULL,
			passcode_threads INTEGER NOT NULL
		);
CREATE TABLE portal_snapshots (
			profile_id TEXT PRIMARY KEY REFERENCES portal_profiles(profile_id) ON DELETE CASCADE,
			version INTEGER NOT NULL,
			generated_at TEXT NOT NULL,
			horizon_end TEXT NOT NULL,
			status TEXT NOT NULL,
			nonce BLOB NOT NULL,
			ciphertext BLOB NOT NULL
		);
CREATE TABLE portal_sessions (
			session_hash BLOB PRIMARY KEY,
			profile_id TEXT NOT NULL REFERENCES portal_profiles(profile_id) ON DELETE CASCADE,
			created_at TEXT NOT NULL,
			expires_at TEXT NOT NULL
		);
CREATE TABLE portal_rate_buckets (
			bucket_key TEXT PRIMARY KEY,
			window_start TEXT NOT NULL,
			count INTEGER NOT NULL,
			failures INTEGER NOT NULL DEFAULT 0,
			blocked_until TEXT NOT NULL DEFAULT ''
		);
CREATE TABLE portal_access_audit (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			profile_id TEXT NOT NULL,
			event TEXT NOT NULL,
			source_hmac TEXT NOT NULL,
			occurred_at TEXT NOT NULL
		);
CREATE TABLE portal_audit_keys (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			key BLOB NOT NULL,
			created_at TEXT NOT NULL
		);
CREATE TABLE portal_requests (
			request_id TEXT PRIMARY KEY,
			profile_id TEXT NOT NULL REFERENCES portal_profiles(profile_id) ON DELETE CASCADE,
			session_hash BLOB NOT NULL,
			secret_hash BLOB NOT NULL,
			window_start TEXT NOT NULL,
			window_end TEXT NOT NULL,
			zone_id TEXT NOT NULL,
			duration_minutes INTEGER NOT NULL DEFAULT 0,
			beyond_horizon INTEGER NOT NULL DEFAULT 0,
			status TEXT NOT NULL,
			decided_start TEXT NOT NULL DEFAULT '',
			decided_end TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			nonce BLOB NOT NULL,
			ciphertext BLOB NOT NULL
		);
CREATE TABLE portal_outbox (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			kind TEXT NOT NULL,
			request_id TEXT NOT NULL,
			idempotency_key TEXT NOT NULL UNIQUE,
			created_at TEXT NOT NULL,
			attempts INTEGER NOT NULL DEFAULT 0,
			last_error TEXT NOT NULL DEFAULT ''
		);
CREATE TABLE portal_request_sessions (
			session_hash BLOB PRIMARY KEY,
			request_id TEXT NOT NULL REFERENCES portal_requests(request_id) ON DELETE CASCADE,
			profile_id TEXT NOT NULL,
			expires_at TEXT NOT NULL
		);
CREATE INDEX idx_portal_sessions_profile
			ON portal_sessions(profile_id);
CREATE INDEX idx_portal_access_audit_profile
			ON portal_access_audit(profile_id, id);
CREATE INDEX idx_portal_requests_profile_status
			ON portal_requests(profile_id, status);
CREATE INDEX idx_portal_requests_session
			ON portal_requests(session_hash, created_at);
