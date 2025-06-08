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
