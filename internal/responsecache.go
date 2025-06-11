package internal

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/bartventer/httpcache/store"
)

type Cache = store.Cache

// ResponseCache is an interface for caching HTTP responses.
// It provides methods to delete any cached item by its key,
// retrieve and set full cached responses, and manage references associated
// with a given URL key.
type ResponseCache interface {
	Get(key string, req *http.Request) (*Response, error)
	Set(key string, entry *Response) error
	Delete(key string) error
	GetRefs(key string) (ResponseRefs, error)
	SetRefs(key string, entry ResponseRefs) error
}

type responseCache struct {
	cache Cache
}

func NewResponseCache(cache Cache) *responseCache {
	return &responseCache{cache}
}

var _ ResponseCache = (*responseCache)(nil)

func (r *responseCache) Get(responseKey string, req *http.Request) (*Response, error) {
	data, err := r.cache.Get(responseKey)
	if err != nil {
		return nil, err
	}
	entry, err := ParseResponse(data, req)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal cached entry: %w", err)
	}
	return entry, nil
}

func (r *responseCache) Set(responseKey string, entry *Response) error {
	data, err := entry.MarshalBinary()
	if err != nil {
		return fmt.Errorf("failed to marshal entry: %w", err)
	}
	return r.cache.Set(responseKey, data)
}

func (r *responseCache) Delete(key string) error {
	return r.cache.Delete(key)
}

func (r *responseCache) GetRefs(urlKey string) (ResponseRefs, error) {
	data, err := r.cache.Get(urlKey)
	if err != nil {
		return nil, err
	}
	var refs ResponseRefs
	if unmarshalErr := json.Unmarshal(data, &refs); unmarshalErr != nil {
		return nil, fmt.Errorf("failed to unmarshal cached entries: %w", unmarshalErr)
	}
	return refs, nil
}

func (r *responseCache) SetRefs(urlKey string, headers ResponseRefs) error {
	data, err := json.Marshal(headers)
	if err != nil {
		return fmt.Errorf("failed to marshal headers: %w", err)
	}
	return r.cache.Set(urlKey, data)
}
