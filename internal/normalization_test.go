package internal

import (
	"strconv"
	"testing"

	"github.com/bartventer/httpcache/internal/testutil"
)

func TestNormalizeHeaderValue(t *testing.T) {
	tests := []struct {
		field, value, want string
	}{
		// byQValue
		{
			"Accept",
			"text/html,application/xml;q=0.9,*/*;q=0.8",
			"text/html,application/xml;q=0.9,*/*;q=0.8",
		},
		{
			"Accept",
			"text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.1234,foo;q=1.234,bar;q=-0.5,foo;q=0.8",
			"application/xhtml+xml,foo,text/html,application/xml;q=0.9,foo;q=0.8,*/*;q=0.123,bar;q=0.001",
		},
		{"Accept", "text/html,foo;q=0", "text/html"},
		{"Accept", "text/html; foo=bar; baz=qux", "text/html;baz=qux;foo=bar"},
		{"Accept-Charset", "utf-8;q=0.7,iso-8859-1", "iso-8859-1,utf-8;q=0.7"},
		{"Accept-Language", "en-US,en;q=0.9", "en-US,en;q=0.9"},
		{"TE", "trailers, deflate;q=0.5, gzip;q=1.0", "gzip,trailers,deflate;q=0.5"},
		// byEncoding
		{"Accept-Encoding", "gzip, x-gzip, br", "br,gzip"},
		{"Content-Encoding", "x-compress, deflate, gzip", "compress,deflate,gzip"},
		// byOrderInsensitive
		{"Cache-Control", "no-cache, max-age=0", "max-age=0,no-cache"},
		{"Connection", "keep-alive, close", "close,keep-alive"},
		{"Content-Language", "en, fr, de", "de,en,fr"},
		{"Expect", "100-continue, foo", "100-continue,foo"},
		{"Pragma", "no-cache, foo", "foo,no-cache"},
		{"Upgrade", "websocket, h2c", "h2c,websocket"},
		{"Vary", "Accept, Accept-Encoding", "Accept,Accept-Encoding"},
		{"Via", "1.1 vegur, 1.0 fred", "1.0 fred,1.1 vegur"},
		// byCaseInsensitive
		{"Content-Type", "APPLICATION/JSON", "application/json"},
		{"Content-Disposition", "INLINE", "inline"},
		{"Host", "EXAMPLE.COM", "example.com"},
		{"Referer", "HTTP://EXAMPLE.COM", "http://example.com"},
		{"User-Agent", "Go-http-client/1.1", "go-http-client/1.1"},
		{"Server", "APACHE", "apache"},
		{"Origin", "HTTPS://EXAMPLE.COM", "https://example.com"},
		// byTimeInsensitive
		{"If-Modified-Since", "  Tue, 15 Nov 1994 08:12:31 GMT  ", "Tue, 15 Nov 1994 08:12:31 GMT"},
		{
			"If-Unmodified-Since",
			"\tWed, 21 Oct 2015 07:28:00 GMT\n",
			"Wed, 21 Oct 2015 07:28:00 GMT",
		},
		{"Date", "  Fri, 01 Jan 2021 00:00:00 GMT ", "Fri, 01 Jan 2021 00:00:00 GMT"},
		// Authorization (with and without space)
		{"Authorization", "Bearer ABC123", "bearer ABC123"},
		{
			"Authorization",
			"Basic QWxhZGRpbjpvcGVuIHNlc2FtZQ==",
			"basic QWxhZGRpbjpvcGVuIHNlc2FtZQ==",
		},
		{"Authorization", "Unknown", "Unknown"},
		// Default (not in any category)
		{"X-Custom-Header", "SomeValue", "SomeValue"},
		// Empty value
		{"Accept", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.field+"_"+tt.value, func(t *testing.T) {
			got := normalizeHeaderValue(tt.field, tt.value)
			testutil.AssertEqual(t, tt.want, got)
		})
	}
}

func Test_makeVaryKey(t *testing.T) {
	urlKey := "https://example.com/resource"
	tests := []struct {
		name       string
		urlKey     string
		vary       map[string]string
		wantPrefix string
		wantHash   func() string
	}{
		{
			name:       "no vary headers (empty map)",
			urlKey:     urlKey,
			vary:       map[string]string{},
			wantPrefix: urlKey,
			wantHash:   func() string { return "" },
		},
		{
			name:       "no vary headers (nil map)",
			urlKey:     urlKey,
			vary:       nil,
			wantPrefix: urlKey,
			wantHash:   func() string { return "" },
		},
		{
			name:       "single header",
			urlKey:     urlKey,
			vary:       map[string]string{"Accept": "text/html"},
			wantPrefix: urlKey + "#",
			wantHash: func() string {
				return strconv.FormatUint(
					makeVaryHash(map[string]string{"Accept": "text/html"}),
					10,
				)
			},
		},
		{
			name:       "multiple headers, order-insensitive",
			urlKey:     urlKey,
			vary:       map[string]string{"Accept": "text/html", "Accept-Encoding": "gzip"},
			wantPrefix: urlKey + "#",
			wantHash: func() string {
				return strconv.FormatUint(
					makeVaryHash(
						map[string]string{"Accept": "text/html", "Accept-Encoding": "gzip"},
					),
					10,
				)
			},
		},
		{
			name:       "multiple headers, different order",
			urlKey:     urlKey,
			vary:       map[string]string{"Accept-Encoding": "gzip", "Accept": "text/html"},
			wantPrefix: urlKey + "#",
			wantHash: func() string {
				return strconv.FormatUint(
					makeVaryHash(
						map[string]string{"Accept": "text/html", "Accept-Encoding": "gzip"},
					),
					10,
				)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := makeVaryKey(tt.urlKey, tt.vary)
			if tt.wantHash() == "" {
				testutil.AssertEqual(t, tt.wantPrefix, got)
			} else {
				want := tt.wantPrefix + tt.wantHash()
				testutil.AssertEqual(t, want, got)
			}
		})
	}
}
