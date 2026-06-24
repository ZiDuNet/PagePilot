package cc.pagepilot.screen

import android.app.Activity
import android.content.res.Configuration
import android.graphics.Bitmap
import android.graphics.Canvas
import android.os.Build
import android.os.Bundle
import android.provider.Settings
import android.util.Base64
import android.view.Gravity
import android.view.View
import android.view.ViewGroup
import android.widget.Button
import android.widget.EditText
import android.widget.FrameLayout
import android.widget.ImageView
import android.widget.LinearLayout
import android.widget.ScrollView
import android.widget.TextView
import com.tencent.smtt.sdk.QbSdk
import com.tencent.smtt.sdk.CookieManager
import com.tencent.smtt.sdk.WebSettings
import com.tencent.smtt.sdk.WebView
import com.tencent.smtt.sdk.WebViewClient
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import org.json.JSONObject
import java.io.ByteArrayOutputStream
import java.net.URL
import java.util.Locale
import java.util.TimeZone
import kotlin.math.max
import kotlin.math.roundToInt

class MainActivity : Activity() {
  private val scopeJob = SupervisorJob()
  private val scope = CoroutineScope(scopeJob + Dispatchers.Main)
  private lateinit var config: ScreenConfig
  private var pairingJob: Job? = null
  private var playbackJob: Job? = null
  private var currentEntryUrl = ""
  private var currentManifest: ScreenManifest? = null
  private var currentWebView: WebView? = null
  private var lastScreenshotRequestId = ""
  private var lastCommandRequestId = ""
  private var hiddenTapCount = 0
  private var hiddenTapStartedAt = 0L
  private var sleeping = false

  override fun onCreate(savedInstanceState: Bundle?) {
    super.onCreate(savedInstanceState)
    config = ScreenConfig(this)
    keepFullscreen()
    route()
  }

  override fun onWindowFocusChanged(hasFocus: Boolean) {
    super.onWindowFocusChanged(hasFocus)
    if (hasFocus) keepFullscreen()
  }

  override fun onDestroy() {
    pairingJob?.cancel()
    playbackJob?.cancel()
    scopeJob.cancel()
    currentWebView?.destroy()
    super.onDestroy()
  }

  private fun keepFullscreen() {
    window.decorView.systemUiVisibility =
      View.SYSTEM_UI_FLAG_FULLSCREEN or
      View.SYSTEM_UI_FLAG_HIDE_NAVIGATION or
      View.SYSTEM_UI_FLAG_IMMERSIVE_STICKY or
      View.SYSTEM_UI_FLAG_LAYOUT_FULLSCREEN or
      View.SYSTEM_UI_FLAG_LAYOUT_HIDE_NAVIGATION
  }

  private fun route() {
    val local = config.load()
    when {
      local.serverUrl.isBlank() -> showServerConfig()
      local.deviceToken.isBlank() -> showPairing(local.serverUrl)
      else -> showPlayer(local.serverUrl, local.deviceToken)
    }
  }

  private fun showServerConfig(error: String = "") {
    stopPlayback()
    val input = EditText(this).apply {
      hint = "https://pagepilot.example.com"
      setSingleLine(true)
      textSize = 18f
      setText(config.load().serverUrl)
    }
    val save = Button(this).apply { text = "保存服务器地址" }
    save.setOnClickListener {
      val url = input.text.toString().trim().trimEnd('/')
      if (!serverUrlAllowed(url)) {
        showServerConfig("生产服务器必须使用 HTTPS；本地调试可使用 localhost、10.0.2.2 或局域网 HTTP。")
        return@setOnClickListener
      }
      config.saveServerUrl(url)
      config.clearDeviceToken()
      route()
    }
    setContentView(centerPanel(
      title = "PagePilot Screen",
      subtitle = "输入服务器地址后开始配对。服务器地址会保存在本机，可在隐藏设置中随时修改。",
      children = listOf(input, save),
      error = error,
    ))
  }

  private fun showPairing(serverUrl: String) {
    stopPlayback(keepPairing = true)
    val title = TextView(this).apply {
      text = "正在创建配对码..."
      textSize = 22f
      gravity = Gravity.CENTER
    }
    val code = TextView(this).apply {
      text = "------"
      textSize = 56f
      gravity = Gravity.CENTER
      letterSpacing = 0.08f
    }
    val tip = TextView(this).apply {
      text = "在 PagePilot 后台或 Agent Skill 中输入配对码。一次配对后设备令牌长期有效，除非后台解绑或本机清除绑定。"
      textSize = 15f
      gravity = Gravity.CENTER
      setTextColor(0xff64748b.toInt())
    }
    val changeServer = Button(this).apply { text = "更换服务器地址" }
    changeServer.setOnClickListener {
      config.saveServerUrl("")
      config.clearDeviceToken()
      showServerConfig()
    }
    setContentView(centerPanel(
      title = "绑定屏幕",
      subtitle = serverUrl,
      children = listOf(title, code, tip, changeServer),
    ))
    pairingJob?.cancel()
    pairingJob = scope.launch {
      try {
        val api = PagePilotApi(serverUrl)
        val session = api.startPairing(deviceName(), deviceInfo())
        title.text = "配对码 5 分钟内有效"
        code.text = session.pairingCode
        while (true) {
          delay(3000)
          val complete = api.completePairing(session.pairingId, session.pairingSecret)
          if (complete.paired && complete.deviceToken.isNotBlank()) {
            config.saveDeviceToken(complete.deviceToken)
            route()
            break
          }
        }
      } catch (err: Exception) {
        showPairingError(serverUrl, err.message ?: "配对失败")
      }
    }
  }

  private fun showPairingError(serverUrl: String, message: String) {
    val retry = Button(this).apply { text = "重试配对" }
    retry.setOnClickListener { showPairing(serverUrl) }
    val changeServer = Button(this).apply { text = "更换服务器地址" }
    changeServer.setOnClickListener {
      config.saveServerUrl("")
      config.clearDeviceToken()
      showServerConfig()
    }
    setContentView(centerPanel(
      title = "配对失败",
      subtitle = serverUrl,
      children = listOf(retry, changeServer),
      error = message,
    ))
  }

  private fun showPlayer(serverUrl: String, deviceToken: String) {
    pairingJob?.cancel()
    currentEntryUrl = ""
    sleeping = false
    val webView = WebView(this)
    currentWebView = webView
    configureWebView(webView)
    val root = FrameLayout(this)
    root.addView(webView, FrameLayout.LayoutParams(
      ViewGroup.LayoutParams.MATCH_PARENT,
      ViewGroup.LayoutParams.MATCH_PARENT,
    ))
    root.addView(hiddenSettingsHotspot(), FrameLayout.LayoutParams(
      dp(92),
      dp(92),
      Gravity.TOP or Gravity.RIGHT,
    ))
    setContentView(root)
    playbackJob?.cancel()
    playbackJob = scope.launch {
      val api = PagePilotApi(serverUrl)
      while (true) {
        try {
          api.heartbeat(deviceToken, deviceInfo())
          val manifest = api.manifest(deviceToken)
          currentManifest = manifest
          handleScreenshotCommand(api, deviceToken, manifest, webView)
          handleScreenCommand(api, deviceToken, manifest, webView)
          applyManifest(serverUrl, manifest, webView)
        } catch (err: Exception) {
          if (currentEntryUrl.isBlank() && !sleeping) {
            webView.loadDataWithBaseURL(serverUrl, errorHtml(err.message ?: "连接失败"), "text/html", "UTF-8", null)
          }
        }
        delay(15000)
      }
    }
  }

  private suspend fun handleScreenshotCommand(
    api: PagePilotApi,
    deviceToken: String,
    manifest: ScreenManifest,
    webView: WebView,
  ) {
    val shot = manifest.screenshot ?: return
    if (shot.requestId == lastScreenshotRequestId) return
    val captured = captureWebView(webView)
    api.uploadScreenshot(
      deviceToken = deviceToken,
      requestId = shot.requestId,
      contentBase64 = captured.base64,
      mimeType = captured.mimeType,
      width = captured.width,
      height = captured.height,
    )
    lastScreenshotRequestId = shot.requestId
  }

  private suspend fun handleScreenCommand(
    api: PagePilotApi,
    deviceToken: String,
    manifest: ScreenManifest,
    webView: WebView,
  ) {
    val command = manifest.command ?: return
    if (command.requestId == lastCommandRequestId) return
    when (command.type) {
      "refresh" -> {
        val target = manifest.entryUrl.ifBlank { currentEntryUrl }
        if (target.isNotBlank()) {
          applyAccessCookie(manifest, target, webView)
          currentEntryUrl = target
          webView.loadUrl(target)
        } else {
          webView.reload()
        }
      }
      "sleep" -> {
        sleeping = true
        currentEntryUrl = ""
        webView.loadDataWithBaseURL(manifest.entryUrl.ifBlank { "about:blank" }, standbyHtml("屏幕已休眠"), "text/html", "UTF-8", null)
      }
      "wake" -> {
        sleeping = false
        val target = manifest.entryUrl
        currentEntryUrl = target
        if (target.isNotBlank()) {
          applyAccessCookie(manifest, target, webView)
          webView.loadUrl(target)
        }
      }
      "shutdown" -> {
        sleeping = true
        currentEntryUrl = ""
        webView.loadDataWithBaseURL(manifest.entryUrl.ifBlank { "about:blank" }, standbyHtml("软关机待机"), "text/html", "UTF-8", null)
      }
    }
    api.ackCommand(deviceToken, command.requestId, command.type)
    lastCommandRequestId = command.requestId
  }

  private fun applyManifest(serverUrl: String, manifest: ScreenManifest, webView: WebView) {
    if (sleeping) return
    if (manifest.mode == "webapp" && manifest.entryUrl.isNotBlank()) {
      applyAccessCookie(manifest, manifest.entryUrl, webView)
      if (manifest.entryUrl != currentEntryUrl) {
        currentEntryUrl = manifest.entryUrl
        webView.loadUrl(manifest.entryUrl)
      }
      return
    }
    if (currentEntryUrl.isBlank()) {
      webView.loadDataWithBaseURL(serverUrl, idleHtml(), "text/html", "UTF-8", null)
    }
  }

  private fun stopPlayback(keepPairing: Boolean = false) {
    if (!keepPairing) pairingJob?.cancel()
    playbackJob?.cancel()
    currentWebView?.let {
      runCatching { it.stopLoading() }
      runCatching { it.clearHistory() }
      runCatching { it.destroy() }
    }
    currentWebView = null
    currentEntryUrl = ""
  }

  private fun applyAccessCookie(manifest: ScreenManifest, entryUrl: String, webView: WebView) {
    val cookie = manifest.accessCookie ?: return
    val parsed = runCatching { URL(entryUrl) }.getOrNull() ?: return
    val port = if (parsed.port > 0) ":${parsed.port}" else ""
    val cookieURL = "${parsed.protocol}://${parsed.host}${port}/"
    val secure = if (parsed.protocol.equals("https", ignoreCase = true)) "; Secure" else ""
    val cookieText = "${cookie.name}=${cookie.value}; Path=${cookie.path}; Max-Age=${cookie.maxAgeSeconds}; SameSite=Lax$secure"
    val manager = CookieManager.getInstance()
    manager.setAcceptCookie(true)
    runCatching { manager.setAcceptThirdPartyCookies(webView, true) }
    manager.setCookie(cookieURL, cookieText)
    runCatching { manager.flush() }
  }

  private fun hiddenSettingsHotspot(): View {
    return View(this).apply {
      setBackgroundColor(0x00000000)
      setOnClickListener {
        val now = System.currentTimeMillis()
        if (now - hiddenTapStartedAt > 2500) {
          hiddenTapStartedAt = now
          hiddenTapCount = 0
        }
        hiddenTapCount += 1
        if (hiddenTapCount >= 5) {
          hiddenTapCount = 0
          showSettings()
        }
      }
    }
  }

  private fun showSettings() {
    stopPlayback()
    val local = config.load()
    val serverInput = EditText(this).apply {
      hint = "服务器地址"
      setSingleLine(true)
      textSize = 16f
      setText(local.serverUrl)
    }
    val saveServer = Button(this).apply { text = "保存服务器并重新配对" }
    saveServer.setOnClickListener {
      val url = serverInput.text.toString().trim().trimEnd('/')
      if (!serverUrlAllowed(url)) {
        showServerConfig("服务器地址不合法")
        return@setOnClickListener
      }
      config.saveServerUrl(url)
      config.clearDeviceToken()
      route()
    }
    val unbind = Button(this).apply { text = "清除本机绑定" }
    unbind.setOnClickListener {
      config.clearDeviceToken()
      route()
    }
    val back = Button(this).apply { text = "返回播放" }
    back.setOnClickListener { route() }
    val info = settingsInfo(local)
    setContentView(ScrollView(this).apply {
      addView(centerPanel(
        title = "屏幕设置",
        subtitle = "右上角连续点击 5 次可打开本页。",
        children = listOf(serverInput, info, saveServer, unbind, back),
      ))
    })
  }

  private fun settingsInfo(local: LocalScreenConfig): View {
    val manifest = currentManifest
    val text = listOf(
      "服务器：${local.serverUrl.ifBlank { "-" }}",
      "屏幕：${manifest?.screenName?.ifBlank { manifest.screenId } ?: "-"}",
      "用户：${manifest?.ownerUsername?.ifBlank { manifest.ownerUserId } ?: "-"}",
      "分辨率：${resources.displayMetrics.widthPixels} x ${resources.displayMetrics.heightPixels} px",
      "方向：${orientationLabel()}",
      "Android：${Build.VERSION.RELEASE} (SDK ${Build.VERSION.SDK_INT})",
      "设备：${Build.MANUFACTURER} ${Build.MODEL}",
      "X5：${x5VersionLabel()}",
      "开发者：武硕：http://武硕.top",
    ).joinToString("\n")
    return TextView(this).apply {
      this.text = text
      textSize = 15f
      setTextColor(0xff334155.toInt())
      setLineSpacing(4f, 1.0f)
    }
  }

  private fun configureWebView(webView: WebView) {
    webView.settings.javaScriptEnabled = true
    webView.settings.domStorageEnabled = true
    webView.settings.databaseEnabled = true
    webView.settings.cacheMode = WebSettings.LOAD_DEFAULT
    webView.settings.mediaPlaybackRequiresUserGesture = false
    webView.settings.textZoom = 100
    webView.settings.useWideViewPort = true
    webView.settings.loadWithOverviewMode = true
    webView.settings.setSupportZoom(false)
    webView.settings.builtInZoomControls = false
    webView.settings.displayZoomControls = false
    webView.settings.loadsImagesAutomatically = true
    webView.webViewClient = object : WebViewClient() {
      override fun onPageFinished(view: WebView?, url: String?) {
        view?.loadUrl(
          "javascript:(function(){var h=document.head||document.getElementsByTagName('head')[0];" +
            "if(!h)return;var m=document.querySelector('meta[name=\"viewport\"]');" +
            "if(!m){m=document.createElement('meta');m.name='viewport';h.appendChild(m);}" +
            "if(!m.content)m.content='width=device-width, initial-scale=1, maximum-scale=1, user-scalable=no';})()"
        )
      }
    }
  }

  private suspend fun captureWebView(webView: WebView): CapturedScreenshot {
    val width = webView.width.coerceAtLeast(1)
    val height = webView.height.coerceAtLeast(1)
    val maxSide = 1920f
    val scale = (maxSide / max(width, height).toFloat()).coerceAtMost(1f)
    val outWidth = (width * scale).roundToInt().coerceAtLeast(1)
    val outHeight = (height * scale).roundToInt().coerceAtLeast(1)
    val bitmap = Bitmap.createBitmap(outWidth, outHeight, Bitmap.Config.ARGB_8888)
    val canvas = Canvas(bitmap)
    canvas.scale(scale, scale)
    webView.draw(canvas)
    val bytes = withContext(Dispatchers.Default) {
      val qualitySteps = listOf(78, 68, 58, 48)
      var best = ByteArray(0)
      for (quality in qualitySteps) {
        val out = ByteArrayOutputStream()
        bitmap.compress(Bitmap.CompressFormat.JPEG, quality, out)
        best = out.toByteArray()
        if (best.size <= 2_800_000) break
      }
      best
    }
    bitmap.recycle()
    return CapturedScreenshot(
      base64 = Base64.encodeToString(bytes, Base64.NO_WRAP),
      mimeType = "image/jpeg",
      width = outWidth,
      height = outHeight,
    )
  }

  private fun centerPanel(title: String, subtitle: String, children: List<View>, error: String = ""): View {
    return LinearLayout(this).apply {
      orientation = LinearLayout.VERTICAL
      gravity = Gravity.CENTER
      setPadding(dp(32), dp(32), dp(32), dp(32))
      setBackgroundColor(0xffeef7fb.toInt())
      addView(ImageView(context).apply {
        setImageResource(R.drawable.pagepilot_logo)
        contentDescription = "PagePilot"
      }, LinearLayout.LayoutParams(dp(72), dp(72)).apply {
        gravity = Gravity.CENTER_HORIZONTAL
        setMargins(0, 0, 0, dp(12))
      })
      addView(TextView(context).apply {
        text = title
        textSize = 32f
        gravity = Gravity.CENTER
        setTextColor(0xff07101f.toInt())
      }, panelParams())
      addView(TextView(context).apply {
        text = subtitle
        textSize = 17f
        gravity = Gravity.CENTER
        setTextColor(0xff5b6d82.toInt())
      }, panelParams())
      if (error.isNotBlank()) {
        addView(TextView(context).apply {
          text = error
          textSize = 16f
          gravity = Gravity.CENTER
          setTextColor(0xffbe123c.toInt())
        }, panelParams())
      }
      children.forEach { addView(it, panelParams()) }
    }
  }

  private fun panelParams(): LinearLayout.LayoutParams {
    return LinearLayout.LayoutParams(
      ViewGroup.LayoutParams.MATCH_PARENT,
      ViewGroup.LayoutParams.WRAP_CONTENT,
    ).apply {
      width = resources.displayMetrics.widthPixels.coerceAtMost(dp(760))
      setMargins(0, dp(8), 0, dp(8))
    }
  }

  private fun deviceName(): String {
    val androidId = Settings.Secure.getString(contentResolver, Settings.Secure.ANDROID_ID).orEmpty()
    val model = "${Build.MANUFACTURER} ${Build.MODEL}".trim()
    return "${model.ifBlank { "Screen" }}-${androidId.takeLast(6)}"
  }

  private fun deviceInfo(): JSONObject {
    val metrics = resources.displayMetrics
    return JSONObject()
      .put("manufacturer", Build.MANUFACTURER)
      .put("brand", Build.BRAND)
      .put("model", Build.MODEL)
      .put("device", Build.DEVICE)
      .put("androidRelease", Build.VERSION.RELEASE)
      .put("androidSdk", Build.VERSION.SDK_INT)
      .put("screenWidthPx", metrics.widthPixels)
      .put("screenHeightPx", metrics.heightPixels)
      .put("orientation", orientationLabel())
      .put("densityDpi", metrics.densityDpi)
      .put("density", metrics.density)
      .put("locale", Locale.getDefault().toLanguageTag())
      .put("timeZone", TimeZone.getDefault().id)
      .put("x5Version", x5VersionLabel())
      .put("appVersion", BuildConfig.VERSION_NAME)
  }

  private fun orientationLabel(): String {
    return if (resources.configuration.orientation == Configuration.ORIENTATION_LANDSCAPE) "landscape" else "portrait"
  }

  private fun x5VersionLabel(): String {
    val version = QbSdk.getTbsVersion(this)
    return if (version > 0) version.toString() else "system-webview"
  }

  private fun serverUrlAllowed(url: String): Boolean {
    if (url.startsWith("https://")) return true
    if (!url.startsWith("http://")) return false
    val host = url.removePrefix("http://").substringBefore('/').substringBefore(':')
    return host == "127.0.0.1" ||
      host == "localhost" ||
      host == "10.0.2.2" ||
      host.startsWith("192.168.") ||
      host.startsWith("10.") ||
      host.matches(Regex("^172\\.(1[6-9]|2\\d|3[01])\\..+"))
  }

  private fun idleHtml(): String {
    return """<!doctype html><html><head><meta name="viewport" content="width=device-width,initial-scale=1"></head><body style="margin:0;height:100vh;display:grid;place-items:center;background:#082f49;color:#e0f2fe;font-family:sans-serif"><div style="text-align:center"><h1 style="font-size:42px;margin:0 0 12px">PagePilot Screen</h1><p style="font-size:20px;margin:0;color:#bae6fd">等待投放内容</p></div></body></html>"""
  }

  private fun standbyHtml(message: String): String {
    return """<!doctype html><html><head><meta name="viewport" content="width=device-width,initial-scale=1"></head><body style="margin:0;height:100vh;background:#020617;color:#334155;font-family:sans-serif;display:grid;place-items:center"><div style="font-size:18px">${escapeHtml(message)}</div></body></html>"""
  }

  private fun errorHtml(message: String): String {
    return """<!doctype html><html><head><meta name="viewport" content="width=device-width,initial-scale=1"></head><body style="margin:0;height:100vh;display:grid;place-items:center;background:#111827;color:#fecaca;font-family:sans-serif"><div style="text-align:center;max-width:80vw"><h1>连接失败</h1><p>${escapeHtml(message)}</p></div></body></html>"""
  }

  private fun escapeHtml(value: String): String {
    return value
      .replace("&", "&amp;")
      .replace("<", "&lt;")
      .replace(">", "&gt;")
      .replace("\"", "&quot;")
  }

  private fun dp(value: Int): Int {
    return (value * resources.displayMetrics.density).roundToInt()
  }
}

data class CapturedScreenshot(
  val base64: String,
  val mimeType: String,
  val width: Int,
  val height: Int,
)
