package registry

import (
	"net/url"
	"slices"
	"testing"

	"github.com/bartventer/httpcache/internal/testutil"
	"github.com/bartventer/httpcache/store/driver"
)

var _ driver.Conn = (*mockCache)(nil)

type mockCache struct{}

func (m *mockCache) Get(key string) ([]byte, error)     { return nil, driver.ErrNotExist }
func (m *mockCache) Set(key string, value []byte) error { return nil }
func (m *mockCache) Delete(key string) error            { return driver.ErrNotExist }

var _ driver.Driver = (*mockDriver)(nil)

type mockDriver struct {
	cache driver.Conn
	err   error
}

func (d *mockDriver) Open(u *url.URL) (driver.Conn, error) {
	return d.cache, d.err
}

func TestRegistry_RegisterDriverAndOpenConn(t *testing.T) {
	reg := New()
	driver := &mockDriver{cache: &mockCache{}}
	reg.RegisterDriver("foo", driver)

	// Should open successfully
	u, _ := url.Parse("foo://")
	cache, err := reg.OpenConn(u.String())
	testutil.RequireNoError(t, err, "Failed to open foo driver")
	testutil.RequireNotNil(t, cache, "expected non-nil cache")

	// Should fail for unknown scheme
	_, err = reg.OpenConn("bar://")
	testutil.RequireErrorIs(t, err, ErrUnknownDriver)

	// Should fail for invalid DSN
	// not well-formed per RFC 3986, golang.org/issue/33646
	_, err = reg.OpenConn("redis://x@y(z:123)/foo")
	var urlErr *url.Error
	testutil.RequireErrorAs(t, err, &urlErr, "expected url.Error for invalid DSN")
}

func TestRegistry_RegisterDriver_nilPanic(t *testing.T) {
	reg := &driverRegistry{drivers: make(map[string]driver.Driver)}
	testutil.RequirePanics(t, func() { reg.RegisterDriver("nil", nil) })
}

func TestRegistry_RegisterDriver_duplicatePanic(t *testing.T) {
	reg := New()
	driver := &mockDriver{cache: &mockCache{}}
	reg.RegisterDriver("dup", driver)
	testutil.RequirePanics(t, func() { reg.RegisterDriver("dup", driver) })
}

func TestRegistry_Drivers(t *testing.T) {
	reg := &driverRegistry{drivers: make(map[string]driver.Driver)}
	reg.RegisterDriver("foo", &mockDriver{cache: &mockCache{}})
	reg.RegisterDriver("bar", &mockDriver{cache: &mockCache{}})
	got := reg.Drivers()
	want := []string{"bar", "foo"}

	slices.Sort(got)
	slices.Sort(want)
	testutil.AssertTrue(t, slices.Equal(got, want))
}
