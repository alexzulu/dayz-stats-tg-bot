package app

import (
	"os"
	"time"
)

type evnVar string

// String returns the string representation of the environment variable.
func (e evnVar) String() string { return string(e) }

// envOrDur returns the parsed duration from the named environment variable, or fallback if the
// variable is unset or its value is not a valid duration.
func envOrDur(name evnVar, fallback time.Duration) time.Duration {
	v, ok := os.LookupEnv(name.String())
	if !ok {
		return fallback
	}

	d, err := time.ParseDuration(v)
	if err != nil {
		return fallback
	}

	return d
}

// envOrStr returns the value of the named environment variable, or fallback if it is unset.
func envOrStr(name evnVar, fallback string) string {
	if v, ok := os.LookupEnv(name.String()); ok {
		return v
	}

	return fallback
}
