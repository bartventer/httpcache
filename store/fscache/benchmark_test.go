//go:build httpcache_acceptance_benchmarks
// +build httpcache_acceptance_benchmarks

package fscache

import (
	"testing"

	"github.com/bartventer/httpcache/store/acceptance"
	"github.com/bartventer/httpcache/store/driver"
)

func BenchmarkFSCache(b *testing.B) {
	acceptance.RunB(b, acceptance.FactoryFunc(func() (driver.Conn, func()) {
		u := makeRootURL(b)
		cache, err := fromURL(u)
		if err != nil {
			b.Fatalf("Failed to create fscache: %v", err)
		}
		cleanup := func() { cache.Close() }
		return cache, cleanup
	}))
}
