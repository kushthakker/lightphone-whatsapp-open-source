package bridge

import (
	"errors"
	"net"
	"net/url"
	"strings"
)

// ValidatePublicBaseURL returns a canonical public base URL suitable for QR
// configuration. HTTP is allowed only for an explicit local development URL.
func ValidatePublicBaseURL(raw string, allowLocalHTTP bool) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || parsed.User != nil || (parsed.Path != "" && parsed.Path != "/") || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", errors.New("PUBLIC_BASE_URL must be an origin URL without user info, path, query, or fragment")
	}
	if parsed.Scheme != "https" && !(allowLocalHTTP && parsed.Scheme == "http" && isLoopbackHost(parsed.Hostname())) {
		return "", errors.New("PUBLIC_BASE_URL must use HTTPS")
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	return parsed.String(), nil
}

func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
