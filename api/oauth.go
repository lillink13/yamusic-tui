package api

import (
	"net/url"
	"strings"
)

// OAuthTokenURL is the Yandex implicit-grant authorize URL. Opening it (after a
// Yandex login) redirects to music.yandex.ru with the access token in the URL
// fragment, which ParseOAuthToken extracts. This is the only flow available for
// the music client id (no localhost capture, no custom OAuth app with Music scope).
func OAuthTokenURL() string {
	return yaOauthServerURL + "authorize?response_type=token&client_id=" + yaOauthClientID
}

// ParseOAuthToken extracts the access token from whatever the user pastes: a raw
// token, the whole redirect URL (https://music.yandex.ru/#access_token=...&...),
// or just the fragment (access_token=...). If no access_token marker is present
// the trimmed input is returned as-is (assumed to be a bare token).
func ParseOAuthToken(raw string) string {
	raw = strings.TrimSpace(raw)

	const marker = "access_token="
	idx := strings.Index(raw, marker)
	if idx < 0 {
		return raw
	}

	rest := raw[idx+len(marker):]
	// The token ends at the next URL-parameter separator.
	if cut := strings.IndexAny(rest, "&#? \t\r\n"); cut >= 0 {
		rest = rest[:cut]
	}
	if decoded, err := url.QueryUnescape(rest); err == nil {
		return decoded
	}
	return rest
}
