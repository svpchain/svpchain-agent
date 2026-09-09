// Package netretry provides bounded retry handling for idempotent network
// exchanges. It is intentionally not used for agent execution or transaction
// broadcasts: a lost response there does not prove the remote side did nothing.
package netretry

import (
	"context"
	"errors"
	"io"
	"net"
	"strings"
	"syscall"
	"time"
)

const (
	MaxRetries = 3
	Interval   = 5 * time.Second
)

// Do retries a transient connection failure up to MaxRetries times, waiting
// Interval between attempts. A canceled caller context always stops promptly.
func Do(ctx context.Context, call func() error) error {
	return do(ctx, MaxRetries, Interval, call)
}

func do(ctx context.Context, maxRetries int, interval time.Duration, call func() error) error {
	var err error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if err = ctx.Err(); err != nil {
			return err
		}
		err = call()
		if err == nil || !Transient(err) || attempt == maxRetries {
			return err
		}
		if err = wait(ctx, interval); err != nil {
			return err
		}
	}
	return err
}

// Transient reports connection failures that may succeed on a new connection.
// HTTP application responses are deliberately not included: callers must make
// their own policy for a returned status code.
func Transient(err error) bool {
	if err == nil || errors.Is(err, context.Canceled) {
		return false
	}
	var netErr net.Error
	if errors.As(err, &netErr) && (netErr.Timeout() || netErr.Temporary()) {
		return true
	}
	for _, target := range []error{
		io.EOF,
		io.ErrUnexpectedEOF,
		syscall.ECONNRESET,
		syscall.ECONNREFUSED,
		syscall.ECONNABORTED,
		syscall.EPIPE,
	} {
		if errors.Is(err, target) {
			return true
		}
	}
	message := strings.ToLower(err.Error())
	for _, text := range []string{
		"connection reset",
		"connection closed",
		"broken pipe",
		"unexpected eof",
		"server closed idle connection",
		"tls handshake timeout",
		"gateway timeout",
		"bad gateway",
		"service unavailable",
	} {
		if strings.Contains(message, text) {
			return true
		}
	}
	return false
}

func wait(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
