package cache

import (
	"context"
	"container/list"
	"sync"
	"time"
	"github.com/vineet-motwani/Tross-Hiring/models"
)

type entry struct {
	key       string
	response  *models.ProfileResponse
	expiresAt time.Time
}

type ProfileCache struct {
	ttl        time.Duration
	maxEntries int
	items      map[string]*list.Element
	evictList  *list.List
	lock       sync.Mutex
}

func NewProfileCache(ttlSeconds int, maxEntries int) *ProfileCache {
	return &ProfileCache{
		ttl:        time.Duration(ttlSeconds) * time.Second,
		maxEntries: maxEntries,
		items:      make(map[string]*list.Element),
		evictList:  list.New(),
	}
}

func (c *ProfileCache) Get(ctx context.Context, key string) (*models.ProfileResponse, error) {
	if c.ttl == 0 {
		return nil, nil
	}
	c.lock.Lock()
	defer c.lock.Unlock()

	if ent, ok := c.items[key]; ok {
		e := ent.Value.(*entry)
		if time.Now().After(e.expiresAt) {
			c.evictList.Remove(ent)
			delete(c.items, key)
			return nil, nil
		}
		c.evictList.MoveToFront(ent)

		// Create a shallow copy and update cached flag
		respCopy := *e.response
		respCopy.Meta.Cached = true
		return &respCopy, nil
	}
	return nil, nil
}

func (c *ProfileCache) Set(ctx context.Context, key string, response *models.ProfileResponse) error {
	if c.ttl == 0 {
		return nil
	}
	c.lock.Lock()
	defer c.lock.Unlock()

	if ent, ok := c.items[key]; ok {
		c.evictList.MoveToFront(ent)
		e := ent.Value.(*entry)
		e.response = response
		e.expiresAt = time.Now().Add(c.ttl)
		return nil
	}

	ent := &entry{
		key:       key,
		response:  response,
		expiresAt: time.Now().Add(c.ttl),
	}
	elem := c.evictList.PushFront(ent)
	c.items[key] = elem

	for c.evictList.Len() > c.maxEntries {
		c.removeOldest()
	}
	return nil
}

func (c *ProfileCache) removeOldest() {
	ent := c.evictList.Back()
	if ent != nil {
		c.evictList.Remove(ent)
		e := ent.Value.(*entry)
		delete(c.items, e.key)
	}
}
