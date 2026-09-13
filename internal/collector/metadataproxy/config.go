// Package metadataproxy owns optional, independently paced metadata proxy channels.
package metadataproxy

import (
	"net"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"unicode"

	"github.com/fuzzy-moose/tana/internal/collectorapi"
)

const defaultUserAgent = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/128.0.0.0 Safari/537.36"

func normalize(input collectorapi.MetadataProxyInput) (collectorapi.MetadataProxyInput, error) {
	invalid := func() (collectorapi.MetadataProxyInput, error) { return input, collectorapi.ErrInvalidProxy }
	input.Name = strings.TrimSpace(input.Name)
	input.UserAgent = strings.TrimSpace(input.UserAgent)
	if input.UserAgent == "" {
		input.UserAgent = defaultUserAgent
	}
	if input.Name == "" || len(input.Name) > 120 || strings.IndexFunc(input.Name, unicode.IsControl) >= 0 ||
		len(input.UserAgent) > 1024 || strings.IndexFunc(input.UserAgent, func(r rune) bool { return r < 32 || r > 126 }) >= 0 {
		return invalid()
	}
	u, err := url.Parse(strings.TrimSpace(input.ProxyURL))
	if err != nil || u.Opaque != "" || u.Hostname() == "" || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return invalid()
	}
	u.Scheme = strings.ToLower(u.Scheme)
	if u.Scheme == "socks5h" {
		u.Scheme = "socks5"
	}
	defaultPort := map[string]string{"http": "80", "https": "443", "socks5": "1080"}[u.Scheme]
	if defaultPort == "" {
		return invalid()
	}
	if u.User != nil {
		if input.Username != "" || input.Password != nil {
			return invalid()
		}
		input.Username = u.User.Username()
		password, _ := u.User.Password()
		input.Password = &password
	}
	if len(input.Username) > 255 || (input.Password != nil && len(*input.Password) > 255) || strings.IndexFunc(input.Username, unicode.IsControl) >= 0 {
		return invalid()
	}
	host := strings.TrimSuffix(strings.ToLower(u.Hostname()), ".")
	if address, err := netip.ParseAddr(host); err == nil {
		host = address.String()
	}
	if host == "" || strings.ContainsAny(host, " /\\") {
		return invalid()
	}
	port := u.Port()
	if port == "" {
		port = defaultPort
	}
	n, err := strconv.Atoi(port)
	if err != nil || n < 1 || n > 65535 {
		return invalid()
	}
	input.ProxyURL = (&url.URL{Scheme: u.Scheme, Host: net.JoinHostPort(host, strconv.Itoa(n))}).String()
	return input, nil
}
