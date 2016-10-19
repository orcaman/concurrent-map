package cmap

import (
	"encoding/json"
	"strings"
	"sync"
)

type ConcurrentHashMap struct {
	Shards  int
	HashMap ConcurrentMap
}

// A "thread" safe map of type string:Anything.
// To avoid lock bottlenecks this map is dived to several (Shards) map shards.
type ConcurrentMap []*ConcurrentMapShared

// A "thread" safe string to anything map.
type ConcurrentMapShared struct {
	items        map[string]interface{}
	sync.RWMutex // Read Write mutex, guards access to internal map.
}

// Creates a new concurrent map.
func New(shards int) *ConcurrentHashMap {
	m := &ConcurrentHashMap{Shards: shards, HashMap: make(ConcurrentMap, shards)}
	for i := 0; i < shards; i++ {
		m.HashMap[i] = &ConcurrentMapShared{items: make(map[string]interface{})}
	}
	return m
}

// Returns shard under given key
func (m *ConcurrentHashMap) GetShard(key string) *ConcurrentMapShared {
	return m.HashMap[uint(fnv32(key))%uint(m.Shards)]
}

// Sets the given map
func (m *ConcurrentHashMap) MSet(data map[string]interface{}) {
	for key, value := range data {
		shard := m.GetShard(key)
		shard.Lock()
		shard.items[key] = value
		shard.Unlock()
	}
}

// Sets the given value under the specified key.
func (m *ConcurrentHashMap) Set(key string, value interface{}) {
	// Get map shard.
	shard := m.GetShard(key)
	shard.Lock()
	shard.items[key] = value
	shard.Unlock()
}

// Callback to return new element to be inserted into the map
// It is called while lock is held, therefore it MUST NOT
// try to access other keys in same map, as it can lead to deadlock since
// Go sync.RWLock is not reentrant
type UpsertCb func(exist bool, valueInMap interface{}, newValue interface{}) interface{}

// Insert or Update - updates existing element or inserts a new one using UpsertCb
func (m *ConcurrentHashMap) Upsert(key string, value interface{}, cb UpsertCb) (res interface{}) {
	shard := m.GetShard(key)
	shard.Lock()
	v, ok := shard.items[key]
	res = cb(ok, v, value)
	shard.items[key] = res
	shard.Unlock()
	return res
}

// Sets the given value under the specified key if no value was associated with it.
func (m *ConcurrentHashMap) SetIfAbsent(key string, value interface{}) bool {
	// Get map shard.
	shard := m.GetShard(key)
	shard.Lock()
	_, ok := shard.items[key]
	if !ok {
		shard.items[key] = value
	}
	shard.Unlock()
	return !ok
}

// Sets the given value under the specified key if oldValue was associated with it.
func (m *ConcurrentHashMap) SetIfPresent(key string, newValue, oldValue interface{}) bool {
	// Get map shard.
	shard := m.GetShard(key)
	shard.Lock()
	val, ok := shard.items[key]
	ok = ok && (val == oldValue)
	if ok {
		shard.items[key] = newValue
	}
	shard.Unlock()
	return ok
}

// Retrieves an element from map under given key.
func (m *ConcurrentHashMap) Get(key string) (interface{}, bool) {
	// Get shard
	shard := m.GetShard(key)
	shard.RLock()
	// Get item from shard.
	val, ok := shard.items[key]
	shard.RUnlock()
	return val, ok
}

// Returns the number of elements within the map.
func (m *ConcurrentHashMap) Count() int {
	count := 0
	for i := 0; i < m.Shards; i++ {
		shard := m.HashMap[i]
		shard.RLock()
		count += len(shard.items)
		shard.RUnlock()
	}
	return count
}

// Looks up an item under specified key
func (m *ConcurrentHashMap) Has(key string) bool {
	// Get shard
	shard := m.GetShard(key)
	shard.RLock()
	// See if element is within shard.
	_, ok := shard.items[key]
	shard.RUnlock()
	return ok
}

// Removes an element from the map.
func (m *ConcurrentHashMap) Remove(key string) {
	// Try to get shard.
	shard := m.GetShard(key)
	shard.Lock()
	delete(shard.items, key)
	shard.Unlock()
}

// Removes an element from the map and returns it
func (m *ConcurrentHashMap) Pop(key string) (v interface{}, exists bool) {
	// Try to get shard.
	shard := m.GetShard(key)
	shard.Lock()
	v, exists = shard.items[key]
	delete(shard.items, key)
	shard.Unlock()
	return v, exists
}

// Checks if map is empty.
func (m *ConcurrentHashMap) IsEmpty() bool {
	return m.Count() == 0
}

// Used by the Iter & IterBuffered functions to wrap two variables together over a channel,
type Tuple struct {
	Key string
	Val interface{}
}

// Returns an iterator which could be used in a for range loop.
//
// Deprecated: using IterBuffered() will get a better performence
func (m *ConcurrentHashMap) Iter() <-chan Tuple {
	ch := make(chan Tuple)
	go func() {
		wg := sync.WaitGroup{}
		wg.Add(m.Shards)
		// Foreach shard.
		for _, shard := range m.HashMap {
			go func(shard *ConcurrentMapShared) {
				// Foreach key, value pair.
				shard.RLock()
				for key, val := range shard.items {
					ch <- Tuple{key, val}
				}
				shard.RUnlock()
				wg.Done()
			}(shard)
		}
		wg.Wait()
		close(ch)
	}()
	return ch
}

// Returns a buffered iterator which could be used in a for range loop.
func (m *ConcurrentHashMap) IterBuffered() <-chan Tuple {
	ch := make(chan Tuple, m.Count())
	go func() {
		wg := sync.WaitGroup{}
		wg.Add(m.Shards)
		// Foreach shard.
		for _, shard := range m.HashMap {
			go func(shard *ConcurrentMapShared) {
				// Foreach key, value pair.
				shard.RLock()
				for key, val := range shard.items {
					ch <- Tuple{key, val}
				}
				shard.RUnlock()
				wg.Done()
			}(shard)
		}
		wg.Wait()
		close(ch)
	}()
	return ch
}

// Returns all items as map[string]interface{}
func (m *ConcurrentHashMap) Items() map[string]interface{} {
	tmp := make(map[string]interface{})

	// Insert items to temporary map.
	for item := range m.IterBuffered() {
		tmp[item.Key] = item.Val
	}

	return tmp
}

// Iterator callback,called for every key,value found in
// maps. RLock is held for all calls for a given shard
// therefore callback sess consistent view of a shard,
// but not across the shards
type IterCb func(key string, v interface{})

// Callback based iterator, cheapest way to read
// all elements in a map.
func (m *ConcurrentHashMap) IterCb(fn IterCb) {
	for idx := range m.HashMap {
		shard := m.HashMap[idx]
		shard.RLock()
		for key, value := range shard.items {
			fn(key, value)
		}
		shard.RUnlock()
	}
}

// Return all keys as []string
func (m *ConcurrentHashMap) Keys() []string {
	count := m.Count()
	ch := make(chan string, count)
	go func() {
		// Foreach shard.
		wg := sync.WaitGroup{}
		wg.Add(m.Shards)
		for _, shard := range m.HashMap {
			go func(shard *ConcurrentMapShared) {
				// Foreach key, value pair.
				shard.RLock()
				for key := range shard.items {
					ch <- key
				}
				shard.RUnlock()
				wg.Done()
			}(shard)
		}
		wg.Wait()
		close(ch)
	}()

	// Generate keys
	keys := make([]string, count)
	for i := 0; i < count; i++ {
		keys[i] = <-ch
	}
	return keys
}

//Reviles ConcurrentHashMap "private" variables to json marshal.
func (m *ConcurrentHashMap) MarshalJSON() ([]byte, error) {
	// Create a temporary map, which will hold all item spread across shards.
	tmp := make(map[string]interface{})

	// Insert items to temporary map.
	for item := range m.IterBuffered() {
		tmp[item.Key] = item.Val
	}
	return json.Marshal(tmp)
}

// Returns a buffered iterator which could be used in a for range loop.
func (m *ConcurrentHashMap) IterBufferedLike(k string) <-chan Tuple {
	ch := make(chan Tuple, m.Count())
	go func(k string) {
		wg := sync.WaitGroup{}
		wg.Add(m.Shards)
		// Foreach shard.
		for _, shard := range m.HashMap {
			go func(shard *ConcurrentMapShared, k string) {
				// Foreach key, value pair.
				shard.RLock()
				for key, val := range shard.items {
					if strings.Index(key, k) != -1 {
						ch <- Tuple{key, val}
					}
				}
				shard.RUnlock()
				wg.Done()
			}(shard, k)
		}
		wg.Wait()
		close(ch)
	}(k)
	return ch
}

// Returns all items as map[string]interface{}
func (m *ConcurrentHashMap) ItemsLike(like string) map[string]interface{} {
	tmp := make(map[string]interface{})

	// Insert items to temporary map.
	for item := range m.IterBufferedLike(like) {
		tmp[item.Key] = item.Val
	}

	return tmp
}

func fnv32(key string) uint32 {
	hash := uint32(2166136261)
	const prime32 = uint32(16777619)
	for i := 0; i < len(key); i++ {
		hash *= prime32
		hash ^= uint32(key[i])
	}
	return hash
}

