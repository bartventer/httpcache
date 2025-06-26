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
	"net/url"
	"testing"

	"github.com/bartventer/httpcache/internal/testutil"
)

func Test_makeKey(t *testing.T) {
	tests := []struct {
		raw      string
		expected string
	}{
		// Scheme and host normalization
		{"HTTP://EXAMPLE.COM", "http://example.com/"},
		{"http://EXAMPLE.com/", "http://example.com/"},
		{"http://example.com", "http://example.com/"},
		{"http://example.com:/", "http://example.com/"},
		{"http://example.com:80/", "http://example.com/"},
		{"https://example.com:443/", "https://example.com/"},
		{"https://example.com:444/", "https://example.com:444/"},
		{"http://example.com:8080/", "http://example.com:8080/"},
		// Path normalization
		{"http://example.com/a/./b/../c", "http://example.com/a/c"},
		{"http://example.com/a/b/../c/./d", "http://example.com/a/c/d"},
		{"http://example.com//a//b", "http://example.com//a//b"},
		// Percent-encoding normalization
		{"http://example.com/%7Euser", "http://example.com/~user"},
		{"http://example.com/%7euser", "http://example.com/~user"},
		{"http://example.com/a%2fb", "http://example.com/a%2Fb"},
		{"http://example.com/a%2Fb%2f", "http://example.com/a%2Fb%2F"},
		{"http://example.com/a%2fb%2F", "http://example.com/a%2Fb%2F"},
		// Query normalization
		{"http://example.com/?q=%7euser", "http://example.com/?q=~user"},
		{"http://example.com/?q=%7Euser", "http://example.com/?q=~user"},
		{"http://example.com/?q=a%2fb", "http://example.com/?q=a%2Fb"},
		// Host casing
		{"http://EXAMPLE.com/%7Euser", "http://example.com/~user"},
		// Opaque URLs
		{"mailto:user@example.com", "user@example.com"},
		// Fragment should be dropped
		{"http://example.com/foo#bar", "http://example.com/foo"},
		{"http://example.com/foo?x=1#frag", "http://example.com/foo?x=1"},
		// Empty path for http/https
		{"http://example.com", "http://example.com/"},
		{"https://example.com", "https://example.com/"},
		// Non-empty path
		{"http://example.com/foo", "http://example.com/foo"},
		// Path with dot-segments and query
		{"http://example.com/a/./b/../c?x=%7e", "http://example.com/a/c?x=~"},
		// Path with encoded reserved characters (should remain encoded)
		{"http://example.com/a%2Fb", "http://example.com/a%2Fb"},
		// Path with unreserved percent-encoding (should decode)
		{"http://example.com/%41", "http://example.com/A"},
		{"http://example.com/%7E", "http://example.com/~"},
		// Path with mixed case percent-encoding (should be uppercase)
		{"http://example.com/%7e%7E", "http://example.com/~~"},
		// Query with mixed case percent-encoding
		{"http://example.com/?q=%7e%7E", "http://example.com/?q=~~"},
	}

	for _, tt := range tests {
		u, err := url.Parse(tt.raw)
		if err != nil {
			t.Errorf("url.Parse(%q) failed: %v", tt.raw, err)
			continue
		}
		testutil.AssertEqual(t, makeURLKey(u), tt.expected, "makeURLKey(%q)", tt.raw)
	}
}
