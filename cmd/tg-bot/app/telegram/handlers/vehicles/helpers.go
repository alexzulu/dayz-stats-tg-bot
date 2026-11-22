package vehicles

import "strings"

// iff returns trueVal when condition holds, otherwise falseVal - an inline ternary.
func iff[T any](condition bool, trueVal, falseVal T) T {
	if condition {
		return trueVal
	}

	return falseVal
}

// bw (buffer write) appends every fragment to b; an empty fragment is a no-op.
func bw(b *strings.Builder, parts ...string) {
	for _, p := range parts {
		if p == "" {
			continue
		}

		b.WriteString(p)
	}
}

// warnIf returns a warning glyph when condition holds, otherwise an empty string.
func warnIf(condition bool) string {
	if condition {
		return " ⚠️ "
	}

	return ""
}
