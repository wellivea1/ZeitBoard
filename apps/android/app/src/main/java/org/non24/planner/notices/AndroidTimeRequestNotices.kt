package org.non24.planner.notices

import android.Manifest
import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.PendingIntent
import android.content.Context
import android.content.Intent
import android.content.pm.PackageManager
import android.os.Build
import androidx.core.app.NotificationCompat
import androidx.core.app.NotificationManagerCompat
import androidx.core.content.ContextCompat
import org.non24.planner.MainActivity
import org.non24.planner.R
import org.non24.planner.data.NoticeCursorStore
import org.non24.planner.data.NoticeKind
import org.non24.planner.data.NoticePoster
import org.non24.planner.data.commitDurably

private const val CHANNEL_ID = "time_requests"
private const val NOTICE_ID = 1

/**
 * Posts time-request notices. Every word is fixed here: a notice says that
 * something happened, never who asked, what they wrote or when they meant.
 */
class AndroidNoticePoster(context: Context) : NoticePoster {
    private val context = context.applicationContext
    private val manager = NotificationManagerCompat.from(this.context)

    override fun prepare() = ensureChannel()

    fun ensureChannel() {
        val channel = NotificationChannel(CHANNEL_ID, "Time requests", NotificationManager.IMPORTANCE_DEFAULT).apply {
            description = "When someone you shared a link with asks for a time or writes about one."
            setShowBadge(true)
        }
        context.getSystemService(NotificationManager::class.java).createNotificationChannel(channel)
    }

    fun canPost(): Boolean =
        manager.areNotificationsEnabled() && (Build.VERSION.SDK_INT < Build.VERSION_CODES.TIRAMISU ||
            ContextCompat.checkSelfPermission(context, Manifest.permission.POST_NOTIFICATIONS) == PackageManager.PERMISSION_GRANTED)

    override fun show(subject: String, kind: NoticeKind) {
        if (!canPost()) return
        ensureChannel()
        val open = PendingIntent.getActivity(
            context, 0,
            Intent(context, MainActivity::class.java).addFlags(Intent.FLAG_ACTIVITY_NEW_TASK or Intent.FLAG_ACTIVITY_CLEAR_TOP),
            PendingIntent.FLAG_IMMUTABLE or PendingIntent.FLAG_UPDATE_CURRENT,
        )
        val text = when (kind) {
            NoticeKind.REQUEST -> "Someone asked for a time. Answer it in ZeitBoard on your computer."
            NoticeKind.MESSAGE -> "A new message about a time request. Read it in ZeitBoard on your computer."
        }
        val notice = NotificationCompat.Builder(context, CHANNEL_ID)
            .setSmallIcon(R.drawable.ic_notice)
            .setContentTitle(if (kind == NoticeKind.REQUEST) "Time request" else "Message about a time request")
            .setContentText(text)
            .setStyle(NotificationCompat.BigTextStyle().bigText(text))
            .setVisibility(NotificationCompat.VISIBILITY_PRIVATE)
            .setPublicVersion(
                NotificationCompat.Builder(context, CHANNEL_ID)
                    .setSmallIcon(R.drawable.ic_notice)
                    .setContentTitle("ZeitBoard")
                    .build(),
            )
            .setContentIntent(open)
            .setAutoCancel(true)
            .build()
        try {
            manager.notify(subject, NOTICE_ID, notice)
        } catch (_: SecurityException) {
            // Permission withdrawn between the check and the post: nothing shown.
        }
    }

    override fun retire(subject: String) = manager.cancel(subject, NOTICE_ID)

    override fun retireAll() {
        manager.activeNotifications
            .filter { it.notification.channelId == CHANNEL_ID }
            .forEach { manager.cancel(it.tag, it.id) }
    }
}

class SharedPreferencesNoticeCursorStore(context: Context) : NoticeCursorStore {
    private val preferences = context.applicationContext.getSharedPreferences("non24_notices", Context.MODE_PRIVATE)

    override fun load(scope: String): Long? =
        if (preferences.getString(SCOPE_KEY, null) == scope && preferences.contains(CURSOR_KEY)) preferences.getLong(CURSOR_KEY, 0) else null

    override fun save(scope: String, cursor: Long) {
        check(preferences.commitDurably { putString(SCOPE_KEY, scope); putLong(CURSOR_KEY, cursor) }) {
            "The notice position could not be saved."
        }
    }

    override fun clear() {
        preferences.commitDurably { clear() }
    }

    private companion object {
        const val SCOPE_KEY = "scope"
        const val CURSOR_KEY = "cursor"
    }
}
