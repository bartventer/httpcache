package internal

import (
	"net/http"
	"net/url"
)

// CacheInvalidator describes the interface implemented by types that can invalidate cache entries
// based on request and response headers, as specified in RFC 9111 §4.4.
type CacheInvalidator interface {
	InvalidateCache(reqURL *url.URL, respHeader http.Header, headers VaryHeaderEntries, key string)
}

type cacheInvalidator struct {
	cache ResponseCache
	cke   URLKeyer
}

func NewCacheInvalidator(cache ResponseCache, cke URLKeyer) *cacheInvalidator {
	return &cacheInvalidator{cache, cke}
}

func (r *cacheInvalidator) InvalidateCache(
	reqURL *url.URL,
	respHeader http.Header,
	headers VaryHeaderEntries,
	key string,
) {
	_ = r.cache.Delete(key)
	if len(headers) > 0 {
		for _, h := range headers.ResponseIDs() {
			_ = r.cache.Delete(h)
		}
	}
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
			locKey := r.cke.URLKey(locURL)
			headers, _ := r.cache.GetHeaders(locKey)
			if len(headers) > 0 {
				for _, h := range headers.ResponseIDs() {
					_ = r.cache.Delete(h)
				}
			}
		}
	}
}
