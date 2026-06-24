plugins {
  id("com.android.application")
  id("org.jetbrains.kotlin.android")
}

android {
  namespace = "cc.pagepilot.screen"
  compileSdk = 35

  defaultConfig {
    applicationId = "cc.pagepilot.screen"
    minSdk = 23
    targetSdk = 35
    versionCode = 1
    versionName = "0.1.0"
  }
}

dependencies {
  implementation("com.tencent.tbs:tbssdk:44286")
  implementation("org.jetbrains.kotlinx:kotlinx-coroutines-android:1.9.0")
}
