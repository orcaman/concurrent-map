package cmap

import "fmt"

// every key must implements this interface
type Key interface {
	HashCode() string
}

// two typical example
type Integer int

func (this Integer) HashCode() string {
	return fmt.Sprintf("%d", this)
}

type String string

func (this String) HashCode() string {
	return string(this)
}
