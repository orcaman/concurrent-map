package cmap

import (
	"strconv"
	"sync"
	"testing"
)

var K = 100000

func BenchmarkItemsCMap(b *testing.B) {
	m := New()

	// Insert K elements.
	for i := 0; i < K; i++ {
		m.Set(strconv.Itoa(i), Animal{strconv.Itoa(i)})
	}
	for i := 0; i < K; i++ {
		m.Get(strconv.Itoa(i))
	}
}

func BenchmarkItemsSyncMap(b *testing.B) {
	m := sync.Map{}

	// Insert 100000 elements.
	for i := 0; i < K; i++ {
		m.Store(strconv.Itoa(i), Animal{strconv.Itoa(i)})
	}
	for i := 0; i < K; i++ {
		m.Load(strconv.Itoa(i))
	}
}

func BenchmarkItemsCMapConcurrent(b *testing.B) {
	m := New()
	wg := &sync.WaitGroup{}

	for i := 0; i < K; i++ {
		wg.Add(1)
		go func(i int) {
			m.Set(strconv.Itoa(i), Animal{strconv.Itoa(i)})
			wg.Done()
		}(i)
	}

	wg.Wait()

	for i := 0; i < K; i++ {
		m.Get(strconv.Itoa(i))
	}
}

func BenchmarkItemsSyncMapConcurrent(b *testing.B) {
	m := sync.Map{}
	wg := &sync.WaitGroup{}

	for i := 0; i < K; i++ {
		wg.Add(1)
		go func(i int) {
			m.Store(strconv.Itoa(i), Animal{strconv.Itoa(i)})
			wg.Done()
		}(i)
	}

	wg.Wait()

	for i := 0; i < K; i++ {
		m.Load(strconv.Itoa(i))
	}
}
