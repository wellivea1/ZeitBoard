package org.non24.planner.data

import java.security.MessageDigest
import java.time.Instant
import java.time.ZoneId
import java.time.format.DateTimeFormatter
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
 * **A record is never labelled with a time zone that contradicts its own
 * evidence.** Health Connect stores UTC offsets, not IANA zones, and a v1
 * observation requires an IANA zone. Rather than inventing one, an episode
 * syncs only when its stored offset matches the configured home zone at that
 * instant. Episodes that disagree — typically travel — are held back and
 * reported rather than guessed at or silently dropped. Carrying true offsets
 * end to end needs a v2 observation contract.
 */
object SyncContract {

    const val SCHEMA_VERSION: String = "v1"

    /** Mirrors the server rule `^[a-z][a-z0-9_-]{2,63}$`. */
    private val IDENTIFIER = Regex("^[a-z][a-z0-9_-]{2,63}$")

    private val RFC3339: DateTimeFormatter = DateTimeFormatter.ISO_INSTANT

    /** Why an episode cannot be represented in the v1 contract. */
    enum class HoldReason {
        /** The stored offset disagrees with the configured home zone. */
        ZONE_OFFSET_MISMATCH,

        /** No offset was recorded, so it cannot be checked at all. */
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
     */
    fun map(
        episodes: List<SleepEpisode>,
        homeZone: ZoneId,
        alreadySynced: Map<String, Instant>,
        now: Instant,
    ): Mapping {
        val records = mutableListOf<OutboxRecord>()
        val held = mutableListOf<HeldEpisode>()

        for (episode in episodes.sortedBy { it.start }) {
            val hold = holdReason(episode, homeZone)
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

                revision.isAfter(syncedRevision) ->
                    records += correctionRecord(recordId, episode, revision, syncedRevision, now)

                else -> Unit // Already current; pushing again would be noise.
            }
        }
        return Mapping(records, held)
    }

    /** Reports why an episode cannot be placed in civil time, or null. */
    fun holdReason(episode: SleepEpisode, homeZone: ZoneId): HoldReason? {
        val startOffset = episode.startZoneOffset ?: return HoldReason.MISSING_OFFSET
        val endOffset = episode.endZoneOffset ?: return HoldReason.MISSING_OFFSET
        val homeStart = homeZone.rules.getOffset(episode.start)
        val homeEnd = homeZone.rules.getOffset(episode.end)
        return if (startOffset == homeStart && endOffset == homeEnd) {
            null
        } else {
            HoldReason.ZONE_OFFSET_MISMATCH
        }
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
            append("}}")
        }
        return OutboxRecord(
            recordId = recordId,
            kind = "observation",
            createdAt = now,
            sourceRevision = revision,
            payload = payload,
        )
    }

    private fun correctionRecord(
        observationRecordId: String,
        episode: SleepEpisode,
        revision: Instant,
        supersedes: Instant,
        now: Instant,
    ): OutboxRecord {
        val id = correctionId(observationRecordId, revision)
        val payload = buildString {
            append("{")
            append("\"correction_id\":").append(quote(id)).append(",")
            append("\"target_observation_id\":").append(quote(observationRecordId)).append(",")
            append("\"supersedes_correction_id\":")
                .append(quote(correctionId(observationRecordId, supersedes))).append(",")
            append("\"created_at\":").append(quote(RFC3339.format(revision))).append(",")
            // The source revised its own record, so what is stored and what the
            // source now reports disagree. `source_conflict` is the closest of
            // the four contract reasons; none of them says "source revision".
            append("\"reason\":\"source_conflict\",")
            append("\"changes\":{")
            append("\"start_at\":").append(quote(RFC3339.format(episode.start))).append(",")
            append("\"end_at\":").append(quote(RFC3339.format(episode.end)))
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
    private fun digest(value: String): String {
        val bytes = MessageDigest.getInstance("SHA-256").digest(value.toByteArray(Charsets.UTF_8))
        val hex = "0123456789abcdef"
        val builder = StringBuilder(24)
        for (index in 0 until 12) {
            builder.append(hex[(bytes[index].toInt() shr 4) and 0xf])
            builder.append(hex[bytes[index].toInt() and 0xf])
        }
        return builder.toString()
    }

    private fun quote(value: String): String {
        val escaped = value.replace("\\", "\\\\").replace("\"", "\\\"")
        return "\"" + escaped + "\""
    }

    /** Guards every id this object mints against the server rule. */
    fun isValidIdentifier(value: String): Boolean = IDENTIFIER.matches(value)
}
