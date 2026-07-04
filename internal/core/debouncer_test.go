package core

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestDebouncer_OnlyLatestFires(t *testing.T) {
	db := NewDebouncer(40 * time.Millisecond)
	var calls int32
	for i := 0; i < 5; i++ {
		db.Call(func() { atomic.AddInt32(&calls, 1) })
		time.Sleep(5 * time.Millisecond)
	}
	time.Sleep(120 * time.Millisecond)
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Errorf("debouncer fired %d times, want 1 (latest only)", got)
	}
}

func TestDebouncer_FlushRunsPending(t *testing.T) {
	db := NewDebouncer(10 * time.Second) // long; we'll flush
	var ran int32
	db.Call(func() { atomic.AddInt32(&ran, 1) })
	db.Flush()
	if got := atomic.LoadInt32(&ran); got != 1 {
		t.Errorf("Flush should run pending call; ran=%d", got)
	}
}

func TestDebouncer_StopCancelsPending(t *testing.T) {
	db := NewDebouncer(20 * time.Millisecond)
	var ran int32
	db.Call(func() { atomic.AddInt32(&ran, 1) })
	db.Stop()
	time.Sleep(60 * time.Millisecond)
	if got := atomic.LoadInt32(&ran); got != 0 {
		t.Errorf("Stop should cancel pending call; ran=%d", got)
	}
}

func TestDebouncer_ConcurrentCalls(t *testing.T) {
	db := NewDebouncer(30 * time.Millisecond)
	var wg sync.WaitGroup
	var calls int32
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			db.Call(func() { atomic.AddInt32(&calls, 1) })
		}()
	}
	wg.Wait()
	time.Sleep(80 * time.Millisecond)
	if got := atomic.LoadInt32(&calls); got < 1 {
		t.Errorf("expected at least 1 call under concurrency; got %d", got)
	}
}
