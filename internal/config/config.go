package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Config is the top-level configuration.
type Config struct {
	Garmin    GarminConfig  `yaml:"garmin"`
	CharLimit int           `yaml:"char_limit"`
	Decoder   DecoderConfig `yaml:"decoder"`
	APIKeys   APIKeysConfig `yaml:"api_keys"`
	Log       LogConfig     `yaml:"log"`
}

// DecoderConfig configures the embedded decoder web UI.
type DecoderConfig struct {
	Enabled bool   `yaml:"enabled"`
	Listen  string `yaml:"listen"`
}

// GarminConfig holds Garmin Hermes connection settings.
type GarminConfig struct {
	Phone      string `yaml:"phone"`
	SessionDir string `yaml:"session_dir"`
}

// APIKeysConfig holds external API keys.
type APIKeysConfig struct {
	OpenAI           string `yaml:"openai"`
	OpenAIModel      string `yaml:"openai_model"`
	OpenAIPrompt     string `yaml:"openai_prompt"`
	TimezoneDB       string `yaml:"timezonedb"`
	OpenRouteService string `yaml:"openrouteservice"`
}

// LogConfig configures logging.
type LogConfig struct {
	Level  string `yaml:"level"`
	Pretty bool   `yaml:"pretty"`
}

// DefaultConfig returns a config with sensible defaults.
func DefaultConfig() *Config {
	return &Config{
		Garmin: GarminConfig{
			SessionDir: "./sessions",
		},
		CharLimit: 1600,
		Decoder: DecoderConfig{
			Enabled: true,
			Listen:  ":8080",
		},
		APIKeys: APIKeysConfig{
			OpenAIModel:  "o3-mini",
			OpenAIPrompt: "You are a concise assistant. Answer in max {{.CharLimit}} characters. Build on previous conversation context when available.",
		},
		Log: LogConfig{
			Level:  "info",
			Pretty: true,
		},
	}
}

// Load reads a YAML config file and merges it with defaults.
func Load(path string) (*Config, error) {
	cfg := DefaultConfig()

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config %s: %w", path, err)
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parsing config %s: %w", path, err)
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	return cfg, nil
}

// Validate checks required fields.
func (c *Config) Validate() error {
	if c.Garmin.Phone == "" {
		return fmt.Errorf("garmin.phone is required")
	}
	if c.CharLimit <= 0 {
		return fmt.Errorf("char_limit must be positive")
	}
	return nil
}

// DetailedCharLimit returns the char limit adjusted for encoded commands.
// For old devices (160), detailed weather/avalanche need ~15 chars less.
// For new devices (1600+), use full limit.
func (c *Config) DetailedCharLimit() int {
	if c.CharLimit <= 160 {
		return c.CharLimit - 15
	}
	return c.CharLimit
}

// ChatGPTCharLimit returns the char limit for ChatGPT responses.
// For old devices (160), ChatGPT gets ~5 chars less.
// For new devices (1600+), use full limit.
func (c *Config) ChatGPTCharLimit() int {
	if c.CharLimit <= 160 {
		return c.CharLimit - 5
	}
	return c.CharLimit
}
