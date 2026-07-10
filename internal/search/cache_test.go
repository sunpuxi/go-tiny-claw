package search

import (
	"sync"
	"testing"
	"time"
)

func TestCacheSetGet(t *testing.T) {
	c := NewSearchCache(1 * time.Minute)
	defer c.Stop()

	c.Set("key1", "value1", 10*time.Second)
	val, ok := c.Get("key1")
	if !ok {
		t.Fatal("expected cache hit, got miss")
	}
	if val != "value1" {
		t.Errorf("expected 'value1', got '%v'", val)
	}
}

func TestCacheGetNonExistent(t *testing.T) {
	c := NewSearchCache(1 * time.Minute)
	defer c.Stop()

	_, ok := c.Get("nonexistent")
	if ok {
		t.Error("expected cache miss for nonexistent key")
	}
}

func TestCacheExpiry(t *testing.T) {
	c := NewSearchCache(1 * time.Minute)
	defer c.Stop()

	c.Set("key1", "value1", 10*time.Millisecond)

	// 等待过期
	time.Sleep(50 * time.Millisecond)

	_, ok := c.Get("key1")
	if ok {
		t.Error("expected cache miss for expired key")
	}
}

func TestCacheNonExpiry(t *testing.T) {
	c := NewSearchCache(1 * time.Minute)
	defer c.Stop()

	c.Set("key1", "value1", 5*time.Second)

	val, ok := c.Get("key1")
	if !ok {
		t.Fatal("expected cache hit within TTL")
	}
	if val != "value1" {
		t.Errorf("expected 'value1', got '%v'", val)
	}
}

func TestCacheDelete(t *testing.T) {
	c := NewSearchCache(1 * time.Minute)
	defer c.Stop()

	c.Set("key1", "value1", 10*time.Second)
	c.Delete("key1")

	_, ok := c.Get("key1")
	if ok {
		t.Error("expected cache miss after delete")
	}
}

func TestCacheOverwrite(t *testing.T) {
	c := NewSearchCache(1 * time.Minute)
	defer c.Stop()

	c.Set("key1", "first", 10*time.Second)
	c.Set("key1", "second", 10*time.Second)

	val, ok := c.Get("key1")
	if !ok {
		t.Fatal("expected cache hit")
	}
	if val != "second" {
		t.Errorf("expected 'second', got '%v'", val)
	}
}

func TestCacheCleanExpired(t *testing.T) {
	c := NewSearchCache(1 * time.Minute)
	defer c.Stop()

	c.Set("expired", "old", 1*time.Millisecond)
	c.Set("fresh", "new", 10*time.Second)

	time.Sleep(50 * time.Millisecond)

	c.CleanExpired()

	_, ok := c.Get("expired")
	if ok {
		t.Error("expected expired key to be gone after CleanExpired")
	}

	val, ok := c.Get("fresh")
	if !ok {
		t.Fatal("expected fresh key to survive CleanExpired")
	}
	if val != "new" {
		t.Errorf("expected 'new', got '%v'", val)
	}
}

func TestCacheLen(t *testing.T) {
	c := NewSearchCache(1 * time.Minute)
	defer c.Stop()

	if c.Len() != 0 {
		t.Errorf("expected empty cache, got len=%d", c.Len())
	}

	c.Set("a", 1, 10*time.Second)
	c.Set("b", 2, 10*time.Second)

	if c.Len() != 2 {
		t.Errorf("expected len=2, got len=%d", c.Len())
	}
}

func TestCacheConcurrent(t *testing.T) {
	c := NewSearchCache(1 * time.Minute)
	defer c.Stop()

	var wg sync.WaitGroup
	numGoroutines := 50

	// 并发写入
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			key := "key" + string(rune('A'+idx%26))
			c.Set(key, idx, 10*time.Second)
		}(i)
	}

	// 并发读取
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			key := "key" + string(rune('A'+idx%26))
			c.Get(key)
		}(i)
	}

	wg.Wait()
	// 如果没有 panic 或数据竞争，测试通过
}

func TestCacheWithStructValue(t *testing.T) {
	c := NewSearchCache(1 * time.Minute)
	defer c.Stop()

	type testData struct {
		Name  string
		Count int
	}

	original := testData{Name: "test", Count: 42}
	c.Set("struct_key", &original, 10*time.Second)

	val, ok := c.Get("struct_key")
	if !ok {
		t.Fatal("expected cache hit for struct value")
	}

	retrieved := val.(*testData)
	if retrieved.Name != "test" || retrieved.Count != 42 {
		t.Errorf("expected {test 42}, got {%v %v}", retrieved.Name, retrieved.Count)
	}
}

func TestCacheStop(t *testing.T) {
	c := NewSearchCache(50 * time.Millisecond)
	c.Stop()

	// 确保 Stop 可重复调用（不 panic）
	c.Stop()
}
