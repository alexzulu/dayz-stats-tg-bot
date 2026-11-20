package validate

import (
	"errors"
	"fmt"
	"net/url"
	"unicode"
)

// ServerName reports whether name is a non-empty alias built only from letters, digits and _ . # -.
func ServerName(name string) error {
	if name == "" {
		return errors.New("must not be empty")
	}

	var hasAlnum bool

	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z',
			r >= 'A' && r <= 'Z',
			r >= '0' && r <= '9':
			hasAlnum = true
		case r == '_', r == '.', r == '#', r == '-':
		default:
			return errors.New("contains invalid characters")
		}
	}

	if !hasAlnum {
		return errors.New("must contain at least one letter or digit")
	}

	// this limit is required to allow pass the name as callback data in Telegram inline buttons, which has a 64-byte
	// limit (I reserve half of that for the prefix)
	if len(name) > 32 { //nolint:mnd
		return errors.New("must not exceed 32 bytes")
	}

	return nil
}

// BaseURL reports whether string is an absolute http(s) URL with a host.
func BaseURL(s string) error {
	u, err := url.Parse(s)
	if err != nil {
		return err
	}

	switch {
	case u.Scheme != "http" && u.Scheme != "https":
		return fmt.Errorf("invalid scheme %q, expected http or https", u.Scheme)
	case u.Host == "":
		return errors.New("missing host")
	}

	return nil
}

// AuthToken reports whether token is non-empty and free of control characters. Control characters (CR,
// LF, tab, DEL, ...) would make net/http reject the "Authorization: Bearer <token>" header. The token format
// itself is the daemon's concern, so nothing else is checked.
func AuthToken(token string) error {
	if token == "" {
		return errors.New("must not be empty")
	}

	for _, r := range token {
		if unicode.IsControl(r) {
			return errors.New("contains control characters")
		}
	}

	return nil
}
