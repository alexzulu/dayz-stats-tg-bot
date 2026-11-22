package players

import (
	"math"
	"strconv"
	"strings"
)

// fixed formats f as a fixed-point string rounded to the given number of decimals.
func fixed(f float64, decimals int) string { return strconv.FormatFloat(f, 'f', decimals, 64) }

// percent formats a normalized 0..1 ratio as a whole-number percent string, without the "%" sign.
func percent(ratio float64) string { return fixed(ratio*100, 0) } //nolint:mnd

// secondsToHours rounds a duration in seconds to whole hours.
func secondsToHours[T int | float64](seconds T) int { return int(math.Round(float64(seconds) / 3600)) } //nolint:mnd

// plural returns one when n == 1, otherwise many.
func plural[T int | float64](n T, one, many string) string {
	if n == 1 {
		return one
	}

	return many
}

// removeBackticks strips backticks so s cannot break out of a Markdown code span.
func removeBackticks(s string) string { return strings.ReplaceAll(s, "`", "") }

// bw (buffer write) appends every fragment to b; an empty fragment is a no-op.
func bw(b *strings.Builder, parts ...string) {
	for _, p := range parts {
		if p == "" {
			continue
		}

		b.WriteString(p)
	}
}

// iff returns trueVal when condition holds, otherwise falseVal - an inline ternary.
func iff[T any](condition bool, trueVal, falseVal T) T {
	if condition {
		return trueVal
	}

	return falseVal
}

// warnIf returns a warning glyph when condition holds, otherwise an empty string.
func warnIf(condition bool) string {
	if condition {
		return " ⚠️ "
	}

	return ""
}
