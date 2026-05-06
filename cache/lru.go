package cache

import (
	"container/list"
)

// CacheEntry predstavlja jedan element u listi
type CacheEntry struct {
	Key   string
	Value []byte // Ovde možeš čuvati konkretnu vrednost ili ceo blok podataka, zavisno od dizajna
}

type LRUCache struct {
	capacity int
	items    map[string]*list.Element
	list     *list.List
}

// NewLRUCache kreira novu instancu keša
func NewLRUCache(capacity int) *LRUCache {
	return &LRUCache{
		capacity: capacity,
		items:    make(map[string]*list.Element),
		list:     list.New(),
	}
}

// Get proverava da li je ključ u kešu.
func (c *LRUCache) Get(key string) ([]byte, bool) {
	if element, exists := c.items[key]; exists {
		// Ako postoji, pomeri taj element na početak liste (jer je upravo korišćen)
		c.list.MoveToFront(element)

		// Vrati vrednost i true
		entry := element.Value.(*CacheEntry) // Kastujemo iz 'any' tipa u naš CacheEntry
		return entry.Value, true
	}

	return nil, false
}

// Put dodaje novi element u keš.
func (c *LRUCache) Put(key string, value []byte) {
	// Proveri da li ključ već postoji.
	if element, exists := c.items[key]; exists {
		// Ako postoji, ažuriraj vrednost i pomeri ga na početak liste.
		c.list.MoveToFront(element)
		entry := element.Value.(*CacheEntry)
		entry.Value = value
		return
	}

	// Ako ne postoji, napravi novi entry, dodaj ga na početak liste i u mapu.
	newEntry := &CacheEntry{
		Key:   key,
		Value: value,
	}
	element := c.list.PushFront(newEntry)
	c.items[key] = element

	// Proveri da li je veličina liste sada veća od 'capacity'.
	if c.list.Len() > c.capacity {
		// Ako jeste, obriši poslednji element iz liste i iz mape.
		backElement := c.list.Back()
		if backElement != nil {
			// Uklanjamo iz liste
			c.list.Remove(backElement)

			// Uklanjamo iz mape
			backEntry := backElement.Value.(*CacheEntry)
			delete(c.items, backEntry.Key)
		}
	}
}
