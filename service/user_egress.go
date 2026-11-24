package service

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
)

// Only these adapters use the shared HTTP relay for synchronous model requests.
// SDKs and task adapters with independent clients must opt in separately.
func ValidateUserProvider(ch *model.Channel) error {
	switch ch.Type {
	case constant.ChannelTypeOpenAI, constant.ChannelTypeAzure, constant.ChannelTypeOpenRouter, constant.ChannelTypeAnthropic, constant.ChannelTypeGemini:
	default:
		return errors.New("unsupported adapter for a user-owned provider")
	}
	if ch.Setting != nil && strings.TrimSpace(*ch.Setting) != "" {
		var fields map[string]any
		if err := common.UnmarshalJsonStr(*ch.Setting, &fields); err != nil {
			return errors.New("invalid personal channel settings")
		}
		for key, value := range fields {
			switch key {
			case "force_format", "thinking_to_content", "system_prompt", "system_prompt_override":
				continue
			case "proxy":
				if value == "" {
					continue
				}
			}
			return errors.New("advanced channel transport settings are not permitted for user-owned providers")
		}
	}
	for _, override := range []*string{ch.HeaderOverride, ch.ParamOverride} {
		if override != nil && strings.TrimSpace(*override) != "" && strings.TrimSpace(*override) != "{}" {
			return errors.New("request overrides are not permitted for user-owned providers")
		}
	}
	base := ch.GetBaseURL()
	if base == "" {
		base = constant.GetChannelBaseURL(ch.Type)
	}
	return ValidateUserEgressURL(base)
}

// User destinations must never inherit operator proxy, private-network or TLS exceptions.
var userNetworkPolicy = common.SSRFProtection{
	AllowedPorts: []int{443}, ApplyIPFilterForDomain: true,
	IpList: []string{"168.63.129.16/32", "192.88.99.0/24", "::/96", "64:ff9b:1::/48", "2002::/16", "fec0::/10"},
}

func ValidateUserEgressURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil ||
		strings.Contains(u.Host, "%") || (u.Port() != "" && u.Port() != "443") {
		return errors.New("user endpoint must use public HTTPS on port 443")
	}
	host := strings.ToLower(strings.TrimSuffix(u.Hostname(), "."))
	if host == "localhost" || strings.HasSuffix(host, ".localhost") || (net.ParseIP(host) == nil && !strings.Contains(host, ".")) {
		return errors.New("user endpoint must be a public host")
	}
	if err := userNetworkPolicy.ValidateNetworkTarget(host, 443); err != nil {
		return errors.New("user endpoint must be a public host")
	}
	return nil
}

type userEgressTransport struct{ transport *http.Transport }

// Bound even chunked or transparently decompressed responses from personal
// upstreams. Consumers cannot safely infer buffering policy from Content-Type.
const maxUserUpstreamResponseBytes = 64 << 20

func (t userEgressTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.Host != "" && !strings.EqualFold(req.Host, req.URL.Host) {
		return nil, errors.New("user endpoint host override is not permitted")
	}
	if err := ValidateUserEgressURL(req.URL.String()); err != nil {
		return nil, err
	}
	resp, err := t.transport.RoundTrip(req)
	if err != nil {
		return nil, err
	}
	if resp.Body != http.NoBody && resp.ContentLength > maxUserUpstreamResponseBytes {
		resp.Body.Close()
		return nil, &http.MaxBytesError{Limit: maxUserUpstreamResponseBytes}
	}
	resp.Body = http.MaxBytesReader(nil, resp.Body, maxUserUpstreamResponseBytes)
	return resp, nil
}

var userDialer = protectedFetchDialer{
	resolver:      net.DefaultResolver,
	dialContext:   dialUserPublicIP,
	getProtection: func() (*common.SSRFProtection, bool, error) { return &userNetworkPolicy, true, nil },
}

// A server with a globally routable interface still must not dial itself.
func dialUserPublicIP(ctx context.Context, network, addr string) (net.Conn, error) {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}
	ip := net.ParseIP(host)
	if ip == nil || !userNetworkPolicy.IsIPAccessAllowed(ip) {
		return nil, errors.New("expected a validated IP address")
	}
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return nil, err
	}
	for _, local := range addrs {
		if network, ok := local.(*net.IPNet); ok && network.IP.Equal(ip) {
			return nil, errors.New("user endpoint must not address this server")
		}
	}
	dialer := net.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second}
	return dialer.DialContext(ctx, network, addr)
}

var userEgressClient = &http.Client{
	Transport: userEgressTransport{&http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			// Bound DNS resolution as well as the aggregate of candidate dials.
			ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
			defer cancel()
			conn, err := userDialer.DialContext(ctx, network, addr)
			if err != nil {
				return nil, errors.New("user endpoint is unavailable or disallowed")
			}
			return conn, nil
		}, ForceAttemptHTTP2: true,
		MaxIdleConns: 100, IdleConnTimeout: 90 * time.Second, TLSHandshakeTimeout: 10 * time.Second, ResponseHeaderTimeout: 120 * time.Second,
	}},
	// Requests can carry provider keys or signed storage headers.
	CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
}
