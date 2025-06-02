package internal

import (
	"net/http"
	"time"
)

// ResponseStorer describes the interface implemented by types that can store HTTP responses
// in a cache, as specified in RFC 9111 §3.1.
type ResponseStorer interface {
	StoreResponse(resp *http.Response, key string, reqTime, respTime time.Time) error
}

type responseStorer struct {
	cache ResponseCache // Cache to store the response
}

// NewResponseStorer creates a new ResponseStorer with the given cache.
func NewResponseStorer(cache ResponseCache) ResponseStorer {
	return &responseStorer{cache}
}

func (r *responseStorer) StoreResponse(
	resp *http.Response,
	key string,
	reqTime, respTime time.Time,
) error {
	// Remove hop-by-hop headers as per RFC 9111 §3.1
	removeHopByHopHeaders(resp)

	// Store the response in the cache
	entry := &Entry{
		Response: resp,
		ReqTime:  reqTime,
		RespTime: respTime,
	}
	return r.cache.Set(key, entry)
}
