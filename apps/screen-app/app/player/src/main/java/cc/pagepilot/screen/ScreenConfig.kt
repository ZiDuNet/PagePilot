package cc.pagepilot.screen

import android.content.Context

data class LocalScreenConfig(
  val serverUrl: String,
  val deviceToken: String,
)

class ScreenConfig(context: Context) {
  private val prefs = context.getSharedPreferences("pagepilot_screen", Context.MODE_PRIVATE)

  fun load(): LocalScreenConfig {
    return LocalScreenConfig(
      serverUrl = prefs.getString("server_url", "").orEmpty(),
      deviceToken = prefs.getString("device_token", "").orEmpty(),
    )
  }

  fun saveServerUrl(url: String) {
    prefs.edit().putString("server_url", normalizeServerUrl(url)).apply()
  }

  fun saveDeviceToken(token: String) {
    prefs.edit().putString("device_token", token).apply()
  }

  fun clearDeviceToken() {
    prefs.edit().remove("device_token").apply()
  }

  fun clearPairing() {
    prefs.edit().remove("device_token").apply()
  }

  private fun normalizeServerUrl(url: String): String {
    return url.trim().trimEnd('/')
  }
}
