package app

import (
	"sync"
	"testing"
	"time"
)

func TestGroupMakesASingleCall(t *testing.T) {
	var g group
	var calls int
	var mu sync.Mutex

	start := make(chan struct{})
	var wait sync.WaitGroup
	for i := 0; i < 5; i++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			v, err := g.Do("same-key", func() (any, error) {
				mu.Lock()
				calls++
				mu.Unlock()
				time.Sleep(50 * time.Millisecond)
				return "result", nil
			})
			if err != nil || v.(string) != "result" {
				t.Errorf("unexpected result: %v %v", v, err)
			}
		}()
	}
	close(start)
	wait.Wait()

	if calls != 1 {
		t.Fatalf("expected 1 network call, got %d", calls)
	}
}

func TestGroupSeparatesKeys(t *testing.T) {
	var g group
	var calls int
	var mu sync.Mutex

	for _, key := range []string{"a", "b", "a"} {
		if _, err := g.Do(key, func() (any, error) {
			mu.Lock()
			calls++
			mu.Unlock()
			return nil, nil
		}); err != nil {
			t.Fatal(err)
		}
	}
	if calls != 3 {
		t.Fatalf("sequential calls must not be grouped, got %d", calls)
	}
}
