package utils

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// ErrBlockedURL is returned for URLs that point at a private, loopback,
// link-local or otherwise internal address.
var ErrBlockedURL = errors.New("url not allowed")

// ValidateExternalURL vets a user-supplied URL before the server fetches it.
// It rejects non-HTTP(S) schemes and hosts resolving to an internal address —
// the classic SSRF targets (http://169.254.169.254/, http://10.0.0.5/,
// http://localhost:6379/).
//
// Pair it with SafeHTTPClient's dial-time check: validation alone is a
// time-of-check/time-of-use gap, because a hostname may resolve differently on
// the next lookup.
func ValidateExternalURL(raw string) (*url.URL, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return nil, fmt.Errorf("invalid url: %w", err)
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https":
	default:
		return nil, fmt.Errorf("%w: unsupported scheme %q", ErrBlockedURL, u.Scheme)
	}
	host := u.Hostname()
	if host == "" {
		return nil, fmt.Errorf("%w: missing host", ErrBlockedURL)
	}
	if ip := net.ParseIP(host); ip != nil {
		if isBlockedIP(ip) {
			return nil, blockedErr(host, ip)
		}
		return u, nil
	}
	ips, err := net.LookupIP(host)
	if err != nil {
		return nil, fmt.Errorf("cannot resolve %s: %w", host, err)
	}
	for _, ip := range ips {
		if isBlockedIP(ip) {
			return nil, blockedErr(host, ip)
		}
	}
	return u, nil
}

func blockedErr(host string, ip net.IP) error {
	return fmt.Errorf("%w: %s -> %s", ErrBlockedURL, host, ip)
}

// isBlockedIP reports whether an address must never be reachable by a
// server-side fetch.
func isBlockedIP(ip net.IP) bool {
	return ip.IsLoopback() ||
		ip.IsPrivate() ||
		ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() ||
		ip.IsInterfaceLocalMulticast() ||
		ip.IsMulticast() ||
		ip.IsUnspecified()
}

// SafeHTTPClient returns a client that refuses to connect to internal addresses
// and gives up if a server never sends response headers. timeout = 0 means "no
// overall deadline", which is what background transfers want.
//
// Callers that read a response body should still cap its size themselves.
func SafeHTTPClient(timeout time.Duration) *http.Client {
	dialer := &net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}
	return &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			ResponseHeaderTimeout: 30 * time.Second,
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				host, port, err := net.SplitHostPort(addr)
				if err != nil {
					return nil, err
				}
				// Re-resolve here and refuse internal answers, so a DNS reply that
				// changes after ValidateExternalURL cannot slip through.
				addrs, err := net.DefaultResolver.LookupIPAddr(ctx, host)
				if err != nil {
					return nil, err
				}
				for _, a := range addrs {
					if isBlockedIP(a.IP) {
						return nil, blockedErr(host, a.IP)
					}
				}
				return dialer.DialContext(ctx, network, net.JoinHostPort(addrs[0].IP.String(), port))
			},
		},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return errors.New("too many redirects")
			}
			if _, err := ValidateExternalURL(req.URL.String()); err != nil {
				return err
			}
			return nil
		},
	}
}
