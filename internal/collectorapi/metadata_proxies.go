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
	ErrInvalidProxy      = errors.New("proxy_invalid_configuration")
	ErrDuplicateProxy    = errors.New("proxy_duplicate")
	ErrProxyNotFound     = errors.New("proxy_not_found")
	ErrProxyChanging     = errors.New("proxy_changing")
	ErrInvalidProxyList  = errors.New("proxy_list_invalid")
	ErrProxyListTooLarge = errors.New("proxy_list_too_large")
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

type MetadataProxyImportInput struct {
	Proxies  string `json:"proxies"`
	Protocol string `json:"protocol"`
	Enabled  bool   `json:"enabled"`
}

type MetadataProxyImportResult struct {
	Status     MetadataProxyStatus `json:"status"`
	Added      int                 `json:"added"`
	Duplicates int                 `json:"duplicates"`
}

func (c *Client) ImportMetadataProxies(ctx context.Context, input MetadataProxyImportInput) (MetadataProxyImportResult, error) {
	var result MetadataProxyImportResult
	err := c.metadataProxyRequest(ctx, http.MethodPost, "/import", input, &result)
	if err == nil && result.Status.Channels == nil {
		err = errors.New("collector returned invalid proxy import result")
	}
	return result, err
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
	var status MetadataProxyStatus
	err := c.metadataProxyRequest(ctx, method, path, input, &status)
	if err == nil && status.Channels == nil {
		err = errors.New("collector returned invalid proxy status")
	}
	return status, err
}

func (c *Client) metadataProxyRequest(ctx context.Context, method, path string, input, result any) error {
	ctx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	var body io.Reader
	if input != nil {
		data, err := json.Marshal(input)
		if err != nil {
			return err
		}
		body = bytes.NewReader(data)
	}
	response, err := c.controlRequest(ctx, method, "/api/metadata/proxies"+path, body)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		var failure struct {
			Error string `json:"error"`
		}
		if response.StatusCode >= 400 && response.StatusCode < 500 && json.NewDecoder(io.LimitReader(response.Body, 4096)).Decode(&failure) == nil {
			for _, known := range []error{ErrInvalidProxy, ErrDuplicateProxy, ErrProxyNotFound, ErrProxyChanging,
				ErrInvalidProxyList, ErrProxyListTooLarge} {
				if failure.Error == known.Error() {
					return known
				}
			}
		}
		return &HTTPError{StatusCode: response.StatusCode}
	}
	return json.NewDecoder(io.LimitReader(response.Body, 16<<20)).Decode(result)
}
