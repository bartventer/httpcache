package internal

import (
	"maps"
	"net/http"
	"slices"
	"time"
)

// ResponseStorer describes the interface implemented by types that can store HTTP responses
// in a cache, as specified in RFC 9111 §3.1.
type ResponseStorer interface {
	StoreResponse(
		resp *http.Response,
		key string,
		headers HeaderEntries,
		reqTime, respTime time.Time,
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

func resolveDate(dateRaw string, fallback time.Time) time.Time {
	date, ok := RawTime(dateRaw).Value()
	if !ok {
		return fallback
	}
	return date
}

func (r *responseStorer) StoreResponse(
	resp *http.Response,
	key string,
	headers HeaderEntries,
	reqTime, respTime time.Time,
) error {
	// Remove hop-by-hop headers as per RFC 9111 §3.1
	removeHopByHopHeaders(resp)

	if headers == nil {
		headers = make(HeaderEntries, 0, 1)
	} else {
		headers = slices.Grow(headers, 1)
	}

	varyResolved := maps.Collect(
		r.vhn.NormalizeVaryHeader(resp.Header.Get("Vary"), resp.Request.Header),
	)
	responseID := r.vk.VaryKey(key, varyResolved)
	headers = append(headers, &HeaderEntry{
		Vary:         resp.Header.Get("Vary"),
		VaryResolved: varyResolved,
		ResponseID:   responseID,
		Timestamp:    resolveDate(resp.Header.Get("Date"), respTime),
	})

	if err := r.cache.SetHeaders(key, headers); err != nil {
		return err
	}

	return r.cache.Set(responseID, &ResponseEntry{
		Response: resp,
		ReqTime:  reqTime,
		RespTime: respTime,
	})
}
