package config

import (
	"testing"
)

func TestLoadConfig_MissingFile(t *testing.T) {
	// Test loading a non-existent file should return default config without error
	cfg, err := LoadConfig("non_existent_file.yaml")
	if err != nil {
		t.Fatalf("Expected no error for missing file, got: %v", err)
	}

	if cfg.Port != DefaultConfig.Port {
		t.Errorf("Expected default port %s, got %s", DefaultConfig.Port, cfg.Port)
	}
}

func TestGetConfig_Singleton(t *testing.T) {
	cfg1 := GetConfig()
	cfg2 := GetConfig()

	if cfg1.Port != cfg2.Port {
		t.Error("GetConfig should return consistent config")
	}
}
