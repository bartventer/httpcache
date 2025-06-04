package store

import (
	"net/url"
	"slices"
	"testing"

	"github.com/bartventer/httpcache/internal/testutil"
)

var _ Cache = (*mockCache)(nil)

type mockCache struct{}

func (m *mockCache) Get(key string) ([]byte, error)     { return nil, ErrNotExist }
func (m *mockCache) Set(key string, value []byte) error { return nil }
func (m *mockCache) Delete(key string) error            { return ErrNotExist }

var _ Driver = (*mockDriver)(nil)

type mockDriver struct {
	cache Cache
	err   error
}

func (d *mockDriver) Open(u *url.URL) (Cache, error) {
	return d.cache, d.err
}

func TestDriverRegistry_RegisterAndOpen(t *testing.T) {
	reg := &driverRegistry{drivers: make(map[string]Driver)}
	driver := &mockDriver{cache: &mockCache{}}
	reg.Register("foo", driver)

	// Should open successfully
	u, _ := url.Parse("foo://")
	cache, err := reg.Open(u.String())
	testutil.RequireNoError(t, err, "Failed to open foo driver")
	testutil.RequireNotNil(t, cache, "expected non-nil cache")

	// Should fail for unknown scheme
	_, err = reg.Open("bar://")
	testutil.RequireErrorIs(t, err, ErrUnknownDriver)

	// Should fail for invalid DSN
	// not well-formed per RFC 3986, golang.org/issue/33646
	_, err = reg.Open("redis://x@y(z:123)/foo")
	var urlErr *url.Error
	testutil.RequireErrorAs(t, err, &urlErr, "expected url.Error for invalid DSN")
}

func TestDriverRegistry_RegisterPanics(t *testing.T) {
	reg := &driverRegistry{drivers: make(map[string]Driver)}
	testutil.RequirePanics(t, func() { reg.Register("nil", nil) })
}

func TestDriverRegistry_RegisterDuplicatePanics(t *testing.T) {
	reg := &driverRegistry{drivers: make(map[string]Driver)}
	driver := &mockDriver{cache: &mockCache{}}
	reg.Register("dup", driver)
	testutil.RequirePanics(t, func() { reg.Register("dup", driver) })
}

func TestDriverRegistry_Drivers(t *testing.T) {
	reg := &driverRegistry{drivers: make(map[string]Driver)}
	reg.Register("foo", &mockDriver{cache: &mockCache{}})
	reg.Register("bar", &mockDriver{cache: &mockCache{}})
	got := reg.Drivers()
	want := []string{"bar", "foo"}

	slices.Sort(got)
	slices.Sort(want)
	testutil.AssertTrue(t, slices.Equal(got, want))
}
