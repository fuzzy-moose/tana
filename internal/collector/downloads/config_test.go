package downloads

import "testing"

func TestStorageConfig(t *testing.T) {
	for _, tc := range []struct {
		pause, resume string
		want          StorageConfig
		invalid       bool
	}{
		{"", "", StorageConfig{5 << 30, 10 << 30}, false},
		{"1024", "2048", StorageConfig{1024, 2048}, false},
		{"-1", "2048", StorageConfig{}, true},
		{"0", "2048", StorageConfig{}, true},
		{"bad", "2048", StorageConfig{}, true},
		{"2048", "2048", StorageConfig{}, true},
		{"2048", "1024", StorageConfig{}, true},
		{"1", "9223372036854775808", StorageConfig{}, true},
	} {
		t.Run(tc.pause+"/"+tc.resume, func(t *testing.T) {
			cfg, err := LoadStorageConfig(func(key string) string {
				if key == "TANA_COLLECTOR_DOWNLOAD_PAUSE_BELOW_BYTES" {
					return tc.pause
				}
				return tc.resume
			})
			if (err != nil) != tc.invalid || !tc.invalid && cfg != tc.want {
				t.Fatalf("config: %+v, %v", cfg, err)
			}
		})
	}
}
