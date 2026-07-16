package retry_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/alexzulu/dayz-stats-tg-bot/internal/retry"
)

var errFake = errors.New("fake error")

func TestDo_SuccessOnFirstAttempt(t *testing.T) {
	t.Parallel()

	var calls int

	result, err := retry.Do(t.Context(), func() (int, error) {
		calls++
		return 42, nil
	}, retry.Attempts(3), retry.Interval(0))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result != 42 {
		t.Fatalf("expected 42, got %d", result)
	}

	if calls != 1 {
		t.Fatalf("fn called %d time(s), expected 1", calls)
	}
}

func TestDo_SucceedsAfterRetries(t *testing.T) {
	t.Parallel()

	var calls int

	result, err := retry.Do(t.Context(), func() (int, error) {
		calls++
		if calls < 3 {
			return 0, errFake
		}

		return 7, nil
	}, retry.Attempts(3), retry.Interval(0))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result != 7 {
		t.Fatalf("expected 7, got %d", result)
	}

	if calls != 3 {
		t.Fatalf("fn called %d time(s), expected 3", calls)
	}
}

func TestDo_AllAttemptsFail(t *testing.T) {
	t.Parallel()

	const attempts = 4

	var calls int

	_, err := retry.Do(t.Context(), func() (int, error) {
		calls++
		return 0, errFake
	}, retry.Attempts(attempts), retry.Interval(0))

	if !errors.Is(err, errFake) {
		t.Fatalf("expected errFake, got %v", err)
	}

	if calls != attempts {
		t.Fatalf("fn called %d time(s), expected %d", calls, attempts)
	}
}

func TestDo_SingleAttempt(t *testing.T) {
	t.Parallel()

	var calls int

	_, err := retry.Do(t.Context(), func() (int, error) {
		calls++
		return 0, errFake
	}, retry.Attempts(1), retry.Interval(0))

	if !errors.Is(err, errFake) {
		t.Fatalf("expected errFake, got %v", err)
	}

	if calls != 1 {
		t.Fatalf("fn called %d time(s), expected 1", calls)
	}
}

func TestAttempts_ClampsToOne(t *testing.T) {
	t.Parallel()

	var calls int

	_, _ = retry.Do(t.Context(), func() (int, error) {
		calls++
		return 0, errFake
	}, retry.Attempts(0), retry.Interval(0))

	if calls != 1 {
		t.Fatalf("Attempts(0) should clamp to 1 attempt, fn called %d time(s)", calls)
	}
}

func TestDo_OnErrorCallback(t *testing.T) {
	t.Parallel()

	type record struct {
		err     error
		attempt int
	}

	const attempts = 3

	var records []record

	_, _ = retry.Do(t.Context(), func() (int, error) {
		return 0, errFake
	}, retry.Attempts(attempts), retry.Interval(0), retry.OnError(func(err error, attempt int) {
		records = append(records, record{err, attempt})
	}))

	if len(records) != attempts {
		t.Fatalf("expected %d OnError calls (every failure), got %d", attempts, len(records))
	}

	for i, r := range records {
		if !errors.Is(r.err, errFake) {
			t.Errorf("call %d: unexpected error %v", i, r.err)
		}

		if r.attempt != i+1 {
			t.Errorf("call %d: expected attempt=%d, got %d", i, i+1, r.attempt)
		}
	}
}

func TestDo_OnErrorNilNoPanic(t *testing.T) {
	t.Parallel()

	_, _ = retry.Do(t.Context(), func() (int, error) {
		return 0, errFake
	}, retry.Attempts(2), retry.Interval(0), retry.OnError(nil))
}

func TestDo_DefaultAttempts(t *testing.T) {
	t.Parallel()

	var calls int

	_, _ = retry.Do(t.Context(), func() (int, error) {
		calls++
		return 0, errFake
	}, retry.Interval(0))

	if calls != 3 {
		t.Fatalf("default should be 3 attempts, fn called %d time(s)", calls)
	}
}

func TestDo_CtxAlreadyCancelled(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	var calls int

	_, err := retry.Do(ctx, func() (int, error) {
		calls++
		return 0, errFake
	}, retry.Attempts(3), retry.Interval(0))

	if calls != 0 {
		t.Fatalf("fn should not be called with pre-canceled ctx, got %d calls", calls)
	}

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

func TestDo_CtxCancelledDuringSleep(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(t.Context())

	fnEntered := make(chan struct{}, 1)
	errc := make(chan error, 1)

	go func() {
		_, err := retry.Do(ctx, func() (int, error) {
			select {
			case fnEntered <- struct{}{}:
			default:
			}

			return 0, errFake
		}, retry.Attempts(5), retry.Interval(time.Hour))
		errc <- err
	}()

	<-fnEntered
	cancel()

	select {
	case err := <-errc:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected context.Canceled in error, got %v", err)
		}

		if !errors.Is(err, errFake) {
			t.Fatalf("expected errFake in joined error, got %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Do did not return after context cancellation")
	}
}
