package panda

import (
	"fmt"
	"golang.org/x/net/publicsuffix"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"time"
)

type AuthenticatedClient struct {
	httpClient *http.Client
	endpoint   *url.URL
	archiver   *url.URL
}

// NewAuthenticatedClient seeds a private, in-memory jar; server Set-Cookie values
// subsequently take precedence. The caller supplies pacing and ban coordination.
func NewAuthenticatedClient(cfg AuthenticatedConfig, httpClient *http.Client) (*AuthenticatedClient, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: time.Minute}
	}
	copyClient := *httpClient
	jar, err := cookiejar.New(&cookiejar.Options{PublicSuffixList: publicsuffix.List})
	if err != nil {
		return nil, err
	}
	endpoint, _ := url.Parse(cfg.FavoritesURL)
	cookies := make([]*http.Cookie, 0, len(cfg.Cookies))
	for name, value := range cfg.Cookies {
		cookies = append(cookies, &http.Cookie{Name: name, Value: value, Path: "/", Secure: endpoint.Scheme == "https"})
	}
	jar.SetCookies(endpoint, cookies)
	copyClient.Jar = jar
	copyClient.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		original := via[0].URL
		if len(via) >= 10 || !sameFavoritesEndpoint(original, req.URL) {
			return fmt.Errorf("%w: unexpected redirect", redirectError(original, endpoint))
		}
		if req.URL.Query().Get("favcat") != via[0].URL.Query().Get("favcat") {
			return fmt.Errorf("%w: redirected to another category", redirectError(original, endpoint))
		}
		return nil
	}
	archiver, _ := url.Parse(cfg.ArchiverURL)
	return &AuthenticatedClient{httpClient: &copyClient, endpoint: endpoint, archiver: archiver}, nil
}

func redirectError(original, favorites *url.URL) error {
	if sameFavoritesEndpoint(original, favorites) {
		return ErrFavoritesPage
	}
	return ErrArchivePage
}
