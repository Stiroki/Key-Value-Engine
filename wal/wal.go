package wal

import (
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Stiroki/Key-Value-Engine/block"
	"github.com/Stiroki/Key-Value-Engine/model"
)

const (
	FragFull   byte = 0
	FragFirst  byte = 1
	FragMiddle byte = 2
	FragLast   byte = 3
)

const FragHeader = 7

type WAL struct {
	Directory    string
	BlockSize    int
	BlockCount   int // Broj blokova po segmentu
	BM           *block.BlockManager
	CurrentPath  string
	CurrentBlock []byte
	BlockIndex   int // Trenutni indeks bloka unutar segmenta
}

// NewWAL inicijalizuje WAL sa BlockManager-om
func NewWAL(directory string, blockSize int, blockCount int, bm *block.BlockManager) (*WAL, error) {
	err := os.MkdirAll(directory, 0755)
	if err != nil {
		return nil, fmt.Errorf("greska pri kreiranju WAL foldera: %v", err)
	}

	wal := &WAL{
		Directory:    directory,
		BlockSize:    blockSize,
		BlockCount:   blockCount,
		BM:           bm,
		CurrentBlock: make([]byte, 0, blockSize),
		BlockIndex:   0,
	}

	return wal, nil
}

func (w *WAL) openNewSegment() error {
	if w.CurrentPath != "" {
		if err := w.flushCurrentBlock(); err != nil {
			return err
		}
	}

	fileName := fmt.Sprintf("wal_%d.log", time.Now().UnixNano())
	w.CurrentPath = filepath.Join(w.Directory, fileName)
	w.BlockIndex = 0
	w.CurrentBlock = w.CurrentBlock[:0]
	return nil
}

func (w *WAL) flushCurrentBlock() error {
	if len(w.CurrentBlock) == 0 {
		return nil
	}

	// Padding do pune veličine bloka nulama
	if len(w.CurrentBlock) < w.BlockSize {
		padding := make([]byte, w.BlockSize-len(w.CurrentBlock))
		w.CurrentBlock = append(w.CurrentBlock, padding...)
	}

	// Upisujemo blok preko BlockManager-a
	err := w.BM.WriteBlock(w.CurrentPath, w.BlockIndex, w.CurrentBlock)
	if err != nil {
		return fmt.Errorf("greska pri upisu bloka preko BlockManager-a: %v", err)
	}

	w.BlockIndex++
	w.CurrentBlock = w.CurrentBlock[:0]
	return nil
}

func serializeRecord(record *model.Record) []byte {
	keyBytes := []byte(record.Key)
	keySize := uint64(len(keyBytes))
	valSize := uint64(len(record.Value))

	totalSize := 8 + 1 + 8 + 8 + keySize + valSize
	buf := make([]byte, totalSize)
	offset := 0

	binary.LittleEndian.PutUint64(buf[offset:], uint64(record.Timestamp.UnixNano()))
	offset += 8

	if record.Tombstone {
		buf[offset] = 1
	} else {
		buf[offset] = 0
	}
	offset += 1

	binary.LittleEndian.PutUint64(buf[offset:], keySize)
	offset += 8

	binary.LittleEndian.PutUint64(buf[offset:], valSize)
	offset += 8

	copy(buf[offset:], keyBytes)
	offset += int(keySize)

	copy(buf[offset:], record.Value)

	return buf
}

func deserializeRecord(data []byte) model.Record {
	offset := 0

	timestampNano := binary.LittleEndian.Uint64(data[offset:])
	offset += 8

	tombstone := data[offset] == 1
	offset += 1

	keySize := binary.LittleEndian.Uint64(data[offset:])
	offset += 8

	valSize := binary.LittleEndian.Uint64(data[offset:])
	offset += 8

	keyBytes := make([]byte, keySize)
	copy(keyBytes, data[offset:offset+int(keySize)])
	offset += int(keySize)

	valBytes := make([]byte, valSize)
	copy(valBytes, data[offset:offset+int(valSize)])

	return model.Record{
		Key:       string(keyBytes),
		Value:     valBytes,
		Tombstone: tombstone,
		Timestamp: time.Unix(0, int64(timestampNano)),
	}
}

func (w *WAL) writeFragment(fragType byte, data []byte) {
	header := make([]byte, FragHeader)
	header[0] = fragType
	crc := crc32.ChecksumIEEE(data)
	binary.LittleEndian.PutUint32(header[1:5], crc)
	binary.LittleEndian.PutUint16(header[5:7], uint16(len(data)))

	w.CurrentBlock = append(w.CurrentBlock, header...)
	w.CurrentBlock = append(w.CurrentBlock, data...)
}

func (w *WAL) Put(record *model.Record) error {
	data := serializeRecord(record)

	if w.CurrentPath == "" {
		if err := w.openNewSegment(); err != nil {
			return err
		}
	}

	dataOffset := 0
	first := true

	for dataOffset < len(data) {
		remainingInBlock := w.BlockSize - len(w.CurrentBlock)

		// Ako nema mesta ni za zaglavlje, zatvaramo i šaljemo trenutni blok
		if remainingInBlock < FragHeader {
			if err := w.flushCurrentBlock(); err != nil {
				return err
			}

			// Provera da li smo popunili segment
			if w.BlockIndex >= w.BlockCount {
				if err := w.openNewSegment(); err != nil {
					return err
				}
			}
			remainingInBlock = w.BlockSize - len(w.CurrentBlock)
		}

		spaceForData := remainingInBlock - FragHeader
		leftToWrite := len(data) - dataOffset

		if leftToWrite <= spaceForData {
			var fragType byte
			if first {
				fragType = FragFull
			} else {
				fragType = FragLast
			}

			w.writeFragment(fragType, data[dataOffset:dataOffset+leftToWrite])
			dataOffset += leftToWrite
		} else {
			var fragType byte
			if first {
				fragType = FragFirst
			} else {
				fragType = FragMiddle
			}

			w.writeFragment(fragType, data[dataOffset:dataOffset+spaceForData])
			dataOffset += spaceForData
			first = false

			if err := w.flushCurrentBlock(); err != nil {
				return err
			}

			if w.BlockIndex >= w.BlockCount {
				if err := w.openNewSegment(); err != nil {
					return err
				}
			}
		}
	}

	// Odmah upisujemo trenutno stanje na disk preko BM
	return w.BM.WriteBlock(w.CurrentPath, w.BlockIndex, w.CurrentBlock)
}

func (w *WAL) ReadAll(directory string) ([]model.Record, error) {
	var records []model.Record

	files, err := os.ReadDir(directory)
	if err != nil {
		return nil, fmt.Errorf("greska pri citanju WAL foldera: %v", err)
	}

	for _, fileInfo := range files {
		if fileInfo.IsDir() || !strings.HasSuffix(fileInfo.Name(), ".log") {
			continue
		}

		filePath := filepath.Join(directory, fileInfo.Name())
		fileRecords, err := w.readSegment(filePath)
		if err != nil {
			return nil, fmt.Errorf("greska pri citanju WAL segmenta %s: %v", fileInfo.Name(), err)
		}
		records = append(records, fileRecords...)
	}

	return records, nil
}

func (w *WAL) readSegment(filePath string) ([]model.Record, error) {
	var records []model.Record
	var assembleBuffer []byte

	blockIdx := 0
	for {
		blockData, err := w.BM.ReadBlock(filePath, blockIdx)
		if err != nil || len(blockData) == 0 {
			break
		}
		blockIdx++

		position := 0
		for position+FragHeader <= len(blockData) {
			fragType := blockData[position]
			savedCRC := binary.LittleEndian.Uint32(blockData[position+1 : position+5])
			dataLen := int(binary.LittleEndian.Uint16(blockData[position+5 : position+7]))

			// Ako je padding (nule) do kraja bloka
			if fragType == 0 && savedCRC == 0 && dataLen == 0 {
				break
			}

			position += FragHeader

			if position+dataLen > len(blockData) {
				break
			}

			fragData := make([]byte, dataLen)
			copy(fragData, blockData[position:position+dataLen])
			position += dataLen

			calculatedCRC := crc32.ChecksumIEEE(fragData)
			if savedCRC != calculatedCRC {
				return nil, fmt.Errorf("CRC greska u segmentu %s, blok %d", filePath, blockIdx-1)
			}

			switch fragType {
			case FragFull:
				rec := deserializeRecord(fragData)
				records = append(records, rec)
			case FragFirst:
				assembleBuffer = make([]byte, 0, len(fragData))
				assembleBuffer = append(assembleBuffer, fragData...)
			case FragMiddle:
				assembleBuffer = append(assembleBuffer, fragData...)
			case FragLast:
				assembleBuffer = append(assembleBuffer, fragData...)
				rec := deserializeRecord(assembleBuffer)
				records = append(records, rec)
				assembleBuffer = nil
			}
		}
	}

	return records, nil
}
