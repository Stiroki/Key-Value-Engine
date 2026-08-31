package ratelimit

import (
	"encoding/binary"
	"time"
)

type TokenBucket struct {
	Capacity       int
	Tokens         int
	RefillInterval time.Duration
	LastRefill     time.Time
}

func NewTokenBucket(capacity int, refillInterval time.Duration) *TokenBucket {
	return &TokenBucket{
		Capacity:       capacity,
		Tokens:         capacity,
		RefillInterval: refillInterval,
		LastRefill:     time.Now(),
	}
}

func (tb *TokenBucket) Allow() bool {
	now := time.Now()

	if now.Sub(tb.LastRefill) >= tb.RefillInterval {
		tb.Tokens = tb.Capacity
		tb.LastRefill = now
	}

	if tb.Tokens > 0 {
		tb.Tokens--
		return true
	}

	return false
}

func (tb *TokenBucket) Serialize() []byte {
	buf := make([]byte, 4+4+8+8)
	offset := 0

	binary.LittleEndian.PutUint32(buf[offset:], uint32(tb.Capacity))
	offset += 4

	binary.LittleEndian.PutUint32(buf[offset:], uint32(tb.Tokens))
	offset += 4

	binary.LittleEndian.PutUint64(buf[offset:], uint64(tb.RefillInterval))
	offset += 8

	binary.LittleEndian.PutUint64(buf[offset:], uint64(tb.LastRefill.UnixNano()))

	return buf
}

func Deserialize(data []byte) *TokenBucket {
	offset := 0

	capacity := int(binary.LittleEndian.Uint32(data[offset:]))
	offset += 4

	tokens := int(binary.LittleEndian.Uint32(data[offset:]))
	offset += 4

	refillInterval := time.Duration(binary.LittleEndian.Uint64(data[offset:]))
	offset += 8

	lastRefillNano := int64(binary.LittleEndian.Uint64(data[offset:]))

	return &TokenBucket{
		Capacity:       capacity,
		Tokens:         tokens,
		RefillInterval: refillInterval,
		LastRefill:     time.Unix(0, lastRefillNano),
	}

}
