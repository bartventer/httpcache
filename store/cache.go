// Package store defines interfaces to be implemented by cache backends as used by the
// github.com/bartventer/httpcache package.
//
// # Implementing a Cache Backend
//
// To implement a custom cache backend, provide a type that implements the [Cache] interface.
//
// Implementations must be safe for concurrent use by multiple goroutines.
//
// Implementations can be found in sub-packages such as store/memcache and store/fscache.
package store

import (
	"errors"
	"fmt"
	"maps"
	"net/url"
	"slices"
	"sync"
)

// ErrNotExist is returned when a cache entry does not exist.
//
// Methods such as [Cache.Get] and [Cache.Delete] should return an error
// that satisfies errors.Is(err, store.ErrNotExist) if the entry is not found.
var ErrNotExist = errors.New("cache: entry does not exist")

// Cache describes the interface implemented by types that can store, retrieve, and delete
// cached values by key.
type Cache interface {
	// Get retrieves the cached value for the given key.
	// If the key does not exist, it should return an error satisfying
	// errors.Is(err, store.ErrNotExist).
	Get(key string) ([]byte, error)

	// Set stores the value for the given key.
	// If the key already exists, it should overwrite the existing value.
	Set(key string, value []byte) error

	// Delete removes the cached value for the given key.
	// If the key does not exist, it should return an error satisfying
	// errors.Is(err, store.ErrNotExist).
	Delete(key string) error
}

// Driver is the interface implemented by cache backends that can create a Cache
// from a URL. The URL scheme determines which driver is used.
type Driver interface {
	Open(u *url.URL) (Cache, error)
}

// DriverFunc is an adapter to allow the use of ordinary functions as Drivers.
type DriverFunc func(u *url.URL) (Cache, error)

func (f DriverFunc) Open(u *url.URL) (Cache, error) {
	return f(u)
}

type driverRegistry struct {
	mu      sync.RWMutex
	drivers map[string]Driver
}

func (dr *driverRegistry) Register(name string, driver Driver) {
	dr.mu.Lock()
	defer dr.mu.Unlock()
	if driver == nil {
		panic("store: Register driver is nil")
	}
	if _, dup := dr.drivers[name]; dup {
		panic("store: Register called twice for driver " + name)
	}
	dr.drivers[name] = driver
}

var ErrUnknownDriver = errors.New("store: unknown driver")

func (dr *driverRegistry) Open(dsn string) (Cache, error) {
	u, err := url.Parse(dsn)
	if err != nil {
		return nil, err
	}

	dr.mu.RLock()
	driver, ok := dr.drivers[u.Scheme]
	dr.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrUnknownDriver, u.Scheme)
	}

	return driver.Open(u)
}

func (dr *driverRegistry) Drivers() []string {
	dr.mu.RLock()
	defer dr.mu.RUnlock()
	return slices.Sorted(maps.Keys(dr.drivers))
}

var defaultRegistry = &driverRegistry{drivers: make(map[string]Driver)}

// Register makes a cache implementation available by the provided name.
// If Register is called twice with the same name or if driver is nil, it panics.
func Register(name string, driver Driver) {
	defaultRegistry.Register(name, driver)
}

// Drivers returns a sorted list of the names of the registered drivers.
func Drivers() []string {
	return defaultRegistry.Drivers()
}

// Open opens a cache using the provided dsn.
func Open(dsn string) (Cache, error) {
	return defaultRegistry.Open(dsn)
}
