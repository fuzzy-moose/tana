package panda

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const FavoritesRateInterval = 10 * time.Second

type FavoritesConfig struct {
	URL        string
	AccountKey string
	Cookies    map[string]string
}

func LoadFavoritesConfig(getenv func(string) string) (FavoritesConfig, error) {
	c := FavoritesConfig{
		URL:        strings.TrimSpace(getenv("PANDA_FAVORITES_URL")),
		AccountKey: strings.TrimSpace(getenv("PANDA_FAVORITES_ACCOUNT_KEY")),
		Cookies:    make(map[string]string),
	}
	var values map[string]*string
	if err := json.Unmarshal([]byte(getenv("PANDA_FAVORITES_COOKIES")), &values); err != nil || values == nil {
		return FavoritesConfig{}, fmt.Errorf("PANDA_FAVORITES_COOKIES must be a JSON object of cookie names and string values")
	}
	for name, value := range values {
		if value == nil {
			return FavoritesConfig{}, fmt.Errorf("PANDA_FAVORITES_COOKIES values must be strings")
		}
		c.Cookies[name] = *value
	}
	if err := c.validate(); err != nil {
		return FavoritesConfig{}, err
	}
	if c.AccountKey == "" {
		c.AccountKey = strings.TrimSpace(c.Cookies["ipb_member_id"])
	}
	if c.AccountKey == "" {
		return FavoritesConfig{}, fmt.Errorf("PANDA_FAVORITES_COOKIES must include a nonempty ipb_member_id when PANDA_FAVORITES_ACCOUNT_KEY is unset")
	}
	u, _ := url.Parse(c.URL)
	u.Host = strings.ToLower(u.Host)
	c.URL = u.String()
	return c, nil
}

// Origin identifies the upstream host independently of its favorites endpoint.
// Callers use a configuration validated by LoadFavoritesConfig.
func (c FavoritesConfig) Origin() string {
	u, _ := url.Parse(c.URL)
	return (&url.URL{Scheme: u.Scheme, Host: strings.ToLower(u.Host)}).String()
}

func (c FavoritesConfig) validate() error {
	u, err := url.Parse(c.URL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.User != nil ||
		u.Fragment != "" {
		return fmt.Errorf("PANDA_FAVORITES_URL must be an absolute HTTP(S) URL without credentials or fragment; no default is provided")
	}
	if _, err := url.ParseQuery(u.RawQuery); err != nil {
		return fmt.Errorf("PANDA_FAVORITES_URL has an invalid query")
	}
	for name, value := range c.Cookies {
		if (&http.Cookie{Name: name, Value: value}).Valid() != nil {
			return fmt.Errorf("PANDA_FAVORITES_COOKIES contains an invalid cookie")
		}
	}
	return nil
}
