package internal

import (
	"net/http"
)

// VaryMatcher describes the interface implemented by types that can
// check whether the Vary headers of a cached response match the request headers
// according to RFC 9111 §4.1.
type VaryMatcher interface {
	VaryHeadersMatch(cachedHdr, reqHdr http.Header) bool
}

type VaryMatcherFunc func(cachedHdr, reqHdr http.Header) bool

func (f VaryMatcherFunc) VaryHeadersMatch(cachedHdr, reqHdr http.Header) bool {
	return f(cachedHdr, reqHdr)
}

func NewVaryMatcher() VaryMatcher {
	return VaryMatcherFunc(varyHeadersMatch)
}

func varyHeadersMatch(cachedHdr, reqHdr http.Header) bool {
	vary := cachedHdr.Get("Vary")
	if vary == "" {
		return true
	}
	for field := range TrimmedCSVSeq(vary) {
		if field == "*" || reqHdr.Get(field) != cachedHdr.Get(field) {
			return false
		}
	}
	return true
}
