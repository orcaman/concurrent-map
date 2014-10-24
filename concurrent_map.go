package cmap

import (
	"crypto/sha1"
	"encoding/json"
	"fmt"
	"sync"
)

// TODO: Add Keys function which returns an array of keys for the map.

// A "thread" safe map of type string:Anything.
// To avoid lock bottlenecks this map is dived to several (256) map shards.
type ConcurrentMap struct {
	shards       map[string]*ConcurrentMapShared
	sync.RWMutex // Read Write mutex, guards access to internal map.
}

// A "thread" safe string to anything map.
type ConcurrentMapShared struct {
	items        map[string]interface{}
	sync.RWMutex // Read Write mutex, guards access to internal map.
}

// Creates a new concurrent map.
func New() *ConcurrentMap {
	return &ConcurrentMap{shards: make(map[string]*ConcurrentMapShared, 256)}
}

// Returns shard under given key, incase shared is missing and create is true
// the function will create the missing shard.
func (m *ConcurrentMap) GetShard(key string, create bool) (shard *ConcurrentMapShared, ok bool) {

	// Hash key.
	hasher := sha1.New()
	hasher.Write([]byte(key))

	// Use last two digits of hashed key as our shard key.
	shardKey := fmt.Sprintf("%x", hasher.Sum(nil))[0:2]

	// Try to get shard.
	m.RLock()
	shard, ok = m.shards[shardKey]
	m.RUnlock()

	// Did we got a shard? Are we asked to create a missing shard?
	if ok || !create {
		return
	}

	// Recheck, write lock!
	m.Lock()
	defer m.Unlock()
	shard, ok = m.shards[shardKey]
	if ok {
		return
	}

	// Create shard.
	shard = &ConcurrentMapShared{items: make(map[string]interface{}, 2048)}
	m.shards[shardKey] = shard
	ok = true
	return
}

// Sets the given value under the specified key.
func (m *ConcurrentMap) Set(key string, value interface{}) {

	// Get map shard.
	shard, _ := m.GetShard(key, true)
	shard.Lock()
	defer shard.Unlock()
	shard.items[key] = value
}

// Retrieves an element from map under given key.
func (m *ConcurrentMap) Get(key string) (interface{}, bool) {

	// Get shard, don't create if missing.
	shard, ok := m.GetShard(key, false)
	if ok == false {
		return nil, false
	}

	shard.RLock()
	defer shard.RUnlock()

	// Get item from shard.
	val, ok := shard.items[key]
	return val, ok
}

// Returns the number of elements within the map.
func (m *ConcurrentMap) Count() int {
	count := 0

	m.RLock()
	defer m.RUnlock()

	// Count number of items within each shard.
	for _, val := range m.shards {
		val.RLock()
		count += len(val.items)
		val.RUnlock()
	}

	return count
}

// Looks up an item under specified key
func (m *ConcurrentMap) Has(key string) bool {
	// Get shard, don't create if missing.
	shard, ok := m.GetShard(key, false)
	if ok == false {
		return false
	}

	shard.RLock()
	defer shard.RUnlock()

	// See if element is within shard.
	_, ok = shard.items[key]
	return ok
}

// Removes an element from the map.
func (m *ConcurrentMap) Remove(key string) {
	// Try to get shard.
	shard, ok := m.GetShard(key, false)
	if ok == false {
		return
	}

	shard.Lock()
	defer shard.Unlock()
	delete(shard.items, key)
}

// Clears map by constructing a new one.
func (m *ConcurrentMap) Clear() {
	m.Lock()
	defer m.Unlock()
	m.shards = make(map[string]*ConcurrentMapShared, 256)
}

// Checks if map is empty.
func (m *ConcurrentMap) IsEmpty() bool {
	return m.Count() == 0
}

// Used by the Iter & IterBuffered functions to wrap two variables together over a channel,
type Tuple struct {
	Key string
	Val interface{}
}

// Returns an iterator which could be used in a for range loop.
func (m *ConcurrentMap) Iter() <-chan Tuple {
	ch := make(chan Tuple)
	go func() {
		m.RLock()
		defer m.RUnlock()
		// Foreach shard.
		for _, shard := range m.shards {
			// Foreach key, value pair.
			shard.RLock()
			for key, val := range shard.items {
				ch <- Tuple{key, val}
			}
			shard.RUnlock()
		}
		close(ch)
	}()
	return ch
}

// Returns a buffered iterator which could be used in a for range loop.
func (m *ConcurrentMap) IterBuffered() <-chan Tuple {
	ch := make(chan Tuple, m.Count())
	go func() {
		m.RLock()
		defer m.RUnlock()
		// Foreach shard.
		for _, shard := range m.shards {
			// Foreach key, value pair.
			shard.RLock()
			for key, val := range shard.items {
				ch <- Tuple{key, val}
			}
			shard.RUnlock()
		}
		close(ch)
	}()
	return ch
}

// Reviles ConcurrentMap "private" variables to json marshal.
func (m ConcurrentMap) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		M map[string]*ConcurrentMapShared
	}{M: m.shards})
}

// Reviles ConcurrentMapShared "private" variables to json marshal.
func (m ConcurrentMapShared) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		M map[string]interface{}
	}{M: m.items})
}
