// Copyright (c) 2025 Bart Venter <bartventer@proton.me>
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//   http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package internal

import (
	"net/http"
	"testing"
	"time"
)

func makeHeaderEntry(vary string, varyResolved map[string]string, ts time.Time) *ResponseRef {
	return &ResponseRef{
		Vary:         vary,
		VaryResolved: varyResolved,
		ReceivedAt:   ts,
	}
}

func TestVaryMatcher_VaryHeadersMatch(t *testing.T) {
	now := time.Now()
	tests := []struct {
		name    string
		entries ResponseRefs
		reqHdr  http.Header
		wantIdx int
		wantOk  bool
	}{
		{
			name: "no vary header, always matches",
			entries: ResponseRefs{
				makeHeaderEntry("", map[string]string{}, now),
			},
			reqHdr:  http.Header{"Accept": []string{"text/html"}},
			wantIdx: 0,
			wantOk:  true,
		},
		{
			name: "vary header matches",
			entries: ResponseRefs{
				makeHeaderEntry("Accept", map[string]string{"Accept": "text/html"}, now),
			},
			reqHdr:  http.Header{"Accept": []string{"text/html"}},
			wantIdx: 0,
			wantOk:  true,
		},
		{
			name: "vary header does not match",
			entries: ResponseRefs{
				makeHeaderEntry("Accept", map[string]string{"Accept": "text/html"}, now),
			},
			reqHdr:  http.Header{"Accept": []string{"application/json"}},
			wantIdx: -1,
			wantOk:  false,
		},
		{
			name: "vary header with wildcard",
			entries: ResponseRefs{
				makeHeaderEntry("*", map[string]string{}, now),
			},
			reqHdr:  http.Header{"Accept": []string{"text/html"}},
			wantIdx: -1,
			wantOk:  false,
		},
		{
			name: "multiple entries, first matches",
			entries: ResponseRefs{
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
			entries: ResponseRefs{
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
			entries: ResponseRefs{
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
			entries: ResponseRefs{
				makeHeaderEntry("Accept", map[string]string{"Accept": "text/html"}, now),
			},
			reqHdr:  http.Header{}, // Accept header missing
			wantIdx: -1,
			wantOk:  false,
		},
		{
			name: "entry with multiple fields, all match",
			entries: ResponseRefs{
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
			entries: ResponseRefs{
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
