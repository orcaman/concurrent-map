package cmap

import (
	"sort"
	"strconv"
	"testing"
)

type Animal struct {
	name string
}

func TestMapCreation(t *testing.T) {
	m := NewConcurretMap()
	if m == nil {
		t.Error("map is null.")
	}

	if m.Count() != 0 {
		t.Error("new map should be empty.")
	}
}

func TestInsert(t *testing.T) {
	m := NewConcurretMap()
	elephant := Animal{"elephant"}
	monkey := Animal{"monkey"}

	m.Add("elephant", elephant)
	m.Add("monkey", monkey)

	if m.Count() != 2 {
		t.Error("map should contain exactly two elements.")
	}
}

func TestGet(t *testing.T) {
	m := NewConcurretMap()

	// Get a missing element.
	val, ok := m.Get("Money")

	if ok == true {
		t.Error("ok should be false when item is missing from map.")
	}

	if val != nil {
		t.Error("Missing values should return as null.")
	}

	elephant := Animal{"elephant"}
	m.Add("elephant", elephant)

	// Retrieve inserted element.

	tmp, ok := m.Get("elephant")
	elephant = tmp.(Animal) // Type assertion.

	if ok == false {
		t.Error("ok should be true for item stored within the map.")
	}

	if &elephant == nil {
		t.Error("expecting an element, not null.")
	}

	if elephant.name != "elephant" {
		t.Error("item was modified.")
	}
}

func TestHas(t *testing.T) {
	m := NewConcurretMap()

	// Get a missing element.
	if m.Has("Money") == true {
		t.Error("element shouldn't exists")
	}

	elephant := Animal{"elephant"}
	m.Add("elephant", elephant)

	if m.Has("elephant") == false {
		t.Error("element exists, expecting Has to return True.")
	}
}

func TestRemove(t *testing.T) {
	m := NewConcurretMap()

	monkey := Animal{"monkey"}
	m.Add("monkey", monkey)

	m.Remove("monkey")

	if m.Count() != 0 {
		t.Error("Expecting count to be zero once item was removed.")
	}

	temp, ok := m.Get("monkey")

	if ok != false {
		t.Error("Expecting ok to be false for missing items.")
	}

	if temp != nil {
		t.Error("Expecting item to be nil after its removal.")
	}

	// Remove a none existing element.
	m.Remove("noone")
}

func TestCount(t *testing.T) {
	m := NewConcurretMap()
	for i := 0; i < 100; i++ {
		m.Add(strconv.Itoa(i), Animal{strconv.Itoa(i)})
	}

	if m.Count() != 100 {
		t.Error("Expecting 100 element within map.")
	}
}

func TestClear(t *testing.T) {
	m := NewConcurretMap()

	m.Clear()
	if m.Count() != 0 {
		t.Error("Expecting an empty map")
	}

	monkey := Animal{"monkey"}

	m.Add("monkey", monkey)

	m.Clear()
	if m.Count() != 0 {
		t.Error("Expecting an empty map")
	}

	if &monkey == nil {
		t.Error("Element should still exits")
	}
}

func TestIsEmpty(t *testing.T) {
	m := NewConcurretMap()

	if m.IsEmpty() == false {
		t.Error("new map should be empty")
	}

	m.Add("elephant", Animal{"elephant"})

	if m.IsEmpty() != false {
		t.Error("map shouldn't be empty.")
	}
}

func TestRange(t *testing.T) {
	m := NewConcurretMap()

	// Insert 100 elements.
	for i := 0; i < 100; i++ {
		m.Add(strconv.Itoa(i), Animal{strconv.Itoa(i)})
	}

	counter := 0
	// Iterate over elements.
	for item := range m.Iter() {
		val := item.val

		if val == nil {
			t.Error("Expecting an object.")
		}
		counter++
	}

	if counter != 100 {
		t.Error("We should have counted 100 elements.")
	}
}

func TestConcurrent(t *testing.T) {
	m := NewConcurretMap()
	ch := make(chan int)
	var a [1000]int

	// Using go routines insert 1000 ints into our map.
	for i := 0; i < 1000; i++ {
		go func(j int) {
			// Add item to map.
			m.Add(strconv.Itoa(j), j)

			// Retrieve item from map.
			val, _ := m.Get(strconv.Itoa(j))

			// Write to channel inserted value.
			ch <- val.(int)
		}(i) // Call go routine with current index.
	}

	// Wait for all go routines to finish.
	counter := 0
	for elem := range ch {
		a[counter] = elem
		counter++
		if counter == 1000 {
			break
		}
	}

	// Sorts array, will make is simpler to verify all inserted values we're returned.
	sort.Ints(a[0:1000])

	// Make sure map contains 1000 elements.
	if m.Count() != 1000 {
		t.Error("Expecting 1000 elements.")
	}

	// Make sure all inserted values we're fetched from map.
	for i := 0; i < 1000; i++ {
		if i != a[i] {
			t.Error("missing value", i)
		}
	}
}
