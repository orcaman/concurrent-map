# concurrent map

Golang map doesn't support concurrent reads and writes, concurrent map is a wrapper around Go's map, more specifically around a String -> interface{} kinda map, which enforces concurrency.

## usage

Import the package:

```go
import (
	"github.com/streamrail/concurrent-map"
)

```
and go get it using the goapp gae command:

```bash
goapp get "github.com/streamrail/concurrent-map"
```

The package is now imported under the "cmap" namespace. 

## example


```go

	// Create a new map.
	map := cmap.NewConcurretMap()
	
	// Add item to map, adds "bar" under key "foo"
	map.Add("foo", "bar")

	// Retrieve item from map.
	tmp, ok := map.Get("foo")

	// Checks if item exists
	if ok == true {
		// Map stores items as interface{}, hence we'll have to cast.
		bar := tmp.(string)
	}

	// Removes item under key "foo"
	map.Remove("foo")

```

For more examples have a look at concurrent_map_test.go.


Running tests:
```bash
go test "github.com/streamrail/concurrent-map"
```


## license 
MIT (see [LICENSE](https://github.com/streamrail/concurrent-map/LICENSE) file)