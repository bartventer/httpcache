package fscache

import (
	"net/url"
	"testing"

	"github.com/bartventer/httpcache/internal/testutil"
	"github.com/bartventer/httpcache/store/acceptance"
	"github.com/bartventer/httpcache/store/driver"
)

func (c *fsCache) Close() error { return c.root.Close() }

func makeRoot(t testing.TB) *url.URL {
	t.Helper()
	u, err := url.Parse("fscache://" + t.TempDir() + "?appname=testapp")
	if err != nil {
		t.Fatalf("Failed to parse cache URL: %v", err)
	}
	return u
}

func TestFSCache_Acceptance(t *testing.T) {
	acceptance.Run(t, acceptance.FactoryFunc(func() (driver.Conn, func()) {
		u := makeRoot(t)
		cache, err := Open(u)
		testutil.RequireNoError(t, err, "Failed to create fscache")
		cleanup := func() { cache.Close() }
		return cache, cleanup
	}))
}

func Test_fsCache_SetError(t *testing.T) {
	u := makeRoot(t)
	cache, err := Open(u)
	testutil.RequireNoError(t, err, "Failed to create fscache")
	t.Cleanup(func() { cache.Close() })

	cache.fn = fileNamerFunc(func(key string) string {
		return key // no sanitization
	})
	err = cache.Set("../../invalid", []byte("value"))
	testutil.RequireError(t, err)
}

func TestOpen(t *testing.T) {
	type args struct {
		dsn string
	}
	tests := []struct {
		name      string
		args      args
		setup     func(*testing.T)
		assertion func(tt *testing.T, got *fsCache, err error)
	}{
		{
			name: "Valid Root Directory",
			args: args{
				dsn: "fscache://" + t.TempDir() + "?appname=myapp",
			},
			assertion: func(tt *testing.T, got *fsCache, err error) {
				testutil.RequireNoError(tt, err)
				testutil.RequireNotNil(tt, got)
			},
		},
		{
			name: "Default User Cache Directory",
			args: args{
				dsn: "fscache://?appname=myapp",
			},
			assertion: func(tt *testing.T, got *fsCache, err error) {
				testutil.RequireNoError(tt, err)
				testutil.RequireNotNil(tt, got)
			},
		},
		{
			name: "Invalid Cache Directory",
			args: args{
				dsn: "fscache://?appname=myapp",
			},
			setup: func(tt *testing.T) {
				tt.Setenv("XDG_CACHE_HOME", "")
				tt.Setenv("HOME", "")
			},
			assertion: func(tt *testing.T, got *fsCache, err error) {
				testutil.RequireErrorIs(tt, err, ErrUserCacheDir)
				testutil.AssertNil(tt, got)
			},
		},
		{
			name: "Missing App Name",
			args: args{
				dsn: "fscache://",
			},
			assertion: func(tt *testing.T, got *fsCache, err error) {
				testutil.RequireErrorIs(tt, err, ErrMissingAppName)
				testutil.AssertNil(tt, got)
			},
		},
		{
			name: "Invalid Root Directory",
			args: args{
				dsn: "fscache:///../invalid?appname=myapp",
			},
			assertion: func(tt *testing.T, got *fsCache, err error) {
				testutil.RequireErrorIs(tt, err, ErrCreateCacheDir)
				testutil.AssertNil(tt, got)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u, err := url.Parse(tt.args.dsn)
			if err != nil {
				t.Fatalf("Failed to parse URL: %v", err)
			}
			if tt.setup != nil {
				tt.setup(t)
			}
			got, err := Open(u)
			tt.assertion(t, got, err)
		})
	}
}
