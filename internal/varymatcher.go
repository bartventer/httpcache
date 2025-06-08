package internal

import (
	"net/http"
	"slices"
	"strings"
)

// VaryMatcher defines the interface implemented by types that can match
// request headers nominated by the a cached response's Vary header against
// the headers of an incoming request. Implementations should return the index
// of the cached response that matches the request headers, or -1 if no match
// is found.
type VaryMatcher interface {
	VaryHeadersMatch(cachedHdrs HeaderEntries, reqHdr http.Header) (int, bool)
}

type VaryMatcherFunc func(cachedHdrs HeaderEntries, reqHdr http.Header) (int, bool)

func (f VaryMatcherFunc) VaryHeadersMatch(
	cachedHdrs HeaderEntries,
	reqHdr http.Header,
) (int, bool) {
	return f(cachedHdrs, reqHdr)
}

func NewVaryMatcher(hvn HeaderValueNormalizer) *varyMatcher {
	return &varyMatcher{hvn: hvn}
}

type varyMatcher struct {
	hvn HeaderValueNormalizer
}

func (vm *varyMatcher) VaryHeadersMatch(entries HeaderEntries, reqHdr http.Header) (int, bool) {
	slices.SortFunc(entries, func(a, b *HeaderEntry) int {
		aVary := strings.TrimSpace(a.Vary)
		bVary := strings.TrimSpace(b.Vary)

		// Responses with Vary: "*" are least preferred
		aIsStar := aVary == "*"
		bIsStar := bVary == "*"
		if aIsStar && !bIsStar {
			return 1 // b preferred
		}
		if bIsStar && !aIsStar {
			return -1 // a preferred
		}

		// Responses with Vary headers are preferred over those without
		aHasVary := aVary != ""
		bHasVary := bVary != ""
		if aHasVary && !bHasVary {
			return -1 // a preferred
		}
		if !aHasVary && bHasVary {
			return 1 // b preferred
		}

		// If both have Vary headers, sort by Date or ResponseTime
		return a.Timestamp.Compare(b.Timestamp)
	})

	for i, entry := range entries {
		if entry.Vary == "*" {
			return -1, false // Not cachable due to Vary: "*"
		}
		for field, value := range entry.VaryResolved {
			reqValue := reqHdr[field] // field is already canonicalized
			if len(reqValue) == 0 {
				goto nextEntry
			}
			// NOTE: The policy of this cache is to use just the first header line
			normalizedValue := vm.hvn.NormalizeHeaderValue(field, reqValue[0])
			if normalizedValue != value {
				goto nextEntry
			}
		}
		return i, true

	nextEntry:
		continue
	}

	return -1, false // No match found
}
