package internal

import (
	"net/http"
	"testing"

	"github.com/bartventer/httpcache/internal/testutil"
)

func Test_varyHeadersMatches(t *testing.T) {
	type args struct {
		cachedHeader, reqHeader http.Header
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "no vary header",
			args: args{
				cachedHeader: http.Header{"Accept": []string{"text/html"}},
			},
			want: true,
		},
		{
			name: "vary header matches",
			args: args{
				cachedHeader: http.Header{
					"Vary":   []string{"Accept"},
					"Accept": []string{"text/html"},
				},
				reqHeader: http.Header{"Accept": []string{"text/html"}},
			},
			want: true,
		},
		{
			name: "vary header does not match",
			args: args{
				cachedHeader: http.Header{
					"Vary":   []string{"Accept"},
					"Accept": []string{"text/html"},
				},
				reqHeader: http.Header{"Accept": []string{"application/json"}},
			},
			want: false,
		},
		{
			name: "vary header with wildcard",
			args: args{
				cachedHeader: http.Header{
					"Vary": []string{"*"},
				},
				reqHeader: http.Header{"Accept": []string{"text/html"}},
			},
			want: false, // Wildcard means no match
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testutil.AssertTrue(
				t,
				tt.want == varyHeadersMatch(tt.args.cachedHeader, tt.args.reqHeader),
			)
		})
	}
}
