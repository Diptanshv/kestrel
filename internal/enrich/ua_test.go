package enrich

import "testing"

func TestParseUA(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		browser string
		os      string
		device  string
		bot     bool
	}{
		{
			name:    "desktop chrome",
			in:      "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
			browser: "Chrome", os: "macOS", device: "desktop",
		},
		{
			name:    "iphone safari",
			in:      "Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Mobile/15E148 Safari/604.1",
			browser: "Safari", os: "iOS", device: "mobile",
		},
		{
			name: "googlebot",
			in:   "Mozilla/5.0 (compatible; Googlebot/2.1; +http://www.google.com/bot.html)",
			bot:  true,
		},
		{name: "empty is treated as a bot", in: "", bot: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseUA(tt.in)
			if got.Bot != tt.bot {
				t.Fatalf("Bot = %v, want %v", got.Bot, tt.bot)
			}
			if tt.bot {
				return
			}
			if got.Browser != tt.browser {
				t.Errorf("Browser = %q, want %q", got.Browser, tt.browser)
			}
			if got.OS != tt.os {
				t.Errorf("OS = %q, want %q", got.OS, tt.os)
			}
			if got.Device != tt.device {
				t.Errorf("Device = %q, want %q", got.Device, tt.device)
			}
		})
	}
}
