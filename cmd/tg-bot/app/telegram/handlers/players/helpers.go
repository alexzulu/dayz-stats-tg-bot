package players

import (
	"math"
	"strconv"
	"strings"
)

func floatToPercentStr(f float64) string { return strconv.Itoa(int(math.Round(f * 100))) } //nolint:mnd

func floatToIntStr(f float64) string { return strconv.Itoa(int(math.Round(f))) }

func floatRound2ToStr(f float64) string {
	return strconv.FormatFloat(math.Round(f*100)/100, 'f', 2, 64) //nolint:mnd
}

func secondsToHours(seconds float64) int { return int(math.Round(seconds / 3600)) } //nolint:mnd

func plural[T int | float64](value T, singular, plural string) string {
	if value == 1 {
		return singular
	}

	return plural
}

func removeBackticks(s string) string { return strings.ReplaceAll(s, "`", "") }
