package sitemap

import (
	"fmt"
	"net/url"
	"strings"
)

type Config struct {
	URL string
}

func LoadConfig(getenv func(string) string) (Config, error) {
	address := strings.TrimSpace(getenv("TANA_COLLECTOR_SITEMAP_URL"))
	if address == "" {
		address = "https://xml.e-hentai.org/sitemap_index.xml"
	}
	if !validDocumentURL(address) {
		return Config{}, fmt.Errorf("TANA_COLLECTOR_SITEMAP_URL must be a complete HTTP(S) URL without credentials or a fragment")
	}
	return Config{URL: address}, nil
}

func validDocumentURL(address string) bool {
	u, err := url.Parse(address)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Hostname() != "" && u.User == nil && u.Fragment == ""
}
