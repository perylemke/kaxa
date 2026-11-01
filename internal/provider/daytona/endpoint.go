package daytona

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
)

// checkEndpoint accepts an HTTPS base URL. Plain HTTP is accepted only for a
// loopback host, which tests use. The API key is sent to every endpoint that
// passes, so a URL from an API response must never reach a plaintext host.
func checkEndpoint(raw string) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return nil, errors.New("daytona: invalid endpoint URL")
	}
	if u.User != nil {
		return nil, errors.New("daytona: endpoint URL has credentials")
	}
	switch u.Scheme {
	case "https":
		return u, nil
	case "http":
		if isLoopback(u.Hostname()) {
			return u, nil
		}
	}
	return nil, fmt.Errorf("daytona: refusing to send the API key to a non-HTTPS endpoint (%s)", u.Scheme)
}

func isLoopback(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(strings.Trim(host, "[]"))
	return ip != nil && ip.IsLoopback()
}
