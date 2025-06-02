package internal

import (
	"net/http"
	"net/url"
	"slices"
	"testing"
)

type mockDeleterCache struct {
	deletedKeys []string
}

func (m *mockDeleterCache) Delete(key string) error {
	m.deletedKeys = append(m.deletedKeys, key)
	return nil
}

//nolint:nilnil // This is a mock implementation, so returning nil is acceptable.
func (m *mockDeleterCache) Get(key string, req *http.Request) (*Entry, error) { return nil, nil }
func (m *mockDeleterCache) Set(key string, entry *Entry) error                { return nil }

type mockKeyer struct {
	calledWith []*url.URL
	returnKey  string
}

func (m *mockKeyer) CacheKey(u *url.URL) string {
	m.calledWith = append(m.calledWith, u)
	return m.returnKey
}

func Test_cacheInvalidator_InvalidateCache(t *testing.T) {
	reqURL, _ := url.Parse("https://example.com/foo")
	tests := []struct {
		name           string
		respHeaders    map[string]string
		locationOrigin *url.URL
		expectDeletes  []string
	}{
		{
			name:          "no location headers",
			respHeaders:   map[string]string{},
			expectDeletes: []string{"main"},
		},
		{
			name: "invalid location header",
			respHeaders: map[string]string{
				"Location": "::::not-a-url",
			},
			expectDeletes: []string{"main"},
		},
		{
			name: "location header, same origin",
			respHeaders: map[string]string{
				"Location": "/bar",
			},
			expectDeletes: []string{"main", "loc"},
		},
		{
			name: "content-location header, same origin",
			respHeaders: map[string]string{
				"Content-Location": "/baz",
			},
			expectDeletes: []string{"main", "loc"},
		},
		{
			name: "location header, different origin",
			respHeaders: map[string]string{
				"Location": "https://other.com/bar",
			},
			expectDeletes: []string{"main"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mc := &mockDeleterCache{}
			mk := &mockKeyer{returnKey: "loc"}
			respHeader := make(http.Header)
			for k, v := range tt.respHeaders {
				respHeader.Set(k, v)
			}
			ci := &cacheInvalidator{cache: mc, cke: mk}
			ci.InvalidateCache(reqURL, respHeader, "main")
			if !slices.Equal(mc.deletedKeys, tt.expectDeletes) {
				t.Errorf("expected deleted keys %v, got %v", tt.expectDeletes, mc.deletedKeys)
			}
		})
	}
}
