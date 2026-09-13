package downloads

import (
	"fmt"
	"strconv"
	"strings"
)

type StorageConfig struct {
	PauseBelowBytes int64
	ResumeAtBytes   int64
}

func LoadStorageConfig(getenv func(string) string) (StorageConfig, error) {
	cfg := StorageConfig{PauseBelowBytes: 5 << 30, ResumeAtBytes: 10 << 30}
	for _, setting := range []struct {
		key   string
		value *int64
	}{
		{"TANA_COLLECTOR_DOWNLOAD_PAUSE_BELOW_BYTES", &cfg.PauseBelowBytes},
		{"TANA_COLLECTOR_DOWNLOAD_RESUME_AT_BYTES", &cfg.ResumeAtBytes},
	} {
		if raw := strings.TrimSpace(getenv(setting.key)); raw != "" {
			value, err := strconv.ParseInt(raw, 10, 64)
			if err != nil || value <= 0 {
				return StorageConfig{}, fmt.Errorf("%s must be a positive byte count", setting.key)
			}
			*setting.value = value
		}
	}
	return cfg, cfg.validate()
}

func (cfg StorageConfig) validate() error {
	if cfg.PauseBelowBytes <= 0 || cfg.ResumeAtBytes <= cfg.PauseBelowBytes {
		return fmt.Errorf("TANA_COLLECTOR_DOWNLOAD_RESUME_AT_BYTES must exceed positive TANA_COLLECTOR_DOWNLOAD_PAUSE_BELOW_BYTES")
	}
	return nil
}
