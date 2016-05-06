package cmap

import (
	"encoding/json"
	"hash/fnv"
	"sync"
)

var SHARD_COUNT = 32

// TODO: Add Keys function which returns an array of keys for the map.

// A "thread" safe map of type string:Anything.
// To avoid lock bottlenecks this map is dived to several (SHARD_COUNT) map shards.
type ConcurrentMap []*ConcurrentMapShared

// A "thread" safe string to anything map.
type InternalValueType struct {
	Key   Key
	Value interface{}
}
type ConcurrentMapShared struct {
	// items        map[string]interface{}
	items        map[string]*InternalValueType
	sync.RWMutex // Read Write mutex, guards access to internal map.
}

// Creates a new concurrent map.
func New() ConcurrentMap {
	m := make(ConcurrentMap, SHARD_COUNT)
	for i := 0; i < SHARD_COUNT; i++ {
		m[i] = &ConcurrentMapShared{items: make(map[string]*InternalValueType)}
	}
	return m
}

// Returns shard under given key
func (m ConcurrentMap) GetShard(key Key) *ConcurrentMapShared {
	hasher := fnv.New32()
	hasher.Write([]byte(key.HashCode()))
	return m[uint(hasher.Sum32())%uint(SHARD_COUNT)]
}

// Sets the given value under the specified key.
func (m *ConcurrentMap) Set(key Key, value interface{}) {
	// Get map shard.
	shard := m.GetShard(key)
	shard.Lock()
	shard.items[key.HashCode()] = &InternalValueType{key, value}
	shard.Unlock()
}

// Sets the given value under the specified key if no value was associated with it.
func (m *ConcurrentMap) SetIfAbsent(key Key, value interface{}) bool {
	// Get map shard.
	shard := m.GetShard(key)
	shard.Lock()
	_, ok := shard.items[key.HashCode()]
	if !ok {
		shard.items[key.HashCode()] = &InternalValueType{key, value}
	}
	shard.Unlock()
	return !ok
}

// Retrieves an element from map under given key.
func (m ConcurrentMap) Get(key Key) (interface{}, bool) {
	// Get shard
	shard := m.GetShard(key)
	shard.RLock()
	// Get item from shard.
	val, ok := shard.items[key.HashCode()]
	shard.RUnlock()
	if !ok {
		return nil, ok
	}
	return val.Value, ok
}

// Returns the number of elements within the map.
func (m ConcurrentMap) Count() int {
	count := 0
	for i := 0; i < SHARD_COUNT; i++ {
		shard := m[i]
		shard.RLock()
		count += len(shard.items)
		shard.RUnlock()
	}
	return count
}

// Looks up an item under specified key
func (m *ConcurrentMap) Has(key Key) bool {
	// Get shard
	shard := m.GetShard(key)
	shard.RLock()
	// See if element is within shard.
	_, ok := shard.items[key.HashCode()]
	shard.RUnlock()
	return ok
}

// Removes an element from the map.
func (m *ConcurrentMap) Remove(key Key) {
	// Try to get shard.
	shard := m.GetShard(key)
	shard.Lock()
	delete(shard.items, key.HashCode())
	shard.Unlock()
}

// Checks if map is empty.
func (m *ConcurrentMap) IsEmpty() bool {
	return m.Count() == 0
}

// Used by the Iter & IterBuffered functions to wrap two variables together over a channel,
type Tuple struct {
	Key Key
	Val interface{}
}

// Returns an iterator which could be used in a for range loop.
func (m ConcurrentMap) Iter() <-chan Tuple {
	ch := make(chan Tuple)
	go func() {
		// Foreach shard.
		for _, shard := range m {
			// Foreach key, value pair.
			shard.RLock()
			for _, val := range shard.items {
				ch <- Tuple{val.Key, val.Value}
			}
			shard.RUnlock()
		}
		close(ch)
	}()
	return ch
}

// Returns a buffered iterator which could be used in a for range loop.
func (m ConcurrentMap) IterBuffered() <-chan Tuple {
	ch := make(chan Tuple, m.Count())
	go func() {
		// Foreach shard.
		for _, shard := range m {
			// Foreach key, value pair.
			shard.RLock()
			for _, val := range shard.items {
				ch <- Tuple{val.Key, val.Value}
			}
			shard.RUnlock()
		}
		close(ch)
	}()
	return ch
}

// Returns all items as map[string]interface{}
func (m ConcurrentMap) Items() map[string]interface{} {
	tmp := make(map[string]interface{})

	// Insert items to temporary map.
	for item := range m.Iter() {
		tmp[item.Key.HashCode()] = item.Val
	}

	return tmp
}

//Reviles ConcurrentMap "private" variables to json marshal.
func (m ConcurrentMap) MarshalJSON() ([]byte, error) {
	// Create a temporary map, which will hold all item spread across shards.
	tmp := make(map[string]interface{})

	// Insert items to temporary map.
	for item := range m.Iter() {
		tmp[item.Key.HashCode()] = item.Val
	}
	return json.Marshal(tmp)
}

//Keys function which returns an array of keys for the map.
func (m ConcurrentMap) Keys() []Key {
	var result []Key
	for item := range m.Iter() {
		result = append(result, item.Key)
	}
	return result
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
