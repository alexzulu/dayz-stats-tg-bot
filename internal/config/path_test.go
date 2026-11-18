package config_test

import (
	"testing"

	"github.com/alexzulu/dayz-stats-tg-bot/internal/config"
)

func TestDefaultDirPath(t *testing.T) {
	t.Parallel()

	if config.DefaultDirPath() == "" {
		t.Error("DefaultDirPath is empty")
	}
}
