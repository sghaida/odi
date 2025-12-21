package config

import (
	"fmt"
	"os"
	"strconv"
)

type Config struct {
	Env       string
	LogPrefix string
	TimeoutMs int
}

// LoadFromEnv is intentionally simple for examples.
// You can expand this later (flags, files, etc.).
func LoadFromEnv() (Config, error) {
	cfg := Config{
		Env:       getenv("ODI_ENV", "local"),
		LogPrefix: getenv("ODI_LOG_PREFIX", "v4"),
		TimeoutMs: getenvInt("ODI_TIMEOUT_MS", 10_000),
	}
	if cfg.TimeoutMs <= 0 {
		return Config{}, fmt.Errorf("ODI_TIMEOUT_MS must be > 0")
	}
	return cfg, nil
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func getenvInt(k string, def int) int {
	v := os.Getenv(k)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}
