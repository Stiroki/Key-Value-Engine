package cache

import (
	"container/list"
)

// CacheEntry predstavlja jedan element u listi
type CacheEntry struct {
	Key   string
	Value []byte
}

type LRUCache struct {
	capacity int
	items    map[string]*list.Element
	list     *list.List
}

// NewLRUCache kreira novu instancu cache
func NewLRUCache(capacity int) *LRUCache {
	return &LRUCache{
		capacity: capacity,
		items:    make(map[string]*list.Element),
		list:     list.New(),
	}
}

// Get proverava da li je kljuc u cache-u i vraca vrednost ako postoji
func (c *LRUCache) Get(key string) ([]byte, bool) {
	if element, exists := c.items[key]; exists {
		// Ako postoji, pomeri taj element na pocetak liste (jer je upravo koriscen)
		c.list.MoveToFront(element)

		entry := element.Value.(*CacheEntry)
		return entry.Value, true
	}

	return nil, false
}

// Put dodaje novi element u cache.
func (c *LRUCache) Put(key string, value []byte) {
	// Proveri da li kljuc vec postoji
	if element, exists := c.items[key]; exists {
		// Ako postoji, azuriraj vrednost i pomeri ga na pocetak liste.
		c.list.MoveToFront(element)
		entry := element.Value.(*CacheEntry)
		entry.Value = value
		return
	}

	// Ako ne postoji, napravi novi entry, dodaj ga na pocetak liste i u mapu.
	newEntry := &CacheEntry{
		Key:   key,
		Value: value,
	}
	element := c.list.PushFront(newEntry)
	c.items[key] = element

	// Proveri da li je velicina liste veca od capacity-a
	if c.list.Len() > c.capacity {
		// ako jeste, brisemo poslednji element iz liste i iz mape
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

// Remove brise element iz cache-a ako postoji
func (c *LRUCache) Remove(key string) {
	if element, exists := c.items[key]; exists {
		// Uklanjamo ga iz liste
		c.list.Remove(element)
		// Uklanjamo ga iz mape
		delete(c.items, key)
	}
}
