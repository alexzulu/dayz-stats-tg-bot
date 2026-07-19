package server

import (
	"errors"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/woozymasta/a2s/pkg/a2s"
	"github.com/woozymasta/bercon-cli/pkg/beparser"
	"github.com/woozymasta/bercon-cli/pkg/bercon"
)

// Info contains information about the current DayZ server state.
type Info struct {
	mu sync.Mutex

	Name            string
	Version         string
	ServerTime      *severTime
	PlayersCount    byte
	MaxPlayersCount byte
	PlayerNames     []string
	Ping            time.Duration
}

// severTime represents the server time in hours and minutes.
type severTime [2]byte

func (t severTime) Hours() int     { return int(t[0]) }
func (t severTime) Minutes() int   { return int(t[1]) }
func (t severTime) String() string { return fmt.Sprintf("%02d:%02d", t[0], t[1]) }

// GetFromA2S retrieves server information using the A2S protocol and populates the Info struct.
func (si *Info) GetFromA2S(ac *a2s.Client) error {
	if ac == nil {
		return errors.New("a2s client is nil")
	}

	raw, infoErr := ac.GetInfo()
	if infoErr != nil {
		return infoErr
	}

	var (
		hb, mb     byte // hours and minutes in bytes
		timeParsed bool
	)

	// extract server time (e.g. `03:12`) from keywords if available
	for _, keyword := range raw.Keywords {
		if len(keyword) != 5 || keyword[2] != ':' {
			continue
		}

		// parse hours and minutes
		if v, err := strconv.ParseUint(keyword[:2], 10, 8); err == nil && v < 24 {
			hb = byte(v)
		} else {
			continue
		}

		if v, err := strconv.ParseUint(keyword[3:], 10, 8); err == nil && v < 60 {
			mb = byte(v)
		} else {
			continue
		}

		timeParsed = true

		break
	}

	si.mu.Lock()

	si.Name = raw.Name
	si.Version = raw.Version
	si.PlayersCount = raw.Players
	si.MaxPlayersCount = raw.MaxPlayers
	si.Ping = raw.Ping

	if timeParsed {
		si.ServerTime = &severTime{hb, mb}
	}

	si.mu.Unlock()

	return nil
}

// GetFromRCon retrieves player names from the server using an RCON connection and populates the Info struct.
func (si *Info) GetFromRCon(rc *bercon.Connection) error {
	if rc == nil {
		return errors.New("rcon connection is nil")
	}

	raw, cmdErr := rc.Send("players")
	if cmdErr != nil {
		return cmdErr
	}

	// parse the raw response
	players := beparser.NewPlayers()
	players.Parse(raw)

	names := make([]string, 0, len(*players))
	for _, p := range *players {
		if p.Name != "" {
			names = append(names, p.Name)
		}
	}

	si.mu.Lock()
	si.PlayerNames = names
	si.mu.Unlock()

	return nil
}
