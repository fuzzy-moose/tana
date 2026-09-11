package panda

import (
	"context"
	"errors"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var ErrArchivePage = errors.New("panda: archive unavailable or authentication rejected")

var archiveLink = regexp.MustCompile(`["'](https://[a-zA-Z0-9-]+\.hath\.network(?::[0-9]+)?/[^"'<>\s]+)["']`)

// GetArchiveURL follows the original-archive form used by the legacy downloader.
// It needs the gallery reference and session cookies, not a metadata API key.
func (c *AuthenticatedClient) GetArchiveURL(ctx context.Context, ref GalleryRef) (string, error) {
	if ref.ID <= 0 || strings.TrimSpace(ref.Token) == "" {
		return "", ErrArchivePage
	}
	u := *c.archiver
	u.RawQuery = url.Values{"gid": {strconv.FormatInt(ref.ID, 10)}, "token": {ref.Token}}.Encode()
	form := url.Values{"dltype": {"org"}, "dlcheck": {"Download Original Archive"}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "text/html")
	req.Header.Set("User-Agent", "Tana")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	const maxPageBytes = 8 * 1024 * 1024
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxPageBytes+1))
	if err != nil {
		return "", err
	}
	if len(body) > maxPageBytes {
		return "", ErrArchivePage
	}
	if ban := DetectBan(body, time.Now()); ban != nil {
		return "", ban
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", &HTTPError{StatusCode: resp.StatusCode, RetryAfter: resp.Header.Get("Retry-After")}
	}
	match := archiveLink.FindSubmatch(body)
	if len(match) != 2 {
		return "", ErrArchivePage
	}
	uLink, err := url.Parse(html.UnescapeString(string(match[1])))
	if err != nil || !ValidArchiveURL(uLink) {
		return "", ErrArchivePage
	}
	query := uLink.Query()
	query.Set("start", "1")
	uLink.RawQuery = query.Encode()
	return uLink.String(), nil
}

// ValidArchiveURL restricts archive transfers and redirects to H@H hosts.
func ValidArchiveURL(u *url.URL) bool {
	return u.Scheme == "https" && u.User == nil && u.Fragment == "" &&
		strings.HasSuffix(strings.ToLower(u.Hostname()), ".hath.network")
}
