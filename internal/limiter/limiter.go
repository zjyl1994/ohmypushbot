package limiter

import (
	"sync"
	"time"

	lru "github.com/hashicorp/golang-lru/v2"
)

type LRULimiter[K comparable] struct {
	cache *lru.Cache[K, int64]
	mu    sync.Mutex
}

func NewLRULimiter[K comparable](size int) *LRULimiter[K] {
	c, _ := lru.New[K, int64](size)
	return &LRULimiter[K]{cache: c}
}

func (l *LRULimiter[K]) Allow(id K) bool {
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
