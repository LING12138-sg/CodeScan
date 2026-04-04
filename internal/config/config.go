package config

const ProjectsDir = "projects"

type DBConfig struct {
	Host     string `json:"host"`
	Port     int    `json:"port"`
	User     string `json:"user"`
	Password string `json:"password"`
	DBName   string `json:"dbname"`
}

type AIConfig struct {
	APIKey  string `json:"api_key"`
	BaseURL string `json:"base_url"`
	Model   string `json:"model"`
}

type ContextCompressionConfig struct {
	Enabled                bool `json:"enabled"`
	SoftLimitBytes         int  `json:"soft_limit_bytes"`
	HardLimitBytes         int  `json:"hard_limit_bytes"`
	SummaryWindowMessages  int  `json:"summary_window_messages"`
	MicrocompactKeepRecent int  `json:"microcompact_keep_recent_rounds"`
	ArtifactMaxBytes       int  `json:"artifact_max_bytes"`
	CompactMinTailMessages int  `json:"compact_min_tail_messages"`
	SessionMemoryEnabled   bool `json:"session_memory_enabled"`
}

type SessionMemoryConfig struct {
	Enabled                bool `json:"enabled"`
	MinGrowthBytes         int  `json:"min_growth_bytes"`
	MinToolCalls           int  `json:"min_tool_calls"`
	MaxUpdateBytes         int  `json:"max_update_bytes"`
	RequestTimeoutSeconds  int  `json:"request_timeout_seconds"`
	MaxRetries             int  `json:"max_retries"`
	RetryBackoffSeconds    int  `json:"retry_backoff_seconds"`
	FailureCooldownSeconds int  `json:"failure_cooldown_seconds"`
}

type ScannerConfig struct {
	ContextSoftLimitBytes        int                      `json:"context_soft_limit_bytes"`
	ContextHardLimitBytes        int                      `json:"context_hard_limit_bytes"`
	ContextSummaryWindowMessages int                      `json:"context_summary_window_messages"`
	ContextCompression           ContextCompressionConfig `json:"context_compression"`
	SessionMemory                SessionMemoryConfig      `json:"session_memory"`
}

type Config struct {
	AuthKey       string        `json:"auth_key"`
	DBConfig      DBConfig      `json:"db_config"`
	AIConfig      AIConfig      `json:"ai_config"`
	ScannerConfig ScannerConfig `json:"scanner_config"`
}

func DefaultScannerConfig() ScannerConfig {
	return ScannerConfig{
		ContextSoftLimitBytes:        90000,
		ContextHardLimitBytes:        140000,
		ContextSummaryWindowMessages: 12,
		ContextCompression: ContextCompressionConfig{
			Enabled:                true,
			SoftLimitBytes:         90000,
			HardLimitBytes:         140000,
			SummaryWindowMessages:  12,
			MicrocompactKeepRecent: 2,
			ArtifactMaxBytes:       64 * 1024,
			CompactMinTailMessages: 4,
			SessionMemoryEnabled:   true,
		},
		SessionMemory: SessionMemoryConfig{
			Enabled:                true,
			MinGrowthBytes:         24 * 1024,
			MinToolCalls:           4,
			MaxUpdateBytes:         32 * 1024,
			RequestTimeoutSeconds:  180,
			MaxRetries:             3,
			RetryBackoffSeconds:    2,
			FailureCooldownSeconds: 300,
		},
	}
}

func NormalizeScannerConfig(cfg ScannerConfig) (ScannerConfig, []string) {
	defaults := DefaultScannerConfig()
	warnings := []string{}
	originalContextCompression := cfg.ContextCompression
	originalSessionMemory := cfg.SessionMemory

	if cfg.ContextSoftLimitBytes <= 0 {
		cfg.ContextSoftLimitBytes = defaults.ContextSoftLimitBytes
	}
	if cfg.ContextHardLimitBytes <= 0 {
		cfg.ContextHardLimitBytes = defaults.ContextHardLimitBytes
	}
	if cfg.ContextSummaryWindowMessages <= 0 {
		cfg.ContextSummaryWindowMessages = defaults.ContextSummaryWindowMessages
	}
	if cfg.ContextHardLimitBytes <= cfg.ContextSoftLimitBytes {
		cfg.ContextHardLimitBytes = defaults.ContextHardLimitBytes
		if cfg.ContextHardLimitBytes <= cfg.ContextSoftLimitBytes {
			cfg.ContextHardLimitBytes = cfg.ContextSoftLimitBytes + 1
		}
		warnings = append(warnings, "scanner_config.context_hard_limit_bytes must be greater than context_soft_limit_bytes; falling back to a safe hard limit")
	}

	if !cfg.ContextCompression.Enabled && originalContextCompression == (ContextCompressionConfig{}) {
		cfg.ContextCompression.Enabled = defaults.ContextCompression.Enabled
	}
	if cfg.ContextCompression.SoftLimitBytes <= 0 {
		if cfg.ContextSoftLimitBytes > 0 {
			cfg.ContextCompression.SoftLimitBytes = cfg.ContextSoftLimitBytes
		} else {
			cfg.ContextCompression.SoftLimitBytes = defaults.ContextCompression.SoftLimitBytes
		}
	}
	if cfg.ContextCompression.HardLimitBytes <= 0 {
		if cfg.ContextHardLimitBytes > 0 {
			cfg.ContextCompression.HardLimitBytes = cfg.ContextHardLimitBytes
		} else {
			cfg.ContextCompression.HardLimitBytes = defaults.ContextCompression.HardLimitBytes
		}
	}
	if cfg.ContextCompression.SummaryWindowMessages <= 0 {
		if cfg.ContextSummaryWindowMessages > 0 {
			cfg.ContextCompression.SummaryWindowMessages = cfg.ContextSummaryWindowMessages
		} else {
			cfg.ContextCompression.SummaryWindowMessages = defaults.ContextCompression.SummaryWindowMessages
		}
	}
	if cfg.ContextCompression.HardLimitBytes <= cfg.ContextCompression.SoftLimitBytes {
		cfg.ContextCompression.HardLimitBytes = defaults.ContextCompression.HardLimitBytes
		if cfg.ContextCompression.HardLimitBytes <= cfg.ContextCompression.SoftLimitBytes {
			cfg.ContextCompression.HardLimitBytes = cfg.ContextCompression.SoftLimitBytes + 1
		}
		warnings = append(warnings, "scanner_config.context_compression.hard_limit_bytes must be greater than soft_limit_bytes; falling back to a safe hard limit")
	}
	if cfg.ContextCompression.MicrocompactKeepRecent <= 0 {
		cfg.ContextCompression.MicrocompactKeepRecent = defaults.ContextCompression.MicrocompactKeepRecent
	}
	if cfg.ContextCompression.ArtifactMaxBytes <= 0 {
		cfg.ContextCompression.ArtifactMaxBytes = defaults.ContextCompression.ArtifactMaxBytes
	}
	if cfg.ContextCompression.CompactMinTailMessages <= 0 {
		cfg.ContextCompression.CompactMinTailMessages = defaults.ContextCompression.CompactMinTailMessages
	}
	if !cfg.ContextCompression.SessionMemoryEnabled && originalContextCompression == (ContextCompressionConfig{}) {
		cfg.ContextCompression.SessionMemoryEnabled = defaults.ContextCompression.SessionMemoryEnabled
	}

	if !cfg.SessionMemory.Enabled && originalSessionMemory == (SessionMemoryConfig{}) {
		cfg.SessionMemory.Enabled = defaults.SessionMemory.Enabled
	}
	if cfg.SessionMemory.MinGrowthBytes <= 0 {
		cfg.SessionMemory.MinGrowthBytes = defaults.SessionMemory.MinGrowthBytes
	}
	if cfg.SessionMemory.MinToolCalls <= 0 {
		cfg.SessionMemory.MinToolCalls = defaults.SessionMemory.MinToolCalls
	}
	if cfg.SessionMemory.MaxUpdateBytes <= 0 {
		cfg.SessionMemory.MaxUpdateBytes = defaults.SessionMemory.MaxUpdateBytes
	}
	if cfg.SessionMemory.RequestTimeoutSeconds <= 0 {
		cfg.SessionMemory.RequestTimeoutSeconds = defaults.SessionMemory.RequestTimeoutSeconds
	}
	if cfg.SessionMemory.MaxRetries <= 0 {
		cfg.SessionMemory.MaxRetries = defaults.SessionMemory.MaxRetries
	}
	if cfg.SessionMemory.RetryBackoffSeconds <= 0 {
		cfg.SessionMemory.RetryBackoffSeconds = defaults.SessionMemory.RetryBackoffSeconds
	}
	if cfg.SessionMemory.FailureCooldownSeconds <= 0 {
		cfg.SessionMemory.FailureCooldownSeconds = defaults.SessionMemory.FailureCooldownSeconds
	}

	// Keep deprecated flat fields in sync after normalization.
	cfg.ContextSoftLimitBytes = cfg.ContextCompression.SoftLimitBytes
	cfg.ContextHardLimitBytes = cfg.ContextCompression.HardLimitBytes
	cfg.ContextSummaryWindowMessages = cfg.ContextCompression.SummaryWindowMessages

	return cfg, warnings
}

// Global AI config accessible by scanner
var AI AIConfig

// Global scanner config accessible by scanner
var Scanner ScannerConfig
