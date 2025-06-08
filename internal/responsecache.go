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
// retrieve and set full cached responses, and manage headers associated
// with a given URL key.
type ResponseCache interface {
	Get(responseKey string, req *http.Request) (*Entry, error)
	Set(responseKey string, entry *Entry) error
	Delete(key string) error
	GetHeaders(urlKey string) (HeaderEntries, error)
	SetHeaders(urlKey string, headers HeaderEntries) error
}

type responseCache struct {
	cache Cache
}

func NewResponseCache(cache Cache) *responseCache {
	return &responseCache{cache}
}

var _ ResponseCache = (*responseCache)(nil)

func (r *responseCache) Get(responseKey string, req *http.Request) (*Entry, error) {
	data, err := r.cache.Get(responseKey)
	if err != nil {
		return nil, err
	}
	entry := &Entry{}
	if unmarshalErr := entry.UnmarshalBinaryWithRequest(data, req); unmarshalErr != nil {
		return nil, fmt.Errorf("failed to unmarshal cached entry: %w", unmarshalErr)
	}
	return entry, nil
}

func (r *responseCache) Set(responseKey string, entry *Entry) error {
	data, err := entry.MarshalBinary()
	if err != nil {
		return fmt.Errorf("failed to marshal entry: %w", err)
	}
	return r.cache.Set(responseKey, data)
}

func (r *responseCache) Delete(key string) error {
	return r.cache.Delete(key)
}

func (r *responseCache) GetHeaders(urlKey string) (HeaderEntries, error) {
	data, err := r.cache.Get(urlKey)
	if err != nil {
		return nil, err
	}
	var refs HeaderEntries
	if unmarshalErr := json.Unmarshal(data, &refs); unmarshalErr != nil {
		return nil, fmt.Errorf("failed to unmarshal cached entries: %w", unmarshalErr)
	}
	return refs, nil
}

func (r *responseCache) SetHeaders(urlKey string, headers HeaderEntries) error {
	data, err := json.Marshal(headers)
	if err != nil {
		return fmt.Errorf("failed to marshal headers: %w", err)
	}
	return r.cache.Set(urlKey, data)
}
