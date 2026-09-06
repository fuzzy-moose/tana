package enrichment

import "testing"

func TestOptionalCollectorConfiguration(t *testing.T) {
	for _, tt := range []struct {
		url, token       string
		enabled, invalid bool
	}{
		{"", "", false, false},
		{"", "collector-server-token", false, false},
		{"http://localhost:8081/", "secret", true, false},
		{"https://collector.test", "", false, true},
		{"relative", "secret", false, true},
		{"https://user:password@collector.test", "secret", false, true},
		{"https://collector.test", "bad token", false, true},
	} {
		env := map[string]string{"TANA_COLLECTOR_URL": tt.url, "TANA_COLLECTOR_API_TOKEN": tt.token}
		client, err := ConfiguredClient(func(key string) string { return env[key] })
		if (err != nil) != tt.invalid || (client != nil) != tt.enabled {
			t.Errorf("URL=%q: enabled=%t error=%v", tt.url, client != nil, err)
		}
	}
}
