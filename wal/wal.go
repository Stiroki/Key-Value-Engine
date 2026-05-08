package wal

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/Stiroki/Key-Value-Engine/model"
)

type WAL struct {
	Directory   string
	SegmentSize int64
	CurrentFile *os.File
	CurrentSize int64
}

// NewWAL inicijalizuje WAL, kreira folder ako ne postoji i priprema ga za rad
func NewWAL(directory string, segmentSize int64) (*WAL, error) {
	err := os.MkdirAll(directory, 0755)
	if err != nil {
		return nil, fmt.Errorf("greska pri kreiranju WAL foldera: %v", err)
	}

	wal := &WAL{
		Directory:   directory,
		SegmentSize: segmentSize,
	}

	return wal, nil
}

func (w *WAL) openNewSegment() error {
	if w.CurrentFile != nil {
		w.CurrentFile.Close()
	}

	fileName := fmt.Sprintf("wal_%d.log", time.Now().UnixMilli())
	filePath := filepath.Join(w.Directory, fileName)

	file, err := os.OpenFile(filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}

	w.CurrentFile = file
	w.CurrentSize = 0
	return nil
}

func (w *WAL) Put(record *model.Record) error {
	keyBytes := []byte(record.Key)
	keySize := uint64(len(keyBytes))
	valSize := uint64(len(record.Value))

	// 8 (timestamp) + 1 (tombstone) + 8 (keySize) + 8 (valSize) + duzina kljuca + duzina vrednosti
	totalSize := int64(8 + 1 + 8 + 8 + keySize + valSize)

	// Proveravamo da li treba da otvorimo novi segment
	if w.CurrentFile == nil || w.CurrentSize+totalSize > w.SegmentSize {
		err := w.openNewSegment()
		if err != nil {
			return fmt.Errorf("greska pri otvaranju novog WAL segmenta: %v", err)
		}
	}

	// Alociramo memorijski buffer u koji pakujemo sve podatke
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

	_, err := w.CurrentFile.Write(buf)
	if err != nil {
		return fmt.Errorf("greska pri upisu u WAL fajl: %v", err)
	}

	w.CurrentSize += totalSize

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

		filePath := filepath.Join(directory, fileInfo.Name())
		file, err := os.Open(filePath)
		if err != nil {
			return nil, err
		}

		for {
			// 8 (timestamp) + 1 (tombstone) + 8 (keySize) + 8 (valSize) = 25
			header := make([]byte, 25)
			_, err := io.ReadFull(file, header)
			if err == io.EOF {
				break
			} else if err != nil {
				file.Close()
				return nil, fmt.Errorf("greska pri citanju zaglavlja iz %s: %v", fileInfo.Name(), err)
			}

			// Parsiramo zaglavlje
			timestampNano := binary.LittleEndian.Uint64(header[0:8])
			tombstone := header[8] == 1
			keySize := binary.LittleEndian.Uint64(header[9:17])
			valSize := binary.LittleEndian.Uint64(header[17:25])

			// Citamo kljuc na osnovu procitane velicine
			keyBytes := make([]byte, keySize)
			_, err = io.ReadFull(file, keyBytes)
			if err != nil {
				file.Close()
				return nil, err
			}

			// Citamo vrednost na osnovu procitane velicine
			valBytes := make([]byte, valSize)
			_, err = io.ReadFull(file, valBytes)
			if err != nil {
				file.Close()
				return nil, err
			}

			// Pakujemo sve nazad u Record strukturu
			record := model.Record{
				Key:       string(keyBytes),
				Value:     valBytes,
				Tombstone: tombstone,
				Timestamp: time.Unix(0, int64(timestampNano)),
			}

			records = append(records, record)
		}

		file.Close()
	}

	return records, nil
}
