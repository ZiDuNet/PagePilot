package cc.pagepilot.screen

import android.app.Application
import com.tencent.smtt.sdk.QbSdk

class ScreenApplication : Application() {
  override fun onCreate() {
    super.onCreate()
    QbSdk.initX5Environment(this, null)
  }
}
