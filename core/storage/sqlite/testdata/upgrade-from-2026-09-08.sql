-- Written by an earlier ZeitBoard build's own store. Synthetic data only.
CREATE TABLE source_observations (
			id TEXT PRIMARY KEY,
			source_id TEXT NOT NULL,
			external_id TEXT NOT NULL DEFAULT '',
			kind TEXT NOT NULL,
			observed_utc TEXT NOT NULL,
			zone_id TEXT NOT NULL,
			recorded_at TEXT NOT NULL,
			evidence_json BLOB NOT NULL,
			payload_json BLOB NOT NULL
		);
CREATE TABLE source_collection_preferences (
			source_id TEXT PRIMARY KEY,
			enabled INTEGER NOT NULL CHECK(enabled IN (0, 1)),
			zone_id TEXT NOT NULL
		);
CREATE TABLE manual_corrections (
			id TEXT PRIMARY KEY,
			target_id TEXT NOT NULL,
			created_at TEXT NOT NULL,
			correction_json BLOB NOT NULL
		);
CREATE TABLE phase_estimates (
			id TEXT PRIMARY KEY,
			created_at TEXT NOT NULL,
			estimate_json BLOB NOT NULL
		);
CREATE TABLE medication_events (
			id TEXT PRIMARY KEY,
			taken_at TEXT NOT NULL,
			event_json BLOB NOT NULL
		);
CREATE TABLE share_profiles (
			id TEXT PRIMARY KEY,
			created_at TEXT NOT NULL,
			profile_json BLOB NOT NULL
		);
CREATE TABLE local_sleep_observations (
			observation_id TEXT PRIMARY KEY,
			kind TEXT NOT NULL,
			start_at TEXT NOT NULL,
			end_at TEXT NOT NULL,
			zone_id TEXT NOT NULL,
			classification TEXT NOT NULL,
			acquisition_method TEXT NOT NULL,
			evidence_status TEXT NOT NULL,
			recorded_at TEXT NOT NULL,
			source_record_id TEXT NOT NULL DEFAULT '',
			payload_json BLOB NOT NULL
		);
INSERT INTO "local_sleep_observations" VALUES('obs_sleep_upgrade_01', 'sleep_episode', '2026-09-06T03:00:00Z', '2026-09-06T11:00:00Z', 'America/New_York', 'principal', 'manual', 'user_reported', '2026-09-06T11:00:00Z', 'desktop-manual', X'7b226f62736572766174696f6e5f6964223a226f62735f736c6565705f757067726164655f3031222c226b696e64223a22736c6565705f657069736f6465222c2273746172745f6174223a22323032362d30392d30365430333a30303a30305a222c22656e645f6174223a22323032362d30392d30365431313a30303a30305a222c227a6f6e655f6964223a22416d65726963612f4e65775f596f726b222c22736c656570223a7b22636c617373696669636174696f6e223a227072696e636970616c227d2c2270726f76656e616e6365223a7b226163717569736974696f6e5f6d6574686f64223a226d616e75616c222c2265766964656e63655f737461747573223a22757365725f7265706f72746564222c227265636f726465645f6174223a22323032362d30392d30365431313a30303a30305a222c22736f757263655f7265636f72645f6964223a226465736b746f702d6d616e75616c227d7d');
INSERT INTO "local_sleep_observations" VALUES('obs_sleep_upgrade_02', 'sleep_episode', '2026-09-07T04:00:00Z', '2026-09-07T12:00:00Z', 'America/New_York', 'principal', 'manual', 'user_reported', '2026-09-07T12:00:00Z', 'desktop-manual', X'7b226f62736572766174696f6e5f6964223a226f62735f736c6565705f757067726164655f3032222c226b696e64223a22736c6565705f657069736f6465222c2273746172745f6174223a22323032362d30392d30375430343a30303a30305a222c22656e645f6174223a22323032362d30392d30375431323a30303a30305a222c227a6f6e655f6964223a22416d65726963612f4e65775f596f726b222c22736c656570223a7b22636c617373696669636174696f6e223a227072696e636970616c227d2c2270726f76656e616e6365223a7b226163717569736974696f6e5f6d6574686f64223a226d616e75616c222c2265766964656e63655f737461747573223a22757365725f7265706f72746564222c227265636f726465645f6174223a22323032362d30392d30375431323a30303a30305a222c22736f757263655f7265636f72645f6964223a226465736b746f702d6d616e75616c227d7d');
CREATE TABLE local_sleep_pending (
			id INTEGER PRIMARY KEY CHECK (id = 1),
			started_at TEXT NOT NULL,
			zone_id TEXT NOT NULL,
			marked_at TEXT NOT NULL
		);
CREATE TABLE local_sleep_corrections (
			correction_id TEXT PRIMARY KEY,
			target_observation_id TEXT NOT NULL,
			created_at TEXT NOT NULL,
			reason TEXT NOT NULL,
			changes_json BLOB NOT NULL,
			payload_json BLOB NOT NULL,
			FOREIGN KEY(target_observation_id) REFERENCES local_sleep_observations(observation_id)
		);
INSERT INTO "local_sleep_corrections" VALUES('corr_sleep_upgrade_01', 'obs_sleep_upgrade_02', '2026-09-08T12:00:00Z', 'user_edit', X'7b2273746172745f6174223a22323032362d30392d30375430343a33303a30305a227d', X'7b22636f7272656374696f6e5f6964223a22636f72725f736c6565705f757067726164655f3031222c227461726765745f6f62736572766174696f6e5f6964223a226f62735f736c6565705f757067726164655f3032222c22637265617465645f6174223a22323032362d30392d30385431323a30303a30305a222c22726561736f6e223a22757365725f65646974222c226368616e676573223a7b2273746172745f6174223a22323032362d30392d30375430343a33303a30305a227d7d');
CREATE TABLE local_sync_state (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL
		);
CREATE TABLE local_sleep_sync_records (
			record_id TEXT PRIMARY KEY,
			kind TEXT NOT NULL,
			payload_hash TEXT NOT NULL,
			pushed_at TEXT NOT NULL
		);
CREATE TABLE local_sleep_erasures (
			record_id TEXT PRIMARY KEY,
			erased_at TEXT NOT NULL
		);
CREATE TABLE local_tasks (
			task_id TEXT PRIMARY KEY,
			status TEXT NOT NULL,
			revision INTEGER NOT NULL DEFAULT 1 CHECK(revision >= 1),
			created_at TEXT NOT NULL,
			payload_json BLOB NOT NULL
		);
INSERT INTO "local_tasks" VALUES('task_upgrade_01', 'open', 1, '2026-09-08T12:00:00Z', X'7b227461736b5f6964223a227461736b5f757067726164655f3031222c227469746c65223a2253796e7468657469632075706772616465207461736b222c226475726174696f6e5f6d696e75746573223a34352c22737461747573223a226f70656e222c22637265617465645f6174223a22323032362d30392d30385431323a30303a30305a222c226c61746573745f66696e6973685f6174223a22323032362d30392d31315431323a30303a30305a222c227265766973696f6e223a312c22757064617465645f6174223a22323032362d30392d30385431323a30303a30305a227d');
CREATE TABLE local_task_sync_records (
			record_id TEXT PRIMARY KEY,
			task_id TEXT NOT NULL,
			pushed_at TEXT NOT NULL
		);
CREATE TABLE local_medications (
			medication_id TEXT PRIMARY KEY,
			active INTEGER NOT NULL CHECK(active IN (0, 1)),
			revision INTEGER NOT NULL CHECK(revision >= 1),
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			payload_json BLOB NOT NULL
		);
INSERT INTO "local_medications" VALUES('med_local_01', 1, 1, '2026-09-08T12:00:00Z', '2026-09-08T12:00:00Z', X'7b226d656469636174696f6e5f6964223a226d65645f6c6f63616c5f3031222c226c6162656c223a2250726976617465206d656469636174696f6e206c6162656c222c22666f726d223a227461626c6574222c22737472656e6774685f6c6162656c223a22757365722d656e746572656420737472656e677468222c22616374697665223a747275652c227363686564756c65223a7b226b696e64223a2261735f6e6565646564222c2272656d696e6465725f656e61626c6564223a66616c73657d2c22637265617465645f6174223a22323032362d30392d30385431323a30303a30305a222c227265766973696f6e223a312c22757064617465645f6174223a22323032362d30392d30385431323a30303a30305a227d');
CREATE TABLE local_medication_events (
			event_id TEXT PRIMARY KEY,
			medication_id TEXT NOT NULL,
			dose_at TEXT NOT NULL,
			status TEXT NOT NULL CHECK(status IN ('taken', 'skipped')),
			scheduled INTEGER NOT NULL CHECK(scheduled IN (0, 1)),
			recorded_at TEXT NOT NULL,
			payload_json BLOB NOT NULL,
			FOREIGN KEY(medication_id) REFERENCES local_medications(medication_id) ON DELETE CASCADE
		);
INSERT INTO "local_medication_events" VALUES('dose_local_01', 'med_local_01', '2026-09-08T11:00:00Z', 'taken', 0, '2026-09-08T12:00:00Z', X'7b226576656e745f6964223a22646f73655f6c6f63616c5f3031222c226d656469636174696f6e5f6964223a226d65645f6c6f63616c5f3031222c22646f73655f6174223a22323032362d30392d30385431313a30303a30305a222c227a6f6e655f6964223a22416d65726963612f4e65775f596f726b222c22737461747573223a2274616b656e222c227363686564756c6564223a66616c73652c226e6f7465223a2250726976617465206576656e74206e6f7465222c2270726f76656e616e6365223a7b226163717569736974696f6e5f6d6574686f64223a226d616e75616c222c2265766964656e63655f737461747573223a22757365725f7265706f72746564222c227265636f726465645f6174223a22323032362d30392d30385431323a30303a30305a227d7d');
CREATE TABLE local_medication_event_corrections (
			correction_id TEXT PRIMARY KEY,
			target_event_id TEXT NOT NULL,
			supersedes_correction_id TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL,
			reason TEXT NOT NULL CHECK(reason IN ('user_edit', 'duplicate', 'invalid_time')),
			changes_json BLOB NOT NULL,
			payload_json BLOB NOT NULL,
			FOREIGN KEY(target_event_id) REFERENCES local_medication_events(event_id) ON DELETE CASCADE
		);
INSERT INTO "local_medication_event_corrections" VALUES('dose_correction_upgrade_01', 'dose_local_01', '', '2026-09-08T12:01:00Z', 'user_edit', X'7b22737461747573223a22736b6970706564227d', X'7b22636f7272656374696f6e5f6964223a22646f73655f636f7272656374696f6e5f757067726164655f3031222c227461726765745f6576656e745f6964223a22646f73655f6c6f63616c5f3031222c22637265617465645f6174223a22323032362d30392d30385431323a30313a30305a222c22726561736f6e223a22757365725f65646974222c226368616e676573223a7b22737461747573223a22736b6970706564227d7d');
CREATE TABLE local_medication_reminder_claims (
			occurrence_id TEXT PRIMARY KEY,
			medication_id TEXT NOT NULL,
			scheduled_at TEXT NOT NULL,
			claimed_at TEXT NOT NULL,
			FOREIGN KEY(medication_id) REFERENCES local_medications(medication_id) ON DELETE CASCADE
		);
CREATE TABLE local_rhythm_markers (
			marker_id TEXT PRIMARY KEY,
			kind TEXT NOT NULL CHECK(kind IN ('travel', 'illness', 'disruption', 'forced_schedule')),
			start_at TEXT NOT NULL,
			end_at TEXT NOT NULL DEFAULT '',
			zone_id TEXT NOT NULL,
			recorded_at TEXT NOT NULL,
			payload_json BLOB NOT NULL,
			CHECK(end_at = '' OR end_at > start_at)
		);
INSERT INTO "local_rhythm_markers" VALUES('marker_test_01', 'travel', '2026-09-08T10:00:00Z', '2026-09-08T11:30:00Z', 'America/New_York', '2026-09-08T12:00:00Z', X'7b226d61726b65725f6964223a226d61726b65725f746573745f3031222c226b696e64223a2274726176656c222c2273746172745f6174223a22323032362d30392d30385431303a30303a30305a222c22656e645f6174223a22323032362d30392d30385431313a33303a30305a222c227a6f6e655f6964223a22416d65726963612f4e65775f596f726b222c226e6f7465223a22507269766174652074726176656c20636f6e74657874222c2270726f76656e616e6365223a7b226163717569736974696f6e5f6d6574686f64223a226d616e75616c222c2265766964656e63655f737461747573223a22757365725f7265706f72746564222c227265636f726465645f6174223a22323032362d30392d30385431323a30303a30305a227d7d');
CREATE TABLE local_calendar_sources (
			source_id TEXT PRIMARY KEY,
			label TEXT NOT NULL,
			kind TEXT NOT NULL CHECK(kind IN ('ics', 'caldav', 'zeitboard')),
			read_only INTEGER NOT NULL CHECK(read_only IN (0, 1)),
			coverage_start_at TEXT NOT NULL,
			coverage_end_at TEXT NOT NULL,
			last_imported_at TEXT NOT NULL,
			endpoint TEXT NOT NULL DEFAULT '',
			CHECK(
				(kind IN ('ics', 'caldav') AND read_only = 1) OR
				(kind = 'zeitboard' AND read_only = 0)
			)
		);
INSERT INTO "local_calendar_sources" VALUES('calendar_source_import_01', 'Imported commitments', 'ics', 1, '2026-01-01T00:00:00Z', '2027-01-01T00:00:00Z', '2026-01-02T00:00:00Z', '');
CREATE TABLE local_calendar_events (
			event_id TEXT PRIMARY KEY,
			source_id TEXT NOT NULL,
			source_record_id TEXT NOT NULL,
			title TEXT NOT NULL,
			start_at TEXT NOT NULL,
			end_at TEXT NOT NULL,
			zone_id TEXT NOT NULL,
			all_day INTEGER NOT NULL CHECK(all_day IN (0, 1)),
			busy INTEGER NOT NULL CHECK(busy IN (0, 1)),
			ownership TEXT NOT NULL CHECK(ownership IN ('imported', 'app_owned')),
			created_at TEXT NOT NULL,
			location TEXT NOT NULL DEFAULT '',
			notes TEXT NOT NULL DEFAULT '',
			task_id TEXT NOT NULL DEFAULT '',
			task_revision INTEGER NOT NULL DEFAULT 0,
			proposal_id TEXT NOT NULL DEFAULT '',
			FOREIGN KEY(source_id) REFERENCES local_calendar_sources(source_id) ON DELETE CASCADE,
			UNIQUE(source_id, source_record_id),
			CHECK(end_at >= start_at),
			CHECK(busy = 0 OR end_at > start_at),
			CHECK(
				(ownership = 'imported' AND task_id = '' AND task_revision = 0 AND proposal_id = '') OR
				(ownership = 'app_owned' AND task_id <> '' AND task_revision >= 1 AND proposal_id <> '')
			)
		);
INSERT INTO "local_calendar_events" VALUES('calendar_event_upgrade_01', 'calendar_source_import_01', 'upgrade-uid/20260910T150000Z', 'Synthetic commitment', '2026-09-10T15:00:00Z', '2026-09-10T16:00:00Z', 'America/New_York', 0, 1, 'imported', '2026-01-02T00:00:00Z', '', '', '', 0, '');
CREATE TABLE local_proposal_decisions (
			decision_id TEXT PRIMARY KEY,
			proposal_id TEXT NOT NULL,
			task_id TEXT NOT NULL,
			task_revision INTEGER NOT NULL,
			estimate_id TEXT NOT NULL,
			proposal_title TEXT NOT NULL,
			proposal_start_at TEXT NOT NULL,
			proposal_end_at TEXT NOT NULL,
			zone_id TEXT NOT NULL,
			confidence TEXT NOT NULL CHECK(confidence IN ('low', 'medium', 'high')),
			explanation_codes_json BLOB NOT NULL,
			decision TEXT NOT NULL CHECK(decision IN ('approved', 'rejected', 'undone')),
			decided_at TEXT NOT NULL,
			supersedes_decision_id TEXT NOT NULL DEFAULT '',
			event_id TEXT NOT NULL DEFAULT '',
			snapshot_start_at TEXT NOT NULL,
			snapshot_end_at TEXT NOT NULL,
			event_snapshot_hash TEXT NOT NULL,
			sleep_snapshot_hash TEXT NOT NULL DEFAULT '',
			CHECK(snapshot_end_at > snapshot_start_at),
			CHECK(proposal_end_at > proposal_start_at),
			CHECK(
				(decision = 'approved' AND event_id <> '' AND supersedes_decision_id = '') OR
				(decision = 'rejected' AND event_id = '' AND supersedes_decision_id = '') OR
				(decision = 'undone' AND supersedes_decision_id <> '')
			)
		);
CREATE TABLE local_sleep_analysis(id INTEGER PRIMARY KEY CHECK(id=1), payload_json BLOB NOT NULL);
CREATE TABLE local_recompute_runs(id INTEGER PRIMARY KEY AUTOINCREMENT, state TEXT NOT NULL, payload_json BLOB NOT NULL);
CREATE UNIQUE INDEX idx_source_external
			ON source_observations(source_id, external_id) WHERE external_id <> '';
CREATE INDEX idx_source_observations_source ON source_observations(source_id);
CREATE INDEX idx_local_sleep_observations_start
			ON local_sleep_observations(start_at);
CREATE UNIQUE INDEX idx_local_sleep_import_source_record
            ON local_sleep_observations(source_record_id)
            WHERE acquisition_method = 'file_import' AND source_record_id <> '';
CREATE INDEX idx_local_sleep_corrections_target
			ON local_sleep_corrections(target_observation_id, created_at);
CREATE INDEX idx_local_medications_active
			ON local_medications(active, updated_at);
CREATE INDEX idx_local_medication_events_dose
			ON local_medication_events(dose_at, event_id);
CREATE INDEX idx_local_medication_corrections_target
			ON local_medication_event_corrections(target_event_id, created_at, correction_id);
CREATE UNIQUE INDEX idx_local_medication_corrections_supersedes
			ON local_medication_event_corrections(supersedes_correction_id)
			WHERE supersedes_correction_id <> '';
CREATE UNIQUE INDEX idx_local_medication_corrections_root
			ON local_medication_event_corrections(target_event_id)
			WHERE supersedes_correction_id = '';
CREATE INDEX idx_local_medication_reminder_claims_medication
			ON local_medication_reminder_claims(medication_id, scheduled_at);
CREATE INDEX idx_local_rhythm_markers_start
			ON local_rhythm_markers(start_at, marker_id);
CREATE INDEX idx_local_calendar_events_interval
			ON local_calendar_events(start_at, end_at);
CREATE INDEX idx_local_calendar_events_busy
			ON local_calendar_events(busy, start_at, end_at);
CREATE INDEX idx_local_proposal_decisions_proposal
			ON local_proposal_decisions(proposal_id, decided_at, decision_id);
CREATE TRIGGER trg_local_medication_events_immutable
			BEFORE UPDATE ON local_medication_events
			BEGIN
				SELECT RAISE(ABORT, 'medication events are immutable');
			END;
CREATE TRIGGER trg_local_medication_corrections_immutable
			BEFORE UPDATE ON local_medication_event_corrections
			BEGIN
				SELECT RAISE(ABORT, 'medication event corrections are immutable');
			END;
CREATE TRIGGER trg_local_medication_reminder_claims_immutable
			BEFORE UPDATE ON local_medication_reminder_claims
			BEGIN
				SELECT RAISE(ABORT, 'medication reminder claims are immutable');
			END;
CREATE TRIGGER trg_local_rhythm_markers_immutable
			BEFORE UPDATE ON local_rhythm_markers
			BEGIN
				SELECT RAISE(ABORT, 'rhythm markers are immutable');
			END;
CREATE TRIGGER trg_local_calendar_imported_immutable
			BEFORE UPDATE ON local_calendar_events
			WHEN OLD.ownership = 'imported'
			BEGIN
				SELECT RAISE(ABORT, 'imported calendar events are immutable');
			END;
CREATE TRIGGER local_sleep_observations_analysis_INSERT AFTER INSERT ON local_sleep_observations BEGIN DELETE FROM local_sleep_analysis;  END;
CREATE TRIGGER local_sleep_observations_analysis_UPDATE AFTER UPDATE ON local_sleep_observations BEGIN DELETE FROM local_sleep_analysis;  END;
CREATE TRIGGER local_sleep_observations_analysis_DELETE AFTER DELETE ON local_sleep_observations BEGIN DELETE FROM local_sleep_analysis; DELETE FROM local_recompute_runs; END;
CREATE TRIGGER local_sleep_corrections_analysis_INSERT AFTER INSERT ON local_sleep_corrections BEGIN DELETE FROM local_sleep_analysis;  END;
CREATE TRIGGER local_sleep_corrections_analysis_UPDATE AFTER UPDATE ON local_sleep_corrections BEGIN DELETE FROM local_sleep_analysis;  END;
CREATE TRIGGER local_sleep_corrections_analysis_DELETE AFTER DELETE ON local_sleep_corrections BEGIN DELETE FROM local_sleep_analysis; DELETE FROM local_recompute_runs; END;
