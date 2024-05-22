// supported by https://github.com/orcaman/concurrent-map/blob/master/concurrent_map.go
package container

import (
	"encoding/json"
	"fmt"
	"strconv"
	"sync"
)

const defaultShardCount = 32

type Stringer interface {
	fmt.Stringer
	comparable
}

// A "thread" safe map of type string:Anything.
// To avoid lock bottlenecks this map is dived to several (defaultShardCount) map shards.
type ConcurrentMulMap[K Stringer, V any] struct {
	shards    []*concurrentMulMapShared[K, V]
	shardsNum int
	shardFn   HashFn[K]
}

type HashFn[K Stringer]func(key K) uint32

func (m *ConcurrentMulMap[K, V]) apply(opts []MMapOpt[K, V]) {
	for _, opt := range opts {
		opt(m)
	}
}

type MMapOpt[K Stringer, V any] func(mulMap *ConcurrentMulMap[K, V])

func WithShardsNum[K Stringer, V any](shardsNum int) MMapOpt[K, V] {
	if shardsNum < 2 {
		shardsNum = defaultShardCount
	}
	
	return func(mulMap *ConcurrentMulMap[K, V]) {
		mulMap.shardsNum = shardsNum
		mulMap.shards = make([]*concurrentMulMapShared[K, V], shardsNum)
	}
}

func WithShardFn[K Stringer, V any](f func(key K) uint32) MMapOpt[K, V] {
	return func(mulMap *ConcurrentMulMap[K, V]) {
		mulMap.shardFn = f
	}
}

// A "thread" safe string to anything map.
type concurrentMulMapShared[K Stringer, V any] struct {
	items        map[K]V
	sync.RWMutex // Read Write mutex, guards access to internal map.
}

func create[K Stringer, V any](opts ...MMapOpt[K, V]) *ConcurrentMulMap[K, V] {
	m := &ConcurrentMulMap[K, V]{
		shardFn:   strfnv32[K],
		shardsNum: defaultShardCount,
		shards:    make([]*concurrentMulMapShared[K, V], defaultShardCount),
	}
	
	m.apply(opts)
	
	return m
}

// Creates a new concurrent map.
func NewMMap[K Stringer, V any](opts ...MMapOpt[K, V]) *ConcurrentMulMap[K, V] {
	return create[K, V](opts...)
}

// getShard returns shard under given key
func (m *ConcurrentMulMap[K, V]) getShard(key K) *concurrentMulMapShared[K, V] {
	idx := uint(m.shardFn(key)) % uint(m.shardsNum)
	m.needAllocate(int(idx))
	
	return m.shards[idx]
}

func (m *ConcurrentMulMap[K, V]) needAllocate(idx int) {
	if m.shards[idx] == nil {
		m.shards[idx] = &concurrentMulMapShared[K, V]{items: make(map[K]V)}
	}
}

func (m *ConcurrentMulMap[K, V]) MSet(data map[K]V) {
	for key, value := range data {
		shard := m.getShard(key)
		shard.Lock()
		shard.items[key] = value
		shard.Unlock()
	}
}

// Sets the given value under the specified key.
func (m *ConcurrentMulMap[K, V]) Set(key K, value V) {
	// Get map shard.
	shard := m.getShard(key)
	shard.Lock()
	shard.items[key] = value
	shard.Unlock()
}

// Callback to return new element to be inserted into the map
// It is called while lock is held, therefore it MUST NOT
// try to access other keys in same map, as it can lead to deadlock since
// Go sync.RWLock is not reentrant
type UpsertCb[V any] func(exist bool, valueInMap V, newValue V) V

// Insert or Update - updates existing element or inserts a new one using UpsertCb
func (m *ConcurrentMulMap[K, V]) Upsert(key K, value V, cb UpsertCb[V]) (res V) {
	shard := m.getShard(key)
	shard.Lock()
	v, ok := shard.items[key]
	res = cb(ok, v, value)
	shard.items[key] = res
	shard.Unlock()
	return res
}

// Sets the given value under the specified key if no value was associated with it.
func (m *ConcurrentMulMap[K, V]) SetIfAbsent(key K, value V) bool {
	// Get map shard.
	shard := m.getShard(key)
	shard.Lock()
	_, ok := shard.items[key]
	if !ok {
		shard.items[key] = value
	}
	shard.Unlock()
	return !ok
}

// Get retrieves an element from map under given key.
func (m *ConcurrentMulMap[K, V]) Get(key K) (V, bool) {
	// Get shard
	shard := m.getShard(key)
	shard.RLock()
	// Get item from shard.
	val, ok := shard.items[key]
	shard.RUnlock()
	return val, ok
}

// Count returns the number of elements within the map.
func (m *ConcurrentMulMap[K, V]) Count() int {
	count := 0
	for i := 0; i < m.shardsNum; i++ {
		shard := m.shards[i]
		if shard == nil {
			continue
		}
		
		shard.RLock()
		count += len(shard.items)
		shard.RUnlock()
	}
	return count
}

// Looks up an item under specified key
func (m *ConcurrentMulMap[K, V]) Has(key K) bool {
	// Get shard
	shard := m.getShard(key)
	shard.RLock()
	// See if element is within shard.
	_, ok := shard.items[key]
	shard.RUnlock()
	return ok
}

// Remove removes an element from the map.
func (m *ConcurrentMulMap[K, V]) Remove(key K) {
	// Try to get shard.
	shard := m.getShard(key)
	shard.Lock()
	delete(shard.items, key)
	shard.Unlock()
}

// RemoveCb is a callback executed in a map.RemoveCb() call, while Lock is held
// If returns true, the element will be removed from the map
type RemoveCb[K any, V any] func(key K, v V, exists bool) bool

// RemoveCb locks the shard containing the key, retrieves its current value and calls the callback with those params
// If callback returns true and element exists, it will remove it from the map
// Returns the value returned by the callback (even if element was not present in the map)
func (m *ConcurrentMulMap[K, V]) RemoveCb(key K, cb RemoveCb[K, V]) bool {
	// Try to get shard.
	shard := m.getShard(key)
	shard.Lock()
	v, ok := shard.items[key]
	remove := cb(key, v, ok)
	if remove && ok {
		delete(shard.items, key)
	}
	shard.Unlock()
	return remove
}

// Pop removes an element from the map and returns it
func (m *ConcurrentMulMap[K, V]) Pop(key K) (v V, exists bool) {
	// Try to get shard.
	shard := m.getShard(key)
	shard.Lock()
	v, exists = shard.items[key]
	delete(shard.items, key)
	shard.Unlock()
	return v, exists
}

// IsEmpty checks if map is empty.
func (m *ConcurrentMulMap[K, V]) IsEmpty() bool {
	return m.Count() == 0
}

// Used by the Iter & IterBuffered functions to wrap two variables together over a channel,
type Tuple[K comparable, V any] struct {
	Key K
	Val V
}

// Iter returns an iterator which could be used in a for range loop.
//
// Deprecated: using IterBuffered() will get a better performence
func (m *ConcurrentMulMap[K, V]) Iter() <-chan Tuple[K, V] {
	chans := snapshot(m)
	ch := make(chan Tuple[K, V])
	go fanIn(chans, ch)
	return ch
}

// IterBuffered returns a buffered iterator which could be used in a for range loop.
func (m *ConcurrentMulMap[K, V]) IterBuffered() <-chan Tuple[K, V] {
	chans := snapshot(m)
	total := 0
	for _, c := range chans {
		total += cap(c)
	}
	ch := make(chan Tuple[K, V], total)
	go fanIn(chans, ch)
	return ch
}

// Clear removes all items from map.
func (m *ConcurrentMulMap[K, V]) Clear() {
	for item := range m.IterBuffered() {
		m.Remove(item.Key)
	}
}

// Returns a array of channels that contains elements in each shard,
// which likely takes a snapshot of `m`.
// It returns once the size of each buffered channel is determined,
// before all the channels are populated using goroutines.
func snapshot[K Stringer, V any](m *ConcurrentMulMap[K, V]) (chans []chan Tuple[K, V]) {
	// When you access map items before initializing.
	if len(m.shards) == 0 {
		panic(`container.ConcurrentMulMap is not initialized. Should run New() before usage.`)
	}
	chans = make([]chan Tuple[K, V], m.shardsNum)
	wg := sync.WaitGroup{}
	wg.Add(m.shardsNum)
	// Foreach shard.
	for index, shard := range m.shards {
		go func(index int, shard *concurrentMulMapShared[K, V]) {
			// Foreach key, value pair.
			shard.RLock()
			chans[index] = make(chan Tuple[K, V], len(shard.items))
			wg.Done()
			for key, val := range shard.items {
				chans[index] <- Tuple[K, V]{key, val}
			}
			shard.RUnlock()
			close(chans[index])
		}(index, shard)
	}
	wg.Wait()
	return chans
}

// fanIn reads elements from channels `chans` into channel `out`
func fanIn[K Stringer, V any](chans []chan Tuple[K, V], out chan Tuple[K, V]) {
	wg := sync.WaitGroup{}
	wg.Add(len(chans))
	for _, ch := range chans {
		go func(ch chan Tuple[K, V]) {
			for t := range ch {
				out <- t
			}
			wg.Done()
		}(ch)
	}
	wg.Wait()
	close(out)
}

// Items returns all items as map[string]V
func (m *ConcurrentMulMap[K, V]) Items() map[K]V {
	tmp := make(map[K]V)
	
	// Insert items to temporary map.
	for item := range m.IterBuffered() {
		tmp[item.Key] = item.Val
	}
	
	return tmp
}

// Iterator callbacalled for every key,value found in
// maps. RLock is held for all calls for a given shard
// therefore callback sess consistent view of a shard,
// but not across the shards
type IterCb[K Stringer, V any] func(key K, v V)

// Callback based iterator, cheapest way to read
// all elements in a map.
func (m *ConcurrentMulMap[K, V]) IterCb(fn IterCb[K, V]) {
	for idx := range m.shards {
		shard := (m.shards)[idx]
		shard.RLock()
		for key, value := range shard.items {
			fn(key, value)
		}
		shard.RUnlock()
	}
}

// Keys returns all keys as []string
func (m *ConcurrentMulMap[K, V]) Keys() []K {
	count := m.Count()
	ch := make(chan K, count)
	go func() {
		// Foreach shard.
		wg := sync.WaitGroup{}
		wg.Add(m.shardsNum)
		for _, shard := range m.shards {
			go func(shard *concurrentMulMapShared[K, V]) {
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
	keys := make([]K, 0, count)
	for k := range ch {
		keys = append(keys, k)
	}
	return keys
}

// Reviles ConcurrentMulMap "private" variables to json marshal.
func (m *ConcurrentMulMap[K, V]) MarshalJSON() ([]byte, error) {
	// Create a temporary map, which will hold all item spread across shards.
	tmp := make(map[K]V)
	
	// Insert items to temporary map.
	for item := range m.IterBuffered() {
		tmp[item.Key] = item.Val
	}
	return json.Marshal(tmp)
}

func strfnv32[K Stringer](key K) uint32 {
	return fnv32(key.String())
}

func fnv32(key string) uint32 {
	hash := uint32(2166136261)
	const prime32 = uint32(16777619)
	keyLength := len(key)
	for i := 0; i < keyLength; i++ {
		hash *= prime32
		hash ^= uint32(key[i])
	}
	return hash
}

// Reverse process of Marshal.
func (m *ConcurrentMulMap[K, V]) UnmarshalJSON(b []byte) (err error) {
	tmp := make(map[K]V)
	
	// Unmarshal into a single map.
	if err := json.Unmarshal(b, &tmp); err != nil {
		return err
	}
	
	// foreach key,value pair in temporary map insert into our concurrent map.
	for key, val := range tmp {
		m.Set(key, val)
	}
	return nil
}

type String string

func (s String) String() string {
	return string(s)
}

type Rune rune

func (s Rune) String() string {
	return string(s)
}

type Runes []rune

func (s Runes) String() string {
	return string(s)
}

type Bytes []byte

func (s Bytes) String() string {
	return string(s)
}

type Int int

func (i Int) String() string {
	return strconv.Itoa(int(i))
}

type Int8 int8

func (i Int8) String() string {
	return strconv.Itoa(int(i))
}

type Int16 int16

func (i Int16) String() string {
	return strconv.Itoa(int(i))
}

type Int32 int32

func (i Int32) String() string {
	return strconv.Itoa(int(i))
}

type Int64 int64

func (i Int64) String() string {
	return strconv.FormatInt(int64(i), 10)
}

type Uint uint

func (i Uint) String() string {
	return strconv.Itoa(int(i))
}

type Uint8 uint8

func (i Uint8) String() string {
	return strconv.FormatUint(uint64(i), 10)
}

type Uint16 uint16

func (i Uint16) String() string {
	return strconv.FormatUint(uint64(i), 10)
}

type Uint32 uint32

func (i Uint32) String() string {
	return strconv.FormatUint(uint64(i), 10)
}

type Uint64 uint64

func (i Uint64) String() string {
	return strconv.FormatUint(uint64(i), 10)
}
