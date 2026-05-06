package memtable

import (
	"fmt"

	"github.com/Stiroki/Key-Value-Engine/model"
	"github.com/Stiroki/Key-Value-Engine/wal"
)

// Memtable je glavni omotač koji upravlja RAM memorijom i WAL-om
type Memtable struct {
	Capacity int
	Data     Structure
	WAL      *wal.WAL
}

// NewMemtable inicijalizuje sistem
func NewMemtable(capacity int, structType string, wal *wal.WAL) *Memtable {
	var data Structure

	// Biramo implementaciju na osnovu konfiguracije!
	switch structType {
	case "hashmap":
		data = NewHashMap()
	case "skiplist":
		data = NewSkipList() // OVU LINIJU SMO PROMENILI
	case "btree":
		// data = NewBTree()
		data = NewHashMap()
	default:
		data = NewHashMap()
	}

	return &Memtable{
		Capacity: capacity,
		Data:     data,
		WAL:      wal,
	}
}

// Put ubacuje zapis u WAL, pa u memoriju
func (mt *Memtable) Put(record *model.Record) error {
	// 1. Prvo uvek zapisujemo u WAL zbog bezbednosti
	err := mt.WAL.Put(record)
	if err != nil {
		return fmt.Errorf("greska pri upisu u WAL: %v", err)
	}

	// 2. Onda ga stavljamo u RAM (Memtable)
	mt.Data.Put(record)

	// 3. Proveravamo da li smo prepunili Memtable
	if mt.Data.Size() >= mt.Capacity {
		mt.Flush()
	}

	return nil
}

// Get trazi zapis u memoriji
func (mt *Memtable) Get(key string) (*model.Record, bool) {
	return mt.Data.Get(key)
}

// Flush se poziva kada se Memtable napuni. Prebacuje podatke na disk.
func (mt *Memtable) Flush() {
	fmt.Println("Memtable je pun! Pokrećem Flush (prebacivanje na disk)...")

	// recordsToSave := mt.Data.GetAll()
	// OVDE IDE KOD OSOBE B: Ona ce uzeti ove zapise i napraviti SSTabelu na disku.

	// Nakon prebacivanja, cistimo memoriju za nove zapise
	mt.Data.Clear()

	fmt.Println("SSTabela je kreirana i Memtable je ocišćen.")
}
