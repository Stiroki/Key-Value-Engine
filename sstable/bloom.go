package sstable

import (
	"hash/fnv"
	"math"
)

// BloomFilter predstavlja strukturu za probabilisticku proveru postojanja elemenata
type BloomFilter struct {
	BitSet []byte // Niz bajtova nad kojima radimo operacije bitova
	M      uint   // Velicina niza u bit-ovima
	K      uint   // Broj hes funkcija
}

// NewBloomFilter kreira novi filter i racuna optimalne M i K vrednosti
func NewBloomFilter(expectedElements int, falsePositiveRate float64) *BloomFilter {
	m := calculateM(expectedElements, falsePositiveRate)
	k := calculateK(expectedElements, m)

	return &BloomFilter{
		BitSet: make([]byte, (m+7)/8),
		M:      m,
		K:      k,
	}
}

// calculateM racuna optimalnu velicinu M
func calculateM(expectedElements int, falsePositiveRate float64) uint {
	num := float64(expectedElements) * math.Log(falsePositiveRate)
	den := math.Log(2.0) * math.Log(2.0)
	return uint(math.Ceil(-num / den))
}

// calculateK racuna optimalan broj hes funkcije K
func calculateK(expectedElements int, m uint) uint {
	return uint(math.Ceil((float64(m) / float64(expectedElements)) * math.Log(2.0)))
}

// hashValues generise K hash vrednosti za dati kljuc
func (bf *BloomFilter) hashValues(key []byte) []uint {
	var result []uint
	h := fnv.New32a()

	for i := uint(0); i < bf.K; i++ {
		h.Reset()
		h.Write(append(key, byte(i)))
		hashVal := h.Sum32()
		result = append(result, uint(hashVal)%bf.M)
	}
	return result
}

// Add dodaje kljuc u Bloom filter
func (bf *BloomFilter) Add(key []byte) {
	hashes := bf.hashValues(key)
	for _, hash := range hashes {
		byteIdx := hash / 8                 // Koji bajt u nizu
		bitIdx := hash % 8                  // Koji bit u tom bajtu
		bf.BitSet[byteIdx] |= (1 << bitIdx) // Postavi taj bit na 1
	}
}

// Has proverava da li kljuc potencijalno postoji u filteru
func (bf *BloomFilter) Has(key []byte) bool {
	hashes := bf.hashValues(key)
	for _, hash := range hashes {
		byteIdx := hash / 8
		bitIdx := hash % 8
		if (bf.BitSet[byteIdx] & (1 << bitIdx)) == 0 {
			return false
		}
	}
	return true // Ako su svi bitovi 1, onda verovatno postoji
}
