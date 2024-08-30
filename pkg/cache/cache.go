// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0
package cache

import (
	"sync"
	"time"
)

type cacheEntry struct {
	value      any
	expiration int64
}

type Cache struct {
	items map[string]cacheEntry
	mutex sync.RWMutex
}

func (c *Cache) Set(key string, value any, duration time.Duration) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.items[key] = cacheEntry{value: value, expiration: time.Now().Add(duration).Unix()}
}

func (c *Cache) Get(key string) any {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	entry, ok := c.items[key]
	if !ok || time.Now().Unix() > entry.expiration {
		return nil
	}

	return entry.value
}

func New() *Cache {
	return &Cache{
		items: make(map[string]cacheEntry),
	}
}
