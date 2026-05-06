package sstable

import (
	"hash/fnv"
	"math"
)

// BloomFilter predstavlja strukturu za probabilističku proveru postojanja elemenata
type BloomFilter struct {
	BitSet []byte // Go nema niz bitova, pa koristimo niz bajtova i radimo bitske operacije
	M      uint   // Veličina niza u bitovima
	K      uint   // Broj heš funkcija
}

// NewBloomFilter kreira novi filter i računa optimalne M i K vrednosti
func NewBloomFilter(expectedElements int, falsePositiveRate float64) *BloomFilter {
	m := calculateM(expectedElements, falsePositiveRate)
	k := calculateK(expectedElements, m)

	return &BloomFilter{
		// Delimo M sa 8 jer je 1 bajt = 8 bitova, dodajemo 7 da zaokružimo na gore
		BitSet: make([]byte, (m+7)/8),
		M:      m,
		K:      k,
	}
}

// calculateM računa optimalnu veličinu bitseta
func calculateM(expectedElements int, falsePositiveRate float64) uint {
	num := float64(expectedElements) * math.Log(falsePositiveRate)
	den := math.Log(2.0) * math.Log(2.0)
	return uint(math.Ceil(-num / den))
}

// calculateK računa optimalan broj heš funkcija
func calculateK(expectedElements int, m uint) uint {
	return uint(math.Ceil((float64(m) / float64(expectedElements)) * math.Log(2.0)))
}

// hashValues generiše K različitih heš vrednosti za dati ključ
func (bf *BloomFilter) hashValues(key []byte) []uint {
	var result []uint
	h := fnv.New32a()

	for i := uint(0); i < bf.K; i++ {
		h.Reset()
		// Dodajemo 'i' kao takozvani salt na kraj ključa da bismo dobili različite heševe
		h.Write(append(key, byte(i)))
		hashVal := h.Sum32()
		result = append(result, uint(hashVal)%bf.M)
	}
	return result
}

// Add dodaje ključ u Bloom filter
func (bf *BloomFilter) Add(key []byte) {
	hashes := bf.hashValues(key)
	for _, hash := range hashes {
		byteIdx := hash / 8                 // Koji bajt u nizu
		bitIdx := hash % 8                  // Koji bit u tom bajtu
		bf.BitSet[byteIdx] |= (1 << bitIdx) // Postavi taj bit na 1 (OR operacija)
	}
}

// Has proverava da li ključ potencijalno postoji u filteru
func (bf *BloomFilter) Has(key []byte) bool {
	hashes := bf.hashValues(key)
	for _, hash := range hashes {
		byteIdx := hash / 8
		bitIdx := hash % 8
		// Ako je bilo koji od izračunatih bitova 0, element sigurno nije tu
		if (bf.BitSet[byteIdx] & (1 << bitIdx)) == 0 {
			return false
		}
	}
	return true // Ako su svi bitovi 1, onda verovatno postoji
}
