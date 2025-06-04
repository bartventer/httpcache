package memcache

import (
	"net/url"
	"testing"

	"github.com/bartventer/httpcache/store"
	"github.com/bartventer/httpcache/store/acceptance"
)

func TestMemCache_Acceptance(t *testing.T) {
	acceptance.Run(t, acceptance.FactoryFunc(func() (store.Cache, func()) {
		u := &url.URL{Scheme: Scheme}
		cache := Open(u)
		return cache, func() {}
	}))
}
