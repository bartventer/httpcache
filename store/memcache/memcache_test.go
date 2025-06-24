package memcache

import (
	"testing"

	"github.com/bartventer/httpcache/store/acceptance"
	"github.com/bartventer/httpcache/store/driver"
)

func TestMemCache_Acceptance(t *testing.T) {
	acceptance.Run(t, acceptance.FactoryFunc(func() (driver.Conn, func()) {
		cache := Open()
		return cache, func() {}
	}))
}
