package exhttp

import (
	"strconv"
	"strings"
)

// ParsePort parses the server port from a raw string.
func ParsePort(rawPort string, scheme string) (int, error) {
	port := 80
	if rawPort != "" {
		p, err := strconv.ParseInt(rawPort, 10, 32)
		if err != nil {
			return 0, err
		}

		port = int(p)
	} else if strings.HasPrefix(scheme, "https") {
		port = 443
	}

	return port, nil
}
