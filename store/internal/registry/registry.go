// Package registry provides a registry for cache drivers.
package registry

import (
	"errors"
	"fmt"
	"maps"
	"net/url"
	"slices"
	"sync"

	"github.com/bartventer/httpcache/store/driver"
)

var ErrUnknownDriver = errors.New("store: unknown driver")

type driverRegistry struct {
	mu      sync.RWMutex
	drivers map[string]driver.Driver
}

func (dr *driverRegistry) RegisterDriver(name string, driver driver.Driver) {
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

func (dr *driverRegistry) OpenConn(dsn string) (driver.Conn, error) {
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

var defaultRegistry = New()

func Default() *driverRegistry {
	return defaultRegistry
}

func New() *driverRegistry {
	return &driverRegistry{drivers: make(map[string]driver.Driver)}
}
