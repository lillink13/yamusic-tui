package api

import "testing"

func TestParseOAuthToken(t *testing.T) {
	cases := []struct {
		name, in, want string
	}{
		{"raw token", "y0_AgAAAABabc123", "y0_AgAAAABabc123"},
		{"raw token with spaces", "  y0_AgAAAABabc123  ", "y0_AgAAAABabc123"},
		{"full redirect url", "https://music.yandex.ru/#access_token=y0_TOKEN&token_type=bearer&expires_in=31535645", "y0_TOKEN"},
		{"fragment only", "#access_token=y0_TOKEN&token_type=bearer", "y0_TOKEN"},
		{"bare access_token param", "access_token=y0_TOKEN", "y0_TOKEN"},
		{"token then expires", "access_token=abc123&expires_in=100", "abc123"},
		{"url-encoded token", "access_token=ab%2Bcd%3D", "ab+cd="},
		{"empty", "", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := ParseOAuthToken(c.in); got != c.want {
				t.Errorf("ParseOAuthToken(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

func TestOAuthTokenURL(t *testing.T) {
	url := OAuthTokenURL()
	for _, want := range []string{"response_type=token", "client_id=", "oauth.yandex.ru"} {
		if !contains(url, want) {
			t.Errorf("OAuthTokenURL() = %q, missing %q", url, want)
		}
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
