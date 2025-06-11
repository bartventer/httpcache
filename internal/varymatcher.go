package internal

import (
	"net/http"
	"slices"
	"strings"
)

// VaryMatcher defines the interface implemented by types that can match
// request headers nominated by the a cached response's Vary header against
// the headers of an incoming request.
type VaryMatcher interface {
	VaryHeadersMatch(cachedHdrs ResponseRefs, reqHdr http.Header) (int, bool)
}

func NewVaryMatcher(hvn HeaderValueNormalizer) *varyMatcher {
	return &varyMatcher{hvn: hvn}
}

type varyMatcher struct {
	hvn HeaderValueNormalizer
}

func (vm *varyMatcher) VaryHeadersMatch(entries ResponseRefs, reqHdr http.Header) (int, bool) {
	slices.SortFunc(entries, func(a, b *ResponseRef) int {
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
		return a.ReceivedAt.Compare(b.ReceivedAt)
	})

	for i, entry := range entries {
		if vm.varyHeadersMatchOne(entry, reqHdr) {
			return i, true // Found a match
		}
	}

	return -1, false // No match found
}

func (vm *varyMatcher) varyHeadersMatchOne(entry *ResponseRef, reqHdr http.Header) bool {
	if entry.Vary == "*" {
		return false // Vary: "*" never matches
	}
	for field, value := range entry.VaryResolved {
		reqValue := reqHdr[field] // field is already canonicalized
		if len(reqValue) == 0 {
			return false
		}
		// NOTE: The policy of this cache is to use just the first header line
		normalizedValue := vm.hvn.NormalizeHeaderValue(field, reqValue[0])
		if normalizedValue != value {
			return false
		}
	}
	return true
}
