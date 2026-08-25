package app

import (
	"encoding/json"
	"net"
	"net/http"
	"strings"
)

// requestIsHTTPS determines the scheme visible to the user agent.
//
// A reverse proxy terminates TLS before forwarding the request to the local
// HTTP listener, so r.TLS is nil even when the browser is using HTTPS. Forwarded
// protocol headers are trusted only when the immediate peer is a configured
// trusted proxy.
func (a *App) requestIsHTTPS(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}

	remote := requestRemoteIP(r)
	if remote == nil || !a.isTrustedProxy(remote) {
		return false
	}

	if scheme := firstForwardedValue(r.Header.Get("X-Forwarded-Proto")); strings.EqualFold(scheme, "https") {
		return true
	}
	if scheme := forwardedHeaderScheme(r.Header.Get("Forwarded")); strings.EqualFold(scheme, "https") {
		return true
	}
	if scheme := cloudflareVisitorScheme(r.Header.Get("CF-Visitor")); strings.EqualFold(scheme, "https") {
		return true
	}
	return false
}

func requestRemoteIP(r *http.Request) net.IP {
	host := r.RemoteAddr
	if h, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		host = h
	}
	return net.ParseIP(strings.TrimSpace(host))
}

func firstForwardedValue(value string) string {
	if i := strings.IndexByte(value, ','); i >= 0 {
		value = value[:i]
	}
	return strings.TrimSpace(value)
}

func forwardedHeaderScheme(value string) string {
	for _, element := range strings.Split(value, ",") {
		for _, parameter := range strings.Split(element, ";") {
			key, val, ok := strings.Cut(strings.TrimSpace(parameter), "=")
			if !ok || !strings.EqualFold(strings.TrimSpace(key), "proto") {
				continue
			}
			return strings.Trim(strings.TrimSpace(val), `"`)
		}
	}
	return ""
}

func cloudflareVisitorScheme(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	var payload struct {
		Scheme string `json:"scheme"`
	}
	if err := json.Unmarshal([]byte(value), &payload); err != nil {
		return ""
	}
	return payload.Scheme
}
