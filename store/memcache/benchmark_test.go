//go:build httpcache_acceptance_benchmarks
// +build httpcache_acceptance_benchmarks

package memcache

import (
	"testing"

	"github.com/bartventer/httpcache/store/acceptance"
	"github.com/bartventer/httpcache/store/driver"
)

func BenchmarkMemCache(b *testing.B) {
	acceptance.RunB(b, acceptance.FactoryFunc(func() (driver.Conn, func()) {
		cache := Open()
		return cache, func() {}
	}))
}
