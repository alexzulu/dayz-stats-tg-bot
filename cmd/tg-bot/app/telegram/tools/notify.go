package tt

import (
	"context"
	"time"

	"gopkg.in/telebot.v4"
)

// Notify sends chat action act once synchronously, then re-sends it every 4 seconds (chat actions last at most
// ~5s) in a background goroutine until ctx is canceled. It returns the error of the first, synchronous send;
// the background re-sends are best-effort and just stop the loop on error.
func Notify(ctx context.Context, c telebot.Context, act telebot.ChatAction) error {
	if err := c.Notify(act); err != nil {
		return err
	}

	go func() {
		t := time.NewTicker(4 * time.Second) //nolint:mnd // AFAIK, we need to repeat the action every ~5s to keep it alive
		defer t.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				if err := c.Notify(act); err != nil {
					return
				}
			}
		}
	}()

	return nil
}
