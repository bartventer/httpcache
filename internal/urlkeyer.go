package internal

import (
	"net/url"
	"strings"
)

// URLKeyer describes the interface implemented by types that can generate
// a cache key for a given URL according to RFC 9111 §4.1.
type URLKeyer interface {
	URLKey(u *url.URL) string
}

type URLKeyerFunc func(u *url.URL) string

func (f URLKeyerFunc) URLKey(u *url.URL) string {
	return f(u)
}

func NewURLKeyer() URLKeyer { return URLKeyerFunc(makeURLKey) }

// makeURLKey generates a cache key for the given URL according to RFC 9111 §4.1.
// The cache key consists of the scheme, host (including port if non-default), path, and query string,
// but excludes the fragment. The path is encoded using [net/url.EscapedPath]() to ensure proper normalization.
// The result is lowercased for consistency, as scheme and host are case-insensitive per RFC 3986.
func makeURLKey(u *url.URL) string {
	if u.Opaque != "" {
		return strings.ToLower(u.Opaque)
	}

	host, port := splitHostPort(u.Host)
	defaultP := defaultPort(u.Scheme)
	if port == "" {
		port = defaultP
	}

	hostPort := host
	if port != "" && port != defaultP {
		hostPort = host + ":" + port
	}

	result := u.Scheme + "://" + hostPort + u.EscapedPath()
	if u.RawQuery != "" {
		result += "?" + u.RawQuery
	}
	return strings.ToLower(result)
}
