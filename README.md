# concurrent map [![Circle CI](https://circleci.com/gh/streamrail/concurrent-map.png?style=badge)](https://circleci.com/gh/streamrail/concurrent-map)

Golang map doesn't support concurrent reads and writes, Please see (http://golang.org/doc/faq#atomic_maps and http://blog.golang.org/go-maps-in-action), in case you're using multiple Go routines to read and write concurrently from a map some form of guard mechanism should be in-place.

Concurrent map is a wrapper around Go's map, more specifically around a String -> interface{} kinda map, which enforces concurrency.

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
	map := cmap.New()
	
	// Sets item within map, sets "bar" under key "foo"
	map.Set("foo", "bar")

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
MIT (see [LICENSE](https://github.com/streamrail/concurrent-map/blob/master/LICENSE) file)