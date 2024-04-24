package cmap

import (
	"strconv"
	"sync"
	"testing"
)

func BenchmarkItems(b *testing.B) {
	count := 10000
	shardCount := 32
	m := New(count, shardCount)

	// Insert 10000 elements.
	for i := 0; i < count; i++ {
		m.Set(strconv.Itoa(i), Animal{strconv.Itoa(i)})
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.Items()
	}
}

func BenchmarkMarshalJson(b *testing.B) {
	count := 100
	shardCount := 32
	m := New(count*shardCount, shardCount)

	// Insert 100 elements.
	for i := 0; i < count; i++ {
		m.Set(strconv.Itoa(i), Animal{strconv.Itoa(i)})
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := m.MarshalJSON()
		if err != nil {
			b.FailNow()
		}
	}
}

func BenchmarkStrconv(b *testing.B) {
	for i := 0; i < b.N; i++ {
		strconv.Itoa(i)
	}
}

func BenchmarkSingleInsertAbsent(b *testing.B) {
	shardCount := 32
	m := New(100*shardCount, shardCount)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.Set(strconv.Itoa(i), "value")
	}
}

func BenchmarkSingleInsertAbsentSyncMap(b *testing.B) {
	var m sync.Map
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.Store(strconv.Itoa(i), "value")
	}
}

func BenchmarkSingleInsertPresent(b *testing.B) {
	shardCount := 32
	m := New(100*shardCount, shardCount)
	m.Set("key", "value")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.Set("key", "value")
	}
}

func BenchmarkSingleInsertPresentSyncMap(b *testing.B) {
	var m sync.Map
	m.Store("key", "value")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.Store("key", "value")
	}
}

func benchmarkMultiInsertDifferent(b *testing.B, shardCount int) {
	m := New(100*shardCount, shardCount)
	finished := make(chan struct{}, b.N)
	_, set := GetSet(m, finished)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		go set(strconv.Itoa(i), "value")
	}
	for i := 0; i < b.N; i++ {
		<-finished
	}
}

func BenchmarkMultiInsertDifferentSyncMap(b *testing.B) {
	var m sync.Map
	finished := make(chan struct{}, b.N)
	_, set := GetSetSyncMap(&m, finished)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		go set(strconv.Itoa(i), "value")
	}
	for i := 0; i < b.N; i++ {
		<-finished
	}
}

func BenchmarkMultiInsertDifferent_1_Shard(b *testing.B) {
	benchmarkMultiInsertDifferent(b, 1)
}
func BenchmarkMultiInsertDifferent_16_Shard(b *testing.B) {
	benchmarkMultiInsertDifferent(b, 16)
}
func BenchmarkMultiInsertDifferent_32_Shard(b *testing.B) {
	benchmarkMultiInsertDifferent(b, 32)
}
func BenchmarkMultiInsertDifferent_256_Shard(b *testing.B) {
	benchmarkMultiGetSetDifferent(b, 256)
}

func BenchmarkMultiInsertSame(b *testing.B) {
	shardCount := 32
	m := New(100*shardCount, shardCount)
	finished := make(chan struct{}, b.N)
	_, set := GetSet(m, finished)
	m.Set("key", "value")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		go set("key", "value")
	}
	for i := 0; i < b.N; i++ {
		<-finished
	}
}

func BenchmarkMultiInsertSameSyncMap(b *testing.B) {
	var m sync.Map
	finished := make(chan struct{}, b.N)
	_, set := GetSetSyncMap(&m, finished)
	m.Store("key", "value")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		go set("key", "value")
	}
	for i := 0; i < b.N; i++ {
		<-finished
	}
}

func BenchmarkMultiGetSame(b *testing.B) {
	shardCount := 32
	m := New(100*shardCount, shardCount)
	finished := make(chan struct{}, b.N)
	get, _ := GetSet(m, finished)
	m.Set("key", "value")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		go get("key", "value")
	}
	for i := 0; i < b.N; i++ {
		<-finished
	}
}

func BenchmarkMultiGetSameSyncMap(b *testing.B) {
	var m sync.Map
	finished := make(chan struct{}, b.N)
	get, _ := GetSetSyncMap(&m, finished)
	m.Store("key", "value")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		go get("key", "value")
	}
	for i := 0; i < b.N; i++ {
		<-finished
	}
}

func benchmarkMultiGetSetDifferent(b *testing.B, shardCount int) {
	m := New(100*shardCount, shardCount)
	finished := make(chan struct{}, 2*b.N)
	get, set := GetSet(m, finished)
	m.Set("-1", "value")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		go set(strconv.Itoa(i-1), "value")
		go get(strconv.Itoa(i), "value")
	}
	for i := 0; i < 2*b.N; i++ {
		<-finished
	}
}

func BenchmarkMultiGetSetDifferentSyncMap(b *testing.B) {
	var m sync.Map
	finished := make(chan struct{}, 2*b.N)
	get, set := GetSetSyncMap(&m, finished)
	m.Store("-1", "value")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		go set(strconv.Itoa(i-1), "value")
		go get(strconv.Itoa(i), "value")
	}
	for i := 0; i < 2*b.N; i++ {
		<-finished
	}
}

func BenchmarkMultiGetSetDifferent_1_Shard(b *testing.B) {
	benchmarkMultiGetSetDifferent(b, 1)
}
func BenchmarkMultiGetSetDifferent_16_Shard(b *testing.B) {
	benchmarkMultiGetSetDifferent(b, 16)
}
func BenchmarkMultiGetSetDifferent_32_Shard(b *testing.B) {
	benchmarkMultiGetSetDifferent(b, 32)
}
func BenchmarkMultiGetSetDifferent_256_Shard(b *testing.B) {
	benchmarkMultiGetSetDifferent(b, 256)
}

func benchmarkMultiGetSetBlock(b *testing.B, shardCount int) {
	m := New(100*shardCount, shardCount)
	finished := make(chan struct{}, 2*b.N)
	get, set := GetSet(m, finished)
	for i := 0; i < b.N; i++ {
		m.Set(strconv.Itoa(i%(100*shardCount)), "value")
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		go set(strconv.Itoa(i%(100*shardCount)), "value")
		go get(strconv.Itoa(i%(100*shardCount)), "value")
	}
	for i := 0; i < 2*b.N; i++ {
		<-finished
	}
}

func BenchmarkMultiGetSetBlockSyncMap(b *testing.B) {
	var m sync.Map
	shardCount := 32
	finished := make(chan struct{}, 2*b.N)
	get, set := GetSetSyncMap(&m, finished)
	for i := 0; i < b.N; i++ {
		m.Store(strconv.Itoa(i%(100*shardCount)), "value")
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		go set(strconv.Itoa(i%(100*shardCount)), "value")
		go get(strconv.Itoa(i%(100*shardCount)), "value")
	}
	for i := 0; i < 2*b.N; i++ {
		<-finished
	}
}

func BenchmarkMultiGetSetBlock_1_Shard(b *testing.B) {
	benchmarkMultiGetSetBlock(b, 1)
}
func BenchmarkMultiGetSetBlock_16_Shard(b *testing.B) {
	benchmarkMultiGetSetBlock(b, 16)
}
func BenchmarkMultiGetSetBlock_32_Shard(b *testing.B) {
	benchmarkMultiGetSetBlock(b, 32)
}
func BenchmarkMultiGetSetBlock_256_Shard(b *testing.B) {
	benchmarkMultiGetSetBlock(b, 256)
}

func GetSet(m *ConcurrentMap, finished chan struct{}) (set func(key, value string), get func(key, value string)) {
	return func(key, value string) {
		for i := 0; i < 10; i++ {
			m.Get(key)
		}
		finished <- struct{}{}
	}, func(key, value string) {
		for i := 0; i < 10; i++ {
			m.Set(key, value)
		}
		finished <- struct{}{}
	}
}

func GetSetSyncMap(m *sync.Map, finished chan struct{}) (set func(key, value string), get func(key, value string)) {
	return func(key, value string) {
		for i := 0; i < 10; i++ {
			m.Load(key)
		}
		finished <- struct{}{}
	}, func(key, value string) {
		for i := 0; i < 10; i++ {
			m.Store(key, value)
		}
		finished <- struct{}{}
	}
}

func BenchmarkKeys(b *testing.B) {
	count := 100000
	shardCount := 32
	m := New(count, shardCount)

	// Insert 10000 elements.
	for i := 0; i < count; i++ {
		m.Set(strconv.Itoa(i), Animal{strconv.Itoa(i)})
	}
	for i := 0; i < b.N; i++ {
		m.Keys()
	}
}

func BenchmarkKeysSyncMap(b *testing.B) {
	var m sync.Map
	count := 100000
	keys := make([]string, 0)

	// Insert 10000 elements.
	for i := 0; i < count; i++ {
		m.Store(strconv.Itoa(i), Animal{strconv.Itoa(i)})
	}

	for i := 0; i < b.N; i++ {
		m.Range(func(key, value interface{}) bool {
			k, ok := key.(string)
			if !ok {
				return false
			}
			keys = append(keys, k)
			return true
		})
	}
}
