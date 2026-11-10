package secrets

import (
	"net/url"
	"strings"
)

// userinfoSpan locates the "user:password" part of a connection URL without
// re-encoding the rest of it. end is the index of the "@" that closes it, or
// equal to start when the URL has no userinfo.
func userinfoSpan(rawURL string) (start, end int, ok bool) {
	schemeEnd := strings.Index(rawURL, "://")
	if schemeEnd < 0 {
		// Opaque URLs such as "file:data.db" carry no credentials.
		return 0, 0, false
	}
	start = schemeEnd + len("://")
	authorityEnd := len(rawURL)
	if i := strings.IndexAny(rawURL[start:], "/?#"); i >= 0 {
		authorityEnd = start + i
	}
	at := strings.LastIndex(rawURL[start:authorityEnd], "@")
	if at < 0 {
		return start, start, true
	}
	return start, start + at, true
}

// SplitPassword removes the password from a connection URL. ok is false when
// the URL has no password to remove. The password is returned decoded.
func SplitPassword(rawURL string) (stripped, password string, ok bool) {
	start, end, found := userinfoSpan(rawURL)
	if !found || start == end {
		return rawURL, "", false
	}
	username, encoded, hasPassword := strings.Cut(rawURL[start:end], ":")
	if !hasPassword || encoded == "" {
		return rawURL, "", false
	}
	password, err := url.PathUnescape(encoded)
	if err != nil {
		password = encoded
	}
	if username == "" {
		// Nothing left before "@": drop the separator as well.
		return rawURL[:start] + rawURL[end+1:], password, true
	}
	return rawURL[:start] + username + rawURL[end:], password, true
}

// HasPassword reports whether the connection URL embeds a password.
func HasPassword(rawURL string) bool {
	_, _, ok := SplitPassword(rawURL)
	return ok
}

// InjectPassword adds the password to a connection URL that has none. A URL
// that already carries a password is returned unchanged.
func InjectPassword(rawURL, password string) string {
	if HasPassword(rawURL) {
		return rawURL
	}
	start, end, found := userinfoSpan(rawURL)
	if !found {
		return rawURL
	}
	// url.UserPassword escapes exactly what the userinfo grammar requires.
	escaped := strings.TrimPrefix(url.UserPassword("", password).String(), ":")
	if start == end {
		return rawURL[:start] + ":" + escaped + "@" + rawURL[start:]
	}
	username := strings.TrimSuffix(rawURL[start:end], ":")
	return rawURL[:start] + username + ":" + escaped + rawURL[end:]
}
