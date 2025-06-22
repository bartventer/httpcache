package expapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/bartventer/httpcache/internal/testutil"
	"github.com/bartventer/httpcache/store/driver"
)

var _ connOpener = (*mockConnOpener)(nil)

type mockConnOpener struct {
	OpenConnFunc func(dsn string) (driver.Conn, error)
}

func (m *mockConnOpener) OpenConn(dsn string) (driver.Conn, error) {
	return m.OpenConnFunc(dsn)
}

type mockHTTPCache struct {
	GetFunc    func(key string) ([]byte, error)
	SetFunc    func(key string, value []byte) error
	DeleteFunc func(key string) error
	KeysFunc   func(prefix string) []string
}

func (m *mockHTTPCache) Get(key string) ([]byte, error)     { return m.GetFunc(key) }
func (m *mockHTTPCache) Set(key string, value []byte) error { return m.SetFunc(key, value) }
func (m *mockHTTPCache) Delete(key string) error            { return m.DeleteFunc(key) }
func (m *mockHTTPCache) Keys(prefix string) []string        { return m.KeysFunc(prefix) }

func Test_mux_OpenError(t *testing.T) {
	co := &mockConnOpener{
		OpenConnFunc: func(dsn string) (driver.Conn, error) { return nil, testutil.ErrSample },
	}
	m := &kvMux{co: co}
	mux := http.NewServeMux()
	m.Register(WithServeMux(mux))

	tests := []struct {
		name   string
		method string
		url    string
	}{
		{"List", http.MethodGet, "/debug/httpcache?dsn=foo"},
		{"Get", http.MethodGet, "/debug/httpcache/key?dsn=foo"},
		{"Delete", http.MethodDelete, "/debug/httpcache/key?dsn=foo"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.url, nil)
			rr := httptest.NewRecorder()
			mux.ServeHTTP(rr, req)
			testutil.AssertEqual(t, http.StatusInternalServerError, rr.Code)
		})
	}
}

func Test_mux_handlers(t *testing.T) {
	type args struct {
		method string
		url    string
		cache  driver.Conn
	}

	tests := []struct {
		name      string
		args      args
		assertion func(t *testing.T, rr *httptest.ResponseRecorder)
	}{
		{
			name: "List",
			args: args{
				method: http.MethodGet,
				url:    "/debug/httpcache?dsn=foo&prefix=key",
				cache: &mockHTTPCache{
					KeysFunc: func(prefix string) []string {
						return []string{"key1", "key2"}
					},
				},
			},
			assertion: func(t *testing.T, rr *httptest.ResponseRecorder) {
				testutil.AssertEqual(t, http.StatusOK, rr.Code)
				var resp map[string][]string
				err := json.Unmarshal(rr.Body.Bytes(), &resp)
				testutil.RequireNoError(t, err)
				testutil.AssertTrue(t, slices.Equal(resp["keys"], []string{"key1", "key2"}))
			},
		},
		{
			name: "List with Keys Not Supported",
			args: args{
				method: http.MethodGet,
				url:    "/debug/httpcache?dsn=foo",
				cache:  &struct{ driver.Conn }{},
			},
			assertion: func(t *testing.T, rr *httptest.ResponseRecorder) {
				testutil.AssertEqual(t, http.StatusNotImplemented, rr.Code)
				testutil.AssertTrue(
					t,
					strings.Contains(rr.Body.String(), "cache does not support listing keys"),
				)
			},
		},
		{
			name: "Get with KeyExists",
			args: args{
				method: http.MethodGet,
				url:    "/debug/httpcache/exists?dsn=foo",
				cache: &mockHTTPCache{
					GetFunc: func(key string) ([]byte, error) {
						return []byte("value"), nil
					},
				},
			},
			assertion: func(t *testing.T, rr *httptest.ResponseRecorder) {
				testutil.AssertEqual(t, http.StatusOK, rr.Code)
				testutil.AssertEqual(t, "value", rr.Body.String())
			},
		},
		{
			name: "Get with KeyNotFound",
			args: args{
				method: http.MethodGet,
				url:    "/debug/httpcache/notfound?dsn=foo",
				cache: &mockHTTPCache{
					GetFunc: func(key string) ([]byte, error) {
						return nil, driver.ErrNotExist
					},
				},
			},
			assertion: func(t *testing.T, rr *httptest.ResponseRecorder) {
				testutil.AssertEqual(t, http.StatusNotFound, rr.Code)
				testutil.AssertTrue(t, strings.Contains(rr.Body.String(), "not found"))
			},
		},
		{
			name: "Get with Error",
			args: args{
				method: http.MethodGet,
				url:    "/debug/httpcache/error?dsn=foo",
				cache: &mockHTTPCache{
					GetFunc: func(key string) ([]byte, error) { return nil, testutil.ErrSample },
				},
			},
			assertion: func(t *testing.T, rr *httptest.ResponseRecorder) {
				testutil.AssertEqual(t, http.StatusInternalServerError, rr.Code)
				testutil.AssertTrue(
					t,
					strings.Contains(rr.Body.String(), "failed to get value for key"),
				)
			},
		},
		{
			name: "Delete with KeyExists",
			args: args{
				method: http.MethodDelete,
				url:    "/debug/httpcache/exists?dsn=foo",
				cache: &mockHTTPCache{
					DeleteFunc: func(key string) error { return nil },
				},
			},
			assertion: func(t *testing.T, rr *httptest.ResponseRecorder) {
				testutil.AssertEqual(t, http.StatusNoContent, rr.Code)
			},
		},
		{
			name: "Delete with KeyNotFound",
			args: args{
				method: http.MethodDelete,
				url:    "/debug/httpcache/notfound?dsn=foo",
				cache: &mockHTTPCache{
					DeleteFunc: func(key string) error { return driver.ErrNotExist },
				},
			},
			assertion: func(t *testing.T, rr *httptest.ResponseRecorder) {
				testutil.AssertEqual(t, http.StatusNotFound, rr.Code)
				testutil.AssertTrue(t, strings.Contains(rr.Body.String(), "not found"))
			},
		},
		{
			name: "Delete with Error",
			args: args{
				method: http.MethodDelete,
				url:    "/debug/httpcache/error?dsn=foo",
				cache: &mockHTTPCache{
					DeleteFunc: func(key string) error { return testutil.ErrSample },
				},
			},
			assertion: func(t *testing.T, rr *httptest.ResponseRecorder) {
				testutil.AssertEqual(t, http.StatusInternalServerError, rr.Code)
				testutil.AssertTrue(t, strings.Contains(rr.Body.String(), "failed to delete value"))
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			co := &mockConnOpener{
				OpenConnFunc: func(dsn string) (driver.Conn, error) {
					return tt.args.cache, nil
				},
			}
			m := &kvMux{co: co}
			mux := http.NewServeMux()
			m.Register(WithServeMux(mux))

			req := httptest.NewRequest(tt.args.method, tt.args.url, nil)
			rr := httptest.NewRecorder()
			mux.ServeHTTP(rr, req)

			tt.assertion(t, rr)
		})
	}
}
