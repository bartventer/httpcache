package internal

import (
	"net/http"
	"testing"
	"time"
)

func makeHeaderEntry(vary string, varyResolved map[string]string, ts time.Time) *VaryHeaderEntry {
	return &VaryHeaderEntry{
		Vary:         vary,
		VaryResolved: varyResolved,
		Timestamp:    ts,
	}
}

func TestVaryMatcher_VaryHeadersMatch(t *testing.T) {
	now := time.Now()
	tests := []struct {
		name    string
		entries VaryHeaderEntries
		reqHdr  http.Header
		wantIdx int
		wantOk  bool
	}{
		{
			name: "no vary header, always matches",
			entries: VaryHeaderEntries{
				makeHeaderEntry("", map[string]string{}, now),
			},
			reqHdr:  http.Header{"Accept": []string{"text/html"}},
			wantIdx: 0,
			wantOk:  true,
		},
		{
			name: "vary header matches",
			entries: VaryHeaderEntries{
				makeHeaderEntry("Accept", map[string]string{"Accept": "text/html"}, now),
			},
			reqHdr:  http.Header{"Accept": []string{"text/html"}},
			wantIdx: 0,
			wantOk:  true,
		},
		{
			name: "vary header does not match",
			entries: VaryHeaderEntries{
				makeHeaderEntry("Accept", map[string]string{"Accept": "text/html"}, now),
			},
			reqHdr:  http.Header{"Accept": []string{"application/json"}},
			wantIdx: -1,
			wantOk:  false,
		},
		{
			name: "vary header with wildcard",
			entries: VaryHeaderEntries{
				makeHeaderEntry("*", map[string]string{}, now),
			},
			reqHdr:  http.Header{"Accept": []string{"text/html"}},
			wantIdx: -1,
			wantOk:  false,
		},
		{
			name: "multiple entries, first matches",
			entries: VaryHeaderEntries{
				makeHeaderEntry("Accept", map[string]string{"Accept": "text/html"}, now),
				makeHeaderEntry(
					"Accept",
					map[string]string{"Accept": "application/json"},
					now.Add(time.Second),
				),
			},
			reqHdr:  http.Header{"Accept": []string{"text/html"}},
			wantIdx: 0,
			wantOk:  true,
		},
		{
			name: "multiple entries, second matches",
			entries: VaryHeaderEntries{
				makeHeaderEntry("Accept", map[string]string{"Accept": "application/json"}, now),
				makeHeaderEntry(
					"Accept",
					map[string]string{"Accept": "text/html"},
					now.Add(time.Second),
				),
			},
			reqHdr:  http.Header{"Accept": []string{"text/html"}},
			wantIdx: 1,
			wantOk:  true,
		},
		{
			name: "multiple entries, none match",
			entries: VaryHeaderEntries{
				makeHeaderEntry("Accept", map[string]string{"Accept": "application/json"}, now),
				makeHeaderEntry(
					"Accept",
					map[string]string{"Accept": "text/plain"},
					now.Add(time.Second),
				),
			},
			reqHdr:  http.Header{"Accept": []string{"text/html"}},
			wantIdx: -1,
			wantOk:  false,
		},
		{
			name: "entry with missing header in request",
			entries: VaryHeaderEntries{
				makeHeaderEntry("Accept", map[string]string{"Accept": "text/html"}, now),
			},
			reqHdr:  http.Header{}, // Accept header missing
			wantIdx: -1,
			wantOk:  false,
		},
		{
			name: "entry with multiple fields, all match",
			entries: VaryHeaderEntries{
				makeHeaderEntry(
					"Accept,User-Agent",
					map[string]string{"Accept": "text/html", "User-Agent": "Go-http-client"},
					now,
				),
			},
			reqHdr: http.Header{
				"Accept":     []string{"text/html"},
				"User-Agent": []string{"Go-http-client"},
			},
			wantIdx: 0,
			wantOk:  true,
		},
		{
			name: "entry with multiple fields, one does not match",
			entries: VaryHeaderEntries{
				makeHeaderEntry(
					"Accept,User-Agent",
					map[string]string{"Accept": "text/html", "User-Agent": "Go-http-client"},
					now,
				),
			},
			reqHdr:  http.Header{"Accept": []string{"text/html"}, "User-Agent": []string{"curl"}},
			wantIdx: -1,
			wantOk:  false,
		},
	}

	normalizer := HeaderValueNormalizerFunc(func(field, value string) string { return value })
	m := NewVaryMatcher(normalizer)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotIdx, gotOk := m.VaryHeadersMatch(tt.entries, tt.reqHdr)
			if gotIdx != tt.wantIdx || gotOk != tt.wantOk {
				t.Errorf(
					"VaryHeadersMatch() = (%d, %v), want (%d, %v)",
					gotIdx,
					gotOk,
					tt.wantIdx,
					tt.wantOk,
				)
			}
		})
	}
}
