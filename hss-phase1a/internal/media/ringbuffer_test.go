package media

import (
	"sync"
	"testing"
	"time"
)

func TestRingBuffer_PushAndSnapshot(t *testing.T) {
	rb := NewRingBuffer(4)
	if rb.Cap() != 4 {
		t.Fatalf("expected capacity 4, got %d", rb.Cap())
	}

	for i := 1; i <= 6; i++ {
		rb.Push(MediaPacket{
			PTS:        time.Duration(i) * time.Millisecond,
			IsKeyFrame: i == 4,
		})
	}

	if rb.Len() != 4 {
		t.Fatalf("expected length 4, got %d", rb.Len())
	}
	if rb.TotalPushed() != 6 {
		t.Fatalf("expected total pushed 6, got %d", rb.TotalPushed())
	}

	snap := rb.Snapshot()
	if len(snap) != 4 {
		t.Fatalf("expected snapshot len 4, got %d", len(snap))
	}

	// Oldest should be i=3, newest i=6
	expectedPTS := []time.Duration{
		3 * time.Millisecond,
		4 * time.Millisecond,
		5 * time.Millisecond,
		6 * time.Millisecond,
	}
	for idx, exp := range expectedPTS {
		if snap[idx].PTS != exp {
			t.Errorf("index %d: expected PTS %v, got %v", idx, exp, snap[idx].PTS)
		}
	}

	kfSnap := rb.SnapshotFromLatestKeyFrame()
	if len(kfSnap) != 3 {
		t.Fatalf("expected GOP snapshot from i=4 to i=6 (len 3), got %d", len(kfSnap))
	}
	if !kfSnap[0].IsKeyFrame || kfSnap[0].PTS != 4*time.Millisecond {
		t.Errorf("expected first packet in GOP snapshot to be keyframe at 4ms")
	}
}

func TestRingBuffer_Concurrency(t *testing.T) {
	rb := NewRingBuffer(64)
	var wg sync.WaitGroup

	for w := 0; w < 4; w++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for i := 0; i < 200; i++ {
				rb.Push(MediaPacket{
					PTS:        time.Duration(i) * time.Millisecond,
					IsKeyFrame: i%30 == 0,
				})
			}
		}(w)
	}

	for r := 0; r < 4; r++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 100; i++ {
				_ = rb.SnapshotFromLatestKeyFrame()
				_ = rb.Len()
			}
		}()
	}

	wg.Wait()
	if rb.TotalPushed() != 800 {
		t.Fatalf("expected 800 total pushed packets, got %d", rb.TotalPushed())
	}
}
