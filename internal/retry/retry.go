package retry

import (
	"context"
	"errors"
	"time"
)

type options struct {
	Attempts int
	Interval time.Duration
	OnError  func(err error, attempt int)
}

// Option configures the behavior of Do.
type Option func(*options)

// Attempts sets the maximum number of attempts. Clamped to 1 if below 1.
func Attempts(attempts int) Option {
	if attempts < 1 {
		attempts = 1
	}

	return func(o *options) { o.Attempts = attempts }
}

// Interval sets the delay between attempts. Clamped to 0 if negative.
func Interval(interval time.Duration) Option {
	if interval < 0 {
		interval = 0
	}

	return func(o *options) { o.Interval = interval }
}

// OnError sets a callback invoked after every failed attempt, including the last.
// attempt is 1-based. Passing nil is a no-op.
func OnError(onError func(err error, attempt int)) Option {
	if onError == nil {
		return func(o *options) {}
	}

	return func(o *options) { o.OnError = onError }
}

// Do calls fn up to the configured number of attempts, waiting between each.
// Returns immediately on success. If ctx is canceled between attempts, returns
// [errors.Join](ctx.Err(), lastFnErr).
func Do[T any](ctx context.Context, fn func() (T, error), opts ...Option) (T, error) {
	var o = options{
		Attempts: 3,                      //nolint:mnd
		Interval: 100 * time.Millisecond, //nolint:mnd
		OnError:  nil,
	}

	for _, opt := range opts {
		opt(&o)
	}

	var (
		result T
		err    error
	)

	for i := range o.Attempts {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return result, errors.Join(ctxErr, err)
		}

		result, err = fn()
		if err == nil {
			return result, nil
		}

		if o.OnError != nil {
			o.OnError(err, i+1)
		}

		hasNextAttempt := i < o.Attempts-1

		if hasNextAttempt {
			t := time.NewTimer(o.Interval)
			select {
			case <-ctx.Done():
				t.Stop()

				return result, errors.Join(ctx.Err(), err)
			case <-t.C:
			}
		}
	}

	return result, err
}
