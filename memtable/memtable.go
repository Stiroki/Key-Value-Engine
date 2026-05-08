package memtable

import (
	"fmt"

	"github.com/Stiroki/Key-Value-Engine/model"
	"github.com/Stiroki/Key-Value-Engine/wal"
)

type Memtable struct {
	Capacity      int
	Data          Structure
	WAL           *wal.WAL
	FlushCallback func(records []*model.Record) error
}

// NewMemtable inicijalizuje sistem
func NewMemtable(capacity int, structType string, wal *wal.WAL) *Memtable {
	var data Structure

	// Biramo implementaciju na osnovu konfiguracije
	switch structType {
	case "hashmap":
		data = NewHashMap()
	case "skiplist":
		data = NewSkipList()
	case "btree":
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
	// Prvo zapisujemo u WAL
	err := mt.WAL.Put(record)
	if err != nil {
		return fmt.Errorf("greska pri upisu u WAL: %v", err)
	}

	// Pa u memtable
	mt.Data.Put(record)

	// Da li je kapacitet memtable-a popunjen? Ako jeste, pokrecemo flush
	if mt.Data.Size() >= mt.Capacity {
		err := mt.Flush()
		if err != nil {
			return fmt.Errorf("greska pri flush-u: %v", err)
		}
	}

	return nil
}

// Get trazi zapis u memoriji
func (mt *Memtable) Get(key string) (*model.Record, bool) {
	return mt.Data.Get(key)
}

// Flush prebacuje podatke iz memorije na disk (SSTable) i cisti memtable
func (mt *Memtable) Flush() error {
	fmt.Println("[MEMTABLE] Kapacitet je popunjen! Pokrećem Flush (prebacivanje na disk)...")

	// Uzimamo sve zapise iz strukture
	recordsToSave := mt.Data.GetAll()

	// Prosledjivanje Engine-u da kreira SSTabelu sa ovim zapisima
	if mt.FlushCallback != nil {
		err := mt.FlushCallback(recordsToSave)
		if err != nil {
			return fmt.Errorf("engine nije uspeo da kreira SSTabelu: %v", err)
		}
	} else {
		fmt.Println("[UPOZORENJE] FlushCallback nije podešen! Podaci će biti obrisani bez čuvanja na disku.")
	}

	// Cistenje memtable-a nakon flush-a
	mt.Data.Clear()

	fmt.Println("[MEMTABLE] SSTabela je uspešno kreirana i Memtable je očišćen.")
	return nil
}
