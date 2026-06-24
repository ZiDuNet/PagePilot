package cc.pagepilot.screen

import android.app.Activity
import android.os.Bundle
import android.provider.Settings
import android.view.Gravity
import android.view.View
import android.view.ViewGroup
import android.widget.Button
import android.widget.EditText
import android.widget.LinearLayout
import android.widget.TextView
import com.tencent.smtt.sdk.WebSettings
import com.tencent.smtt.sdk.WebView
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext

class MainActivity : Activity() {
  private val scope = CoroutineScope(Dispatchers.Main)
  private lateinit var config: ScreenConfig
  private var pairingJob: Job? = null
  private var playbackJob: Job? = null
  private var currentEntryUrl = ""

  override fun onCreate(savedInstanceState: Bundle?) {
    super.onCreate(savedInstanceState)
    config = ScreenConfig(this)
    keepFullscreen()
    route()
  }

  override fun onDestroy() {
    pairingJob?.cancel()
    playbackJob?.cancel()
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
    pairingJob?.cancel()
    playbackJob?.cancel()
    val input = EditText(this).apply {
      hint = "https://pagepilot.example.com"
      setSingleLine(true)
      textSize = 20f
    }
    val save = Button(this).apply { text = "保存服务器地址" }
    save.setOnClickListener {
      val url = input.text.toString().trim().trimEnd('/')
      if (!serverUrlAllowed(url)) {
        showServerConfig("生产服务器必须使用 HTTPS；本地调试可使用 localhost、10.0.2.2 或局域网 HTTP。")
        return@setOnClickListener
      }
      config.saveServerUrl(url)
      route()
    }
    setContentView(centerPanel(
      title = "PagePilot Screen",
      subtitle = "输入 PagePilot 服务器地址后开始配对",
      children = listOf(input, save),
      error = error,
    ))
  }

  private fun showPairing(serverUrl: String) {
    playbackJob?.cancel()
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
    val changeServer = Button(this).apply { text = "更换服务器地址" }
    changeServer.setOnClickListener {
      config.saveServerUrl("")
      config.clearDeviceToken()
      showServerConfig()
    }
    setContentView(centerPanel(
      title = "绑定屏幕",
      subtitle = "在 PagePilot 后台或 Skill 中输入下方配对码",
      children = listOf(title, code, changeServer),
    ))
    pairingJob?.cancel()
    pairingJob = scope.launch {
      try {
        val api = PagePilotApi(serverUrl)
        val session = api.startPairing(deviceName())
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
    val webView = WebView(this)
    configureWebView(webView)
    setContentView(webView, ViewGroup.LayoutParams(
      ViewGroup.LayoutParams.MATCH_PARENT,
      ViewGroup.LayoutParams.MATCH_PARENT,
    ))
    playbackJob?.cancel()
    playbackJob = scope.launch {
      val api = PagePilotApi(serverUrl)
      while (true) {
        try {
          api.heartbeat(deviceToken)
          val manifest = api.manifest(deviceToken)
          if (manifest.mode == "webapp" && manifest.entryUrl.isNotBlank() && manifest.entryUrl != currentEntryUrl) {
            currentEntryUrl = manifest.entryUrl
            webView.loadUrl(manifest.entryUrl)
          } else if (manifest.mode == "idle" && currentEntryUrl.isBlank()) {
            withContext(Dispatchers.Main) {
              webView.loadDataWithBaseURL(serverUrl, idleHtml(), "text/html", "UTF-8", null)
            }
          }
        } catch (err: Exception) {
          if (currentEntryUrl.isBlank()) {
            webView.loadDataWithBaseURL(serverUrl, errorHtml(err.message ?: "连接失败"), "text/html", "UTF-8", null)
          }
        }
        delay(15000)
      }
    }
  }

  private fun configureWebView(webView: WebView) {
    webView.settings.javaScriptEnabled = true
    webView.settings.domStorageEnabled = true
    webView.settings.databaseEnabled = true
    webView.settings.cacheMode = WebSettings.LOAD_DEFAULT
    webView.settings.mediaPlaybackRequiresUserGesture = false
  }

  private fun centerPanel(title: String, subtitle: String, children: List<View>, error: String = ""): View {
    return LinearLayout(this).apply {
      orientation = LinearLayout.VERTICAL
      gravity = Gravity.CENTER
      setPadding(48, 48, 48, 48)
      setBackgroundColor(0xfff3f6fa.toInt())
      addView(TextView(context).apply {
        text = title
        textSize = 34f
        gravity = Gravity.CENTER
      }, panelParams())
      addView(TextView(context).apply {
        text = subtitle
        textSize = 18f
        gravity = Gravity.CENTER
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
      width = resources.displayMetrics.widthPixels.coerceAtMost(720)
      setMargins(0, 10, 0, 10)
    }
  }

  private fun deviceName(): String {
    val androidId = Settings.Secure.getString(contentResolver, Settings.Secure.ANDROID_ID)
    return "Screen-${androidId.takeLast(6)}"
  }

  private fun serverUrlAllowed(url: String): Boolean {
    if (url.startsWith("https://")) return true
    if (!url.startsWith("http://")) return false
    val host = url.removePrefix("http://").substringBefore('/').substringBefore(':')
    return host == "127.0.0.1" ||
      host == "localhost" ||
      host == "10.0.2.2" ||
      host.startsWith("192.168.") ||
      host.startsWith("10.")
  }

  private fun idleHtml(): String {
    return """<!doctype html><html><body style="margin:0;height:100vh;display:grid;place-items:center;background:#0f172a;color:#e2e8f0;font-family:sans-serif"><div><h1>PagePilot Screen</h1><p>等待投放内容</p></div></body></html>"""
  }

  private fun errorHtml(message: String): String {
    return """<!doctype html><html><body style="margin:0;height:100vh;display:grid;place-items:center;background:#111827;color:#fecaca;font-family:sans-serif"><div><h1>连接失败</h1><p>${message.replace("<", "&lt;")}</p></div></body></html>"""
  }
}
