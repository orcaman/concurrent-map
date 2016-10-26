package icmap

import (
	"encoding/json"
	"strconv"
	"sync"
)

type ConcurrentHashMap struct {
	Shards     int
	HashMap    ConcurrentMap
}

// A "thread" safe map of type int:Anything.
// To avoid lock bottlenecks this map is dived to several (Shards) map shards.
type ConcurrentMap []*ConcurrentMapShared

// A "thread" safe int to anything map.
type ConcurrentMapShared struct {
	items        map[int]string
	sync.RWMutex // Read Write mutex, guards access to internal map.
}

func New(shards int) *ConcurrentHashMap {
    strconv.Itoa(shards)
	m := &ConcurrentHashMap{Shards:shards,HashMap:make(ConcurrentMap, shards)}
	for i := 0; i < shards; i++ {
		m.HashMap[i] = &ConcurrentMapShared{items: make(map[int]string)}
	}
	return m
}


func (m *ConcurrentHashMap) GetShard(key int) *ConcurrentMapShared {
	return  m.HashMap[uint(fnv32( strconv.Itoa(  key ) ))%uint(m.Shards)]
}

func (m *ConcurrentHashMap) MSet(data map[int]string) {
	for key, value := range data {
		shard := m.GetShard(key)
		shard.Lock()
		shard.items[key] = value
		shard.Unlock()
	}
}

// Sets the given value under the specified key.
func (m *ConcurrentHashMap) Set(key int, value string) {
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
type UpsertCb func(exist bool, valueInMap string, newValue string) string

// Insert or Update - updates existing element or inserts a new one using UpsertCb
func (m *ConcurrentHashMap) Upsert(key int, value string, cb UpsertCb) (res string) {
	shard := m.GetShard(key)
	shard.Lock()
	v, ok := shard.items[key]
	res = cb(ok, v, value)
	shard.items[key] = res
	shard.Unlock()
	return res
}

// Sets the given value under the specified key if no value was associated with it.
func (m *ConcurrentHashMap) SetIfAbsent(key int, value string) bool {
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
func (m *ConcurrentHashMap) SetIfPresent(key int, newValue, oldValue string) bool {
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
func (m ConcurrentHashMap) Get(key int) (string, bool) {
	// Get shard
	shard := m.GetShard(key)
	shard.RLock()
	// Get item from shard.
	val, ok := shard.items[key]
	shard.RUnlock()
	return val, ok
}

// Returns the number of elements within the map.
func (m ConcurrentHashMap) Count() int {
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
func (m *ConcurrentHashMap) Has(key int) bool {
	// Get shard
	shard := m.GetShard(key)
	shard.RLock()
	// See if element is within shard.
	_, ok := shard.items[key]
	shard.RUnlock()
	return ok
}

// Removes an element from the map.
func (m *ConcurrentHashMap) Remove(key int) {
	// Try to get shard.
	shard := m.GetShard(key)
	shard.Lock()
	delete(shard.items, key)
	shard.Unlock()
}

// Removes an element from the map and returns it
func (m *ConcurrentHashMap) Pop(key int) (v string, exists bool) {
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
	Key int
	Val string
}

// Returns an iterator which could be used in a for range loop.
//
// Deprecated: using IterBuffered() will get a better performence
func (m ConcurrentHashMap) Iter() <-chan Tuple {
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
func (m ConcurrentHashMap) IterBuffered() <-chan Tuple {
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

// Returns all items as map[int]string
func (m ConcurrentHashMap) Items() map[int]string {
	tmp := make(map[int]string)

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
type IterCb func(key int, v string)

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

// Return all keys as []int
func (m ConcurrentHashMap) Keys() []int {
	count := m.Count()
	ch := make(chan int, count)
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
	keys := make([]int, count)
	for i := 0; i < count; i++ {
		keys[i] = <-ch
	}
	return keys
}

//Reviles ConcurrentHashMap "private" variables to json marshal.
func (m ConcurrentHashMap) MarshalJSON() ([]byte, error) {
	// Create a temporary map, which will hold all item spread across shards.
	tmp := make(map[int]string)

	// Insert items to temporary map.
	for item := range m.IterBuffered() {
		tmp[item.Key] = item.Val
	}
	return json.Marshal(tmp)
}

func (m *ConcurrentHashMap) UnmarshalJSON(b []byte) (err error) {
	// Reverse process of Marshal.

	tmp := make(map[int]string)

	// Unmarshal into a single map.
	if err := json.Unmarshal(b, &tmp); err != nil {
		return nil
	}

	// foreach key,value pair in temporary map insert into our concurrent map.
	for key, val := range tmp {
		m.Set(key, val)
	}
	return nil
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

