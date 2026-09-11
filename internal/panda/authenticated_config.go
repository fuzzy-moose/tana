package panda

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const AuthenticatedRateInterval = 10 * time.Second

type AuthenticatedConfig struct {
	FavoritesURL string
	ArchiverURL  string
	AccountKey   string
	Cookies      map[string]string
}

func LoadAuthenticatedConfig(getenv func(string) string) (AuthenticatedConfig, error) {
	c := AuthenticatedConfig{
		FavoritesURL: strings.TrimSpace(getenv("PANDA_FAVORITES_URL")),
		ArchiverURL:  strings.TrimSpace(getenv("PANDA_ARCHIVER_URL")),
		AccountKey:   strings.TrimSpace(getenv("PANDA_FAVORITES_ACCOUNT_KEY")),
		Cookies:      make(map[string]string),
	}
	var values map[string]*string
	if err := json.Unmarshal([]byte(getenv("PANDA_FAVORITES_COOKIES")), &values); err != nil || values == nil {
		return AuthenticatedConfig{}, fmt.Errorf("PANDA_FAVORITES_COOKIES must be a JSON object of cookie names and string values")
	}
	for name, value := range values {
		if value == nil {
			return AuthenticatedConfig{}, fmt.Errorf("PANDA_FAVORITES_COOKIES values must be strings")
		}
		c.Cookies[name] = *value
	}
	if err := c.validate(); err != nil {
		return AuthenticatedConfig{}, err
	}
	if c.AccountKey == "" {
		c.AccountKey = strings.TrimSpace(c.Cookies["ipb_member_id"])
	}
	if c.AccountKey == "" {
		return AuthenticatedConfig{}, fmt.Errorf("PANDA_FAVORITES_COOKIES must include a nonempty ipb_member_id when PANDA_FAVORITES_ACCOUNT_KEY is unset")
	}
	u, _ := url.Parse(c.FavoritesURL)
	u.Host = strings.ToLower(u.Host)
	c.FavoritesURL = u.String()
	return c, nil
}

// Origin identifies the upstream host independently of its favorites endpoint.
// Callers use a configuration validated by LoadAuthenticatedConfig.
func (c AuthenticatedConfig) Origin() string {
	u, _ := url.Parse(c.FavoritesURL)
	return (&url.URL{Scheme: u.Scheme, Host: strings.ToLower(u.Host)}).String()
}

func (c AuthenticatedConfig) validate() error {
	u, err := url.Parse(c.FavoritesURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.User != nil ||
		u.Fragment != "" {
		return fmt.Errorf("PANDA_FAVORITES_URL must be an absolute HTTP(S) URL without credentials or fragment; no default is provided")
	}
	if _, err := url.ParseQuery(u.RawQuery); err != nil {
		return fmt.Errorf("PANDA_FAVORITES_URL has an invalid query")
	}
	a, err := url.Parse(c.ArchiverURL)
	if err != nil || a.Scheme != u.Scheme || !strings.EqualFold(a.Host, u.Host) || a.User != nil || a.Fragment != "" || a.RawQuery != "" {
		return fmt.Errorf("PANDA_ARCHIVER_URL must be an absolute URL on the favorites origin, without credentials, query, or fragment; no default is provided")
	}
	for name, value := range c.Cookies {
		if (&http.Cookie{Name: name, Value: value}).Valid() != nil {
			return fmt.Errorf("PANDA_FAVORITES_COOKIES contains an invalid cookie")
		}
	}
	return nil
}
