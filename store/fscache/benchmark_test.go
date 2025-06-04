//go:build httpcache_acceptance_benchmarks
// +build httpcache_acceptance_benchmarks

package fscache

import (
	"testing"

	"github.com/bartventer/httpcache/store"
	"github.com/bartventer/httpcache/store/acceptance"
)

func BenchmarkFSCache(b *testing.B) {
	acceptance.RunB(b, acceptance.FactoryFunc(func() (store.Cache, func()) {
		u := makeRoot(b)
		cache, err := Open(u)
		if err != nil {
			b.Fatalf("Failed to create fscache: %v", err)
		}
		cleanup := func() { cache.Close() }
		return cache, cleanup
	}))
}
