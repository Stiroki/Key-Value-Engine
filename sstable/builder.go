package sstable

import (
	"encoding/binary"
	"os"
)

// Record predstavlja jedan podatak koji nam prosleđuje Memtable
// (Ovo je ista ona struktura o kojoj si se dogovorila sa Osobom A)
type Record struct {
	Key       string
	Value     []byte
	Tombstone bool
	Timestamp int64
}

// SSTableBuilder upravlja procesom kreiranja nove SSTabele
type SSTableBuilder struct {
	BasePath      string // Putanja i osnovno ime tabele (npr. "data/usertable-1")
	SummaryDegree int    // Stepen proređenosti (Zahtev za ocenu 7)

	// Fajlovi u koje upisujemo
	dataFile    *os.File
	indexFile   *os.File
	summaryFile *os.File

	// Pomoćne strukture iz prethodnih koraka
	bloomFilter *BloomFilter
	merkleVals  [][]byte // Ovde skupljamo sve vrednosti za Merkle stablo

	// Praćenje trenutnih pozicija (offseta) u fajlovima
	currentDataOffset  int64
	currentIndexOffset int64
	indexEntryCount    int
}

// NewSSTableBuilder inicijalizuje sve potrebne fajlove i strukture
func NewSSTableBuilder(basePath string, summaryDegree int, expectedElements int, falsePositiveRate float64) (*SSTableBuilder, error) {
	// Otvaramo fajlove za Data, Index i Summary
	dataFile, err := os.Create(basePath + "-Data.db")
	if err != nil {
		return nil, err
	}

	indexFile, err := os.Create(basePath + "-Index.db")
	if err != nil {
		return nil, err
	}

	summaryFile, err := os.Create(basePath + "-Summary.db")
	if err != nil {
		return nil, err
	}

	return &SSTableBuilder{
		BasePath:      basePath,
		SummaryDegree: summaryDegree,
		dataFile:      dataFile,
		indexFile:     indexFile,
		summaryFile:   summaryFile,
		bloomFilter:   NewBloomFilter(expectedElements, falsePositiveRate),
		merkleVals:    make([][]byte, 0),
	}, nil
}

// WriteRecord se poziva za svaki zapis iz Memtable-a (koji moraju biti sortirani!)
func (b *SSTableBuilder) WriteRecord(rec *Record) error {
	keyBytes := []byte(rec.Key)

	// 1. Dodajemo ključ u Bloom Filter
	b.bloomFilter.Add(keyBytes)

	// 2. Dodajemo vrednost u listu za Merkle stablo (samo ako nije Tombstone/brisanje)
	if !rec.Tombstone {
		b.merkleVals = append(b.merkleVals, rec.Value)
	}

	// 3. Upisujemo zapis u DATA fajl
	// Format u Data fajlu: [Dužina ključa(uint64)] [Dužina vrednosti(uint64)] [Ključ] [Vrednost] [Tombstone(bool)] [Timestamp(int64)]
	dataWritten, err := b.writeDataEntry(rec)
	if err != nil {
		return err
	}

	// 4. Upisujemo poziciju ovog zapisa u INDEX fajl
	// Format u Index fajlu: [Dužina ključa(uint64)] [Ključ] [Offset u Data fajlu(int64)]
	indexWritten, err := b.writeIndexEntry(keyBytes, b.currentDataOffset)
	if err != nil {
		return err
	}

	// 5. Proređeni SUMMARY fajl (Zahtev za ocenu 7)
	// Ako je ovo prvi zapis ili je deljiv sa stepenom proređenosti, upiši ga u Summary
	if b.indexEntryCount%b.SummaryDegree == 0 {
		// Format u Summary fajlu: [Dužina ključa(uint64)] [Ključ] [Offset u Index fajlu(int64)]
		err := b.writeSummaryEntry(keyBytes, b.currentIndexOffset)
		if err != nil {
			return err
		}
	}

	// Ažuriramo brojace
	b.currentDataOffset += dataWritten
	b.currentIndexOffset += indexWritten
	b.indexEntryCount++

	return nil
}

// Finish zatvara fajlove i kreira Meta i Filter strukture na disku
func (b *SSTableBuilder) Finish() error {
	// Zatvaramo osnovne fajlove
	b.dataFile.Close()
	b.indexFile.Close()
	b.summaryFile.Close()

	// Serijalizujemo i snimamo Bloom Filter
	filterFile, _ := os.Create(b.BasePath + "-Filter.db")
	defer filterFile.Close()
	// Uprošćeno snimanje filtera (zapišemo M, K i BitSet)
	binary.Write(filterFile, binary.LittleEndian, uint64(b.bloomFilter.M))
	binary.Write(filterFile, binary.LittleEndian, uint64(b.bloomFilter.K))
	filterFile.Write(b.bloomFilter.BitSet)

	// Generišemo Merkle stablo od svih prikupljenih vrednosti
	merkleTree := NewMerkleTree(b.merkleVals)

	// Serijalizujemo Root heš Merkle stabla u Metadata fajl
	metaFile, _ := os.Create(b.BasePath + "-Metadata.db")
	defer metaFile.Close()
	if merkleTree.Root != nil {
		metaFile.Write(merkleTree.Root.Hash)
	}

	return nil
}

// writeDataEntry upisuje jedan zapis u Data fajl i vraća broj upisanih bajtova
func (b *SSTableBuilder) writeDataEntry(rec *Record) (int64, error) {
	var written int64 = 0
	keyBytes := []byte(rec.Key)
	keyLen := uint64(len(keyBytes))
	valLen := uint64(len(rec.Value))

	// 1. Upisujemo dužinu ključa (8 bajtova za uint64)
	if err := binary.Write(b.dataFile, binary.LittleEndian, keyLen); err != nil {
		return 0, err
	}
	written += 8

	// 2. Upisujemo dužinu vrednosti (8 bajtova za uint64)
	if err := binary.Write(b.dataFile, binary.LittleEndian, valLen); err != nil {
		return 0, err
	}
	written += 8

	// 3. Upisujemo sam ključ
	n, err := b.dataFile.Write(keyBytes)
	if err != nil {
		return 0, err
	}
	written += int64(n)

	// 4. Upisujemo samu vrednost
	n, err = b.dataFile.Write(rec.Value)
	if err != nil {
		return 0, err
	}
	written += int64(n)

	// 5. Upisujemo Tombstone marker (paket binary ispisuje bool kao 1 bajt: 0 ili 1)
	if err := binary.Write(b.dataFile, binary.LittleEndian, rec.Tombstone); err != nil {
		return 0, err
	}
	written += 1

	// 6. Upisujemo Timestamp (8 bajtova za int64)
	if err := binary.Write(b.dataFile, binary.LittleEndian, rec.Timestamp); err != nil {
		return 0, err
	}
	written += 8

	return written, nil
}

// writeIndexEntry upisuje ključ i njegovu poziciju (offset) u Data fajlu
func (b *SSTableBuilder) writeIndexEntry(key []byte, dataOffset int64) (int64, error) {
	var written int64 = 0
	keyLen := uint64(len(key))

	// 1. Dužina ključa
	if err := binary.Write(b.indexFile, binary.LittleEndian, keyLen); err != nil {
		return 0, err
	}
	written += 8

	// 2. Sam ključ
	n, err := b.indexFile.Write(key)
	if err != nil {
		return 0, err
	}
	written += int64(n)

	// 3. Offset u Data fajlu (8 bajtova za int64)
	if err := binary.Write(b.indexFile, binary.LittleEndian, dataOffset); err != nil {
		return 0, err
	}
	written += 8

	return written, nil
}

// writeSummaryEntry upisuje ključ i njegovu poziciju (offset) u Index fajlu
func (b *SSTableBuilder) writeSummaryEntry(key []byte, indexOffset int64) error {
	keyLen := uint64(len(key))

	// 1. Dužina ključa
	if err := binary.Write(b.summaryFile, binary.LittleEndian, keyLen); err != nil {
		return err
	}

	// 2. Sam ključ
	if _, err := b.summaryFile.Write(key); err != nil {
		return err
	}

	// 3. Offset u Index fajlu (8 bajtova za int64)
	return binary.Write(b.summaryFile, binary.LittleEndian, indexOffset)
}
