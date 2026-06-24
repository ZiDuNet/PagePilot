package cc.pagepilot.screen

import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import org.json.JSONObject
import java.net.HttpURLConnection
import java.net.URL

data class PairingSession(
  val screenId: String,
  val pairingId: String,
  val pairingCode: String,
  val pairingSecret: String,
  val expiresAt: String,
)

data class PairingComplete(
  val paired: Boolean,
  val deviceToken: String,
)

data class ScreenManifest(
  val mode: String,
  val entryUrl: String,
  val siteCode: String,
  val version: Long,
)

class PagePilotApi(private val serverUrl: String) {
  suspend fun startPairing(deviceName: String): PairingSession = withContext(Dispatchers.IO) {
    val body = JSONObject()
      .put("deviceName", deviceName)
      .put("appVersion", BuildConfig.VERSION_NAME)
      .put("runtime", "X5 WebView")
    val json = request("POST", "/api/device/pairing/start", body)
    PairingSession(
      screenId = json.getString("screenId"),
      pairingId = json.getString("pairingId"),
      pairingCode = json.getString("pairingCode"),
      pairingSecret = json.getString("pairingSecret"),
      expiresAt = json.getString("expiresAt"),
    )
  }

  suspend fun completePairing(pairingId: String, pairingSecret: String): PairingComplete = withContext(Dispatchers.IO) {
    val body = JSONObject()
      .put("pairingId", pairingId)
      .put("pairingSecret", pairingSecret)
    val json = request("POST", "/api/device/pairing/complete", body)
    PairingComplete(
      paired = json.optBoolean("paired", false),
      deviceToken = json.optString("deviceToken", ""),
    )
  }

  suspend fun manifest(deviceToken: String): ScreenManifest = withContext(Dispatchers.IO) {
    val json = request("GET", "/api/device/manifest", null, deviceToken)
    ScreenManifest(
      mode = json.optString("mode", "idle"),
      entryUrl = json.optString("entryUrl", ""),
      siteCode = json.optString("siteCode", ""),
      version = json.optLong("version", 0),
    )
  }

  suspend fun heartbeat(deviceToken: String) = withContext(Dispatchers.IO) {
    val body = JSONObject()
      .put("appVersion", BuildConfig.VERSION_NAME)
      .put("runtime", "X5 WebView")
    request("POST", "/api/device/heartbeat", body, deviceToken)
  }

  private fun request(method: String, path: String, body: JSONObject?, deviceToken: String = ""): JSONObject {
    val conn = (URL(serverUrl + path).openConnection() as HttpURLConnection).apply {
      requestMethod = method
      connectTimeout = 12000
      readTimeout = 12000
      setRequestProperty("Accept", "application/json")
      if (deviceToken.isNotBlank()) setRequestProperty("Authorization", "Device $deviceToken")
      if (body != null) {
        doOutput = true
        setRequestProperty("Content-Type", "application/json; charset=utf-8")
      }
    }
    if (body != null) {
      conn.outputStream.use { it.write(body.toString().toByteArray(Charsets.UTF_8)) }
    }
    val stream = if (conn.responseCode in 200..299) conn.inputStream else conn.errorStream
    val text = stream?.bufferedReader(Charsets.UTF_8)?.use { it.readText() }.orEmpty()
    if (conn.responseCode !in 200..299) {
      throw IllegalStateException(text.ifBlank { "HTTP ${conn.responseCode}" })
    }
    return if (text.isBlank()) JSONObject() else JSONObject(text)
  }
}
