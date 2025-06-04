// Package memcache provides an in-memory implementation of store.Cache.
//
// It is suitable for testing or ephemeral caching needs, but does not persist data
// across process restarts.
package memcache

import (
	"errors"
	"fmt"
	"net/url"
	"sync"

	"github.com/bartventer/httpcache/store"
)

const Scheme = "memcache"

//nolint:gochecknoinits // We use init to register the driver.
func init() {
	store.Register(Scheme, store.DriverFunc(func(u *url.URL) (store.Cache, error) {
		return Open(u), nil
	}))
}

type memCache struct {
	mu    sync.RWMutex
	store map[string][]byte
}

// Open creates a new in-memory cache.
//
// This cache is not persistent and will lose all data when the process exits.
func Open(_ *url.URL) *memCache {
	return &memCache{
		store: make(map[string][]byte),
	}
}

var _ store.Cache = (*memCache)(nil)

func (c *memCache) Get(key string) ([]byte, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	val, ok := c.store[key]
	if !ok {
		return nil, errors.Join(
			store.ErrNotExist,
			fmt.Errorf("memcache: key %q does not exist", key),
		)
	}
	// Return a copy to prevent mutation
	cp := make([]byte, len(val))
	copy(cp, val)
	return cp, nil
}

func (c *memCache) Set(key string, value []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	// Store a copy to prevent external mutation
	cp := make([]byte, len(value))
	copy(cp, value)
	c.store[key] = cp
	return nil
}

func (c *memCache) Delete(key string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.store[key]; !ok {
		return errors.Join(
			store.ErrNotExist,
			fmt.Errorf("memcache: key %q does not exist", key),
		)
	}
	delete(c.store, key)
	return nil
}
