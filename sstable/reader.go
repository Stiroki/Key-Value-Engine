package sstable

import (
	"bytes"
	"encoding/binary"
	"io"

	"github.com/Stiroki/Key-Value-Engine/block"
)

type BMReader struct {
	filepath string
	bm       *block.BlockManager
	offset   int64
}

func NewBMReader(filepath string, bm *block.BlockManager) *BMReader {
	return &BMReader{
		filepath: filepath,
		bm:       bm,
		offset:   0,
	}
}

// Read implementira io.Reader koristeci BlockManager
func (r *BMReader) Read(p []byte) (n int, err error) {
	if len(p) == 0 {
		return 0, nil
	}

	blockIdx := int(r.offset) / r.bm.BlockSize
	offsetInBlock := int(r.offset) % r.bm.BlockSize

	blockData, err := r.bm.ReadBlock(r.filepath, blockIdx)
	if err != nil {
		return 0, err
	}

	if len(blockData) == 0 || offsetInBlock >= len(blockData) {
		return 0, io.EOF
	}

	n = copy(p, blockData[offsetInBlock:])
	r.offset += int64(n)
	return n, nil
}

// Seek implementira io.Seeker
func (r *BMReader) Seek(offset int64, whence int) (int64, error) {
	switch whence {
	case io.SeekStart:
		r.offset = offset
	case io.SeekCurrent:
		r.offset += offset
	}
	return r.offset, nil
}

func LoadBloomFilter(path string, bm *block.BlockManager) (*BloomFilter, error) {
	file := NewBMReader(path, bm)

	var m, k uint64
	if err := binary.Read(file, binary.LittleEndian, &m); err != nil {
		return nil, err
	}
	if err := binary.Read(file, binary.LittleEndian, &k); err != nil {
		return nil, err
	}

	bitSet := make([]byte, (m+7)/8)
	if _, err := io.ReadFull(file, bitSet); err != nil {
		return nil, err
	}

	return &BloomFilter{
		M:      uint(m),
		K:      uint(k),
		BitSet: bitSet,
	}, nil
}

type SSTableReader struct {
	BasePath string
	Bloom    *BloomFilter
	BM       *block.BlockManager
}

func NewSSTableReader(basePath string, bm *block.BlockManager) (*SSTableReader, error) {
	bloom, err := LoadBloomFilter(basePath+"-Filter.db", bm)
	if err != nil {
		return nil, err
	}
	return &SSTableReader{
		BasePath: basePath,
		Bloom:    bloom,
		BM:       bm,
	}, nil
}

func (r *SSTableReader) Find(searchKey string) (*Record, error) {
	keyBytes := []byte(searchKey)

	if !r.Bloom.Has(keyBytes) {
		return nil, nil
	}

	indexOffset, err := r.searchSummary(keyBytes)
	if err != nil {
		return nil, err
	}
	if indexOffset == -1 {
		return nil, nil
	}

	dataOffset, err := r.searchIndex(indexOffset, keyBytes)
	if err != nil {
		return nil, err
	}
	if dataOffset == -1 {
		return nil, nil
	}

	return r.readDataRecord(dataOffset)
}

func (r *SSTableReader) searchSummary(searchKey []byte) (int64, error) {
	file := NewBMReader(r.BasePath+"-Summary.db", r.BM)

	// 1. Pročitaj Min Key
	var minLen uint64
	if err := binary.Read(file, binary.LittleEndian, &minLen); err != nil {
		return -1, err
	}
	minKey := make([]byte, minLen)
	if _, err := io.ReadFull(file, minKey); err != nil {
		return -1, err
	}

	// 2. Pročitaj Max Key
	var maxLen uint64
	if err := binary.Read(file, binary.LittleEndian, &maxLen); err != nil {
		return -1, err
	}
	maxKey := make([]byte, maxLen)
	if _, err := io.ReadFull(file, maxKey); err != nil {
		return -1, err
	}

	// Provera granica opsega
	if bytes.Compare(searchKey, minKey) < 0 || bytes.Compare(searchKey, maxKey) > 0 {
		return -1, nil // Ključ je van opsega ove tabele!
	}

	var lastValidIndexOffset int64 = 0

	// 3. Traženje odgovarajućeg segmenta u Index fajlu
	for {
		var keyLen uint64
		err := binary.Read(file, binary.LittleEndian, &keyLen)
		if err == io.EOF {
			break
		}
		if err != nil {
			return -1, err
		}

		key := make([]byte, keyLen)
		if _, err := io.ReadFull(file, key); err != nil {
			return -1, err
		}

		var indexOffset int64
		if err := binary.Read(file, binary.LittleEndian, &indexOffset); err != nil {
			return -1, err
		}

		cmp := bytes.Compare(key, searchKey)
		if cmp == 0 {
			return indexOffset, nil
		} else if cmp > 0 {
			break
		}

		lastValidIndexOffset = indexOffset
	}

	return lastValidIndexOffset, nil
}

func (r *SSTableReader) searchIndex(startIndexOffset int64, searchKey []byte) (int64, error) {
	file := NewBMReader(r.BasePath+"-Index.db", r.BM)

	_, err := file.Seek(startIndexOffset, io.SeekStart)
	if err != nil {
		return -1, err
	}

	for {
		var keyLen uint64
		err := binary.Read(file, binary.LittleEndian, &keyLen)
		if err == io.EOF {
			break
		}
		if err != nil {
			return -1, err
		}

		key := make([]byte, keyLen)
		if _, err := io.ReadFull(file, key); err != nil {
			return -1, err
		}

		var dataOffset int64
		if err := binary.Read(file, binary.LittleEndian, &dataOffset); err != nil {
			return -1, err
		}

		cmp := bytes.Compare(key, searchKey)
		if cmp == 0 {
			return dataOffset, nil
		} else if cmp > 0 {
			return -1, nil
		}
	}

	return -1, nil
}

func (r *SSTableReader) readDataRecord(dataOffset int64) (*Record, error) {
	file := NewBMReader(r.BasePath+"-Data.db", r.BM)

	_, err := file.Seek(dataOffset, io.SeekStart)
	if err != nil {
		return nil, err
	}

	var keyLen, valLen uint64
	if err := binary.Read(file, binary.LittleEndian, &keyLen); err != nil {
		return nil, err
	}
	if err := binary.Read(file, binary.LittleEndian, &valLen); err != nil {
		return nil, err
	}

	keyBytes := make([]byte, keyLen)
	if _, err := io.ReadFull(file, keyBytes); err != nil {
		return nil, err
	}

	valBytes := make([]byte, valLen)
	if _, err := io.ReadFull(file, valBytes); err != nil {
		return nil, err
	}

	var tombstone bool
	if err := binary.Read(file, binary.LittleEndian, &tombstone); err != nil {
		return nil, err
	}

	var timestamp int64
	if err := binary.Read(file, binary.LittleEndian, &timestamp); err != nil {
		return nil, err
	}

	return &Record{
		Key:       string(keyBytes),
		Value:     valBytes,
		Tombstone: tombstone,
		Timestamp: timestamp,
	}, nil
}
