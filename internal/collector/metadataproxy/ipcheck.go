package metadataproxy

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/netip"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/html"
)

var (
	errProxyIPLeak  = errors.New("proxy_ip_leak")
	errProxyIPCheck = errors.New("proxy_ip_check_failed")
)

// ip.wtf asks clients to keep checks to roughly one per hour per source IP.
const ipCheckInterval = time.Hour

type verifiedTransport struct {
	base   http.RoundTripper
	verify func(context.Context, http.RoundTripper) error
	mu     sync.Mutex
	until  time.Time
	err    error
}

func (t *verifiedTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	t.mu.Lock()
	if !t.until.After(time.Now()) {
		t.err = t.verify(req.Context(), t.base)
		t.until = time.Now().Add(ipCheckInterval)
	}
	err := t.err
	t.mu.Unlock()
	if err != nil {
		return nil, err
	}
	return t.base.RoundTrip(req)
}

type ipObservation struct {
	ip    netip.Addr
	until time.Time
}

type ipVerifier struct {
	direct http.RoundTripper
	mu     sync.Mutex
	ips    map[bool]ipObservation
}

func newIPVerifier() *ipVerifier {
	direct := http.DefaultTransport.(*http.Transport).Clone()
	// The baseline must bypass HTTP_PROXY, HTTPS_PROXY and NO_PROXY.
	direct.Proxy = nil
	direct.DisableKeepAlives = true
	return &ipVerifier{direct: direct, ips: make(map[bool]ipObservation)}
}

func (v *ipVerifier) verify(ctx context.Context, proxy http.RoundTripper) error {
	body, err := ipCheckRequest(ctx, proxy, "https://www.ip.wtf/", "text/html")
	if err != nil {
		return err
	}
	addresses, err := reportedIPs(body)
	if err != nil {
		return err
	}
	for _, address := range addresses {
		original, err := v.originalIP(ctx, address.Is4())
		if err != nil {
			return err
		}
		if address == original {
			return errProxyIPLeak
		}
	}
	return nil
}

func (v *ipVerifier) originalIP(ctx context.Context, ipv4 bool) (netip.Addr, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if observation := v.ips[ipv4]; observation.until.After(time.Now()) {
		return observation.ip, nil
	}
	endpoint := "https://v6.ip.wtf/"
	if ipv4 {
		endpoint = "https://v4.ip.wtf/"
	}
	body, err := ipCheckRequest(ctx, v.direct, endpoint, "text/plain")
	var ip netip.Addr
	if err == nil {
		ip, err = netip.ParseAddr(strings.TrimSpace(string(body)))
		ip = ip.Unmap()
		if err != nil || ip.Zone() != "" || ip.Is4() != ipv4 {
			err = errProxyIPCheck
		}
	}
	if err == nil {
		v.ips[ipv4] = ipObservation{ip: ip, until: time.Now().Add(ipCheckInterval)}
	}
	return ip, err
}

func ipCheckRequest(ctx context.Context, transport http.RoundTripper, endpoint, accept string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, errProxyIPCheck
	}
	req.Header.Set("Accept", accept)
	req.Header.Set("User-Agent", defaultUserAgent)
	// No redirects: the check must be answered by the configured HTTPS origin.
	response, err := transport.RoundTrip(req)
	if err != nil {
		return nil, errors.Join(errProxyIPCheck, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, errProxyIPCheck
	}
	const limit = 256 << 10
	body, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if err != nil || len(body) > limit {
		return nil, errProxyIPCheck
	}
	return body, nil
}

// The HTML response includes the actual request seen by ip.wtf; its JSON API
// only reports the exit address and cannot detect IPs exposed in headers.
func reportedIPs(body []byte) ([]netip.Addr, error) {
	doc, err := html.Parse(bytes.NewReader(body))
	if err != nil {
		return nil, errProxyIPCheck
	}
	var exit, request string
	var visit func(*html.Node)
	visit = func(node *html.Node) {
		for _, attr := range node.Attr {
			if attr.Key == "id" && (attr.Val == "ip" || attr.Val == "http-request") {
				var content strings.Builder
				var collect func(*html.Node)
				collect = func(n *html.Node) {
					if n.Type == html.TextNode {
						content.WriteString(n.Data)
					}
					for child := n.FirstChild; child != nil; child = child.NextSibling {
						collect(child)
					}
				}
				collect(node)
				if attr.Val == "ip" {
					exit = strings.TrimSpace(content.String())
				} else {
					request = content.String()
				}
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			visit(child)
		}
	}
	visit(doc)
	ip, err := netip.ParseAddr(exit)
	if err != nil || ip.Zone() != "" || !strings.HasPrefix(strings.TrimSpace(request), "GET / HTTP/") {
		return nil, errProxyIPCheck
	}
	addresses := []netip.Addr{ip.Unmap()}
	for _, token := range strings.FieldsFunc(request, func(r rune) bool { return strings.ContainsRune(" \t\r\n,;=\"'[]()", r) }) {
		address, err := netip.ParseAddr(token)
		if err != nil {
			if hostPort, err := netip.ParseAddrPort(token); err == nil {
				address = hostPort.Addr()
			}
		}
		if address.IsValid() {
			addresses = append(addresses, address.Unmap())
		}
	}
	return addresses, nil
}
