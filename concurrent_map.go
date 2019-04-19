package cmap

import (
	"encoding/json"
	"sync"
)

const ShardCount = 30

// ConcurrentMap is a "thread" safe map of type string:Anything.
// To avoid lock bottlenecks this map is dived to several (ShardCount) map shards.
type ConcurrentMap []*ConcurrentMapShared

// ConcurrentMapShared is a "thread" safe string to anything map.
type ConcurrentMapShared struct {
	mapKeys      []string
	maxSize      int
	items        map[string]interface{}
	sync.RWMutex // Read Write mutex, guards access to internal map.
}

// New creates a new concurrent map.
func New(maxSize int) ConcurrentMap {
	m := make(ConcurrentMap, ShardCount)
	for i := 0; i < ShardCount; i++ {
		m[i] = &ConcurrentMapShared{
			maxSize: maxSize,
			items:   make(map[string]interface{}),
		}
	}
	return m
}

// GetShard returns shard under given key.
func (m ConcurrentMap) GetShard(key string) *ConcurrentMapShared {
	return m[uint(fnv32(key))%uint(ShardCount)]
}

func (m ConcurrentMap) MSet(data map[string]interface{}) {
	for key, value := range data {
		if m.isEvictionOccurred(key) {
			m.removeFirstElement(key)
		}
		shard := m.GetShard(key)
		shard.Lock()
		shard.items[key] = value
		shard.mapKeys = append(shard.mapKeys, key)
		shard.Unlock()
	}
}

// Set sets the given value under the specified key.
func (m ConcurrentMap) Set(key string, value interface{}) {
	// Get map shard.
	shard := m.GetShard(key)
	if m.isEvictionOccurred(key) {
		m.removeFirstElement(key)
	}
	shard.Lock()
	shard.items[key] = value
	shard.mapKeys = append(shard.mapKeys, key)
	shard.Unlock()
}

// UpsertCb is callback function to return new element to be inserted into the map.
// It is called while lock is held, therefore it MUST NOT
// try to access other keys in same map, as it can lead to deadlock since
// Go sync.RWLock is not reentrant
type UpsertCb func(exist bool, valueInMap interface{}, newValue interface{}) interface{}

// Upsert (Insert or Update) - updates existing element or inserts a new one using UpsertCb.
func (m ConcurrentMap) Upsert(key string, value interface{}, cb UpsertCb) (res interface{}) {
	shard := m.GetShard(key)

	if m.isEvictionOccurred(key) {
		m.removeFirstElement(key)
	}
	shard.Lock()
	v, ok := shard.items[key]
	res = cb(ok, v, value)
	shard.items[key] = res
	shard.mapKeys = append(shard.mapKeys, key)
	shard.Unlock()
	return res
}

// SetIfAbsent sets the given value under the specified key if no value was associated with it.
func (m ConcurrentMap) SetIfAbsent(key string, value interface{}) bool {
	// Get map shard.
	shard := m.GetShard(key)

	if m.isEvictionOccurred(key) {
		m.removeFirstElement(key)
	}
	shard.Lock()
	_, ok := shard.items[key]
	if !ok {
		shard.items[key] = value
		shard.mapKeys = append(shard.mapKeys, key)
	}
	shard.Unlock()
	return !ok
}

// Get retrieves an element from map under given key.
func (m ConcurrentMap) Get(key string) (interface{}, bool) {
	// Get shard
	shard := m.GetShard(key)
	shard.RLock()
	// Get item from shard.
	val, ok := shard.items[key]
	shard.RUnlock()
	return val, ok
}

// Count returns the number of elements within the map.
func (m ConcurrentMap) Count() int {
	count := 0
	for i := 0; i < ShardCount; i++ {
		shard := m[i]
		shard.RLock()
		count += len(shard.items)
		shard.RUnlock()
	}
	return count
}

// Has looks up an item under specified key.
func (m ConcurrentMap) Has(key string) bool {
	// Get shard
	shard := m.GetShard(key)
	shard.RLock()
	// See if element is within shard.
	_, ok := shard.items[key]
	shard.RUnlock()
	return ok
}

// Remove removes an element from the map.
func (m ConcurrentMap) Remove(key string) {
	// Try to get shard.
	shard := m.GetShard(key)
	shard.Lock()
	delete(shard.items, key)
	m.removeMapKey(key)
	shard.Unlock()
}

// RemoveCb is a callback executed in a map.RemoveCb() call, while Lock is held.
// If returns true, the element will be removed from the map.
type RemoveCb func(key string, v interface{}, exists bool) bool

// RemoveCb locks the shard containing the key, retrieves its current value and calls the callback with those params.
// If callback returns true and element exists, it will remove it from the map
// Returns the value returned by the callback (even if element was not present in the map)
func (m ConcurrentMap) RemoveCb(key string, cb RemoveCb) bool {
	// Try to get shard.
	shard := m.GetShard(key)
	shard.Lock()
	v, ok := shard.items[key]
	remove := cb(key, v, ok)
	if remove && ok {
		delete(shard.items, key)
		m.removeMapKey(key)
	}
	shard.Unlock()
	return remove
}

// Pop removes an element from the map and returns it.
func (m ConcurrentMap) Pop(key string) (v interface{}, exists bool) {
	// Try to get shard.
	shard := m.GetShard(key)
	shard.Lock()
	v, exists = shard.items[key]
	delete(shard.items, key)
	m.removeMapKey(key)
	shard.Unlock()
	return v, exists
}

// IsEmpty checks if map is empty.
func (m ConcurrentMap) IsEmpty() bool {
	return m.Count() == 0
}

// Tuple is used by the Iter & IterBuffered functions to wrap two variables together over a channel.
type Tuple struct {
	Key string
	Val interface{}
}

// Iter returns an iterator which could be used in a for range loop.
//
// Deprecated: using IterBuffered() will get a better performance.
func (m ConcurrentMap) Iter() <-chan Tuple {
	chans := snapshot(m)
	ch := make(chan Tuple)
	go fanIn(chans, ch)
	return ch
}

// IterBuffered returns a buffered iterator which could be used in a for range loop.
func (m ConcurrentMap) IterBuffered() <-chan Tuple {
	chans := snapshot(m)
	total := 0
	for _, c := range chans {
		total += cap(c)
	}
	ch := make(chan Tuple, total)
	go fanIn(chans, ch)
	return ch
}

// Returns a array of channels that contains elements in each shard,
// which likely takes a snapshot of `m`.
// It returns once the size of each buffered channel is determined,
// before all the channels are populated using goroutines.
func snapshot(m ConcurrentMap) (chans []chan Tuple) {
	chans = make([]chan Tuple, ShardCount)
	wg := sync.WaitGroup{}
	wg.Add(ShardCount)
	// Foreach shard.
	for index, shard := range m {
		go func(index int, shard *ConcurrentMapShared) {
			// Foreach key, value pair.
			shard.RLock()
			chans[index] = make(chan Tuple, len(shard.items))
			wg.Done()
			for key, val := range shard.items {
				chans[index] <- Tuple{key, val}
			}
			shard.RUnlock()
			close(chans[index])
		}(index, shard)
	}
	wg.Wait()
	return chans
}

// fanIn reads elements from channels `chans` into channel `out`
func fanIn(chans []chan Tuple, out chan Tuple) {
	wg := sync.WaitGroup{}
	wg.Add(len(chans))
	for _, ch := range chans {
		go func(ch chan Tuple) {
			for t := range ch {
				out <- t
			}
			wg.Done()
		}(ch)
	}
	wg.Wait()
	close(out)
}

// Items returns all items as map[string]interface{}
func (m ConcurrentMap) Items() map[string]interface{} {
	tmp := make(map[string]interface{})

	// Insert items to temporary map.
	for item := range m.IterBuffered() {
		tmp[item.Key] = item.Val
	}

	return tmp
}

// IterCb iterator callback,called for every key,value found in maps.
// RLock is held for all calls for a given shard
// therefore callback sess consistent view of a shard,
// but not across the shards
type IterCb func(key string, v interface{})

// IterCb callback based iterator, cheapest way to read all elements in a map.
func (m ConcurrentMap) IterCb(fn IterCb) {
	for idx := range m {
		shard := (m)[idx]
		shard.RLock()
		for key, value := range shard.items {
			fn(key, value)
		}
		shard.RUnlock()
	}
}

// Keys returns all keys as []string
func (m ConcurrentMap) Keys() []string {
	count := m.Count()
	ch := make(chan string, count)
	go func() {
		// Foreach shard.
		wg := sync.WaitGroup{}
		wg.Add(ShardCount)
		for _, shard := range m {
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
	keys := make([]string, 0, count)
	for k := range ch {
		keys = append(keys, k)
	}
	return keys
}

// FindOldest search for the oldest key in the key slice.
func (m ConcurrentMap) FindOldest() string {
	keys := []string{}
	for i := 0; i < ShardCount; i++ {
		shard := m[i]
		if len(shard.mapKeys) > 0 {
			shard.Lock()
			keys = append(keys, shard.mapKeys[0])
			shard.Unlock()
		}
	}
	return keys[0]
}

// RemoveOldest removes the oldest key from the key slice.
func (m ConcurrentMap) RemoveOldest() {
	oldest := m.FindOldest()
	m.Remove(oldest)
}

// MarshalJSON reviles ConcurrentMap "private" variables to json marshal.
func (m ConcurrentMap) MarshalJSON() ([]byte, error) {
	// Create a temporary map, which will hold all item spread across shards.
	tmp := make(map[string]interface{})

	// Insert items to temporary map.
	for item := range m.IterBuffered() {
		tmp[item.Key] = item.Val
	}
	return json.Marshal(tmp)
}

// isEvictionOccurred check if the map size overflow the maximum allowed size.
func (m ConcurrentMap) isEvictionOccurred(key string) bool {
	shard := m.GetShard(key)
	if m.Count() > shard.maxSize {
		return true
	}
	return false
}

// removeFirstElement removes the first element from the key slice.
func (m ConcurrentMap) removeFirstElement(key string) {
	shard := m.GetShard(key)
	shard.Lock()
	copy(shard.mapKeys[0:], shard.mapKeys[1:])
	shard.mapKeys = shard.mapKeys[:len(shard.mapKeys)-1]
	shard.Unlock()
}

// removeMapKey removes the map key from the key slice.
func (m ConcurrentMap) removeMapKey(key string) {
	shard := m.GetShard(key)

	if len(shard.mapKeys) > 0 {
		for i := 0; i < len(shard.mapKeys); i++ {
			// The mutex lock will be called by the method invoker.
			if string(key) == shard.mapKeys[i] {
				// delete key from keys slice
				copy(shard.mapKeys[i:], shard.mapKeys[i+1:])
				shard.mapKeys = shard.mapKeys[:len(shard.mapKeys)-1]
			}
		}
	}
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

// Concurrent map uses Interface{} as its value, therefor JSON Unmarshal
// will probably won't know which to type to unmarshal into, in such case
// we'll end up with a value of type map[string]interface{}, In most cases this isn't
// out value type, this is why we've decided to remove this functionality.

// func (m *ConcurrentMap) UnmarshalJSON(b []byte) (err error) {
// 	// Reverse process of Marshal.

// 	tmp := make(map[string]interface{})

// 	// Unmarshal into a single map.
// 	if err := json.Unmarshal(b, &tmp); err != nil {
// 		return nil
// 	}

// 	// foreach key,value pair in temporary map insert into our concurrent map.
// 	for key, val := range tmp {
// 		m.Set(key, val)
// 	}
// 	return nil
// }
