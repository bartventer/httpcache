//go:build httpcache_acceptance_benchmarks
// +build httpcache_acceptance_benchmarks

package memcache

import (
	"net/url"
	"testing"

	"github.com/bartventer/httpcache/store"
	"github.com/bartventer/httpcache/store/acceptance"
)

func BenchmarkMemCache(b *testing.B) {
	acceptance.RunB(b, acceptance.FactoryFunc(func() (store.Cache, func()) {
		u := &url.URL{Scheme: Scheme}
		cache := Open(u)
		return cache, func() {}
	}))
}
