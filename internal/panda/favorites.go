package panda

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strconv"
	"time"

	"golang.org/x/net/publicsuffix"
)

const FavoritesPageSize = 100

var ErrFavoritesPage = errors.New("panda: invalid favorites page or settings profile")

type Favorite struct {
	GalleryRef GalleryRef
	AddedAt    time.Time
}

type FavoritesPage struct {
	CategoryName string
	Entries      []Favorite
	Next         string
}

type FavoritesClient struct {
	httpClient *http.Client
	endpoint   *url.URL
}

// NewFavoritesClient seeds a private, in-memory jar; server Set-Cookie values
// subsequently take precedence. The caller supplies pacing and ban coordination.
func NewFavoritesClient(cfg FavoritesConfig, httpClient *http.Client) (*FavoritesClient, error) {
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
	endpoint, _ := url.Parse(cfg.URL)
	cookies := make([]*http.Cookie, 0, len(cfg.Cookies))
	for name, value := range cfg.Cookies {
		cookies = append(cookies, &http.Cookie{Name: name, Value: value, Path: "/", Secure: endpoint.Scheme == "https"})
	}
	jar.SetCookies(endpoint, cookies)
	copyClient.Jar = jar
	copyClient.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= 10 || !sameFavoritesEndpoint(endpoint, req.URL) {
			return fmt.Errorf("%w: unexpected redirect", ErrFavoritesPage)
		}
		if req.URL.Query().Get("favcat") != via[0].URL.Query().Get("favcat") {
			return fmt.Errorf("%w: redirected to another category", ErrFavoritesPage)
		}
		return nil
	}
	return &FavoritesClient{httpClient: &copyClient, endpoint: endpoint}, nil
}

func sameFavoritesEndpoint(a, b *url.URL) bool {
	return a.Scheme == b.Scheme && a.Host == b.Host && a.EscapedPath() == b.EscapedPath() && b.User == nil && b.Fragment == ""
}

func (c *FavoritesClient) nextURL(base *url.URL, next string, category int) (*url.URL, error) {
	u, err := base.Parse(next)
	if err != nil || !sameFavoritesEndpoint(c.endpoint, u) {
		return nil, fmt.Errorf("%w: pagination left the configured favorites endpoint", ErrFavoritesPage)
	}
	query, err := url.ParseQuery(u.RawQuery)
	if err != nil {
		return nil, fmt.Errorf("%w: malformed pagination query", ErrFavoritesPage)
	}
	if cat := query.Get("favcat"); cat != "" && cat != strconv.Itoa(category) {
		return nil, fmt.Errorf("%w: pagination changed category", ErrFavoritesPage)
	}
	for key, values := range c.endpoint.Query() {
		if _, ok := query[key]; !ok {
			query[key] = values
		}
	}
	query.Set("favcat", strconv.Itoa(category))
	u.RawQuery = query.Encode()
	return u, nil
}

func (c *FavoritesClient) GetFavoritesPage(ctx context.Context, category int, next string) (FavoritesPage, error) {
	if category < 0 || category > 9 {
		return FavoritesPage{}, fmt.Errorf("%w: category must be 0–9", ErrFavoritesPage)
	}
	u := *c.endpoint
	query := u.Query()
	query.Set("favcat", strconv.Itoa(category))
	u.RawQuery = query.Encode()
	if next != "" {
		resolved, err := c.nextURL(&u, next, category)
		if err != nil {
			return FavoritesPage{}, err
		}
		u = *resolved
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return FavoritesPage{}, err
	}
	req.Header.Set("User-Agent", "Tana")
	req.Header.Set("Accept", "text/html")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return FavoritesPage{}, err
	}
	defer resp.Body.Close()
	const maxPageBytes = 8 * 1024 * 1024
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxPageBytes+1))
	if err != nil {
		return FavoritesPage{}, err
	}
	if len(body) > maxPageBytes {
		return FavoritesPage{}, fmt.Errorf("%w: page exceeds size limit", ErrFavoritesPage)
	}
	if ban := DetectBan(body, time.Now()); ban != nil {
		return FavoritesPage{}, ban
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return FavoritesPage{}, &HTTPError{StatusCode: resp.StatusCode, RetryAfter: resp.Header.Get("Retry-After")}
	}
	page, err := ParseFavoritesPage(body, category)
	if err != nil {
		return FavoritesPage{}, err
	}
	if page.Next != "" {
		resolved, err := c.nextURL(resp.Request.URL, page.Next, category)
		if err != nil {
			return FavoritesPage{}, err
		}
		page.Next = resolved.String()
	}
	return page, nil
}
