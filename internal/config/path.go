package config

import "os"

// DefaultDirPath returns the default directory path where the configuration file is looked for by default.
// Only in case of exception, this function returns an empty string.
func DefaultDirPath() string {
	if v := osSpecificConfigDirPath(); v != "" {
		return v
	}

	if v, err := os.Getwd(); err == nil {
		return v // fallback to the current working directory
	}

	return "" // no default path
}
