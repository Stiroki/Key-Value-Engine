package model

import "time"

// Record predstavlja jedan par kljuc-vrednost u našem sistemu
type Record struct {
	Key       string
	Value     []byte
	Tombstone bool      // True ako je podatak obrisan (logičko brisanje)
	Timestamp time.Time // Vreme kada je podatak upisan
}
