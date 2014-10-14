package cmap

import (
	"sync"
)

// TODO: Add Keys function which returns an array of keys for the map.

// A "thread" safe map of type string:Anything.
type ConcurrentMap struct {
	m            map[string]interface{} // map with string key and any type of object as value.
	sync.RWMutex                        // Read Write mutex, guards access to internal map.
}

// Creates a new concurent map.
func NewConcurrentMap() *ConcurrentMap {
	return &ConcurrentMap{m: make(map[string]interface{})}
}

// Adds an element to the map.
func (m *ConcurrentMap) Add(key string, value interface{}) {
	/*m.Lock()
	defer m.Unlock()*/
	m.m[key] = value
}

// Returns the number of elements within the map.
func (m *ConcurrentMap) Count() int {
	return len(m.m)
}

// Retrives an element from map under given key.
func (m *ConcurrentMap) Get(key string) (interface{}, bool) {
	/*m.RLock()
	defer m.RUnlock()*/

	val, err := m.m[key]
	return val, err
}

// Looks up an item under specified key
func (m *ConcurrentMap) Has(key string) bool {
	/*m.RLock()
	defer m.RUnlock()*/
	_, ok := m.m[key]
	return ok
}

// Removes an element from the map.
func (m *ConcurrentMap) Remove(key string) {
	/*m.Lock()
	defer m.Unlock()*/
	delete(m.m, key)
}

// Clears map by constructing a new one.
func (m *ConcurrentMap) Clear() {
	/*m.Lock()
	defer m.Unlock()*/
	m.m = make(map[string]interface{})
}

// Checks if map is empty.
func (m *ConcurrentMap) IsEmpty() bool {
	return len(m.m) == 0
}

// Used by the Iter function to wrap two variables together over a channel,
type Tuple struct {
	Key string
	Val interface{}
}

// Returns an iterator which could be used in a for range loop.
func (m *ConcurrentMap) Iter() <-chan Tuple {
	//ch := make(chan interface{})
	ch := make(chan Tuple)
	go func() {
		/*m.RLock()
		defer m.RUnlock()*/
		for key, val := range m.m {
			ch <- Tuple{key, val}
		}
		close(ch)
	}()
	return ch
}
