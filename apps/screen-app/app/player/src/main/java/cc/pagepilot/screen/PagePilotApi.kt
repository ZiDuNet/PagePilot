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

data class ScreenWSMessage(
  val type: String,
  val manifest: ScreenManifest?,
  val screenshot: ScreenScreenshotCommand?,
  val command: ScreenCommand?,
)

class PagePilotApi(private val serverUrl: String) {
  suspend fun startPairing(deviceName: String, runtime: String, deviceInfo: JSONObject): PairingSession = withContext(Dispatchers.IO) {
    val body = JSONObject()
      .put("deviceName", deviceName)
      .put("appVersion", BuildConfig.VERSION_NAME)
      .put("runtime", runtime)
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
    parseManifest(json)
  }

  suspend fun heartbeat(deviceToken: String, runtime: String, deviceInfo: JSONObject) = withContext(Dispatchers.IO) {
    val body = JSONObject()
      .put("appVersion", BuildConfig.VERSION_NAME)
      .put("runtime", runtime)
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

  fun webSocketUrl(): String {
    val base = serverUrl.trimEnd('/')
    val scheme = when {
      base.startsWith("https://", ignoreCase = true) -> "wss://"
      base.startsWith("http://", ignoreCase = true) -> "ws://"
      else -> "ws://"
    }
    val rest = base.substringAfter("://", base)
    return "$scheme$rest/api/device/ws"
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

  companion object {
    fun parseWSMessage(text: String): ScreenWSMessage {
      val json = JSONObject(text)
      return ScreenWSMessage(
        type = json.optString("type", ""),
        manifest = json.optJSONObject("manifest")?.let { parseManifest(it) },
        screenshot = parseScreenshot(json.optJSONObject("screenshot")),
        command = parseCommand(json.optJSONObject("command")),
      )
    }

    private fun parseManifest(json: JSONObject): ScreenManifest {
      return ScreenManifest(
        mode = json.optString("mode", "idle"),
        entryUrl = json.optString("entryUrl", ""),
        siteCode = json.optString("siteCode", ""),
        version = json.optLong("version", 0),
        screenId = json.optString("screenId", ""),
        screenName = json.optString("screenName", ""),
        ownerUserId = json.optString("ownerUserId", ""),
        ownerUsername = json.optString("ownerUsername", ""),
        accessCookie = parseAccessCookie(json.optJSONObject("accessCookie")),
        screenshot = parseScreenshot(json.optJSONObject("screenshot")),
        command = parseCommand(json.optJSONObject("command")),
      )
    }

    private fun parseAccessCookie(json: JSONObject?): ScreenAccessCookie? {
      json ?: return null
      val name = json.optString("name", "")
      val value = json.optString("value", "")
      if (name.isBlank() || value.isBlank()) return null
      return ScreenAccessCookie(
        name = name,
        value = value,
        path = json.optString("path", "/").ifBlank { "/" },
        maxAgeSeconds = json.optInt("maxAgeSeconds", 300),
      )
    }

    private fun parseScreenshot(json: JSONObject?): ScreenScreenshotCommand? {
      json ?: return null
      val requestId = json.optString("requestId", "")
      return if (requestId.isBlank()) null else ScreenScreenshotCommand(requestId)
    }

    private fun parseCommand(json: JSONObject?): ScreenCommand? {
      json ?: return null
      val requestId = json.optString("requestId", "")
      val type = json.optString("type", "")
      return if (requestId.isBlank() || type.isBlank()) null else ScreenCommand(requestId, type)
    }
  }
}
