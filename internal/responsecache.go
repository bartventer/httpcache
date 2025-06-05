package internal

import (
	"fmt"
	"net/http"

	"github.com/bartventer/httpcache/store"
)

type Cache = store.Cache

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

func (r *responseCache) Set(key string, entry *Entry) error {
	data, err := entry.MarshalBinary()
	if err != nil {
		return fmt.Errorf("failed to marshal entry: %w", err)
	}
	return r.cache.Set(key, data)
}

func (r *responseCache) Delete(key string) error {
	return r.cache.Delete(key)
}
