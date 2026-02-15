package gitlab

import (
	"context"
	"math"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
	"golang.org/x/time/rate"
)

// RateLimiter combines a local token bucket with header-aware backoff
// derived from GitLab's RateLimit-Remaining and RateLimit-Reset response headers.
// It is safe for concurrent use.
type RateLimiter struct {
	mu sync.Mutex

	// local is the token-bucket limiter used for outbound request pacing.
	local *rate.Limiter

	// headerReset is the time at which the remote rate limit resets,
	// parsed from the RateLimit-Reset header.
	headerReset time.Time

	// headerRemaining is the last observed RateLimit-Remaining value.
	headerRemaining int

	// backoffUntil is the time until which we should wait because the
	// remote limit is nearly (or fully) exhausted.
	backoffUntil time.Time

	logger *logrus.Entry
}

// NewRateLimiter creates a RateLimiter with the given requests-per-second and burst.
// A zero or negative rps disables local rate limiting (unlimited).
func NewRateLimiter(rps int, burst int, logger *logrus.Entry) *RateLimiter {
	var limiter *rate.Limiter
	if rps <= 0 {
		limiter = rate.NewLimiter(rate.Inf, 0)
	} else {
		if burst < 1 {
			burst = 1
		}
		limiter = rate.NewLimiter(rate.Limit(rps), burst)
	}
	return &RateLimiter{
		local:           limiter,
		headerRemaining: -1, // unknown
		logger:          logger,
	}
}

// Wait blocks until the rate limiter allows one more request, honouring
// both the local token bucket and any header-derived backoff.  It returns
// ctx.Err() if the context expires while waiting.
func (rl *RateLimiter) Wait(ctx context.Context) error {
	// 1. Honour header-based backoff first.
	rl.mu.Lock()
	backoff := rl.backoffUntil
	rl.mu.Unlock()

	if !backoff.IsZero() && time.Now().Before(backoff) {
		delay := time.Until(backoff)
		rl.logger.WithField("delay", delay.Round(time.Millisecond)).
			Debug("rate limiter: waiting for header-based backoff")
		select {
		case <-time.After(delay):
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	// 2. Then wait for the local token bucket.
	if err := rl.local.Wait(ctx); err != nil {
		return err
	}

	return nil
}

// UpdateFromHeaders inspects the HTTP response headers and adjusts the
// internal backoff state when the remote limit is close to exhaustion.
//
// Recognised headers (case-insensitive via http.Header canonical form):
//
//	RateLimit-Remaining – number of requests remaining in the current window.
//	RateLimit-Reset     – Unix epoch timestamp when the window resets.
//	Retry-After         – seconds to wait (sent on 429 responses).
func (rl *RateLimiter) UpdateFromHeaders(headers http.Header) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	// Parse Retry-After (highest priority – returned on 429).
	if ra := headers.Get("Retry-After"); ra != "" {
		if sec, err := strconv.Atoi(ra); err == nil && sec > 0 {
			until := time.Now().Add(time.Duration(sec) * time.Second)
			if until.After(rl.backoffUntil) {
				rl.backoffUntil = until
				rl.logger.WithField("retry_after_sec", sec).
					Warn("rate limiter: 429 received, backing off")
			}
			return
		}
	}

	// Parse RateLimit-Remaining.
	remainStr := headers.Get("RateLimit-Remaining")
	if remainStr == "" {
		return
	}
	remaining, err := strconv.Atoi(remainStr)
	if err != nil {
		return
	}
	rl.headerRemaining = remaining

	// Parse RateLimit-Reset (Unix epoch seconds).
	resetStr := headers.Get("RateLimit-Reset")
	if resetStr == "" {
		return
	}
	resetEpoch, err := strconv.ParseInt(resetStr, 10, 64)
	if err != nil {
		return
	}
	rl.headerReset = time.Unix(resetEpoch, 0)

	// If remaining requests are dangerously low, set a backoff.
	if remaining <= 0 {
		// Exhausted – wait until reset.
		if rl.headerReset.After(rl.backoffUntil) {
			rl.backoffUntil = rl.headerReset
			rl.logger.Warn("rate limiter: remote limit exhausted, backing off until reset")
		}
	} else if remaining < 10 {
		// Nearly exhausted – spread remaining requests over the window.
		untilReset := time.Until(rl.headerReset)
		if untilReset > 0 {
			perRequest := time.Duration(math.Ceil(float64(untilReset) / float64(remaining+1)))
			until := time.Now().Add(perRequest)
			if until.After(rl.backoffUntil) {
				rl.backoffUntil = until
				rl.logger.WithFields(logrus.Fields{
					"remaining": remaining,
					"delay":     perRequest.Round(time.Millisecond),
				}).Debug("rate limiter: throttling near exhaustion")
			}
		}
	}
}

// Remaining returns the last observed RateLimit-Remaining value, or -1 if
// no header has been seen yet.
func (rl *RateLimiter) Remaining() int {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	return rl.headerRemaining
}

// ResetAt returns the time at which the remote rate limit window resets.
func (rl *RateLimiter) ResetAt() time.Time {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	return rl.headerReset
}
