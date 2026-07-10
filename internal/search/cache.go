package search

import (
	"sync"
	"time"
)

// SearchCache 带 TTL 的线程安全内存缓存
// 用于缓存搜索结果和网页抓取内容（设计文档 8.1 节）
type SearchCache struct {
	store    sync.Map
	stopCh   chan struct{}
	interval time.Duration
	stopOnce sync.Once
}

// NewSearchCache 创建缓存实例并启动后台清理 goroutine
// cleanupInterval: 过期条目清理间隔，建议 1-5 分钟
func NewSearchCache(cleanupInterval time.Duration) *SearchCache {
	c := &SearchCache{
		stopCh:   make(chan struct{}),
		interval: cleanupInterval,
	}
	go c.cleanupLoop()
	return c
}

// Get 读取缓存值，若不存在或已过期返回 (nil, false)
func (c *SearchCache) Get(key string) (any, bool) {
	val, ok := c.store.Load(key)
	if !ok {
		return nil, false
	}
	item := val.(*cachedItem)
	if time.Now().After(item.ExpiresAt) {
		c.store.Delete(key)
		return nil, false
	}
	return item.Data, true
}

// Set 写入缓存值，ttl 为有效期
func (c *SearchCache) Set(key string, data any, ttl time.Duration) {
	item := &cachedItem{
		Data:      data,
		ExpiresAt: time.Now().Add(ttl),
	}
	c.store.Store(key, item)
}

// Delete 删除指定 key
func (c *SearchCache) Delete(key string) {
	c.store.Delete(key)
}

// CleanExpired 遍历并删除所有过期条目（公开方法，也供后台 goroutine 调用）
func (c *SearchCache) CleanExpired() {
	c.store.Range(func(key, val any) bool {
		item := val.(*cachedItem)
		if time.Now().After(item.ExpiresAt) {
			c.store.Delete(key)
		}
		return true
	})
}

// Len 返回当前缓存条目数（含已过期但尚未清理的条目）
func (c *SearchCache) Len() int {
	count := 0
	c.store.Range(func(_, _ any) bool {
		count++
		return true
	})
	return count
}

// Stop 停止后台清理 goroutine（可重复调用）
func (c *SearchCache) Stop() {
	c.stopOnce.Do(func() {
		close(c.stopCh)
	})
}

// cleanupLoop 后台定期清理过期条目
func (c *SearchCache) cleanupLoop() {
	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()
	for {
		select {
		case <-c.stopCh:
			return
		case <-ticker.C:
			c.CleanExpired()
		}
	}
}
