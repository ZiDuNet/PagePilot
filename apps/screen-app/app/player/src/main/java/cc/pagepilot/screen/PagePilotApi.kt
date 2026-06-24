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
  val screenId: String,
  val screenName: String,
  val ownerUserId: String,
  val ownerUsername: String,
  val accessCookie: ScreenAccessCookie?,
  val screenshot: ScreenScreenshotCommand?,
  val command: ScreenCommand?,
)

data class ScreenAccessCookie(
  val name: String,
  val value: String,
  val path: String,
  val maxAgeSeconds: Int,
)

data class ScreenScreenshotCommand(
  val requestId: String,
)

data class ScreenCommand(
  val requestId: String,
  val type: String,
)

class PagePilotApi(private val serverUrl: String) {
  suspend fun startPairing(deviceName: String, deviceInfo: JSONObject): PairingSession = withContext(Dispatchers.IO) {
    val body = JSONObject()
      .put("deviceName", deviceName)
      .put("appVersion", BuildConfig.VERSION_NAME)
      .put("runtime", "X5 WebView")
      .put("deviceInfo", deviceInfo)
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
    val screenshot = json.optJSONObject("screenshot")?.let {
      val requestId = it.optString("requestId", "")
      if (requestId.isBlank()) null else ScreenScreenshotCommand(requestId)
    }
    val command = json.optJSONObject("command")?.let {
      val requestId = it.optString("requestId", "")
      val type = it.optString("type", "")
      if (requestId.isBlank() || type.isBlank()) null else ScreenCommand(requestId, type)
    }
    val accessCookie = json.optJSONObject("accessCookie")?.let {
      val name = it.optString("name", "")
      val value = it.optString("value", "")
      if (name.isBlank() || value.isBlank()) {
        null
      } else {
        ScreenAccessCookie(
          name = name,
          value = value,
          path = it.optString("path", "/").ifBlank { "/" },
          maxAgeSeconds = it.optInt("maxAgeSeconds", 300),
        )
      }
    }
    ScreenManifest(
      mode = json.optString("mode", "idle"),
      entryUrl = json.optString("entryUrl", ""),
      siteCode = json.optString("siteCode", ""),
      version = json.optLong("version", 0),
      screenId = json.optString("screenId", ""),
      screenName = json.optString("screenName", ""),
      ownerUserId = json.optString("ownerUserId", ""),
      ownerUsername = json.optString("ownerUsername", ""),
      accessCookie = accessCookie,
      screenshot = screenshot,
      command = command,
    )
  }

  suspend fun heartbeat(deviceToken: String, deviceInfo: JSONObject) = withContext(Dispatchers.IO) {
    val body = JSONObject()
      .put("appVersion", BuildConfig.VERSION_NAME)
      .put("runtime", "X5 WebView")
      .put("deviceInfo", deviceInfo)
    request("POST", "/api/device/heartbeat", body, deviceToken)
  }

  suspend fun uploadScreenshot(
    deviceToken: String,
    requestId: String,
    contentBase64: String,
    mimeType: String,
    width: Int,
    height: Int,
  ) = withContext(Dispatchers.IO) {
    val body = JSONObject()
      .put("requestId", requestId)
      .put("contentBase64", contentBase64)
      .put("mimeType", mimeType)
      .put("width", width)
      .put("height", height)
    request("POST", "/api/device/screenshot", body, deviceToken)
  }

  suspend fun ackCommand(deviceToken: String, requestId: String, type: String) = withContext(Dispatchers.IO) {
    val body = JSONObject()
      .put("requestId", requestId)
      .put("type", type)
    request("POST", "/api/device/command/ack", body, deviceToken)
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
