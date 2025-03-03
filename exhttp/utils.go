package exhttp

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// ParsePort parses the server port from a raw string.
func ParsePort(rawPort string, scheme string) (int, error) {
	port := 80
	if rawPort != "" {
		p, err := strconv.Atoi(rawPort)
		if err != nil {
			return 0, err
		}

		port = p
	} else if strings.HasPrefix(scheme, "https") {
		port = 443
	}

	return port, nil
}

// ParseHttpURL parses and validate the input string to have http(s) scheme.
func ParseHttpURL(input string) (*url.URL, error) {
	u, err := url.Parse(input)
	if err != nil {
		return nil, err
	}

	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("invalid http(s) scheme, got: %s", u.Scheme)
	}

	return u, nil
}
