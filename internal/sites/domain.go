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
	// Canonical form: "www.example.com" and "example.com" are one site.
	host = strings.TrimPrefix(host, "www.")
	if !validHostname(host) {
		return "", fmt.Errorf("invalid domain")
	}
	return host, nil
}

// validHostname accepts plain ASCII DNS names: dot-separated labels of
// letters, digits and inner hyphens, 1-63 bytes each, 253 in total. Browsers
// send internationalised names in punycode (xn--...) in the Origin header, so
// that is the form to register. IP literals other than dotted IPv4 are out:
// nobody runs a tracked site on a bare IPv6 address.
func validHostname(h string) bool {
	if h == "" || len(h) > 253 {
		return false
	}
	for _, label := range strings.Split(h, ".") {
		if label == "" || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for i := 0; i < len(label); i++ {
			if !isLabelByte(label[i]) {
				return false
			}
		}
	}
	return true
}

func isLabelByte(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-'
}
