package sstable

import (
	"encoding/binary"

	"github.com/Stiroki/Key-Value-Engine/block"
)

type Record struct {
	Key       string
	Value     []byte
	Tombstone bool
	Timestamp int64
}

type summaryEntry struct {
	key         []byte
	indexOffset int64
}

type BMWriter struct {
	filepath   string
	bm         *block.BlockManager
	buffer     []byte
	blockIndex int
	Offset     int64
}

func NewBMWriter(filepath string, bm *block.BlockManager) *BMWriter {
	return &BMWriter{
		filepath: filepath,
		bm:       bm,
		buffer:   make([]byte, 0, bm.BlockSize),
		Offset:   0,
	}
}

func (w *BMWriter) Write(p []byte) (n int, err error) {
	written := 0
	for len(p) > 0 {
		spaceInBuf := w.bm.BlockSize - len(w.buffer)

		if spaceInBuf == 0 {
			err = w.bm.WriteBlock(w.filepath, w.blockIndex, w.buffer)
			if err != nil {
				return written, err
			}
			w.blockIndex++
			w.buffer = w.buffer[:0]
			spaceInBuf = w.bm.BlockSize
		}

		toCopy := len(p)
		if toCopy > spaceInBuf {
			toCopy = spaceInBuf
		}

		w.buffer = append(w.buffer, p[:toCopy]...)
		p = p[toCopy:]
		written += toCopy
		w.Offset += int64(toCopy)
	}
	return written, nil
}

func (w *BMWriter) Flush() error {
	if len(w.buffer) > 0 {
		err := w.bm.WriteBlock(w.filepath, w.blockIndex, w.buffer)
		if err != nil {
			return err
		}
		w.buffer = w.buffer[:0]
	}
	return nil
}

type SSTableBuilder struct {
	BasePath      string
	SummaryDegree int
	BM            *block.BlockManager

	dataFile    *BMWriter
	indexFile   *BMWriter
	summaryFile *BMWriter

	bloomFilter *BloomFilter
	merkleVals  [][]byte

	indexEntryCount int
	firstKey        string
	lastKey         string
	summaryEntries  []summaryEntry
}

func NewSSTableBuilder(basePath string, summaryDegree int, expectedElements int, falsePositiveRate float64, bm *block.BlockManager) (*SSTableBuilder, error) {
	return &SSTableBuilder{
		BasePath:      basePath,
		SummaryDegree: summaryDegree,
		BM:            bm,

		dataFile:    NewBMWriter(basePath+"-Data.db", bm),
		indexFile:   NewBMWriter(basePath+"-Index.db", bm),
		summaryFile: NewBMWriter(basePath+"-Summary.db", bm),

		bloomFilter:    NewBloomFilter(expectedElements, falsePositiveRate),
		merkleVals:     make([][]byte, 0),
		summaryEntries: make([]summaryEntry, 0),
	}, nil
}

func (b *SSTableBuilder) WriteRecord(rec *Record) error {
	dataOffset := b.dataFile.Offset

	if b.indexEntryCount == 0 {
		b.firstKey = rec.Key
	}
	b.lastKey = rec.Key

	b.bloomFilter.Add([]byte(rec.Key))

	if !rec.Tombstone && rec.Value != nil {
		b.merkleVals = append(b.merkleVals, rec.Value)
	}

	if err := b.writeDataRecord(rec); err != nil {
		return err
	}

	indexOffset := b.indexFile.Offset
	if err := b.writeIndexEntry([]byte(rec.Key), dataOffset); err != nil {
		return err
	}

	if b.indexEntryCount%b.SummaryDegree == 0 {
		b.summaryEntries = append(b.summaryEntries, summaryEntry{
			key:         []byte(rec.Key),
			indexOffset: indexOffset,
		})
	}
	b.indexEntryCount++

	return nil
}

func (b *SSTableBuilder) writeDataRecord(rec *Record) error {
	keyBytes := []byte(rec.Key)
	keyLen := uint64(len(keyBytes))
	valLen := uint64(len(rec.Value))

	if err := binary.Write(b.dataFile, binary.LittleEndian, keyLen); err != nil {
		return err
	}
	if err := binary.Write(b.dataFile, binary.LittleEndian, valLen); err != nil {
		return err
	}
	if _, err := b.dataFile.Write(keyBytes); err != nil {
		return err
	}

	if valLen > 0 {
		if _, err := b.dataFile.Write(rec.Value); err != nil {
			return err
		}
	}
	if err := binary.Write(b.dataFile, binary.LittleEndian, rec.Tombstone); err != nil {
		return err
	}
	if err := binary.Write(b.dataFile, binary.LittleEndian, rec.Timestamp); err != nil {
		return err
	}
	return nil
}

func (b *SSTableBuilder) writeIndexEntry(key []byte, dataOffset int64) error {
	keyLen := uint64(len(key))
	if err := binary.Write(b.indexFile, binary.LittleEndian, keyLen); err != nil {
		return err
	}
	if _, err := b.indexFile.Write(key); err != nil {
		return err
	}
	if err := binary.Write(b.indexFile, binary.LittleEndian, dataOffset); err != nil {
		return err
	}
	return nil
}

func (b *SSTableBuilder) Finish() error {
	if err := b.dataFile.Flush(); err != nil {
		return err
	}
	if err := b.indexFile.Flush(); err != nil {
		return err
	}

	// 1. Zapis Min i Max granica na početak Summary fajla
	minKeyBytes := []byte(b.firstKey)
	minKeyLen := uint64(len(minKeyBytes))
	if err := binary.Write(b.summaryFile, binary.LittleEndian, minKeyLen); err != nil {
		return err
	}
	if _, err := b.summaryFile.Write(minKeyBytes); err != nil {
		return err
	}

	maxKeyBytes := []byte(b.lastKey)
	maxKeyLen := uint64(len(maxKeyBytes))
	if err := binary.Write(b.summaryFile, binary.LittleEndian, maxKeyLen); err != nil {
		return err
	}
	if _, err := b.summaryFile.Write(maxKeyBytes); err != nil {
		return err
	}

	// 2. Zapis proređenih stavki indeksa
	for _, entry := range b.summaryEntries {
		kLen := uint64(len(entry.key))
		if err := binary.Write(b.summaryFile, binary.LittleEndian, kLen); err != nil {
			return err
		}
		if _, err := b.summaryFile.Write(entry.key); err != nil {
			return err
		}
		if err := binary.Write(b.summaryFile, binary.LittleEndian, entry.indexOffset); err != nil {
			return err
		}
	}

	if err := b.summaryFile.Flush(); err != nil {
		return err
	}

	// 3. Bloom filter serijalizacija
	bloomWriter := NewBMWriter(b.BasePath+"-Filter.db", b.BM)
	m := uint64(b.bloomFilter.M)
	k := uint64(b.bloomFilter.K)

	if err := binary.Write(bloomWriter, binary.LittleEndian, m); err != nil {
		return err
	}
	if err := binary.Write(bloomWriter, binary.LittleEndian, k); err != nil {
		return err
	}
	if _, err := bloomWriter.Write(b.bloomFilter.BitSet); err != nil {
		return err
	}

	if err := bloomWriter.Flush(); err != nil {
		return err
	}

	return nil
}
