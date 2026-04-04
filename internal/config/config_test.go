package config

import "testing"

func TestNormalizeScannerConfigDefaults(t *testing.T) {
	cfg, warnings := NormalizeScannerConfig(ScannerConfig{})

	if len(warnings) != 0 {
		t.Fatalf("expected no warnings for defaulted config, got %v", warnings)
	}
	if cfg != DefaultScannerConfig() {
		t.Fatalf("expected normalized config to match defaults, got %+v", cfg)
	}
	if cfg.SessionMemory.MaxUpdateBytes != 32*1024 {
		t.Fatalf("expected session memory max update bytes to default to 32768, got %d", cfg.SessionMemory.MaxUpdateBytes)
	}
	if cfg.SessionMemory.RequestTimeoutSeconds != 180 {
		t.Fatalf("expected session memory request timeout to default to 180, got %d", cfg.SessionMemory.RequestTimeoutSeconds)
	}
	if cfg.SessionMemory.MaxRetries != 3 {
		t.Fatalf("expected session memory max retries to default to 3, got %d", cfg.SessionMemory.MaxRetries)
	}
	if cfg.SessionMemory.RetryBackoffSeconds != 2 {
		t.Fatalf("expected session memory retry backoff to default to 2, got %d", cfg.SessionMemory.RetryBackoffSeconds)
	}
	if cfg.SessionMemory.FailureCooldownSeconds != 300 {
		t.Fatalf("expected session memory failure cooldown to default to 300, got %d", cfg.SessionMemory.FailureCooldownSeconds)
	}
}

func TestNormalizeScannerConfigRejectsHardLimitBelowSoftLimit(t *testing.T) {
	cfg, warnings := NormalizeScannerConfig(ScannerConfig{
		ContextSoftLimitBytes:        1000,
		ContextHardLimitBytes:        900,
		ContextSummaryWindowMessages: 6,
	})

	if len(warnings) == 0 {
		t.Fatal("expected warning for invalid hard limit")
	}
	if cfg.ContextHardLimitBytes <= cfg.ContextSoftLimitBytes {
		t.Fatalf("expected hard limit to be greater than soft limit, got soft=%d hard=%d", cfg.ContextSoftLimitBytes, cfg.ContextHardLimitBytes)
	}
	if cfg.ContextSummaryWindowMessages != 6 {
		t.Fatalf("expected valid summary window to be preserved, got %d", cfg.ContextSummaryWindowMessages)
	}
	if cfg.ContextCompression.HardLimitBytes <= cfg.ContextCompression.SoftLimitBytes {
		t.Fatalf("expected nested hard limit to be greater than soft limit, got soft=%d hard=%d", cfg.ContextCompression.SoftLimitBytes, cfg.ContextCompression.HardLimitBytes)
	}
}

func TestNormalizeScannerConfigMapsLegacyFieldsToNestedCompressionConfig(t *testing.T) {
	cfg, warnings := NormalizeScannerConfig(ScannerConfig{
		ContextSoftLimitBytes:        1234,
		ContextHardLimitBytes:        5678,
		ContextSummaryWindowMessages: 9,
	})

	if len(warnings) != 0 {
		t.Fatalf("expected no warnings, got %v", warnings)
	}
	if cfg.ContextCompression.SoftLimitBytes != 1234 {
		t.Fatalf("expected nested soft limit 1234, got %d", cfg.ContextCompression.SoftLimitBytes)
	}
	if cfg.ContextCompression.HardLimitBytes != 5678 {
		t.Fatalf("expected nested hard limit 5678, got %d", cfg.ContextCompression.HardLimitBytes)
	}
	if cfg.ContextCompression.SummaryWindowMessages != 9 {
		t.Fatalf("expected nested summary window 9, got %d", cfg.ContextCompression.SummaryWindowMessages)
	}
	if !cfg.SessionMemory.Enabled {
		t.Fatal("expected session memory to default to enabled")
	}
}
