package dayz_server

import (
	"strconv"
	"time"

	"github.com/woozymasta/a2s/pkg/a2s"
)

// ServerInfo contains information about the current DayZ server state.
type ServerInfo struct {
	Name            string
	Version         string
	ServerTime      [2]byte // hh:mm
	PlayersCount    byte
	MaxPlayersCount byte
	Ping            time.Duration
}

// GetServerInfo retrieves server information using the provided A2S client.
func GetServerInfo(c *a2s.Client) (*ServerInfo, error) {
	raw, infoErr := c.GetInfo()
	if infoErr != nil {
		return nil, infoErr
	}

	info := &ServerInfo{
		Name:            raw.Name,
		Version:         raw.Version,
		PlayersCount:    raw.Players,
		MaxPlayersCount: raw.MaxPlayers,
		Ping:            raw.Ping,
	}

	// extract server time (e.g. `03:12`) from keywords if available
	for _, keyword := range raw.Keywords {
		if len(keyword) != 5 || keyword[2] != ':' {
			continue
		}

		// parse hours and minutes
		var hb, mb byte

		if v, err := strconv.ParseUint(keyword[:2], 10, 8); err == nil {
			hb = byte(v)
		} else {
			continue
		}

		if v, err := strconv.ParseUint(keyword[3:], 10, 8); err == nil {
			mb = byte(v)
		} else {
			continue
		}

		info.ServerTime[0], info.ServerTime[1] = hb, mb

		break
	}

	return info, nil
}
