package topolib

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"golang.org/x/time/rate"
)

type httpClient struct {
	userAgent      string
	client         *http.Client
	rateLimiter    *rate.Limiter
	circuitBreaker *circuitBreaker
}

func (h httpClient) Do(req *http.Request) (*http.Response, error) {
	var cancel context.CancelFunc

	if h.client.Timeout > 0 {
		ctx, cancelRequest := context.WithTimeout(req.Context(), h.client.Timeout)
		cancel = cancelRequest
		req = req.WithContext(ctx)
	}

	req.Header.Set("User-Agent", h.userAgent)

	resp, err := h.circuitBreaker.Do(req.Context(), func(ctx context.Context) (*http.Response, error) {
		if err := h.rateLimiter.Wait(ctx); err != nil {
			return nil, fmt.Errorf("%w: %w", ErrCircuitBreakerIgnore, err)
		}

		resp, err := h.client.Do(req.WithContext(ctx))
		if err != nil {
			if resp != nil {
				err = errors.Join(err, closeResponseBody(resp.Body))
			}

			return nil, err
		}

		if resp.StatusCode >= http.StatusBadRequest {
			return nil, errors.Join(
				fmt.Errorf("netloc has responded with %s", resp.Status),
				closeResponseBody(resp.Body),
			)
		}

		return resp, nil
	})

	if cancel != nil {
		if resp == nil || err != nil {
			cancel()
		} else {
			resp.Body = cancelOnCloseReadCloser{
				ReadCloser: resp.Body,
				cancel:     cancel,
			}
		}
	}

	return resp, err
}

type cancelOnCloseReadCloser struct {
	io.ReadCloser
	cancel context.CancelFunc
}

func (r cancelOnCloseReadCloser) Close() error {
	err := r.ReadCloser.Close()
	r.cancel()

	return err
}

func closeResponseBody(body io.ReadCloser) error {
	_, copyErr := io.Copy(io.Discard, body)

	return errors.Join(copyErr, body.Close())
}

// NewHTTPClient prepares a new HTTP client, wraps it with rate limiter,
// circuit breaker, sets a user agent etc.
//
// Please see https://pkg.go.dev/golang.org/x/time/rate to get a meaning
// of rate limiter parameters.
//
// A meaning of circuit breaker parameters:
//
// circuitBreakerOpenThreshold - this is a threshold of failures when
// circuit breaker becomes OPEN. So, if you pass 3 here, then after 3
// failures, circuit breaker switches into OPEN state and blocks access
// to a target.
//
// circuitBreakerResetFailuresTimeout - is tightly coupled with
// circuitBreakerOpenThreshold. Each time period when circuit breaker
// is closed, we try to reset a failure counter. So, if you pass 10
// here, make 2 errors then after 10 seconds this counter is going to be
// reset.
//
// circuitBreakerHalfOpenTimeout - when circuit breaker is closed, we
// open it after this time perios and it goes into HALF_OPEN state.
// Within this state we allow 1 attempt. If this attempt fails, then it
// goes into OPEN state again. If succeed - goes to CLOSED.
func NewHTTPClient(client *http.Client,
	userAgent string,
	rateLimiterInterval time.Duration,
	rateLimitBurst int,
	circuitBreakerOpenThreshold uint32,
	circuitBreakerHalfOpenTimeout, circuitBreakerResetFailuresTimeout time.Duration) HTTPClient {
	return httpClient{
		userAgent:   userAgent,
		client:      client,
		rateLimiter: rate.NewLimiter(rate.Every(rateLimiterInterval), rateLimitBurst),
		circuitBreaker: newCircuitBreaker(circuitBreakerOpenThreshold,
			circuitBreakerHalfOpenTimeout,
			circuitBreakerResetFailuresTimeout),
	}
}
