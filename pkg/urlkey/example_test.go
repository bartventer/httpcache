// Copyright (c) 2026 Bart Venter <72999113+bartventer@users.noreply.github.com>
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

package urlkey_test

import (
	"fmt"
	"net/url"

	"github.com/bartventer/httpcache/pkg/urlkey"
)

func ExampleNormalize() {
	u, err := url.Parse(
		"https://example.com:8443/abc?query=param&another=value#fragment=part1&part2",
	)
	if err != nil {
		panic(err)
	}
	key := urlkey.Normalize(u)
	fmt.Println(key)
	// Output:
	// https://example.com:8443/abc?query=param&another=value
}
