package cc.pagepilot.screen

import android.app.Application
import android.util.Log
import com.tencent.smtt.sdk.QbSdk

class ScreenApplication : Application() {
  override fun onCreate() {
    super.onCreate()
    X5RuntimeState.markInitStarted()
    QbSdk.initX5Environment(this, object : QbSdk.PreInitCallback {
      override fun onCoreInitFinished() {
        X5RuntimeState.markCoreInitFinished()
        Log.i("PagePilotX5", "X5 core init finished")
      }

      override fun onViewInitFinished(loaded: Boolean) {
        X5RuntimeState.markViewInitFinished(loaded)
        Log.i("PagePilotX5", "X5 view init finished, loaded=$loaded")
      }
    })
  }
}

object X5RuntimeState {
  @Volatile var initStartedAt: Long = 0L
    private set
  @Volatile var coreInitFinished: Boolean = false
    private set
  @Volatile var viewInitFinished: Boolean? = null
    private set

  fun markInitStarted() {
    initStartedAt = System.currentTimeMillis()
    coreInitFinished = false
    viewInitFinished = null
  }

  fun markCoreInitFinished() {
    coreInitFinished = true
  }

  fun markViewInitFinished(loaded: Boolean) {
    viewInitFinished = loaded
  }

  fun summary(): String {
    val viewState = when (viewInitFinished) {
      true -> "通过"
      false -> "回退"
      null -> "等待回调"
    }
    return "核心初始化${if (coreInitFinished) "完成" else "等待"}，视图初始化$viewState"
  }
}
