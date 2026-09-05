package memtable

import (
	"fmt"

	"github.com/Stiroki/Key-Value-Engine/model"
	"github.com/Stiroki/Key-Value-Engine/wal"
)

type MemtablePool struct {
	MaxInstances  int
	Capacity      int
	StructType    string
	WAL           *wal.WAL
	Tables        []*Memtable
	FlushCallback func(records []*model.Record) error
}

func NewMemtablePool(maxInstances int, capacity int, structType string, wal *wal.WAL) *MemtablePool {
	pool := &MemtablePool{
		MaxInstances: maxInstances,
		Capacity:     capacity,
		StructType:   structType,
		WAL:          wal,
		Tables:       make([]*Memtable, 0),
	}

	pool.Tables = append(pool.Tables, NewMemtable(capacity, structType, wal))
	return pool
}

func (p *MemtablePool) Put(record *model.Record) error {
	err := p.WAL.Put(record)
	if err != nil {
		return fmt.Errorf("Greska pri upisu u WAL: %v", err)
	}

	p.Tables[0].Data.Put(record)

	if p.Tables[0].Data.Size() < p.Capacity {
		return nil
	}

	if len(p.Tables) >= p.MaxInstances {
		fmt.Println("[POOL] Svih N tabela je popunjeno - flush svih na disk...")
		err := p.FlushAll()
		if err != nil {
			return fmt.Errorf("Greska pri flush-u: %v", err)
		}
	} else {
		fmt.Printf("[POOL] Rotacija tabela(%d/%d popunjeno)\n", len(p.Tables), p.MaxInstances)
		newTable := NewMemtable(p.Capacity, p.StructType, p.WAL)
		p.Tables = append([]*Memtable{newTable}, p.Tables...)
	}

	return nil

}

func (p *MemtablePool) Get(key string) (*model.Record, bool) {
	for _, table := range p.Tables {
		record, found := table.Get(key)
		if found {
			return record, true
		}
	}
	return nil, false
}

func (p *MemtablePool) PutDirect(record *model.Record) {
	p.Tables[0].Data.Put(record)

	if p.Tables[0].Data.Size() >= p.Capacity && len(p.Tables) < p.MaxInstances {
		newTable := NewMemtable(p.Capacity, p.StructType, p.WAL)
		p.Tables = append([]*Memtable{newTable}, p.Tables...)
	}
	if len(p.Tables) >= p.MaxInstances {
		p.FlushAll()
	}
}

func (p *MemtablePool) FlushAll() error {
	if p.FlushCallback == nil {
		fmt.Println("FlushCallback nije podesen!")
		return nil
	}

	for i := len(p.Tables) - 1; i >= 0; i-- {
		records := p.Tables[i].Data.GetAll()
		if len(records) == 0 {
			continue
		}
		err := p.FlushCallback(records)
		if err != nil {
			return fmt.Errorf("Greska pri flush-u tabele %d: %v", i, err)
		}
	}

	p.Tables = []*Memtable{NewMemtable(p.Capacity, p.StructType, p.WAL)}

	fmt.Println("[POOL] Sve tabele flsuh-ovane i resetovana.")
	return nil
}
