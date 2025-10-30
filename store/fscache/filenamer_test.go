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

package fscache

import (
	"encoding/base64"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bartventer/httpcache/internal/testutil"
)

func Test_fragmentFileName_fragmentedFileNameToKey(t *testing.T) {
	cases := []struct {
		name      string
		url       string
		assertion func(tt *testing.T, encoded, decoded string)
	}{
		{
			name: "Empty string",
			url:  "",
		},
		{
			name: "Short ASCII URL",
			url:  "https://example.com/test?foo=bar",
		},
		{
			name: "Long ASCII URL",
			url:  "https://example.com/" + strings.Repeat("a", 1000),
			assertion: func(tt *testing.T, encoded string, _ string) {
				for frag := range strings.SplitSeq(encoded, string(filepath.Separator)) {
					testutil.AssertTrue(
						tt,
						len(frag) <= fragmentSize,
						"Fragment too long: got %d, want <= %d",
						len(frag),
						fragmentSize,
					)
				}
			},
		},
		{
			name: "Unicode URL",
			url:  "https://例子.测试?emoji=🚀",
		},
		{
			name: "URL with separators",
			url:  "https://foo/bar/baz?x=y/z",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			encoded := fragmentFileName(tc.url)
			// Roundtrip test
			decoded, err := fragmentedFileNameToKey(encoded)
			testutil.RequireNoError(t, err)
			testutil.AssertEqual(t, tc.url, decoded, "Roundtrip failed")
			if tc.assertion != nil {
				tc.assertion(t, encoded, decoded)
			}
		})
	}
}

func Test_fragmentedFileNameToKey_InvalidBase64(t *testing.T) {
	invalidPaths := []string{
		"!!!notbase64",
		"this/is/not/valid/base64/===",
		"foo/bar/baz",
	}
	for _, path := range invalidPaths {
		t.Run(path, func(t *testing.T) {
			_, err := fragmentedFileNameToKey(path)
			var cie base64.CorruptInputError
			testutil.RequireErrorAs(t, err, &cie)
		})
	}
}
