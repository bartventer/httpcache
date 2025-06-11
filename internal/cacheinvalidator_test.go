package internal

import (
	"net/http"
	"net/url"
	"slices"
	"testing"
)

func Test_cacheInvalidator_InvalidateCache(t *testing.T) {
	baseURL, _ := url.Parse("https://example.com/foo")
	otherURL, _ := url.Parse("https://other.com/bar")

	tests := []struct {
		name         string
		respHeaders  map[string]string
		headers      ResponseRefs
		keyerKey     string
		refs         map[string]ResponseRefs
		reqURL       *url.URL
		expectDelete []string
	}{
		{
			name:         "no location headers",
			respHeaders:  map[string]string{},
			keyerKey:     "main",
			reqURL:       baseURL,
			expectDelete: []string{"main"},
		},
		{
			name:         "invalid location header",
			respHeaders:  map[string]string{"Location": "::::not-a-url"},
			keyerKey:     "main",
			reqURL:       baseURL,
			expectDelete: []string{"main"},
		},
		{
			name:         "location header, same origin",
			respHeaders:  map[string]string{"Location": "/bar"},
			keyerKey:     "loc",
			reqURL:       baseURL,
			refs:         map[string]ResponseRefs{"loc": {&ResponseRef{ResponseID: "loc"}}},
			expectDelete: []string{"main", "loc"},
		},
		{
			name:         "content-location header, same origin",
			respHeaders:  map[string]string{"Content-Location": "/baz"},
			keyerKey:     "loc",
			reqURL:       baseURL,
			refs:         map[string]ResponseRefs{"loc": {&ResponseRef{ResponseID: "loc"}}},
			expectDelete: []string{"main", "loc"},
		},
		{
			name:         "location header, different origin",
			respHeaders:  map[string]string{"Location": otherURL.String()},
			keyerKey:     "other",
			reqURL:       baseURL,
			expectDelete: []string{"main"},
		},
		{
			name:        "with headers to delete",
			respHeaders: map[string]string{"Location": "/bar"},
			keyerKey:    "loc",
			reqURL:      baseURL,
			headers: ResponseRefs{
				&ResponseRef{ResponseID: "header1"},
				&ResponseRef{ResponseID: "header2"},
			},
			refs:         map[string]ResponseRefs{"loc": {&ResponseRef{ResponseID: "loc"}}},
			expectDelete: []string{"main", "header1", "header2", "loc"},
		},
		{
			name:         "both location and content-location, same origin",
			respHeaders:  map[string]string{"Location": "/bar", "Content-Location": "/baz"},
			keyerKey:     "loc",
			reqURL:       baseURL,
			refs:         map[string]ResponseRefs{"loc": {&ResponseRef{ResponseID: "loc"}}},
			expectDelete: []string{"main", "loc"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deleted := []string{}
			refs := tt.refs
			if refs == nil {
				refs = map[string]ResponseRefs{}
			}
			mrc := &MockResponseCache{
				DeleteFunc: func(key string) error {
					deleted = append(deleted, key)
					return nil
				},
				GetRefsFunc: func(key string) (ResponseRefs, error) {
					return refs[key], nil
				},
			}
			respHeader := make(http.Header)
			for k, v := range tt.respHeaders {
				respHeader.Set(k, v)
			}
			ci := &cacheInvalidator{cache: mrc, cke: URLKeyerFunc(func(u *url.URL) string {
				return tt.keyerKey
			})}
			ci.InvalidateCache(tt.reqURL, respHeader, tt.headers, "main")
			slices.Sort(deleted)
			slices.Sort(tt.expectDelete)
			if !slices.Equal(deleted, tt.expectDelete) {
				t.Errorf("expected deleted keys %v, got %v", tt.expectDelete, deleted)
			}
		})
	}
}
