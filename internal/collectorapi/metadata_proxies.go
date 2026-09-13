package collectorapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"time"
)

var (
	ErrInvalidProxy   = errors.New("proxy_invalid_configuration")
	ErrDuplicateProxy = errors.New("proxy_duplicate")
	ErrProxyNotFound  = errors.New("proxy_not_found")
	ErrProxyChanging  = errors.New("proxy_changing")
)

type MetadataProxyInput struct {
	Name      string  `json:"name"`
	ProxyURL  string  `json:"proxy_url"`
	Username  string  `json:"username"`
	Password  *string `json:"password,omitempty"`
	UserAgent string  `json:"user_agent"`
	Enabled   bool    `json:"enabled"`
}

type MetadataProxyChannel struct {
	ID            string     `json:"id"`
	Name          string     `json:"name"`
	ProxyURL      string     `json:"proxy_url"`
	Username      string     `json:"username"`
	HasPassword   bool       `json:"has_password"`
	UserAgent     string     `json:"user_agent"`
	Enabled       bool       `json:"enabled"`
	State         string     `json:"state"`
	BatchSize     int        `json:"batch_size"`
	BanUntil      *time.Time `json:"ban_until,omitempty"`
	RetryAt       *time.Time `json:"retry_at,omitempty"`
	LastSuccessAt *time.Time `json:"last_success_at,omitempty"`
	LastError     string     `json:"last_error,omitempty"`
}

type MetadataProxyStatus struct {
	Enabled          bool                   `json:"enabled"`
	Channels         []MetadataProxyChannel `json:"channels"`
	RateIntervalMS   int64                  `json:"rate_interval_ms"`
	DefaultUserAgent string                 `json:"default_user_agent"`
}

type MetadataProxySettings struct {
	Enabled bool `json:"enabled"`
}

func (c *Client) MetadataProxies(ctx context.Context) (MetadataProxyStatus, error) {
	return c.metadataProxies(ctx, http.MethodGet, "", nil)
}

func (c *Client) SetMetadataProxies(ctx context.Context, settings MetadataProxySettings) (MetadataProxyStatus, error) {
	return c.metadataProxies(ctx, http.MethodPut, "", settings)
}

func (c *Client) SaveMetadataProxy(ctx context.Context, id string, input MetadataProxyInput) (MetadataProxyStatus, error) {
	if id == "" {
		return c.metadataProxies(ctx, http.MethodPost, "/channels", input)
	}
	return c.metadataProxies(ctx, http.MethodPut, "/channels/"+url.PathEscape(id), input)
}

func (c *Client) DeleteMetadataProxy(ctx context.Context, id string) (MetadataProxyStatus, error) {
	return c.metadataProxies(ctx, http.MethodDelete, "/channels/"+url.PathEscape(id), nil)
}

func (c *Client) metadataProxies(ctx context.Context, method, path string, input any) (MetadataProxyStatus, error) {
	ctx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	var body io.Reader
	if input != nil {
		data, err := json.Marshal(input)
		if err != nil {
			return MetadataProxyStatus{}, err
		}
		body = bytes.NewReader(data)
	}
	response, err := c.controlRequest(ctx, method, "/api/metadata/proxies"+path, body)
	if err != nil {
		return MetadataProxyStatus{}, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		var failure struct {
			Error string `json:"error"`
		}
		if response.StatusCode >= 400 && response.StatusCode < 500 && json.NewDecoder(io.LimitReader(response.Body, 4096)).Decode(&failure) == nil {
			for _, known := range []error{ErrInvalidProxy, ErrDuplicateProxy, ErrProxyNotFound, ErrProxyChanging} {
				if failure.Error == known.Error() {
					return MetadataProxyStatus{}, known
				}
			}
		}
		return MetadataProxyStatus{}, &HTTPError{StatusCode: response.StatusCode}
	}
	var status MetadataProxyStatus
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&status); err != nil {
		return status, err
	}
	if status.Channels == nil {
		return status, errors.New("collector returned invalid proxy status")
	}
	return status, nil
}
