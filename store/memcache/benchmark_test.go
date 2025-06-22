//go:build httpcache_acceptance_benchmarks
// +build httpcache_acceptance_benchmarks

package memcache

import (
	"net/url"
	"testing"

	"github.com/bartventer/httpcache/store/acceptance"
	"github.com/bartventer/httpcache/store/driver"
)

func BenchmarkMemCache(b *testing.B) {
	acceptance.RunB(b, acceptance.FactoryFunc(func() (driver.Conn, func()) {
		u := &url.URL{Scheme: Scheme}
		cache := Open(u)
		return cache, func() {}
	}))
}
