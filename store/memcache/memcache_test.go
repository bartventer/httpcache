package memcache

import (
	"net/url"
	"testing"

	"github.com/bartventer/httpcache/store/acceptance"
	"github.com/bartventer/httpcache/store/driver"
)

func TestMemCache_Acceptance(t *testing.T) {
	acceptance.Run(t, acceptance.FactoryFunc(func() (driver.Conn, func()) {
		u := &url.URL{Scheme: Scheme}
		cache := Open(u)
		return cache, func() {}
	}))
}
