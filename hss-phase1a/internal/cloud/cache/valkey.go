package cache

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

var (
	Client      *redis.Client
	IsConnected bool
	inMemBroker = newInMemoryBroker()
	inMemKV     = newInMemoryKV()
)

// PubSubSubscription wraps either a Redis PubSub or an In-Memory subscription
type PubSubSubscription struct {
	redisPubSub *redis.PubSub
	msgChan     chan *redis.Message
	closeOnce   sync.Once
	closed      chan struct{}
	unsubscribe func()
}

func (s *PubSubSubscription) Channel() <-chan *redis.Message {
	if s.redisPubSub != nil {
		return s.redisPubSub.Channel()
	}
	return s.msgChan
}

func (s *PubSubSubscription) Close() error {
	if s.redisPubSub != nil {
		return s.redisPubSub.Close()
	}
	var err error
	s.closeOnce.Do(func() {
		close(s.closed)
		if s.unsubscribe != nil {
			s.unsubscribe()
		}
	})
	return err
}

type subscriber struct {
	ch     chan *redis.Message
	closed bool
	mu     sync.Mutex
}

type inMemoryBroker struct {
	mu          sync.RWMutex
	subscribers map[string]map[*subscriber]struct{}
}

func newInMemoryBroker() *inMemoryBroker {
	return &inMemoryBroker{
		subscribers: make(map[string]map[*subscriber]struct{}),
	}
}

func (b *inMemoryBroker) publish(channel string, payload []byte) {
	b.mu.RLock()
	subsMap, ok := b.subscribers[channel]
	if !ok || len(subsMap) == 0 {
		b.mu.RUnlock()
		return
	}

	subs := make([]*subscriber, 0, len(subsMap))
	for sub := range subsMap {
		subs = append(subs, sub)
	}
	b.mu.RUnlock()

	msg := &redis.Message{
		Channel: channel,
		Payload: string(payload),
	}

	for _, sub := range subs {
		sub.mu.Lock()
		if !sub.closed {
			select {
			case sub.ch <- msg:
			default:
				// Non-blocking overflow handling: drop oldest to prevent slow consumers from hanging publisher
				select {
				case <-sub.ch:
				default:
				}
				select {
				case sub.ch <- msg:
				default:
				}
			}
		}
		sub.mu.Unlock()
	}
}

func (b *inMemoryBroker) subscribe(ctx context.Context, channel string) *PubSubSubscription {
	sub := &subscriber{
		ch: make(chan *redis.Message, 128),
	}

	b.mu.Lock()
	if b.subscribers[channel] == nil {
		b.subscribers[channel] = make(map[*subscriber]struct{})
	}
	b.subscribers[channel][sub] = struct{}{}
	b.mu.Unlock()

	subObj := &PubSubSubscription{
		msgChan: sub.ch,
		closed:  make(chan struct{}),
		unsubscribe: func() {
			sub.mu.Lock()
			if !sub.closed {
				sub.closed = true
				close(sub.ch)
			}
			sub.mu.Unlock()

			b.mu.Lock()
			if subs, ok := b.subscribers[channel]; ok {
				delete(subs, sub)
				if len(subs) == 0 {
					delete(b.subscribers, channel)
				}
			}
			b.mu.Unlock()
		},
	}

	if ctx != nil && ctx.Done() != nil {
		go func() {
			select {
			case <-ctx.Done():
				_ = subObj.Close()
			case <-subObj.closed:
			}
		}()
	}

	return subObj
}

// In-Memory Key-Value Cache
type inMemoryKV struct {
	mu    sync.RWMutex
	store map[string]cacheItem
}

type cacheItem struct {
	value      string
	expiration time.Time
}

func newInMemoryKV() *inMemoryKV {
	return &inMemoryKV{
		store: make(map[string]cacheItem),
	}
}

func (kv *inMemoryKV) set(key, value string, expiration time.Duration) {
	kv.mu.Lock()
	defer kv.mu.Unlock()
	var exp time.Time
	if expiration > 0 {
		exp = time.Now().Add(expiration)
	}
	kv.store[key] = cacheItem{value: value, expiration: exp}
}

func (kv *inMemoryKV) get(key string) (string, bool) {
	kv.mu.RLock()
	defer kv.mu.RUnlock()
	item, exists := kv.store[key]
	if !exists {
		return "", false
	}
	if !item.expiration.IsZero() && time.Now().After(item.expiration) {
		return "", false
	}
	return item.value, true
}

func (kv *inMemoryKV) del(keys ...string) {
	kv.mu.Lock()
	defer kv.mu.Unlock()
	for _, k := range keys {
		delete(kv.store, k)
	}
}

func InitValkey(addr string) error {
	Client = redis.NewClient(&redis.Options{
		Addr: addr,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, err := Client.Ping(ctx).Result()
	if err != nil {
		IsConnected = false
		log.Printf("Failed to connect to Valkey (Redis) at %s: %v. Running with in-memory fallback.", addr, err)
		return err
	}

	IsConnected = true
	log.Println("Connected to Valkey (Redis)")
	return nil
}

// PublishSDP broadcasts a signaling payload to the specific device's channel
func PublishSDP(ctx context.Context, deviceID string, payload []byte) error {
	channel := "signaling:" + deviceID
	if IsConnected && Client != nil {
		if err := Client.Publish(ctx, channel, payload).Err(); err == nil {
			return nil
		}
		log.Printf("Valkey publish failed for %s, falling back to in-memory broker", channel)
	}
	inMemBroker.publish(channel, payload)
	return nil
}

// SubscribeSDP subscribes to a specific device's signaling channel
func SubscribeSDP(ctx context.Context, deviceID string) *PubSubSubscription {
	channel := "signaling:" + deviceID
	if IsConnected && Client != nil {
		rps := Client.Subscribe(ctx, channel)
		return &PubSubSubscription{
			redisPubSub: rps,
		}
	}
	return inMemBroker.subscribe(ctx, channel)
}

// Set stores a key-value pair in Redis or in-memory cache
func Set(ctx context.Context, key string, value interface{}, expiration time.Duration) error {
	if IsConnected && Client != nil {
		return Client.Set(ctx, key, value, expiration).Err()
	}
	if str, ok := value.(string); ok {
		inMemKV.set(key, str, expiration)
	} else if b, ok := value.([]byte); ok {
		inMemKV.set(key, string(b), expiration)
	}
	return nil
}

// Get retrieves a key-value pair from Redis or in-memory cache
func Get(ctx context.Context, key string) (string, error) {
	if IsConnected && Client != nil {
		return Client.Get(ctx, key).Result()
	}
	val, ok := inMemKV.get(key)
	if !ok {
		return "", redis.Nil
	}
	return val, nil
}

// Del deletes keys from Redis or in-memory cache
func Del(ctx context.Context, keys ...string) error {
	if IsConnected && Client != nil {
		return Client.Del(ctx, keys...).Err()
	}
	inMemKV.del(keys...)
	return nil
}

// BlacklistToken stores a revoked token identifier (JTI) in cache until its expiration
func BlacklistToken(ctx context.Context, jti string, expiration time.Duration) error {
	if jti == "" {
		return nil
	}
	if expiration <= 0 {
		expiration = 24 * time.Hour
	}
	return Set(ctx, "blacklist:token:"+jti, "revoked", expiration)
}

// IsTokenBlacklisted returns true if the token identifier (JTI) is in the revocation blacklist
func IsTokenBlacklisted(ctx context.Context, jti string) bool {
	if jti == "" {
		return false
	}
	val, err := Get(ctx, "blacklist:token:"+jti)
	return err == nil && val == "revoked"
}

