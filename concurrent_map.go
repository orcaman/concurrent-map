package cmap

import (
	"encoding/json"
	"sync"
)

// ConcurrentMap is a "thread" safe map of type string:Anything.
// To avoid lock bottlenecks this map is dived to several (ShardCount) map shards.
type ConcurrentMap struct {
	shardCount int
	shards     []*ConcurrentMapShard
}

type valWithIndex struct {
	val      interface{}
	arrayIdx int
}

// ConcurrentMapShared is a "thread" safe string to anything map.
type ConcurrentMapShard struct {
	maxSize      int
	idxAdd       int
	mapKeys      []string
	items        map[string]*valWithIndex
	sync.RWMutex // Read Write mutex, guards access to internal map.
}

// New creates a new concurrent map.
func New(maxSize int, shardCount int) *ConcurrentMap {
	m := ConcurrentMap{
		shardCount: shardCount,
		shards:     make([]*ConcurrentMapShard, shardCount),
	}

	shardSize := maxSize / shardCount
	if shardSize == 0 {
		shardSize = 1
	}
	if maxSize%shardCount != 0 {
		shardSize++
	}
	for i := 0; i < shardCount; i++ {
		m.shards[i] = &ConcurrentMapShard{
			maxSize: shardSize,
			idxAdd:  0,
			mapKeys: make([]string, shardSize),
			items:   make(map[string]*valWithIndex),
		}
	}
	return &m
}

// GetShard returns shard under given key.
func (m *ConcurrentMap) GetShard(key string) *ConcurrentMapShard {
	return m.shards[uint(fnv32(key))%uint(m.shardCount)]
}

// MSet creates a sharded concurrent map out of a regular map.
func (m *ConcurrentMap) MSet(data map[string]interface{}) {
	for key, value := range data {
		m.Set(key, value)
	}
}

// Set sets the given value under the specified key.
func (m *ConcurrentMap) Set(key string, value interface{}) {
	// Get map shard.
	shard := m.GetShard(key)
	shard.Lock()
	v, ok := shard.items[key]
	shard.items[key] = &valWithIndex{
		arrayIdx: shard.idxAdd,
		val:      value,
	}

	if ok {
		shard.mapKeys[v.arrayIdx] = ""
	}

	appendKeyToList(key, shard)

	shard.Unlock()
}

// appendKeyToList adds the key to the list, updates the list index and removes last registration if needed.
// Should be called under shard mutex
func appendKeyToList(key string, shard *ConcurrentMapShard) {
	shard.mapKeys[shard.idxAdd] = key
	shard.idxAdd++
	shard.idxAdd %= shard.maxSize
	keyToRemove := shard.mapKeys[shard.idxAdd]
	shard.mapKeys[shard.idxAdd] = ""
	delete(shard.items, keyToRemove)
}

// UpsertCb is callback function to return new element to be inserted into the map.
// It is called while lock is held, therefore it MUST NOT
// try to access other keys in same map, as it can lead to deadlock since
// Go sync.RWLock is not reentrant
type UpsertCb func(exist bool, valueInMap interface{}, newValue interface{}) interface{}

// Upsert (Insert or Update) - updates existing element or inserts a new one using UpsertCb.
func (m *ConcurrentMap) Upsert(key string, value interface{}, cb UpsertCb) (res interface{}) {
	shard := m.GetShard(key)
	shard.Lock()
	v, ok := shard.items[key]
	if v == nil {
		res = cb(ok, nil, value)
	} else {
		res = cb(ok, v.val, value)
	}

	shard.items[key] = &valWithIndex{
		val:      res,
		arrayIdx: shard.idxAdd,
	}

	// if key is new add to the map
	if !ok {
		appendKeyToList(key, shard)
	} else if v != nil {
		shard.mapKeys[v.arrayIdx] = ""
	}

	shard.Unlock()
	return res
}

// SetIfAbsent sets the given value under the specified key if no value was associated with it.
func (m *ConcurrentMap) SetIfAbsent(key string, value interface{}) bool {
	// Get map shard.
	shard := m.GetShard(key)
	shard.Lock()
	_, ok := shard.items[key]
	if !ok {
		shard.items[key] = &valWithIndex{val: value, arrayIdx: shard.idxAdd}
		appendKeyToList(key, shard)
	}
	shard.Unlock()
	return !ok
}

// Get retrieves an element from map under given key.
func (m *ConcurrentMap) Get(key string) (interface{}, bool) {
	// Get shard
	shard := m.GetShard(key)
	shard.RLock()
	// Get item from shard.
	val, ok := shard.items[key]
	shard.RUnlock()
	if !ok {
		return nil, ok
	}

	return val.val, ok
}

// Count returns the number of elements within the map.
func (m *ConcurrentMap) Count() int {
	count := 0
	for i := 0; i < m.shardCount; i++ {
		shard := m.shards[i]
		shard.RLock()
		count += len(shard.items)
		shard.RUnlock()
	}
	return count
}

// Has looks up an item under specified key.
func (m *ConcurrentMap) Has(key string) bool {
	// Get shard
	shard := m.GetShard(key)
	shard.RLock()
	// See if element is within shard.
	_, ok := shard.items[key]
	shard.RUnlock()
	return ok
}

// Remove removes an element from the map.
func (m *ConcurrentMap) Remove(key string) {
	// Try to get shard.
	shard := m.GetShard(key)
	shard.Lock()
	v, ok := shard.items[key]
	if ok {
		shard.mapKeys[v.arrayIdx] = ""
		delete(shard.items, key)
	}
	shard.Unlock()
}

// RemoveCb is a callback executed in a map.RemoveCb() call, while Lock is held.
// If returns true, the element will be removed from the map.
type RemoveCb func(key string, v interface{}, exists bool) bool

// RemoveCb locks the shard containing the key, retrieves its current value and calls the callback with those params.
// If callback returns true and element exists, it will remove it from the map
// Returns the value returned by the callback (even if element was not present in the map)
func (m *ConcurrentMap) RemoveCb(key string, cb RemoveCb) bool {
	var remove bool

	// Try to get shard.
	shard := m.GetShard(key)
	shard.Lock()
	v, ok := shard.items[key]
	if v == nil {
		remove = cb(key, nil, ok)
	} else {
		remove = cb(key, v.val, ok)
	}

	if remove && ok {
		if v != nil {
			shard.mapKeys[v.arrayIdx] = ""
		}
		delete(shard.items, key)
	}
	shard.Unlock()

	return remove
}

// Pop removes an element from the map and returns it.
func (m *ConcurrentMap) Pop(key string) (v interface{}, exists bool) {
	// Try to get shard.
	shard := m.GetShard(key)
	shard.Lock()
	elem, exists := shard.items[key]
	if exists {
		shard.mapKeys[elem.arrayIdx] = ""
		delete(shard.items, key)
		shard.Unlock()

		return elem.val, exists
	}
	shard.Unlock()

	return nil, exists
}

// IsEmpty checks if map is empty.
func (m *ConcurrentMap) IsEmpty() bool {
	return m.Count() == 0
}

// Tuple is used by the Iter & IterBuffered functions to wrap two variables together over a channel.
type Tuple struct {
	Key string
	Val interface{}
}

// IterBuffered returns a buffered iterator which could be used in a for range loop.
func (m *ConcurrentMap) IterBuffered() <-chan Tuple {
	chans := snapshot(m)
	total := 0
	for _, c := range chans {
		total += cap(c)
	}
	ch := make(chan Tuple, total)
	go fanIn(chans, ch)
	return ch
}

// snapshot returns an array of channels that contains elements in each shard,
// which likely takes a snapshot of `m`.
// It returns once the size of each buffered channel is determined,
// before all the channels are populated using goroutines.
func snapshot(m *ConcurrentMap) (chans []chan Tuple) {
	chans = make([]chan Tuple, m.shardCount)
	wg := sync.WaitGroup{}
	wg.Add(m.shardCount)
	// Foreach shard.
	for index, shard := range m.shards {
		go func(index int, shard *ConcurrentMapShard) {
			// Foreach key, value pair.
			shard.RLock()
			chans[index] = make(chan Tuple, len(shard.items))
			wg.Done()
			for key, val := range shard.items {
				chans[index] <- Tuple{key, val.val}
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
func (m *ConcurrentMap) Items() map[string]interface{} {
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
func (m *ConcurrentMap) IterCb(fn IterCb) {
	for idx := range m.shards {
		shard := (m.shards)[idx]
		shard.RLock()
		for key, value := range shard.items {
			fn(key, value.val)
		}
		shard.RUnlock()
	}
}

// Keys returns all keys as []string
func (m *ConcurrentMap) Keys() []string {
	count := m.Count()
	ch := make(chan string, count)
	go func() {
		// Foreach shard.
		wg := sync.WaitGroup{}
		wg.Add(m.shardCount)
		for _, shard := range m.shards {
			go func(shard *ConcurrentMapShard) {
				// Foreach key, value pair.
				shard.RLock()
				last := shard.idxAdd + 1
				last %= shard.maxSize

				for i := last; i != shard.idxAdd; i = (i + 1) % shard.maxSize {
					if len(shard.mapKeys[i]) == 0 {
						continue
					}
					ch <- shard.mapKeys[i]
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

//MarshalJSON reveals ConcurrentMap "private" variables to json marshal.
func (m *ConcurrentMap) MarshalJSON() ([]byte, error) {
	// Create a temporary map, which will hold all item spread across shards.
	tmp := make(map[string]interface{})

	// Insert items to temporary map.
	for item := range m.IterBuffered() {
		tmp[item.Key] = item.Val
	}
	return json.Marshal(tmp)
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
