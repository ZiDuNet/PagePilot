package cc.pagepilot.screen

import android.app.Activity
import android.app.Notification
import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.PendingIntent
import android.app.Service
import android.content.Context
import android.content.Intent
import android.content.pm.ServiceInfo
import android.hardware.display.DisplayManager
import android.media.projection.MediaProjection
import android.media.projection.MediaProjectionManager
import android.os.Build
import android.os.Handler
import android.os.IBinder
import android.os.Looper
import kotlinx.coroutines.CompletableDeferred

/**
 * 系统截屏前台服务。
 *
 * Android 14+ 对 MediaProjection 强制要求前台服务类型，
 * 这里负责先拉起前台服务，再创建投屏会话。
 */
class ScreenCaptureService : Service() {
  private var projection: MediaProjection? = null
  private var isForeground = false

  override fun onBind(intent: Intent?): IBinder? = null

  override fun onCreate() {
    super.onCreate()
    ensureChannel()
  }

  override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
    when (intent?.action) {
      ACTION_STOP -> {
        stopProjection()
        return START_NOT_STICKY
      }
    }

    startAsForeground()

    val resultCode = intent?.getIntExtra(EXTRA_RESULT_CODE, Activity.RESULT_CANCELED)
      ?: Activity.RESULT_CANCELED
    val data = intent?.captureIntentExtra(EXTRA_RESULT_DATA)
    if (resultCode != Activity.RESULT_OK || data == null) {
      failPending()
      stopProjection()
      return START_NOT_STICKY
    }

    val manager = getSystemService(Context.MEDIA_PROJECTION_SERVICE) as? MediaProjectionManager
      ?: run {
        failPending()
        stopProjection()
        return START_NOT_STICKY
      }

    val created = runCatching {
      manager.getMediaProjection(resultCode, data)
    }.getOrNull()
    if (created == null) {
      failPending()
      stopProjection()
      return START_NOT_STICKY
    }

    projection?.stop()
    projection = created
    ScreenCaptureState.mediaProjection = created
    created.registerCallback(object : MediaProjection.Callback() {
      override fun onStop() {
        stopProjection()
      }
    }, Handler(Looper.getMainLooper()))

    completePending(true)
    return START_STICKY
  }

  override fun onDestroy() {
    stopProjection()
    super.onDestroy()
  }

  private fun startAsForeground() {
    if (isForeground) return
    val notification = buildNotification()
    if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.Q) {
      startForeground(
        NOTIFICATION_ID,
        notification,
        ServiceInfo.FOREGROUND_SERVICE_TYPE_MEDIA_PROJECTION,
      )
    } else {
      @Suppress("DEPRECATION")
      startForeground(NOTIFICATION_ID, notification)
    }
    isForeground = true
  }

  private fun stopProjection() {
    val hadProjection = projection != null || ScreenCaptureState.mediaProjection != null || isForeground
    projection = null
    ScreenCaptureState.mediaProjection = null
    completePending(false)
    if (hadProjection) {
      if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.N) {
        stopForeground(STOP_FOREGROUND_REMOVE)
      } else {
        @Suppress("DEPRECATION")
        stopForeground(true)
      }
    }
    isForeground = false
    stopSelf()
  }

  private fun completePending(ok: Boolean) {
    ScreenCaptureState.readyDeferred?.let { deferred ->
      if (!deferred.isCompleted) {
        deferred.complete(ok)
      }
    }
    ScreenCaptureState.readyDeferred = null
  }

  private fun failPending() {
    completePending(false)
  }

  private fun ensureChannel() {
    if (Build.VERSION.SDK_INT < Build.VERSION_CODES.O) return
    val manager = getSystemService(Context.NOTIFICATION_SERVICE) as? NotificationManager ?: return
    val channel = NotificationChannel(
      CHANNEL_ID,
      "PagePilot 截图",
      NotificationManager.IMPORTANCE_LOW,
    ).apply {
      description = "为系统截屏权限保持前台服务"
      setShowBadge(false)
    }
    manager.createNotificationChannel(channel)
  }

  private fun buildNotification(): Notification {
    val openApp = PendingIntent.getActivity(
      this,
      0,
      Intent(this, MainActivity::class.java).addFlags(Intent.FLAG_ACTIVITY_NEW_TASK or Intent.FLAG_ACTIVITY_CLEAR_TOP),
      pendingIntentFlags(),
    )
    return if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
      Notification.Builder(this, CHANNEL_ID)
        .setContentTitle("PagePilot 截图服务")
        .setContentText("正在等待系统截屏授权")
        .setSmallIcon(android.R.drawable.ic_menu_camera)
        .setOngoing(true)
        .setAutoCancel(false)
        .setContentIntent(openApp)
        .build()
    } else {
      @Suppress("DEPRECATION")
      Notification.Builder(this)
        .setContentTitle("PagePilot 截图服务")
        .setContentText("正在等待系统截屏授权")
        .setSmallIcon(android.R.drawable.ic_menu_camera)
        .setOngoing(true)
        .setAutoCancel(false)
        .setContentIntent(openApp)
        .build()
    }
  }

  private fun pendingIntentFlags(): Int {
    return if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.M) {
      PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE
    } else {
      PendingIntent.FLAG_UPDATE_CURRENT
    }
  }

  private fun Intent.captureIntentExtra(key: String): Intent? {
    return if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU) {
      getParcelableExtra(key, Intent::class.java)
    } else {
      @Suppress("DEPRECATION")
      getParcelableExtra(key)
    }
  }

  companion object {
    private const val ACTION_START = "cc.pagepilot.screen.action.START_CAPTURE"
    private const val ACTION_STOP = "cc.pagepilot.screen.action.STOP_CAPTURE"
    private const val EXTRA_RESULT_CODE = "extra_result_code"
    private const val EXTRA_RESULT_DATA = "extra_result_data"
    private const val NOTIFICATION_ID = 4207
    private const val CHANNEL_ID = "pagepilot_screen_capture"

    fun start(context: Context, resultCode: Int, data: Intent) {
      val intent = Intent(context, ScreenCaptureService::class.java).apply {
        action = ACTION_START
        putExtra(EXTRA_RESULT_CODE, resultCode)
        putExtra(EXTRA_RESULT_DATA, data)
      }
      if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
        context.startForegroundService(intent)
      } else {
        context.startService(intent)
      }
    }

    fun stop(context: Context) {
      val intent = Intent(context, ScreenCaptureService::class.java).apply {
        action = ACTION_STOP
      }
      context.startService(intent)
    }
  }
}

object ScreenCaptureState {
  @Volatile var mediaProjection: MediaProjection? = null
  @Volatile var readyDeferred: CompletableDeferred<Boolean>? = null
}
