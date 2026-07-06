package config

import "testing"

func TestDefaultReadsRegistrationAndStorageEnv(t *testing.T) {
	t.Setenv("HOSTCTL_ALLOW_REGISTRATION", "false")
	t.Setenv("HOSTCTL_STORAGE_BACKEND", "oss")
	t.Setenv("HOSTCTL_OSS_ENDPOINT", "https://oss-cn-hangzhou.aliyuncs.com")
	t.Setenv("HOSTCTL_OSS_BUCKET", "pagepilot-assets")
	t.Setenv("HOSTCTL_OSS_ACCESS_KEY_ID", "test-access-key-id")
	t.Setenv("HOSTCTL_OSS_ACCESS_KEY_SECRET", "test-access-key-secret")
	t.Setenv("HOSTCTL_OSS_PREFIX", "prod/pagepilot")
	t.Setenv("HOSTCTL_OSS_PUBLIC_BASE_URL", "https://cdn.example.com/pagepilot")

	cfg := Default()

	if cfg.AllowRegistration {
		t.Fatalf("AllowRegistration = true; want false")
	}
	if cfg.StorageBackend != "oss" {
		t.Fatalf("StorageBackend = %q; want oss", cfg.StorageBackend)
	}
	if cfg.OSSEndpoint != "https://oss-cn-hangzhou.aliyuncs.com" || cfg.OSSBucket != "pagepilot-assets" {
		t.Fatalf("OSS endpoint/bucket not read correctly: endpoint=%q bucket=%q", cfg.OSSEndpoint, cfg.OSSBucket)
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestEmailVerificationRequiresSMTPConfig(t *testing.T) {
	t.Setenv("HOSTCTL_EMAIL_VERIFICATION_ENABLED", "true")

	cfg := Default()

	if !cfg.EmailVerificationEnabled {
		t.Fatalf("EmailVerificationEnabled = false; want true")
	}
	if err := cfg.Validate(); err == nil {
		t.Fatalf("Validate() error = nil; want SMTP config error")
	}
}
