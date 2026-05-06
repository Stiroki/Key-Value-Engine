package memtable

import (
	"math/rand"
	"time"

	"github.com/Stiroki/Key-Value-Engine/model"
)

const maxLevel = 12 // Maksimalan broj nivoa za preskakanje
const p = 0.5       // Verovatnoca za podizanje nivoa

type SkipListNode struct {
	Record  *model.Record
	Forward []*SkipListNode // Niz pokazivaca na sledece elemente po nivoima
}

type SkipList struct {
	head  *SkipListNode
	level int
	size  int
}

// NewSkipList kreira novu praznu Skip listu
func NewSkipList() *SkipList {
	// Inicijalizujemo generator slučajnih brojeva
	rand.Seed(time.Now().UnixNano())
	return &SkipList{
		head:  &SkipListNode{Forward: make([]*SkipListNode, maxLevel)},
		level: 1,
		size:  0,
	}
}

// randomLevel odlucuje na koliko nivoa ce se cvor "podici"
func (sl *SkipList) randomLevel() int {
	lvl := 1
	for rand.Float32() < p && lvl < maxLevel {
		lvl++
	}
	return lvl
}

func (sl *SkipList) Put(record *model.Record) {
	update := make([]*SkipListNode, maxLevel)
	curr := sl.head

	// Tražimo poziciju za ubacivanje na svakom nivou
	for i := sl.level - 1; i >= 0; i-- {
		for curr.Forward[i] != nil && curr.Forward[i].Record.Key < record.Key {
			curr = curr.Forward[i]
		}
		update[i] = curr
	}

	curr = curr.Forward[0]

	// Ako kljuc vec postoji u memoriji, samo azuriramo zapis (npr. pri brisanju - Tombstone)
	if curr != nil && curr.Record.Key == record.Key {
		curr.Record = record
		return
	}

	// Kreiramo novi nivo
	lvl := sl.randomLevel()
	if lvl > sl.level {
		for i := sl.level; i < lvl; i++ {
			update[i] = sl.head
		}
		sl.level = lvl
	}

	// Kreiramo i ubacujemo novi cvor
	newNode := &SkipListNode{
		Record:  record,
		Forward: make([]*SkipListNode, lvl),
	}

	for i := 0; i < lvl; i++ {
		newNode.Forward[i] = update[i].Forward[i]
		update[i].Forward[i] = newNode
	}
	sl.size++
}

func (sl *SkipList) Get(key string) (*model.Record, bool) {
	curr := sl.head
	for i := sl.level - 1; i >= 0; i-- {
		for curr.Forward[i] != nil && curr.Forward[i].Record.Key < key {
			curr = curr.Forward[i]
		}
	}
	curr = curr.Forward[0]
	if curr != nil && curr.Record.Key == key {
		return curr.Record, true
	}
	return nil, false
}

// GetAll vraca sve zapise koji su VEC SORTIRANI (prednost Skip liste!)
func (sl *SkipList) GetAll() []*model.Record {
	var records []*model.Record
	curr := sl.head.Forward[0]
	for curr != nil {
		records = append(records, curr.Record)
		curr = curr.Forward[0]
	}
	return records
}

func (sl *SkipList) Size() int {
	return sl.size
}

func (sl *SkipList) Clear() {
	sl.head = &SkipListNode{Forward: make([]*SkipListNode, maxLevel)}
	sl.level = 1
	sl.size = 0
}
