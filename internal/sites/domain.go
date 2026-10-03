package sites

import (
	"fmt"
	"net/url"
	"strings"
)

func NormalizeDomain(input string) (string, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return "", fmt.Errorf("domain is required")
	}
	if !strings.Contains(input, "://") {
		input = "https://" + input
	}
	u, err := url.Parse(input)
	if err != nil || u.Host == "" {
		return "", fmt.Errorf("invalid domain")
	}
	host := strings.ToLower(u.Hostname())
	if host == "" || strings.Contains(host, " ") {
		return "", fmt.Errorf("invalid domain")
	}
	return host, nil
}
