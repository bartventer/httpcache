// Package store provides a registry and entry point for cache backends.
//
// This package allows you to register cache drivers and open cache connections
// using a DSN string. It acts as a facade over the internal registry and the
// driver interfaces defined in the [driver] subpackage.
//
// Most users will interact with this package to register drivers and open
// cache connections, while backend implementations should use the [driver]
// subpackage to define new cache backends.
package store

import (
	"github.com/bartventer/httpcache/store/driver"
	"github.com/bartventer/httpcache/store/internal/registry"
)

// Register makes a driver implementation available by the provided name.
// If Register is called twice with the same name or if driver is nil, it panics.
func Register(name string, driver driver.Driver) {
	registry.Default().RegisterDriver(name, driver)
}

// Open opens a cache connection using the provided DSN.
func Open(dsn string) (driver.Conn, error) {
	return registry.Default().OpenConn(dsn)
}

// Drivers returns a sorted list of the names of the registered drivers.
func Drivers() []string {
	return registry.Default().Drivers()
}
