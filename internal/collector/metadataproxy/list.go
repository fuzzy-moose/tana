package metadataproxy

import (
	"net"
	"net/netip"
	"strings"

	"github.com/fuzzy-moose/tana/internal/collectorapi"
)

const (
	maxProxyListBytes   = 1 << 20
	maxProxyListEntries = 10000
)

func parseProxyList(proxies, protocol string) ([]collectorapi.MetadataProxyInput, error) {
	if len(proxies) > maxProxyListBytes {
		return nil, collectorapi.ErrProxyListTooLarge
	}
	if protocol != "http" && protocol != "https" && protocol != "socks5" {
		return nil, collectorapi.ErrInvalidProxyList
	}
	proxies = strings.TrimPrefix(strings.TrimSpace(proxies), "\uFEFF")
	var entries []collectorapi.MetadataProxyInput
	for line := range strings.SplitSeq(proxies, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if len(entries) == maxProxyListEntries {
			return nil, collectorapi.ErrProxyListTooLarge
		}
		host, port, err := net.SplitHostPort(line)
		if err != nil || !validProxyListHost(host) || port == "" {
			return nil, collectorapi.ErrInvalidProxyList
		}
		for _, c := range port {
			if c < '0' || c > '9' {
				return nil, collectorapi.ErrInvalidProxyList
			}
		}
		entry, err := normalize(collectorapi.MetadataProxyInput{Name: "Imported proxy", ProxyURL: protocol + "://" + line, Enabled: true})
		if err != nil {
			return nil, collectorapi.ErrInvalidProxyList
		}
		if len(entry.ProxyURL) <= 120 {
			entry.Name = entry.ProxyURL
		}
		entries = append(entries, entry)
	}
	if len(entries) == 0 {
		return nil, collectorapi.ErrInvalidProxyList
	}
	return entries, nil
}

func validProxyListHost(host string) bool {
	if _, err := netip.ParseAddr(host); err == nil {
		return true
	}
	host = strings.TrimSuffix(host, ".")
	if host == "" || len(host) > 253 {
		return false
	}
	for label := range strings.SplitSeq(host, ".") {
		if label == "" || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, c := range label {
			if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-') {
				return false
			}
		}
	}
	return true
}
