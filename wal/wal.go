package wal

import (
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

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
	Directory   string
	BlockSize   int
	BlockCount  int // broj blokova po segmentu
	CurrentFile *os.File
	BlockIndex  int // trenutni blok unutar segmenta
	BlockOffset int // pozicija unutar trenutnog bloka
}

// NewWAL inicijalizuje WAL, kreira folder ako ne postoji i priprema ga za rad
func NewWAL(directory string, blockSize int, blockCount int) (*WAL, error) {
	err := os.MkdirAll(directory, 0755)
	if err != nil {
		return nil, fmt.Errorf("greska pri kreiranju WAL foldera: %v", err)
	}

	wal := &WAL{
		Directory:  directory,
		BlockSize:  blockSize,
		BlockCount: blockCount,
	}

	return wal, nil
}

func (w *WAL) openNewSegment() error {
	if w.CurrentFile != nil {
		w.padCurrentBlock()
		w.CurrentFile.Close()
	}

	fileName := fmt.Sprintf("wal_%d.log", time.Now().UnixMilli())
	filePath := filepath.Join(w.Directory, fileName)

	file, err := os.OpenFile(filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}

	w.CurrentFile = file
	w.BlockIndex = 0
	w.BlockOffset = 0
	return nil
}

func (w *WAL) padCurrentBlock() {
	remaining := w.BlockSize - w.BlockOffset
	if remaining > 0 && remaining < w.BlockSize {
		padding := make([]byte, remaining)
		w.CurrentFile.Write(padding)
	}
	w.BlockIndex++
	w.BlockOffset = 0
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

func (w *WAL) writeFragment(fragType byte, data []byte) error {
	header := make([]byte, FragHeader)
	header[0] = fragType
	crc := crc32.ChecksumIEEE(data)
	binary.LittleEndian.PutUint32(header[1:5], crc)
	binary.LittleEndian.PutUint16(header[5:7], uint16(len(data)))

	_, err := w.CurrentFile.Write(header)
	if err != nil {
		return fmt.Errorf("Greska pri upisu zaglavlja fragmenta: %v", err)
	}

	_, err = w.CurrentFile.Write(data)
	if err != nil {
		return fmt.Errorf("Greska pri upisu podataka fragmenta: %v", err)
	}

	w.BlockOffset += FragHeader + len(data)
	return nil
}

func (w *WAL) Put(record *model.Record) error {
	data := serializeRecord(record)

	// Proveravamo da li treba da otvorimo novi segment
	if w.CurrentFile == nil {
		err := w.openNewSegment()
		if err != nil {
			return fmt.Errorf("greska pri otvaranju novog WAL segmenta: %v", err)
		}
	}

	dataOffset := 0 // Koliko smo vec upisali
	first := true   // Da li je ovo prvi fragment zapisa

	for dataOffset < len(data) {
		remaining := w.BlockSize - w.BlockOffset

		// Ako nema mesta, padding pa novi blok
		if remaining < FragHeader {
			w.padCurrentBlock()

			if w.BlockIndex >= w.BlockCount {
				err := w.openNewSegment()
				if err != nil {
					return fmt.Errorf("Greska pri otvaranju novog segmenta: %v", err)
				}
			}
			remaining = w.BlockSize - w.BlockOffset
		}
		// Koliko podataka moze da stane u ovaj blok bez zaglavlja
		spaceForData := remaining - FragHeader
		leftToWrite := len(data) - dataOffset
		// Ako sve staje u blok
		if leftToWrite <= spaceForData {
			var fragType byte
			if first {
				fragType = FragFull // Ceo zapis u jednom fragmentu
			} else {
				fragType = FragLast // Poslednji deo zapisa
			}

			err := w.writeFragment(fragType, data[dataOffset:dataOffset+leftToWrite])
			if err != nil {
				return fmt.Errorf("Greska pri upisu FULL/LAST fragmenta: %v", err)
			}

			dataOffset += leftToWrite
			// Ukoliko ne staje sve u jedan blok
		} else {
			var fragType byte
			if first {
				fragType = FragFirst // Prvi deo zapisa
			} else {
				fragType = FragMiddle // Srednji deo zapisa
			}

			err := w.writeFragment(fragType, data[dataOffset:dataOffset+spaceForData])
			if err != nil {
				return fmt.Errorf("Greska pri upisu FIRST/MIDDLE fragmenta: %v", err)
			}
			dataOffset += spaceForData
			first = false

			w.padCurrentBlock()

			if w.BlockIndex >= w.BlockCount {
				err := w.openNewSegment()
				if err != nil {
					return fmt.Errorf("Greska pri otvaranju novog segmenta: %v", err)
				}
			}
		}
	}

	return nil
}

func (w *WAL) ReadAll(directory string) ([]model.Record, error) {
	var records []model.Record

	files, err := os.ReadDir(directory)
	if err != nil {
		return nil, fmt.Errorf("greska pri citanju WAL foldera: %v", err)
	}

	for _, fileInfo := range files {
		if fileInfo.IsDir() {
			continue
		}

		if !strings.HasSuffix(fileInfo.Name(), ".log") {
			continue
		}

		filePath := filepath.Join(directory, fileInfo.Name())
		fileRecords, err := w.readSegment(filePath)
		if err != nil {
			return nil, fmt.Errorf("Greska pri citanju WAL segmenta %s: %v", fileInfo.Name(), err)
		}
		records = append(records, fileRecords...)
	}

	return records, nil
}

func (w *WAL) readSegment(filePath string) ([]model.Record, error) {
	var records []model.Record

	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("Greska pri otvaranju WAL fajla: %v", err)
	}
	defer file.Close()

	fileInfo, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("Greska pri citanju informaciju o WAL fajlu: %v", err)
	}
	fileSize := fileInfo.Size()

	var assembleBuffer []byte

	for fileOffset := int64(0); fileOffset < fileSize; {
		blockData := make([]byte, w.BlockSize)
		n, err := file.ReadAt(blockData, fileOffset)
		if err != nil && err != io.EOF {
			return nil, fmt.Errorf("Greska pri citanju bloka: %v", err)
		}
		blockData = blockData[:n] // Samo koliko je stvarno procitano
		fileOffset += int64(w.BlockSize)
		// Parsiranje fragmenata unutar bloka
		position := 0 // Pozicija unutar jednog bloka
		for position+FragHeader <= len(blockData) {
			fragType := blockData[position]
			savedCRC := binary.LittleEndian.Uint32(blockData[position+1 : position+5])
			dataLen := int(binary.LittleEndian.Uint16(blockData[position+5 : position+7]))
			if fragType == 0 && savedCRC == 0 && dataLen == 0 {
				break // Padding do kraja bloka, pa se prelazi na sledeci
			}

			position += FragHeader

			if position+dataLen > len(blockData) {
				break // Neispravan fragment
			}

			fragData := make([]byte, dataLen)
			copy(fragData, blockData[position:position+dataLen])
			position += dataLen
			// Provera CRC
			calculatedCRC := crc32.ChecksumIEEE(fragData)
			if savedCRC != calculatedCRC {
				return nil, fmt.Errorf("CRC se ne podudara u fajlu %s", filePath)
			}
			// Sastavljanje zapisa na osnovu fragmenta
			switch fragType {
			case FragFull:
				record := deserializeRecord(fragData)
				records = append(records, record)

			case FragFirst:
				assembleBuffer = make([]byte, 0)
				assembleBuffer = append(assembleBuffer, fragData...)

			case FragMiddle:
				assembleBuffer = append(assembleBuffer, fragData...)

			case FragLast:
				assembleBuffer = append(assembleBuffer, fragData...)
				record := deserializeRecord(assembleBuffer)
				records = append(records, record)
				assembleBuffer = nil
			}
		}
	}
	return records, nil
}
