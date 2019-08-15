package testutil

import (
	"math/rand"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestCallCounterDoesNotBlockOnZero(t *testing.T) {
	c := NewCallCounter()
	c.Wait(0) // shouldn't block forever
}

func TestCallCounterIncrOnZeroValue(t *testing.T) {
	var zero *CallCounter
	assert.NotPanics(t, zero.Incr, "expected zero value of *callCounter to not panic")
}

func TestCallCounterWaitOnZeroValue(t *testing.T) {
	var zero *CallCounter
	assert.Panics(t, func() { zero.Wait(1) }, "expected zero value of *callCounter to panic")
}

func TestCallCounterBlocks(t *testing.T) {
	c := NewCallCounter()

	done := make(chan struct{}, 1)
	go func() {
		c.Wait(2)
		done <- struct{}{}
	}()

	// first Incr shouldn't unblock Wait(2)
	c.Incr()
	runtime.Gosched() // yield the processor
	assert.Len(t, done, 0)

	// second Incr should unblock Wait(2)
	c.Incr()
	<-done
}

func TestCallCounterDoesNotBlockWithLowCount(t *testing.T) {
	c := NewCallCounter()
	c.Incr()
	c.Incr()
	c.Wait(1) // shouldn't block forever
}

func TestCallCounterFuzzer(t *testing.T) {
	c := NewCallCounter()

	done := make(chan struct{})
	for i := 0; i < 3; i++ {
		go func() {
			for {
				select {
				case <-done:
					return
				default:
					d := rand.Intn(100)
					time.Sleep(time.Duration(d) * time.Millisecond)
					c.Incr()
				}
			}
		}()
	}

	c.Wait(10) // shouldn't block forever
	close(done)
}
