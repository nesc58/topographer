package topolib

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
)

type CircuitBreakerTestSuite struct {
	suite.Suite

	cb        *circuitBreaker
	cbMutex   sync.Mutex
	ctx       context.Context
	ctxCancel context.CancelFunc
}

func (suite *CircuitBreakerTestSuite) SetupTest() {
	suite.cbMutex = sync.Mutex{}
	suite.ctx, suite.ctxCancel = context.WithCancel(context.Background())
	suite.cb = newCircuitBreaker(2, 200*time.Millisecond, 500*time.Millisecond)
}

func (suite *CircuitBreakerTestSuite) CallbackOk(_ context.Context) (*http.Response, error) {
	rec := httptest.NewRecorder()

	rec.WriteHeader(http.StatusCreated)

	return rec.Result(), nil
}

func (suite *CircuitBreakerTestSuite) CallbackErr(_ context.Context) (*http.Response, error) {
	return nil, io.EOF
}

func (suite *CircuitBreakerTestSuite) CallbackIgnore(_ context.Context) (*http.Response, error) {
	return nil, ErrCircuitBreakerIgnore
}

func (suite *CircuitBreakerTestSuite) AssertResponseOk(resp *http.Response) {
	suite.NotNil(resp)
	suite.Equal(http.StatusCreated, resp.StatusCode)
}

func (suite *CircuitBreakerTestSuite) assertState(expected uint32) {
	suite.cb.stateMutexChan <- true
	defer func() {
		<-suite.cb.stateMutexChan
	}()

	suite.EqualValues(expected, suite.cb.state)
}

func (suite *CircuitBreakerTestSuite) assertFailuresCount(expected uint32) {
	suite.cb.stateMutexChan <- true
	defer func() {
		<-suite.cb.stateMutexChan
	}()

	suite.EqualValues(expected, suite.cb.failuresCount)
}

func (suite *CircuitBreakerTestSuite) TearDownTest() {
	suite.ctxCancel()

	suite.cb.stateMutexChan <- true

	suite.cb.stopTimer(&suite.cb.failuresCleanupTimer)
	suite.cb.stopTimer(&suite.cb.halfOpenTimer)
}

func (suite *CircuitBreakerTestSuite) TestManyExecuted() {
	wg := &sync.WaitGroup{}

	wg.Add(5)

	go func() {
		wg.Wait()
		suite.ctxCancel()
	}()

	for i := 0; i < 5; i++ {
		go func() {
			defer wg.Done()

			resp, err := suite.cb.Do(suite.ctx, suite.CallbackOk)

			suite.cbMutex.Lock()
			defer suite.cbMutex.Unlock()

			suite.NoError(err)
			suite.AssertResponseOk(resp)
		}()
	}

	suite.Eventually(func() bool {
		_, ok := <-suite.ctx.Done()

		return !ok
	}, 500*time.Second, 10*time.Millisecond)
}

func (suite *CircuitBreakerTestSuite) TestSomeFailuresButStillWorks() {
	wg := &sync.WaitGroup{}

	wg.Add(5)

	go func() {
		wg.Wait()
		suite.ctxCancel()
	}()

	_, err := suite.cb.Do(suite.ctx, suite.CallbackErr)

	suite.Error(err)

	for i := 0; i < 5; i++ {
		go func() {
			defer wg.Done()

			resp, err := suite.cb.Do(suite.ctx, suite.CallbackOk)

			suite.cbMutex.Lock()
			defer suite.cbMutex.Unlock()

			suite.NoError(err)
			suite.AssertResponseOk(resp)
		}()
	}

	suite.Eventually(func() bool {
		_, ok := <-suite.ctx.Done()

		return !ok
	}, 500*time.Second, 10*time.Millisecond)
	suite.assertFailuresCount(0)
	suite.assertState(circuitBreakerStateClosed)
}

func (suite *CircuitBreakerTestSuite) TestSomeFailuresButStillClosed() {
	_, err := suite.cb.Do(suite.ctx, suite.CallbackErr)

	suite.Error(err)
	suite.assertFailuresCount(1)
	suite.assertState(circuitBreakerStateClosed)

	_, err = suite.cb.Do(suite.ctx, suite.CallbackErr)

	suite.Error(err)
	suite.assertFailuresCount(2)
	suite.assertState(circuitBreakerStateClosed)

	_, err = suite.cb.Do(suite.ctx, suite.CallbackErr)

	suite.Error(err)
	suite.assertFailuresCount(0)
	suite.assertState(circuitBreakerStateOpened)
}

func (suite *CircuitBreakerTestSuite) TestClosedFailureReset() {
	suite.cb.Do(suite.ctx, suite.CallbackErr) // nolint: errcheck
	suite.cb.Do(suite.ctx, suite.CallbackErr) // nolint: errcheck

	time.Sleep(time.Second)

	suite.assertFailuresCount(0)
	suite.assertState(circuitBreakerStateClosed)
}

func (suite *CircuitBreakerTestSuite) TestOpenedExecute() {
	suite.cb.Do(suite.ctx, suite.CallbackErr) // nolint: errcheck
	suite.cb.Do(suite.ctx, suite.CallbackErr) // nolint: errcheck
	suite.cb.Do(suite.ctx, suite.CallbackErr) // nolint: errcheck

	_, err := suite.cb.Do(suite.ctx, suite.CallbackOk) // nolint: errcheck

	suite.Error(err)
	suite.assertState(circuitBreakerStateOpened)
}

func (suite *CircuitBreakerTestSuite) TestOpenedIgnore() {
	suite.cb.Do(suite.ctx, suite.CallbackIgnore) // nolint: errcheck
	suite.cb.Do(suite.ctx, suite.CallbackIgnore) // nolint: errcheck
	suite.cb.Do(suite.ctx, suite.CallbackIgnore) // nolint: errcheck
	suite.cb.Do(suite.ctx, suite.CallbackIgnore) // nolint: errcheck

	_, err := suite.cb.Do(suite.ctx, suite.CallbackOk) // nolint: errcheck

	suite.NoError(err)
	suite.assertState(circuitBreakerStateClosed)
}

func (suite *CircuitBreakerTestSuite) TestHalfOpened() {
	suite.cb.Do(suite.ctx, suite.CallbackErr) // nolint: errcheck
	suite.cb.Do(suite.ctx, suite.CallbackErr) // nolint: errcheck
	suite.cb.Do(suite.ctx, suite.CallbackErr) // nolint: errcheck

	time.Sleep(700 * time.Millisecond)

	suite.assertState(circuitBreakerStateHalfOpened)
}

func (suite *CircuitBreakerTestSuite) TestHalfOpenedErr() {
	suite.cb.Do(suite.ctx, suite.CallbackErr) // nolint: errcheck
	suite.cb.Do(suite.ctx, suite.CallbackErr) // nolint: errcheck
	suite.cb.Do(suite.ctx, suite.CallbackErr) // nolint: errcheck

	time.Sleep(700 * time.Millisecond)

	suite.cb.Do(suite.ctx, suite.CallbackErr) // nolint: errcheck

	suite.assertState(circuitBreakerStateOpened)
}

func (suite *CircuitBreakerTestSuite) TestHalfOpenedOk() {
	suite.cb.Do(suite.ctx, suite.CallbackErr) // nolint: errcheck
	suite.cb.Do(suite.ctx, suite.CallbackErr) // nolint: errcheck
	suite.cb.Do(suite.ctx, suite.CallbackErr) // nolint: errcheck

	time.Sleep(700 * time.Millisecond)

	suite.cb.Do(suite.ctx, suite.CallbackOk) // nolint: errcheck

	suite.assertState(circuitBreakerStateClosed)
}

func (suite *CircuitBreakerTestSuite) TestCheckConcurrentExecutionInHalfOpened() {
	suite.cb.Do(suite.ctx, suite.CallbackErr) // nolint: errcheck
	suite.cb.Do(suite.ctx, suite.CallbackErr) // nolint: errcheck
	suite.cb.Do(suite.ctx, suite.CallbackErr) // nolint: errcheck

	time.Sleep(700 * time.Millisecond)

	go suite.cb.Do(suite.ctx, func(_ context.Context) (*http.Response, error) { // nolint: errcheck
		time.Sleep(500 * time.Millisecond)

		return nil, nil
	})

	time.Sleep(10 * time.Millisecond)

	_, err := suite.cb.Do(suite.ctx, suite.CallbackOk) // nolint: errcheck

	suite.Error(err)
	time.Sleep(time.Second)
	suite.assertState(circuitBreakerStateClosed)
}

func TestCircuitBreaker(t *testing.T) {
	suite.Run(t, &CircuitBreakerTestSuite{})
}
