package internal

import (
	"fmt"
	"net/http"
)

type Cache interface {
	Get(key string) ([]byte, error)
	Set(key string, entry []byte) error
	Delete(key string) error
}

type ResponseCache interface {
	Get(key string, req *http.Request) (*Entry, error)
	Set(key string, entry *Entry) error
	Delete(key string) error
}

type responseCache struct {
	cache Cache
}

func NewResponseCache(cache Cache) *responseCache {
	return &responseCache{cache}
}

var _ ResponseCache = (*responseCache)(nil)

// Get retrieves a cached response entry by its key.
func (r *responseCache) Get(key string, req *http.Request) (*Entry, error) {
	data, err := r.cache.Get(key)
	if err != nil {
		return nil, err
	}
	entry := &Entry{}
	if unmarshalErr := entry.UnmarshalBinaryWithRequest(data, req); unmarshalErr != nil {
		return nil, fmt.Errorf("failed to unmarshal cached entry: %w", unmarshalErr)
	}
	return entry, nil
}

// Set stores a response entry in the cache with the given key.
func (r *responseCache) Set(key string, entry *Entry) error {
	data, err := entry.MarshalBinary()
	if err != nil {
		return fmt.Errorf("failed to marshal entry: %w", err)
	}
	return r.cache.Set(key, data)
}

// Delete removes a cached entry by its key.
func (r *responseCache) Delete(key string) error {
	return r.cache.Delete(key)
}
