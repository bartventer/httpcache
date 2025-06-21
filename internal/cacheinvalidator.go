package internal

import (
	"net/http"
	"net/url"
)

// CacheInvalidator describes the interface implemented by types that can
// invalidate cache entries for a target URI when an unsafe request receives a
// non-error response, as required by RFC 9111 §4.4. It may also invalidate
// entries for URIs in Location or Content-Location headers, but only if they
// share the same origin as the target URI.
type CacheInvalidator interface {
	InvalidateCache(reqURL *url.URL, respHeader http.Header, headers ResponseRefs, key string)
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
	refs ResponseRefs,
	key string,
) {
	deleted := map[string]struct{}{}
	del := func(k string) {
		if _, ok := deleted[k]; !ok {
			_ = r.cache.Delete(k)
			deleted[k] = struct{}{}
		}
	}
	for h := range refs.ResponseIDs() {
		del(h)
	}
	r.invalidateLocationHeaders(reqURL, respHeader, del)
	del(key)
}

var locationHeaders = [...]string{"Location", "Content-Location"}

func (r *cacheInvalidator) invalidateLocationHeaders(
	reqURL *url.URL,
	respHeader http.Header,
	deleteFn func(string),
) {
	for _, hdr := range locationHeaders {
		loc := respHeader.Get(hdr)
		if loc == "" {
			continue
		}
		locURL, err := url.Parse(loc)
		if err != nil {
			continue
		}
		locURL = reqURL.ResolveReference(locURL)
		if sameOrigin(reqURL, locURL) {
			urlKey := r.cke.URLKey(locURL)
			refs, _ := r.cache.GetRefs(urlKey)
			for h := range refs.ResponseIDs() {
				deleteFn(h)
			}
			deleteFn(urlKey)
		}
	}
}
