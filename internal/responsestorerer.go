package internal

import (
	"maps"
	"net/http"
	"slices"
	"time"
)

// ResponseStorer describes the interface implemented by types that can store HTTP responses
// in a cache, as specified in RFC 9111 §3.1.
//
// If refs is nil, a new slice should be  created. If refIndex is valid, the
// reference at that index should be updated; otherwise, a new reference should
// be appended.
type ResponseStorer interface {
	StoreResponse(
		req *http.Request,
		resp *http.Response,
		urlKey string,
		refs ResponseRefs,
		reqTime, respTime time.Time,
		refIndex int,
	) error
}

type responseStorer struct {
	cache ResponseCache
	vhn   VaryHeaderNormalizer
	vk    VaryKeyer
}

func NewResponseStorer(cache ResponseCache, vhn VaryHeaderNormalizer, vk VaryKeyer) ResponseStorer {
	return &responseStorer{cache, vhn, vk}
}

func (r *responseStorer) StoreResponse(
	req *http.Request,
	resp *http.Response,
	urlKey string,
	refs ResponseRefs,
	reqTime, respTime time.Time,
	refIndex int,
) error {
	// Remove hop-by-hop headers as per RFC 9111 §3.1
	removeHopByHopHeaders(resp)

	vary := resp.Header.Get("Vary")
	varyResolved := maps.Collect(
		r.vhn.NormalizeVaryHeader(vary, req.Header),
	)
	responseID := r.vk.VaryKey(urlKey, varyResolved)

	respEntry := &Response{
		Data:        resp,
		RequestedAt: reqTime,
		ReceivedAt:  respTime,
		ID:          responseID,
	}
	_ = r.cache.Set(responseID, respEntry)

	switch {
	case refs == nil:
		refs = make(ResponseRefs, 0, 1)
	case cap(refs) <= len(refs)+1:
		refs = slices.Grow(refs, 1)
	}

	refEntry := &ResponseRef{
		Vary:         vary,
		VaryResolved: varyResolved,
		ReceivedAt:   respEntry.DateHeader(),
		ResponseID:   responseID,
	}

	if refIndex < 0 || refIndex >= len(refs) {
		refs = append(refs, refEntry) // New response reference
	} else {
		refs[refIndex] = refEntry // Update existing response reference
	}

	return r.cache.SetRefs(urlKey, refs)
}
