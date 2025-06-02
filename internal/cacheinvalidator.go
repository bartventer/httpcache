package internal

import (
	"net/http"
	"net/url"
)

// CacheInvalidator describes the interface implemented by types that can invalidate cache entries
// based on request and response headers, as specified in RFC 9111 §4.4.
type CacheInvalidator interface {
	InvalidateCache(reqURL *url.URL, respHeader http.Header, key string)
}

type cacheInvalidator struct {
	cache ResponseCache // Cache to invalidate entries from
	cke   CacheKeyer    // Cache key generator
}

func NewCacheInvalidator(cache ResponseCache, cke CacheKeyer) *cacheInvalidator {
	return &cacheInvalidator{cache, cke}
}

func (r *cacheInvalidator) InvalidateCache(reqURL *url.URL, respHeader http.Header, key string) {
	_ = r.cache.Delete(key)
	r.invalidateLocationHeaders(reqURL, respHeader)
}

var locationHeaders = [...]string{"Location", "Content-Location"}

func (r *cacheInvalidator) invalidateLocationHeaders(reqURL *url.URL, respHeader http.Header) {
	for _, hdr := range locationHeaders {
		loc := respHeader.Get(hdr)
		if loc == "" {
			continue
		}
		locURL, err := url.Parse(loc)
		if err != nil {
			continue // invalid Location header, nothing to do
		}
		// Resolve relative URIs against request URI
		locURL = reqURL.ResolveReference(locURL)
		if sameOrigin(reqURL, locURL) {
			key := r.cke.CacheKey(locURL)
			_ = r.cache.Delete(key)
		}
	}
}
