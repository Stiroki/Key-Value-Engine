package memtable

import "github.com/Stiroki/Key-Value-Engine/model" // Prilagodi sa crticom ako treba!

// Structure interfejs definiše metode koje svaka Memtable struktura mora da ima
type Structure interface {
	Put(record *model.Record)             // Dodaje ili ažurira zapis
	Get(key string) (*model.Record, bool) // Vraća zapis i boolean da li je pronađen
	GetAll() []*model.Record              // Vraća sve zapise (potrebno za prebacivanje na disk)
	Size() int                            // Vraća trenutni broj elemenata
	Clear()                               // Čisti strukturu nakon prebacivanja na disk
}
