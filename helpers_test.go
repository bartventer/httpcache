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

package httpcache

import (
	"net/http"
	"testing"

	"github.com/bartventer/httpcache/internal/testutil"
)

func Test_addConditionalHeaders(t *testing.T) {
	baseReq, _ := http.NewRequest(http.MethodGet, "http://example.com", nil)

	t.Run("without ETag", func(t *testing.T) {
		storedHeaders := make(http.Header)
		storedHeaders.Set("ETag", "foo")
		reqWithHeaders := withConditionalHeaders(baseReq, storedHeaders)
		testutil.AssertNotNil(t, reqWithHeaders)
		testutil.AssertEqual(t, "foo", reqWithHeaders.Header.Get("If-None-Match"))
	})

	t.Run("with Last-Modified", func(t *testing.T) {
		storedHeaders := make(http.Header)
		storedHeaders.Set("Last-Modified", "bar")
		reqWithHeaders := withConditionalHeaders(baseReq, storedHeaders)
		testutil.AssertNotNil(t, reqWithHeaders)
		testutil.AssertEqual(t, "bar", reqWithHeaders.Header.Get("If-Modified-Since"))
	})
}
