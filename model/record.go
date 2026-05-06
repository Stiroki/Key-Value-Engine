package main

import "time"

//izgled podataka
type Record struct {
	Key       string
	Value     []byte
	Tombstone bool
	Timestamp time.Time
}
