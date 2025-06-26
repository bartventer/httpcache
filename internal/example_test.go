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
	"fmt"
	"net/url"
)

func Example_makeURLKey() {
	u, err := url.Parse(
		"https://example.com:8443/abc?query=param&another=value#fragment=part1&part2",
	)
	if err != nil {
		fmt.Println("Error parsing URL:", err)
		return
	}
	cacheKey := makeURLKey(u)
	fmt.Println("Cache Key:", cacheKey)
	// Output:
	// Cache Key: https://example.com:8443/abc?query=param&another=value
}
