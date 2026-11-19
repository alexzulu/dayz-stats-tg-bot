package telegram

import (
	"errors"
	"fmt"
	"net/url"
)

// ValidateServerName reports whether name is a non-empty alias built only from letters, digits and _ . # -.
func ValidateServerName(name string) error {
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

	return nil
}

// ValidateBaseURL reports whether string is an absolute http(s) URL with a host.
func ValidateBaseURL(s string) error {
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
