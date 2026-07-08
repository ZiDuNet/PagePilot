package cc.pagepilot.screen

import android.app.Activity
import android.app.Dialog
import android.content.Context
import android.content.Intent
import android.content.res.Configuration
import android.graphics.Bitmap
import android.graphics.PixelFormat
import android.graphics.drawable.GradientDrawable
import android.hardware.display.DisplayManager
import android.media.ImageReader
import android.media.projection.MediaProjection
import android.media.projection.MediaProjectionManager
import android.net.ConnectivityManager
import android.net.NetworkCapabilities
import android.os.Build
import android.os.Bundle
import android.os.Handler
import android.os.Looper
import android.provider.Settings
import android.util.Base64
import android.view.Gravity
import android.view.View
import android.view.ViewGroup
import android.view.Window
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
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.delay
import kotlinx.coroutines.isActive
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import kotlinx.coroutines.withTimeoutOrNull
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.Response
import okhttp3.WebSocket
import okhttp3.WebSocketListener
import org.json.JSONObject
import java.io.ByteArrayOutputStream
import java.net.URL
import java.util.Locale
import java.util.TimeZone
import java.util.concurrent.TimeUnit
import kotlin.math.max
import kotlin.math.roundToInt

class MainActivity : Activity() {
  companion object {
    private const val SCREEN_CAPTURE_REQUEST_CODE = 7201
  }

  private val scopeJob = SupervisorJob()
  private val scope = CoroutineScope(scopeJob + Dispatchers.Main)
  private val wsClient = OkHttpClient.Builder()
    .pingInterval(25, TimeUnit.SECONDS)
    .build()
  private lateinit var config: ScreenConfig
  private var pairingJob: Job? = null
  private var playbackJob: Job? = null
  private var heartbeatJob: Job? = null
  private var statusJob: Job? = null
  private var screenSocket: WebSocket? = null
  private var currentEntryUrl = ""
  private var currentManifest: ScreenManifest? = null
  private var currentWebView: WebView? = null
  private var lastScreenshotRequestId = ""
  private var lastCommandRequestId = ""
  private var hiddenTapCount = 0
  private var hiddenTapStartedAt = 0L
  private var sleeping = false
  private var socketStatus = "未连接"
  private var socketLastEvent = "-"
  private var lastWebSocketUrl = "-"
  private var lastWebViewRuntime = "未检测"
  private var lastX5Loaded = false
  private var lastX5Version = 0
  private var networkDot: View? = null
  private var socketDot: View? = null
  private var networkStatusLabel: String = "未连接"
  private var socketStatusLabel: String = "未连接"
  private var statusOverlay: View? = null
  private var mediaProjectionManager: MediaProjectionManager? = null
  private var mediaProjection: MediaProjection? = null
  private var pendingProjection: CompletableDeferred<Boolean>? = null

  override fun onCreate(savedInstanceState: Bundle?) {
    super.onCreate(savedInstanceState)
    config = ScreenConfig(this)
    mediaProjectionManager = getSystemService(Context.MEDIA_PROJECTION_SERVICE) as? MediaProjectionManager
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
    heartbeatJob?.cancel()
    statusJob?.cancel()
    screenSocket?.cancel()
    mediaProjection?.stop()
    ScreenCaptureService.stop(this)
    wsClient.dispatcher.executorService.shutdown()
    scopeJob.cancel()
    currentWebView?.destroy()
    super.onDestroy()
  }

  override fun onActivityResult(requestCode: Int, resultCode: Int, data: Intent?) {
    super.onActivityResult(requestCode, resultCode, data)
    if (requestCode != SCREEN_CAPTURE_REQUEST_CODE) return
    val granted = resultCode == RESULT_OK && data != null
    if (!granted) {
      pendingProjection?.complete(false)
      pendingProjection = null
      ScreenCaptureState.readyDeferred = null
      ScreenCaptureService.stop(this)
      keepFullscreen()
      return
    }
    runCatching {
      ScreenCaptureService.start(this, resultCode, data!!)
    }.onFailure {
      pendingProjection?.complete(false)
      pendingProjection = null
      ScreenCaptureState.readyDeferred = null
      ScreenCaptureService.stop(this)
    }
    keepFullscreen()
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
      textSize = 14f
      setText(wrapServerUrlForInput(config.load().serverUrl))
      setSelection(0)
    }
    val save = Button(this).apply { text = "保存服务器地址" }
    save.setOnClickListener {
      val url = normalizeServerInput(input.text.toString())
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
        val session = api.startPairing(deviceName(), webViewRuntimeLabel(), deviceInfo())
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
    root.addView(statusHotspot(), FrameLayout.LayoutParams(
      dp(26),
      dp(14),
      Gravity.TOP or Gravity.RIGHT,
    ).apply {
      setMargins(0, dp(5), dp(5), 0)
    })
    setContentView(root)
    startStatusIndicatorUpdates()
    playbackJob?.cancel()
    playbackJob = scope.launch {
      val api = PagePilotApi(serverUrl)
      try {
        api.heartbeat(deviceToken, webViewRuntimeLabel(), deviceInfo())
        val manifest = api.manifest(deviceToken)
        currentManifest = manifest
        applyManifest(serverUrl, manifest, webView)
        handleScreenshotCommand(api, deviceToken, manifest, webView)
        handleScreenCommand(api, deviceToken, manifest, webView)
      } catch (err: Exception) {
        if (currentEntryUrl.isBlank() && !sleeping) {
          webView.loadDataWithBaseURL(serverUrl, errorHtml(err.message ?: "连接失败"), "text/html", "UTF-8", null)
        }
      }
      startHeartbeat(api, deviceToken)
      var useQueryTokenFallback = false
      while (isActive) {
        try {
          socketStatus = "连接中"
          val shouldUseFallback = openScreenSocket(api, deviceToken, serverUrl, webView, useQueryTokenFallback)
          useQueryTokenFallback = useQueryTokenFallback || shouldUseFallback
        } catch (err: Exception) {
          if (currentEntryUrl.isBlank() && !sleeping) {
            webView.loadDataWithBaseURL(serverUrl, errorHtml(err.message ?: "连接失败"), "text/html", "UTF-8", null)
          }
        }
        socketStatus = "重连等待"
        delay(2500)
      }
    }
  }

  private fun startHeartbeat(api: PagePilotApi, deviceToken: String) {
    heartbeatJob?.cancel()
    heartbeatJob = scope.launch {
      while (isActive) {
        runCatching { api.heartbeat(deviceToken, webViewRuntimeLabel(), deviceInfo()) }
        delay(15000)
      }
    }
  }

  private suspend fun openScreenSocket(
    api: PagePilotApi,
    deviceToken: String,
    serverUrl: String,
    webView: WebView,
    useQueryTokenFallback: Boolean,
  ): Boolean {
    val closed = CompletableDeferred<Boolean>()
    val wsUrl = api.webSocketUrl(if (useQueryTokenFallback) deviceToken else "")
    lastWebSocketUrl = wsUrl.replace(deviceToken, "***")
    val request = Request.Builder()
      .url(wsUrl)
      .header("Authorization", "Device $deviceToken")
      .build()
    val socket = wsClient.newWebSocket(request, object : WebSocketListener() {
      override fun onOpen(webSocket: WebSocket, response: Response) {
        socketStatus = "已连接"
        socketLastEvent = "WebSocket 已连接"
      }

      override fun onMessage(webSocket: WebSocket, text: String) {
        scope.launch {
          runCatching {
            val message = PagePilotApi.parseWSMessage(text)
            handleWSMessage(api, deviceToken, serverUrl, message, webView)
          }.onFailure {
            socketLastEvent = it.message ?: "消息处理失败"
          }
        }
      }

      override fun onFailure(webSocket: WebSocket, t: Throwable, response: Response?) {
        socketStatus = "连接断开"
        val status = response?.code
        socketLastEvent = if (status != null) {
          "${t.message ?: "WebSocket 断开"} (HTTP $status)"
        } else {
          t.message ?: "WebSocket 断开"
        }
        if (!closed.isCompleted) closed.complete(status == 401 || status == 403)
      }

      override fun onClosed(webSocket: WebSocket, code: Int, reason: String) {
        socketStatus = "已断开"
        socketLastEvent = reason.ifBlank { "WebSocket 已关闭" }
        if (!closed.isCompleted) closed.complete(false)
      }
    })
    screenSocket = socket
    return closed.await()
  }

  private suspend fun handleWSMessage(
    api: PagePilotApi,
    deviceToken: String,
    serverUrl: String,
    message: ScreenWSMessage,
    webView: WebView,
  ) {
    socketLastEvent = message.type.ifBlank { "收到消息" }
    when (message.type) {
      "manifest" -> {
        val manifest = message.manifest ?: return
        currentManifest = manifest
        applyManifest(serverUrl, manifest, webView)
        handleScreenshotCommand(api, deviceToken, manifest, webView)
        handleScreenCommand(api, deviceToken, manifest, webView)
      }
      "screenshot" -> {
        val base = currentManifest ?: api.manifest(deviceToken).also { currentManifest = it }
        handleScreenshotCommand(api, deviceToken, base.copy(screenshot = message.screenshot), webView)
      }
      "command" -> {
        val base = currentManifest ?: api.manifest(deviceToken).also { currentManifest = it }
        handleScreenCommand(api, deviceToken, base.copy(command = message.command), webView)
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
    val captured = captureSystemScreen()
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
    heartbeatJob?.cancel()
    statusJob?.cancel()
    screenSocket?.cancel()
    screenSocket = null
    socketStatus = "未连接"
    lastWebSocketUrl = "-"
    networkDot = null
    socketDot = null
    networkStatusLabel = "未连接"
    socketStatusLabel = "未连接"
    statusOverlay = null
    currentWebView?.let {
      lastWebViewRuntime = webViewRuntimeLabel()
      lastX5Loaded = x5Loaded()
      lastX5Version = x5VersionCode()
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

  private fun statusHotspot(): View {
    val container = LinearLayout(this).apply {
      orientation = LinearLayout.HORIZONTAL
      gravity = Gravity.CENTER
      setPadding(dp(3), dp(1), dp(3), dp(1))
      background = roundedDrawable(0x66020617, 0x26ffffff)
      contentDescription = "网络与 WebSocket 状态"
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
    networkDot = statusDot()
    socketDot = statusDot()
    container.addView(networkDot, LinearLayout.LayoutParams(dp(5), dp(5)).apply {
      setMargins(dp(2), 0, dp(2), 0)
    })
    container.addView(socketDot, LinearLayout.LayoutParams(dp(5), dp(5)).apply {
      setMargins(dp(2), 0, dp(2), 0)
    })
    statusOverlay = container
    updateStatusIndicators()
    return container
  }

  private fun statusDot(): View {
    return View(this).apply {
      background = circleDrawable(0xff94a3b8.toInt())
    }
  }

  private fun startStatusIndicatorUpdates() {
    statusJob?.cancel()
    statusJob = scope.launch {
      while (isActive) {
        updateStatusIndicators()
        delay(1000)
      }
    }
  }

  private fun updateStatusIndicators() {
    val networkOnline = isNetworkOnline()
    val websocketOnline = socketStatus == "已连接"
    networkStatusLabel = if (networkOnline) "在线" else "离线"
    socketStatusLabel = socketStatus
    networkDot?.background = circleDrawable(if (networkOnline) 0xff22c55e.toInt() else 0xfffb7185.toInt())
    socketDot?.background = circleDrawable(when {
      websocketOnline -> 0xff22c55e.toInt()
      socketStatus == "连接中" || socketStatus == "重连等待" -> 0xfff59e0b.toInt()
      else -> 0xfffb7185.toInt()
    })
    statusOverlay?.contentDescription =
      "网络${if (networkOnline) "在线" else "离线"}，WebSocket $socketStatus。连续点击 5 次打开设置。"
  }

  private fun isNetworkOnline(): Boolean {
    val manager = getSystemService(Context.CONNECTIVITY_SERVICE) as? ConnectivityManager ?: return false
    return if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.M) {
      val network = manager.activeNetwork ?: return false
      val caps = manager.getNetworkCapabilities(network) ?: return false
      caps.hasCapability(NetworkCapabilities.NET_CAPABILITY_INTERNET)
    } else {
      @Suppress("DEPRECATION")
      manager.activeNetworkInfo?.isConnected == true
    }
  }

  private fun showSettings() {
    val settingsDialog = Dialog(this)
    settingsDialog.requestWindowFeature(Window.FEATURE_NO_TITLE)
    val local = config.load()
    val serverInput = EditText(this).apply {
      hint = "服务器地址"
      setSingleLine(true)
      textSize = 14f
      setText(wrapServerUrlForInput(local.serverUrl))
      setSelection(0)
    }
    val saveServer = Button(this).apply { text = "保存服务器并重新配对" }
    saveServer.setOnClickListener {
      val url = normalizeServerInput(serverInput.text.toString())
      if (!serverUrlAllowed(url)) {
        settingsDialog.dismiss()
        showServerConfig("服务器地址不合法")
        return@setOnClickListener
      }
      settingsDialog.dismiss()
      config.saveServerUrl(url)
      config.clearDeviceToken()
      route()
    }
    val unbind = Button(this).apply { text = "清除本机绑定" }
    unbind.setOnClickListener {
      settingsDialog.dismiss()
      config.clearDeviceToken()
      route()
    }
    val back = Button(this).apply { text = "返回播放" }
    back.setOnClickListener {
      settingsDialog.dismiss()
      keepFullscreen()
    }
    val content = ScrollView(this).apply {
      isFillViewport = true
      addView(centerPanel(
        title = "屏幕设置",
        subtitle = "右上角连续点击 5 次可打开本页。",
        children = listOf(
          serverInput,
          settingsSection("连接状态", listOf(
            "服务器" to local.serverUrl.ifBlank { "-" },
            "WS 地址" to lastWebSocketUrl,
            "网络" to networkStatusLabel,
            "WebSocket" to socketStatusLabel,
            "最近事件" to socketLastEvent,
            "心跳" to "每 15 秒上报一次，控制指令走 WebSocket 实时下发",
            "截图权限" to if (mediaProjection == null) "未授权，首次后台截图会在屏幕端弹出授权" else "已授权系统截屏",
          )),
          settingsSection("账号与屏幕", listOf(
            "屏幕" to screenTitle(),
            "用户" to ownerTitle(),
            "当前应用" to (currentManifest?.siteCode?.ifBlank { "-" } ?: "-"),
            "设备令牌" to if (local.deviceToken.isBlank()) "未绑定" else "已保存",
          )),
          settingsSection("显示环境", listOf(
            "分辨率" to "${resources.displayMetrics.widthPixels} x ${resources.displayMetrics.heightPixels} px",
            "方向" to orientationCN(),
            "密度" to "${resources.displayMetrics.densityDpi} dpi / ${resources.displayMetrics.density}",
            "文字缩放" to "WebView textZoom=100",
          )),
          settingsSection("WebView 内核", listOf(
            "控件" to "腾讯 X5 WebView",
            "运行内核" to if (currentWebView == null) lastWebViewRuntime else webViewRuntimeLabel(),
            "初始化" to X5RuntimeState.summary(),
            "X5 版本" to x5VersionCode().takeIf { it > 0 }?.toString().orEmpty().ifBlank { "未就绪或回退系统内核" },
            "实际加载" to x5ActualLoadText(),
            "CPU ABI" to supportedAbiText(),
          )),
          settingsSection("设备环境", listOf(
            "Android" to "${Build.VERSION.RELEASE} (SDK ${Build.VERSION.SDK_INT})",
            "设备" to "${Build.MANUFACTURER} ${Build.MODEL}",
            "品牌" to Build.BRAND,
            "CPU ABI" to supportedAbiText(),
            "系统语言" to Locale.getDefault().toLanguageTag(),
            "时区" to TimeZone.getDefault().id,
          )),
          settingsSection("应用信息", listOf(
            "版本" to BuildConfig.VERSION_NAME,
            "开发者" to "武硕：http://武硕.top",
          )),
          saveServer,
          unbind,
          back,
        ),
      ), FrameLayout.LayoutParams(
        ViewGroup.LayoutParams.MATCH_PARENT,
        ViewGroup.LayoutParams.WRAP_CONTENT,
      ))
    }
    settingsDialog.setContentView(content)
    settingsDialog.setOnDismissListener { keepFullscreen() }
    settingsDialog.show()
    settingsDialog.window?.setLayout(
      ViewGroup.LayoutParams.MATCH_PARENT,
      ViewGroup.LayoutParams.MATCH_PARENT,
    )
    settingsDialog.window?.decorView?.systemUiVisibility = window.decorView.systemUiVisibility
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

  private suspend fun captureSystemScreen(): CapturedScreenshot {
    val projection = ensureMediaProjection()
      ?: throw IllegalStateException("需要在屏幕端允许系统截屏权限")
    return withContext(Dispatchers.Main) {
      val metrics = resources.displayMetrics
      val width = metrics.widthPixels.coerceAtLeast(1)
      val height = metrics.heightPixels.coerceAtLeast(1)
      val reader = ImageReader.newInstance(width, height, PixelFormat.RGBA_8888, 2)
      val imageReady = CompletableDeferred<Unit>()
      val handler = Handler(Looper.getMainLooper())
      reader.setOnImageAvailableListener({
        if (!imageReady.isCompleted) imageReady.complete(Unit)
      }, handler)
      val display = projection.createVirtualDisplay(
        "PagePilotScreenCapture",
        width,
        height,
        metrics.densityDpi,
        DisplayManager.VIRTUAL_DISPLAY_FLAG_AUTO_MIRROR,
        reader.surface,
        null,
        handler,
      )
      try {
        withTimeoutOrNull(2500) { imageReady.await() }
          ?: throw IllegalStateException("系统截屏超时")
        val image = reader.acquireLatestImage()
          ?: throw IllegalStateException("系统截屏未返回图片")
        try {
          val plane = image.planes[0]
          val buffer = plane.buffer
          val pixelStride = plane.pixelStride
          val rowStride = plane.rowStride
          val rowPadding = rowStride - pixelStride * width
          val rawWidth = width + rowPadding / pixelStride
          val raw = Bitmap.createBitmap(rawWidth, height, Bitmap.Config.ARGB_8888)
          raw.copyPixelsFromBuffer(buffer)
          val cropped = if (rawWidth == width) {
            raw
          } else {
            Bitmap.createBitmap(raw, 0, 0, width, height).also { raw.recycle() }
          }
          compressScreenshotBitmap(cropped)
        } finally {
          image.close()
        }
      } finally {
        display.release()
        reader.close()
      }
    }
  }

  private suspend fun ensureMediaProjection(): MediaProjection? {
    mediaProjection?.let { return it }
    ScreenCaptureState.mediaProjection?.let {
      mediaProjection = it
      return it
    }
    val manager = mediaProjectionManager ?: return null
    pendingProjection?.let {
      it.await()
      pendingProjection = null
      return mediaProjection ?: ScreenCaptureState.mediaProjection?.also { stored ->
        mediaProjection = stored
      }
    }
    val deferred = CompletableDeferred<Boolean>()
    pendingProjection = deferred
    ScreenCaptureState.readyDeferred = deferred
    withContext(Dispatchers.Main) {
      runCatching {
        startActivityForResult(manager.createScreenCaptureIntent(), SCREEN_CAPTURE_REQUEST_CODE)
      }.onFailure {
        if (!deferred.isCompleted) deferred.complete(false)
        ScreenCaptureState.readyDeferred = null
      }
    }
    return if (deferred.await()) {
      pendingProjection = null
      mediaProjection = ScreenCaptureState.mediaProjection
      mediaProjection
    } else {
      pendingProjection = null
      null
    }
  }

  private suspend fun compressScreenshotBitmap(source: Bitmap): CapturedScreenshot = withContext(Dispatchers.Default) {
    val maxSide = 960f
    val scale = (maxSide / max(source.width, source.height).toFloat()).coerceAtMost(1f)
    val outWidth = (source.width * scale).roundToInt().coerceAtLeast(1)
    val outHeight = (source.height * scale).roundToInt().coerceAtLeast(1)
    val output = if (scale < 1f) {
      Bitmap.createScaledBitmap(source, outWidth, outHeight, true).also { source.recycle() }
    } else {
      source
    }
    val qualitySteps = listOf(62, 54, 46, 38, 32)
    var best = ByteArray(0)
    for (quality in qualitySteps) {
      val out = ByteArrayOutputStream()
      output.compress(Bitmap.CompressFormat.JPEG, quality, out)
      best = out.toByteArray()
      if (best.size <= 420_000) break
    }
    output.recycle()
    CapturedScreenshot(
      base64 = Base64.encodeToString(best, Base64.NO_WRAP),
      mimeType = "image/jpeg",
      width = outWidth,
      height = outHeight,
    )
  }

  private fun centerPanel(title: String, subtitle: String, children: List<View>, error: String = ""): View {
    return LinearLayout(this).apply {
      orientation = LinearLayout.VERTICAL
      gravity = Gravity.CENTER
      val horizontalPadding = panelHorizontalPadding()
      val verticalPadding = if (isCompactWidth()) dp(24) else dp(32)
      setPadding(horizontalPadding, verticalPadding, horizontalPadding, verticalPadding)
      setBackgroundColor(0xffeef7fb.toInt())
      addView(ImageView(context).apply {
        setImageResource(R.drawable.pagepilot_logo)
        contentDescription = "PagePilot"
      }, LinearLayout.LayoutParams(
        if (isCompactWidth()) dp(64) else dp(72),
        if (isCompactWidth()) dp(64) else dp(72),
      ).apply {
        gravity = Gravity.CENTER_HORIZONTAL
        setMargins(0, 0, 0, dp(12))
      })
      addView(TextView(context).apply {
        text = title
        textSize = if (isCompactWidth()) 28f else 32f
        gravity = Gravity.CENTER
        setTextColor(0xff07101f.toInt())
      }, panelParams())
      addView(TextView(context).apply {
        text = subtitle
        textSize = if (isCompactWidth()) 15f else 17f
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

  private fun settingsSection(title: String, rows: List<Pair<String, String>>): View {
    return LinearLayout(this).apply {
      orientation = LinearLayout.VERTICAL
      background = roundedDrawable(0xffffffff.toInt(), 0x1f0f766e)
      setPadding(dp(16), dp(14), dp(16), dp(14))
      addView(TextView(context).apply {
        text = title
        textSize = if (isCompactWidth()) 16f else 17f
        setTextColor(0xff0f172a.toInt())
        setTypeface(typeface, android.graphics.Typeface.BOLD)
      })
      rows.forEach { (label, value) ->
        addView(settingsRow(label, value))
      }
    }
  }

  private fun settingsRow(label: String, value: String): View {
    val compact = isCompactWidth()
    return LinearLayout(this).apply {
      orientation = if (compact) LinearLayout.VERTICAL else LinearLayout.HORIZONTAL
      gravity = if (compact) Gravity.LEFT else Gravity.CENTER_VERTICAL
      setPadding(0, dp(10), 0, 0)
      addView(TextView(context).apply {
        text = label
        textSize = if (compact) 13f else 14f
        setTextColor(0xff64748b.toInt())
      }, LinearLayout.LayoutParams(
        if (compact) ViewGroup.LayoutParams.MATCH_PARENT else dp(112),
        ViewGroup.LayoutParams.WRAP_CONTENT,
      ))
      addView(TextView(context).apply {
        text = value.ifBlank { "-" }
        textSize = 14f
        setTextColor(0xff1e293b.toInt())
        setLineSpacing(2f, 1.0f)
      }, LinearLayout.LayoutParams(
        if (compact) ViewGroup.LayoutParams.MATCH_PARENT else 0,
        ViewGroup.LayoutParams.WRAP_CONTENT,
        if (compact) 0f else 1f,
      ).apply {
        if (compact) setMargins(0, dp(4), 0, 0)
      })
    }
  }

  private fun roundedDrawable(fill: Int, stroke: Int): GradientDrawable {
    return GradientDrawable().apply {
      shape = GradientDrawable.RECTANGLE
      cornerRadius = dp(16).toFloat()
      setColor(fill)
      setStroke(dp(1), stroke)
    }
  }

  private fun circleDrawable(fill: Int): GradientDrawable {
    return GradientDrawable().apply {
      shape = GradientDrawable.OVAL
      setColor(fill)
    }
  }

  private fun panelParams(): LinearLayout.LayoutParams {
    return LinearLayout.LayoutParams(
      ViewGroup.LayoutParams.MATCH_PARENT,
      ViewGroup.LayoutParams.WRAP_CONTENT,
    ).apply {
      width = panelContentWidth()
      setMargins(0, dp(8), 0, dp(8))
    }
  }

  private fun panelHorizontalPadding(): Int {
    return if (isCompactWidth()) dp(20) else dp(32)
  }

  private fun panelContentWidth(): Int {
    val available = resources.displayMetrics.widthPixels - panelHorizontalPadding() * 2
    return available.coerceAtLeast(1).coerceAtMost(dp(760))
  }

  private fun isCompactWidth(): Boolean {
    return resources.configuration.screenWidthDp < 480
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
      .put("webViewRuntime", webViewRuntimeLabel())
      .put("webViewClass", currentWebView?.javaClass?.name ?: WebView::class.java.name)
      .put("x5Version", x5VersionCode())
      .put("x5Loaded", x5Loaded())
      .put("x5Init", X5RuntimeState.summary())
      .put("x5Diagnostic", x5ActualLoadText())
      .put("cpuAbi", supportedAbiText())
      .put("socketStatus", socketStatus)
      .put("appVersion", BuildConfig.VERSION_NAME)
  }

  private fun orientationLabel(): String {
    return if (resources.configuration.orientation == Configuration.ORIENTATION_LANDSCAPE) "landscape" else "portrait"
  }

  private fun orientationCN(): String {
    return if (resources.configuration.orientation == Configuration.ORIENTATION_LANDSCAPE) "横屏" else "竖屏"
  }

  private fun x5VersionCode(): Int {
    val version = QbSdk.getTbsVersion(this)
    if (version > 0) {
      lastX5Version = version
    }
    return version.takeIf { it > 0 } ?: lastX5Version
  }

  private fun x5Loaded(): Boolean {
    val webView = currentWebView ?: return lastX5Loaded
    val loaded = runCatching { webView.x5WebViewExtension != null }.getOrDefault(false)
    lastX5Loaded = loaded
    return loaded
  }

  private fun webViewRuntimeLabel(): String {
    val version = x5VersionCode()
    val label = when {
      x5Loaded() && version > 0 -> "X5 WebView $version"
      version > 0 -> "X5 已安装，等待加载 ($version)"
      isX86Abi() -> "系统 WebView 回退（当前主 ABI 为 ${primaryAbiText()}）"
      else -> "系统 WebView 回退（X5 未就绪）"
    }
    lastWebViewRuntime = label
    return label
  }

  private fun x5ActualLoadText(): String {
    if (x5Loaded()) return "X5 内核已加载"
    val callbackState = X5RuntimeState.viewInitFinished
    return when {
      isX86Abi() -> "系统 WebView 回退；当前主 ABI 为 ${primaryAbiText()}，X5 通常需要 ARM 真机环境"
      callbackState == false -> "X5 初始化回调未通过，已回退系统 WebView"
      x5VersionCode() <= 0 -> "X5 内核未就绪，已回退系统 WebView"
      else -> "系统 WebView 回退"
    }
  }

  private fun supportedAbiText(): String {
    return Build.SUPPORTED_ABIS.joinToString(", ").ifBlank { Build.CPU_ABI }
  }

  private fun primaryAbiText(): String {
    return Build.SUPPORTED_ABIS.firstOrNull() ?: Build.CPU_ABI
  }

  private fun isX86Abi(): Boolean {
    return primaryAbiText().contains("x86", ignoreCase = true)
  }

  private fun screenTitle(): String {
    val manifest = currentManifest
    return manifest?.screenName?.ifBlank { manifest.screenId } ?: "-"
  }

  private fun ownerTitle(): String {
    val manifest = currentManifest
    return manifest?.ownerUsername?.ifBlank { manifest.ownerUserId } ?: "-"
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

  private fun normalizeServerInput(value: String): String {
    return value.replace(Regex("\\s+"), "").trimEnd('/')
  }

  private fun wrapServerUrlForInput(value: String): String {
    return value  // 不插入换行，避免含端口的长 URL 在双行显示时丢失
  }

  private fun idleHtml(): String {
    return """<!doctype html><html><head><meta name="viewport" content="width=device-width,initial-scale=1"></head><body style="margin:0;height:100vh;display:grid;place-items:center;background:#082f49;color:#e0f2fe;font-family:sans-serif"><div style="text-align:center"><h1 style="font-size:42px;margin:0 0 12px">PagePilot Screen</h1><p style="font-size:20px;margin:0;color:#bae6fd">等待投放内容</p></div></body></html>"""
  }

  private fun standbyHtml(message: String): String {
    val safeMessage = escapeHtml(message)
    return """<!doctype html><html lang="zh-CN"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><style>
html,body{margin:0;width:100%;height:100%;background:#020617;color:#e2e8f0;font-family:Arial,"Microsoft YaHei",sans-serif;overflow:hidden}
body{display:table}
.wrap{display:table-cell;text-align:center;vertical-align:middle}
.time{font-size:13vw;font-weight:800;line-height:1;letter-spacing:0;text-shadow:0 10px 36px rgba(56,189,248,.18)}
.date{margin-top:24px;color:#94a3b8;font-size:3vw}
.state{margin-top:36px;color:#38bdf8;font-size:2.2vw;letter-spacing:0}
@media (max-width:700px){.time{font-size:17vw}.date{font-size:4vw}.state{font-size:3.2vw}}
@media (min-width:1500px){.time{font-size:168px}.date{font-size:36px}.state{font-size:26px}}
</style></head><body><main class="wrap"><div class="time" id="time">--:--:--</div><div class="date" id="date">----</div><div class="state">$safeMessage</div></main><script>
function pad(n){return n<10?"0"+n:String(n)}
function tick(){var d=new Date();var weeks=["星期日","星期一","星期二","星期三","星期四","星期五","星期六"];document.getElementById("time").innerHTML=pad(d.getHours())+":"+pad(d.getMinutes())+":"+pad(d.getSeconds());document.getElementById("date").innerHTML=d.getFullYear()+"年"+pad(d.getMonth()+1)+"月"+pad(d.getDate())+"日 "+weeks[d.getDay()]}
tick();setInterval(tick,1000)
</script></body></html>"""
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
