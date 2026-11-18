package org.non24.planner.data

import java.security.MessageDigest
import java.time.Instant
import java.time.ZoneId
import java.time.format.DateTimeFormatter
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.JsonObject
import org.non24.planner.domain.AcquisitionMethod
import org.non24.planner.domain.EvidenceStatus
import org.non24.planner.domain.SleepEpisode

/**
 * Maps Health Connect sleep into the v1 sync contract the self-hosted server
 * already speaks.
 *
 * Two decisions shape everything here.
 *
 * **A revised source record becomes a correction, not a second observation.**
 * The record id is derived from the *logical* source key, so it stays stable
 * across revisions. Pushing a fresh observation when Health Connect revises an
 * episode would leave the server holding two episodes for one night, which
 * shifts a drift fit that indexes by cycle. The revision supersedes through the
 * append-only correction chain instead: the same mechanism the desktop uses,
 * and it keeps the original evidence intact.
 *
 * **A night keeps the clock it was recorded on.** Health Connect stores UTC
 * offsets, not IANA zones. A v1 observation names the zone the owner reads
 * their records in, the configured home zone, and a night recorded on another
 * clock, typically while travelling, carries that clock's offsets beside it
 * (`source_offset`, ADR-0073). It counts like any other night: the estimate
 * runs on instants, and every device draws it on the home clock. Only an
 * episode with no offsets at all is held back and reported, since nothing says
 * which clock it was on.
 *
 * **A session Health Connect no longer has is said to be gone, not erased**
 * (ADR-0072). The phone sends a provider correction that leaves the night out,
 * and another if it comes back; see [SourcePresence].
 */
object SyncContract {

    const val SCHEMA_VERSION: String = "v1"

    /** Mirrors the server rule `^[a-z][a-z0-9_-]{2,63}$`. */
    private val IDENTIFIER = Regex("^[a-z][a-z0-9_-]{2,63}$")

    private val RFC3339: DateTimeFormatter = DateTimeFormatter.ISO_INSTANT

    /** Why an episode cannot be represented in the v1 contract. */
    enum class HoldReason {
        UNSUPPORTED_SOURCE,

        /** No offset was recorded, so the clock it was on is unknown. */
        MISSING_OFFSET,
    }

    data class HeldEpisode(val episode: SleepEpisode, val reason: HoldReason)

    data class Mapping(
        val records: List<OutboxRecord>,
        val held: List<HeldEpisode>,
    )

    /**
     * Builds the records to push for [episodes].
     *
     * [alreadySynced] maps a record id to the source revision last accepted for
     * it, so calling again with no source change produces nothing at all. That
     * is what makes this idempotent at the source rather than relying on the
     * far end to deduplicate.
     *
     * With [read], the newest complete read of Health Connect, the sources this
     * phone read and synced ([own]) are also checked for presence (ADR-0072):
     * one the read no longer returned is said to be gone, and one said to be
     * gone that it returned again is said to be back. Each is said once.
     */
    fun map(
        episodes: List<SleepEpisode>,
        homeZone: ZoneId,
        alreadySynced: Map<String, SourceSyncRevision>,
        now: Instant,
        read: CompleteRead? = null,
        own: Map<String, OwnSource> = emptyMap(),
    ): Mapping {
        val records = mutableListOf<OutboxRecord>()
        val held = mutableListOf<HeldEpisode>()

        if (read != null) records += presenceRecords(read, own)

        for (episode in episodes.sortedBy { it.start }) {
            val hold = holdReason(episode)
            if (hold != null) {
                held += HeldEpisode(episode, hold)
                continue
            }
            val recordId = observationId(episode.logicalSourceId)
            val revision = episode.provenance.sourceUpdatedAt ?: episode.end
            val syncedRevision = alreadySynced[recordId]

            when {
                syncedRevision == null ->
                    records += observationRecord(recordId, episode, homeZone, revision, now)

                revision.isAfter(syncedRevision.revision) ->
                    records += sourceCorrection(recordId, episode.start, episode.end, revision, now)

                else -> Unit // Already current; pushing again would be noise.
            }
        }
        return Mapping(records, held)
    }

    /**
     * What the newest complete read says about the presence of the sources
     * this phone synced: gone where it no longer returned one that began a day
     * or more inside its window, and back where it returned one said to be
     * gone. Each word is dated by the read, so it is minted once.
     */
    private fun presenceRecords(read: CompleteRead, own: Map<String, OwnSource>): List<OutboxRecord> {
        val judgedFrom = read.window.start.plus(SourcePresence.MARGIN)
        return own.entries.sortedBy { it.value.start }.mapNotNull { (recordId, source) ->
            val returned = recordId in read.observationIds
            when {
                returned && source.removed -> sourcePresence(recordId, removed = false, read.window.end)
                returned || source.removed -> null
                source.start.isBefore(judgedFrom) || !source.start.isBefore(read.window.end) -> null
                else -> sourcePresence(recordId, removed = true, read.window.end)
            }
        }
    }

    /** Reports why an episode cannot be placed in time, or null. */
    fun holdReason(episode: SleepEpisode): HoldReason? {
        if (episode.provenance.acquisitionMethod != AcquisitionMethod.HEALTH_CONNECT ||
            episode.provenance.evidenceStatus != EvidenceStatus.IMPORTED
        ) return HoldReason.UNSUPPORTED_SOURCE
        if (episode.startZoneOffset == null || episode.endZoneOffset == null) return HoldReason.MISSING_OFFSET
        return null
    }

    /**
     * The clock an episode was recorded on, when it was not the home zone's at
     * one end or the other: a night away (ADR-0073). Null at home, so a record
     * never says the same thing two ways.
     */
    fun awayOffset(episode: SleepEpisode, homeZone: ZoneId): SourceOffset? {
        val start = episode.startZoneOffset ?: return null
        val end = episode.endZoneOffset ?: return null
        if (start == homeZone.rules.getOffset(episode.start) && end == homeZone.rules.getOffset(episode.end)) return null
        return SourceOffset(start.totalSeconds, end.totalSeconds)
    }

    /** Stable, contract-valid record id for one logical source episode. */
    fun observationId(logicalSourceId: String): String = "hc-" + digest(logicalSourceId)

    /** Correction ids are per revision, so a chain replays deterministically. */
    fun correctionId(recordId: String, revision: Instant): String =
        "cor-" + digest(recordId + "|" + RFC3339.format(revision))

    private fun observationRecord(
        recordId: String,
        episode: SleepEpisode,
        homeZone: ZoneId,
        revision: Instant,
        now: Instant,
    ): OutboxRecord {
        val payload = buildString {
            append("{")
            append("\"observation_id\":").append(quote(recordId)).append(",")
            append("\"kind\":\"sleep_episode\",")
            append("\"start_at\":").append(quote(RFC3339.format(episode.start))).append(",")
            append("\"end_at\":").append(quote(RFC3339.format(episode.end))).append(",")
            append("\"zone_id\":").append(quote(homeZone.id)).append(",")
            append("\"sleep\":{\"classification\":\"principal\"},")
            append("\"provenance\":{")
            // The server's enum is closed: manual, health_connect, os_activity,
            // file_import, synthetic. Anything else is rejected as an invalid
            // sync batch, which is how this value was found to be wrong.
            append("\"acquisition_method\":\"health_connect\",")
            append("\"evidence_status\":\"directly_observed\",")
            append("\"recorded_at\":").append(quote(RFC3339.format(revision))).append(",")
            append("\"source_record_id\":").append(quote(sourceRecordId(episode)))
            append("}")
            awayOffset(episode, homeZone)?.let { offset ->
                append(",\"source_offset\":{")
                append("\"start_seconds\":").append(offset.startSeconds).append(",")
                append("\"end_seconds\":").append(offset.endSeconds)
                append("}")
            }
            append("}")
        }
        return OutboxRecord(
            recordId = recordId,
            kind = "observation",
            createdAt = now,
            sourceRevision = revision,
            payload = payload,
        )
    }

    internal fun sourceCorrection(
        observationRecordId: String,
        start: Instant,
        end: Instant,
        revision: Instant,
        now: Instant,
    ): OutboxRecord {
        val id = correctionId(observationRecordId, revision)
        val payload = buildString {
            append("{")
            append("\"correction_id\":").append(quote(id)).append(",")
            append("\"target_observation_id\":").append(quote(observationRecordId)).append(",")
            // A provider revision is a full timestamp replacement. Its payload
            // must not depend on which earlier revisions this phone happened to see.
            append("\"created_at\":").append(quote(RFC3339.format(revision))).append(",")
            // The source revised its own record, so what is stored and what the
            // source now reports disagree. `source_conflict` is the closest of
            // the four contract reasons; none of them says "source revision".
            append("\"reason\":\"source_conflict\",")
            append("\"acquisition_method\":\"health_connect\",")
            append("\"changes\":{")
            append("\"start_at\":").append(quote(RFC3339.format(start))).append(",")
            append("\"end_at\":").append(quote(RFC3339.format(end)))
            append("}}")
        }
        return OutboxRecord(
            recordId = id,
            kind = "correction",
            createdAt = now,
            sourceRevision = revision,
            payload = payload,
        )
    }

    /**
     * A provider's word that its record is gone ([removed]) or back, said at
     * [at] (ADR-0072). It leaves the record out, or counts it again, and
     * changes nothing else. The id follows from the record, the word and the
     * moment, so one read says it once however often its records are mapped.
     */
    internal fun sourcePresence(observationRecordId: String, removed: Boolean, at: Instant): OutboxRecord {
        val said = RFC3339.format(at)
        val id = "cor-" + digest(observationRecordId + "|" + (if (removed) "removed" else "restored") + "|" + said)
        val payload = buildString {
            append("{")
            append("\"correction_id\":").append(quote(id)).append(",")
            append("\"target_observation_id\":").append(quote(observationRecordId)).append(",")
            append("\"created_at\":").append(quote(said)).append(",")
            append("\"reason\":").append(quote(if (removed) SourcePresence.DELETED else SourcePresence.RESTORED)).append(",")
            append("\"acquisition_method\":\"health_connect\",")
            append("\"changes\":{\"excluded\":").append(removed).append("}")
            append("}")
        }
        return OutboxRecord(recordId = id, kind = "correction", createdAt = at, sourceRevision = at, payload = payload)
    }

    /**
     * What one check of Health Connect found, as a v1 source report
     * (ADR-0054). It carries instants of the check and a count, never a sleep
     * time. [held] counts the records in the read window held back above.
     */
    fun sourceReport(check: SourceCheck, held: Int): OutboxRecord {
        require(held >= 0) { "A held count cannot be negative." }
        val reportedAt = RFC3339.format(check.checkedAt)
        val id = "srcrep-" + digest(
            listOf("health_connect", check.outcome.wire, reportedAt, check.backgroundReads, held).joinToString("|"),
        )
        val payload = buildString {
            append("{")
            append("\"report_id\":").append(quote(id)).append(",")
            append("\"source\":\"health_connect\",")
            append("\"outcome\":").append(quote(check.outcome.wire)).append(",")
            append("\"reported_at\":").append(quote(reportedAt)).append(",")
            append("\"background_reads\":").append(check.backgroundReads)
            check.covered?.let { window ->
                append(",\"covered\":{")
                append("\"start_at\":").append(quote(RFC3339.format(window.start))).append(",")
                append("\"end_at\":").append(quote(RFC3339.format(window.end)))
                append("},\"held\":").append(held)
            }
            append("}")
        }
        return OutboxRecord(
            recordId = id,
            kind = SourceReporting.KIND,
            createdAt = check.checkedAt,
            sourceRevision = check.checkedAt,
            payload = payload,
        )
    }

    private fun sourceRecordId(episode: SleepEpisode): String =
        episode.provenance.sourceRecordId?.takeIf { it.isNotBlank() }
            ?: episode.logicalSourceId

    /**
     * Lowercase hex of a truncated SHA-256.
     *
     * Hashing rather than sanitising keeps the id inside the contract's rule
     * whatever a source package name contains, and keeps that package name off
     * the wire.
     */
    internal fun digest(value: String): String {
        val bytes = MessageDigest.getInstance("SHA-256").digest(value.toByteArray(Charsets.UTF_8))
        val hex = "0123456789abcdef"
        val builder = StringBuilder(24)
        for (index in 0 until 12) {
            builder.append(hex[(bytes[index].toInt() shr 4) and 0xf])
            builder.append(hex[bytes[index].toInt() and 0xf])
        }
        return builder.toString()
    }

    private fun quote(value: String): String = JsonPrimitive(value).toString()

    /** Guards every id this object mints against the server rule. */
    fun isValidIdentifier(value: String): Boolean = IDENTIFIER.matches(value)
}
