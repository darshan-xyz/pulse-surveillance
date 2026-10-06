package cache

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestInMemoryPubSub(t *testing.T) {
	// Ensure we are in in-memory mode
	IsConnected = false
	Client = nil

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	deviceID := "test-device-123"
	sub := SubscribeSDP(ctx, deviceID)
	defer sub.Close()

	ch := sub.Channel()

	payload := []byte(`{"type":"offer","sdp":"mock-sdp-test"}`)
	if err := PublishSDP(ctx, deviceID, payload); err != nil {
		t.Fatalf("PublishSDP failed: %v", err)
	}

	select {
	case msg := <-ch:
		if msg == nil {
			t.Fatal("Received nil message")
		}
		if msg.Payload != string(payload) {
			t.Fatalf("Expected payload %s, got %s", string(payload), msg.Payload)
		}
		if msg.Channel != "signaling:"+deviceID {
			t.Fatalf("Expected channel signaling:%s, got %s", deviceID, msg.Channel)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Timed out waiting for message on in-memory pubsub")
	}
}

func TestInMemoryPubSubMultipleSubscribers(t *testing.T) {
	IsConnected = false
	Client = nil

	ctx := context.Background()
	deviceID := "multi-sub-device"

	sub1 := SubscribeSDP(ctx, deviceID)
	defer sub1.Close()

	sub2 := SubscribeSDP(ctx, deviceID)
	defer sub2.Close()

	payload := []byte(`{"test":"multi"}`)
	if err := PublishSDP(ctx, deviceID, payload); err != nil {
		t.Fatalf("PublishSDP failed: %v", err)
	}

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		select {
		case msg := <-sub1.Channel():
			if msg.Payload != string(payload) {
				t.Errorf("sub1 got unexpected payload: %s", msg.Payload)
			}
		case <-time.After(2 * time.Second):
			t.Errorf("sub1 timed out")
		}
	}()

	go func() {
		defer wg.Done()
		select {
		case msg := <-sub2.Channel():
			if msg.Payload != string(payload) {
				t.Errorf("sub2 got unexpected payload: %s", msg.Payload)
			}
		case <-time.After(2 * time.Second):
			t.Errorf("sub2 timed out")
		}
	}()

	wg.Wait()
}

func TestInMemoryKV(t *testing.T) {
	IsConnected = false
	Client = nil

	ctx := context.Background()

	// Test Set and Get
	err := Set(ctx, "foo", "bar", 0)
	if err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	val, err := Get(ctx, "foo")
	if err != nil || val != "bar" {
		t.Fatalf("Get expected 'bar', got '%s' (err: %v)", val, err)
	}

	// Test Expiration
	err = Set(ctx, "short-lived", "temp", 50*time.Millisecond)
	if err != nil {
		t.Fatalf("Set with TTL failed: %v", err)
	}

	val, err = Get(ctx, "short-lived")
	if err != nil || val != "temp" {
		t.Fatalf("Immediate Get before expiry failed: %v", err)
	}

	time.Sleep(70 * time.Millisecond)
	_, err = Get(ctx, "short-lived")
	if err == nil {
		t.Fatalf("Expected key to expire, but Get succeeded")
	}

	// Test Del
	err = Del(ctx, "foo")
	if err != nil {
		t.Fatalf("Del failed: %v", err)
	}

	_, err = Get(ctx, "foo")
	if err == nil {
		t.Fatalf("Expected key to be deleted, but Get succeeded")
	}
}
