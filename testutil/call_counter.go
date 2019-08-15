package testutil

import (
	"errors"
	"sync"
)

// CallCounter provides a mechanism for one goroutine to increment a counter,
// usually once per some method call, and for other goroutines to block until
// the counter reaches some expected value.
type CallCounter struct {
	cond *sync.Cond
	n    int
}

func NewCallCounter() *CallCounter {
	return &CallCounter{
		cond: sync.NewCond(&sync.Mutex{}),
	}
}

// Incr increments the call counter and unblocks any waiters whose expected
// call counter is now met. It is safe to this method on the zero value since
// both test and non-test code is expected to call this. This method is safe to
// be used from multiple goroutines.
func (c *CallCounter) Incr() {
	if c == nil {
		// Incr is safe to be called on zero value
		return
	}

	l := c.cond.L
	l.Lock()
	c.n++
	l.Unlock()
	c.cond.Broadcast()
}

// Wait blocks the calling goroutine until the callCounter reaches atleast
// expected number of calls. Since this method is only expected to be called by
// test code, it will panic if called on the zero value as way to alert the
// user that they forgot to initialize the callCounter. This method is safe to
// be called from multiple goroutines.
func (c *CallCounter) Wait(expected int) {
	if c == nil {
		panic(errors.New("callCounter.Wait called on zero value"))
	}

	l := c.cond.L
	l.Lock()
	defer l.Unlock()
	for c.n < expected {
		c.cond.Wait()
	}
}
