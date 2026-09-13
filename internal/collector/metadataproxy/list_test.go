package metadataproxy

import (
	"errors"
	"strings"
	"testing"

	"github.com/fuzzy-moose/tana/internal/collectorapi"
)

func TestParseProxyListNormalizesEntries(t *testing.T) {
	for _, protocol := range []string{"http", "https", "socks5"} {
		t.Run(protocol, func(t *testing.T) {
			entries, err := parseProxyList("\uFEFF# Public proxies\r\n\n 192.0.2.1:01080 \r\n192.0.2.1:1080\n[2001:db8::1]:1081\nPROXY.example.:080\n", protocol)
			if err != nil {
				t.Fatal(err)
			}
			want := []string{"192.0.2.1:1080", "192.0.2.1:1080", "[2001:db8::1]:1081", "proxy.example:80"}
			if len(entries) != len(want) {
				t.Fatalf("entries = %d, want %d", len(entries), len(want))
			}
			for i, entry := range entries {
				endpoint := protocol + "://" + want[i]
				if entry.ProxyURL != endpoint || entry.Name != endpoint || !entry.Enabled || entry.UserAgent != defaultUserAgent {
					t.Errorf("entry %d = %+v", i, entry)
				}
			}
		})
	}
}

func TestParseProxyListRejectsInvalidOrOversizedInput(t *testing.T) {
	for _, tt := range []struct {
		name, proxies, protocol string
		want                    error
	}{
		{name: "unsupported protocol", proxies: "192.0.2.1:1080", protocol: "socks4"},
		{name: "missing protocol", proxies: "192.0.2.1:1080"},
		{name: "empty", proxies: "# No entries\n\n", protocol: "socks5"},
		{name: "embedded scheme", proxies: "192.0.2.1:1080\nsocks5://192.0.2.2:1080", protocol: "socks5"},
		{name: "embedded credentials", proxies: "user@proxy.example:1080", protocol: "socks5"},
		{name: "missing port", proxies: "192.0.2.1", protocol: "http"},
		{name: "empty port", proxies: "192.0.2.1:", protocol: "http"},
		{name: "zero port", proxies: "192.0.2.1:0", protocol: "http"},
		{name: "invalid port", proxies: "192.0.2.1:65536", protocol: "http"},
		{name: "path suffix", proxies: "192.0.2.1:80/", protocol: "http"},
		{name: "invalid hostname", proxies: "<html>:80", protocol: "https"},
		{name: "too many bytes", proxies: strings.Repeat("#", maxProxyListBytes+1), protocol: "http", want: collectorapi.ErrProxyListTooLarge},
		{name: "too many entries", proxies: strings.Repeat("192.0.2.1:1080\n", maxProxyListEntries+1), protocol: "http", want: collectorapi.ErrProxyListTooLarge},
	} {
		t.Run(tt.name, func(t *testing.T) {
			want := tt.want
			if want == nil {
				want = collectorapi.ErrInvalidProxyList
			}
			entries, err := parseProxyList(tt.proxies, tt.protocol)
			if !errors.Is(err, want) || entries != nil {
				t.Fatalf("entries = %+v, error = %v, want %v", entries, err, want)
			}
		})
	}
}
