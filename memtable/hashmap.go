package memtable

import (
	"sort"

	"github.com/Stiroki/Key-Value-Engine/model"
)

type HashMap struct {
	data map[string]*model.Record
}

// NewHashMap kreira i vraca novu Hash mapu
func NewHashMap() *HashMap {
	return &HashMap{
		data: make(map[string]*model.Record),
	}
}

func (hm *HashMap) Put(record *model.Record) {
	hm.data[record.Key] = record
}

func (hm *HashMap) Get(key string) (*model.Record, bool) {
	record, exists := hm.data[key]
	return record, exists
}

func (hm *HashMap) Size() int {
	return len(hm.data)
}

func (hm *HashMap) Clear() {
	hm.data = make(map[string]*model.Record)
}

// GetAll vraca sve sortirane po kljucevima
func (hm *HashMap) GetAll() []*model.Record {
	var records []*model.Record
	var keys []string

	for key := range hm.data {
		keys = append(keys, key)
	}

	sort.Strings(keys)

	for _, key := range keys {
		records = append(records, hm.data[key])
	}

	return records
}
