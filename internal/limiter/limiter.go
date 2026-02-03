package limiter

import (
	"sync"
	"time"

	lru "github.com/hashicorp/golang-lru/v2"
)

type LRULimiter struct {
	cache *lru.Cache[uint64, int64]
	mu    sync.Mutex
}

func NewLRULimiter(size int) *LRULimiter {
	c, _ := lru.New[uint64, int64](size)
	return &LRULimiter{cache: c}
}

func (l *LRULimiter) Allow(id uint64) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now().Unix()
	if last, ok := l.cache.Get(id); ok {
		if last == now {
			return false
		}
	}
	l.cache.Add(id, now)
	return true
}
