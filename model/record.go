package model

import "time"

// Record predstavlja jedan par kljuc-vrednost
type Record struct {
	Key       string
	Value     []byte
	Tombstone bool // Oznaka za logicko brisanje
	Timestamp time.Time
}
