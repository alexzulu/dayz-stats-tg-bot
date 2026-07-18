package dayz_server

import (
	"errors"
	"strconv"
	"sync"
	"time"

	"github.com/woozymasta/a2s/pkg/a2s"
	"github.com/woozymasta/bercon-cli/pkg/beparser"
	"github.com/woozymasta/bercon-cli/pkg/bercon"
)

// ServerInfo contains information about the current DayZ server state.
type ServerInfo struct {
	Name            string
	Version         string
	ServerTime      [2]byte // [0]=hour, [1]=minute; parsed from A2S server keywords
	PlayersCount    byte
	MaxPlayersCount byte
	PlayerNames     []string
	Ping            time.Duration
}

// GetServerInfo queries the DayZ server using the provided A2S and BattlEye RCon clients
// concurrently. Either client may be nil - if both are nil the call returns an error immediately.
// Returns a joined error only if both queries fail; if exactly one fails, the partial result
// from the successful client is returned without error.
func GetServerInfo(ac *a2s.Client, rc *bercon.Connection) (*ServerInfo, error) {
	if ac == nil && rc == nil {
		return nil, errors.New("no valid clients provided")
	}

	var (
		wg   sync.WaitGroup
		mu   sync.Mutex
		info ServerInfo

		acErr, rcErr error
	)

	if ac != nil {
		wg.Go(func() {
			raw, infoErr := ac.GetInfo()
			if infoErr != nil {
				acErr = infoErr

				return
			}

			mu.Lock()

			info.Name = raw.Name
			info.Version = raw.Version
			info.PlayersCount = raw.Players
			info.MaxPlayersCount = raw.MaxPlayers
			info.Ping = raw.Ping

			// extract server time (e.g. `03:12`) from keywords if available
			for _, keyword := range raw.Keywords {
				if len(keyword) != 5 || keyword[2] != ':' {
					continue
				}

				// parse hours and minutes
				var hb, mb byte

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

				info.ServerTime[0], info.ServerTime[1] = hb, mb

				break
			}

			mu.Unlock()
		})
	}

	if rc != nil {
		wg.Go(func() {
			raw, cmdErr := rc.Send("players")
			if cmdErr != nil {
				rcErr = cmdErr

				return
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

			mu.Lock()
			info.PlayerNames = names
			mu.Unlock()
		})
	}

	wg.Wait()

	if acErr != nil && rcErr != nil {
		return nil, errors.Join(acErr, rcErr)
	}

	return &info, nil
}
