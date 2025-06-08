package internal

import (
	"net/url"
	"testing"

	"github.com/bartventer/httpcache/internal/testutil"
)

func Test_makeKey(t *testing.T) {
	tests := []struct {
		name string
		u    *url.URL
		want string
	}{
		{
			name: "simple http url",
			u:    mustParseURL("http://example.com/foo"),
			want: "http://example.com/foo",
		},
		{
			name: "https with port",
			u:    mustParseURL("https://example.com:8443/bar"),
			want: "https://example.com:8443/bar",
		},
		{
			name: "with query",
			u:    mustParseURL("http://example.com/foo?x=1&y=2"),
			want: "http://example.com/foo?x=1&y=2",
		},
		{
			name: "with fragment (should be ignored)",
			u:    mustParseURL("http://example.com/foo#frag"),
			want: "http://example.com/foo",
		},
		{
			name: "opaque url",
			u:    &url.URL{Opaque: "opaque-data"},
			want: "opaque-data",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testutil.AssertEqual(t, tt.want, makeURLKey(tt.u))
		})
	}
}
