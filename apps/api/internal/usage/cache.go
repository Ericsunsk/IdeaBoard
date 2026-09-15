package usage

import (
	"container/list"
	"fmt"
	"sync"
	"time"
)

const (
	DefaultCacheTTL      = 30 * time.Second
	DefaultCacheCapacity = 2_048
)

type cacheKey struct {
	userID     int64
	usageRange Range
}

// String renders the singleflight de-duplication key. cacheKey itself stays
// a comparable struct for the map index; singleflight requires a string.
func (k cacheKey) String() string {
	return fmt.Sprintf("%d/%s", k.userID, k.usageRange)
}

type cacheEntry struct {
	key       cacheKey
	value     Usage
	expiresAt time.Time
}

type usageCache struct {
	mu       sync.Mutex
	entries  map[cacheKey]*list.Element
	recency  *list.List
	capacity int
	ttl      time.Duration
	now      func() time.Time
}

func newUsageCache(capacity int, ttl time.Duration, now func() time.Time) *usageCache {
	if capacity <= 0 {
		capacity = DefaultCacheCapacity
	}
	if ttl <= 0 {
		ttl = DefaultCacheTTL
	}
	if now == nil {
		now = time.Now
	}
	return &usageCache{
		entries:  make(map[cacheKey]*list.Element, capacity),
		recency:  list.New(),
		capacity: capacity,
		ttl:      ttl,
		now:      now,
	}
}

func (c *usageCache) Get(key cacheKey) (Usage, bool) {
	if c == nil {
		return Usage{}, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	element, ok := c.entries[key]
	if !ok {
		return Usage{}, false
	}
	entry := element.Value.(*cacheEntry)
	if !c.now().Before(entry.expiresAt) {
		c.remove(element)
		return Usage{}, false
	}
	c.recency.MoveToFront(element)
	return cloneUsage(entry.value), true
}

func (c *usageCache) Set(key cacheKey, value Usage) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	expiresAt := c.now().Add(c.ttl)
	if element, ok := c.entries[key]; ok {
		entry := element.Value.(*cacheEntry)
		entry.value = cloneUsage(value)
		entry.expiresAt = expiresAt
		c.recency.MoveToFront(element)
		return
	}
	for len(c.entries) >= c.capacity {
		c.remove(c.recency.Back())
	}
	entry := &cacheEntry{key: key, value: cloneUsage(value), expiresAt: expiresAt}
	element := c.recency.PushFront(entry)
	c.entries[key] = element
}

func (c *usageCache) remove(element *list.Element) {
	if element == nil {
		return
	}
	entry := element.Value.(*cacheEntry)
	delete(c.entries, entry.key)
	c.recency.Remove(element)
}

func cloneUsage(value Usage) Usage {
	cloned := value
	cloned.Trend = append([]DailyUsage(nil), value.Trend...)
	return cloned
}
