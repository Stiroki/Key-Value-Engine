package wal

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/Stiroki/Key-Value-Engine/model"
	// Prilagodi putanju tvom go.mod fajlu
)

// WAL predstavlja glavni objekat za upravljanje log fajlovima
type WAL struct {
	Directory   string   // Folder gde se čuvaju log fajlovi
	SegmentSize int64    // Maksimalna veličina jednog segmenta
	CurrentFile *os.File // Referenca na trenutno otvoreni fajl u koji pišemo
	CurrentSize int64    // Trenutna veličina otvorenog fajla
}

// NewWAL inicijalizuje WAL, kreira folder ako ne postoji i priprema ga za rad
func NewWAL(directory string, segmentSize int64) (*WAL, error) {
	// 1. Proveri da li folder postoji, ako ne - kreiraj ga
	err := os.MkdirAll(directory, 0755)
	if err != nil {
		return nil, fmt.Errorf("greska pri kreiranju WAL foldera: %v", err)
	}

	// 2. Kreiramo instancu WAL-a
	wal := &WAL{
		Directory:   directory,
		SegmentSize: segmentSize,
	}

	// Za sada nemamo otvoren fajl, to cemo obraditi u sledecem koraku kada budemo pisali
	return wal, nil
}

func (w *WAL) openNewSegment() error {
	// Ako postoji trenutni fajl, zatvaramo ga
	if w.CurrentFile != nil {
		w.CurrentFile.Close()
	}

	// Kreiramo jedinstveno ime za novi fajl (npr. wal_1699999999.log)
	// Koristimo trenutno vreme u milisekundama da bismo imali jedinstvena imena
	fileName := fmt.Sprintf("wal_%d.log", time.Now().UnixMilli())
	filePath := filepath.Join(w.Directory, fileName)

	// Otvaramo fajl za dodavanje (O_APPEND), kreiramo ako ne postoji (O_CREATE)
	file, err := os.OpenFile(filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}

	w.CurrentFile = file
	w.CurrentSize = 0
	return nil
}

// Put serijalizuje i upisuje Record u WAL
func (w *WAL) Put(record *model.Record) error {
	keyBytes := []byte(record.Key)
	keySize := uint64(len(keyBytes))
	valSize := uint64(len(record.Value))

	// Racunamo ukupnu velicinu naseg zapisa:
	// 8 (timestamp) + 1 (tombstone) + 8 (keySize) + 8 (valSize) + duzina kljuca + duzina vrednosti
	totalSize := int64(8 + 1 + 8 + 8 + keySize + valSize)

	// Proveravamo da li treba da otvorimo novi segment
	if w.CurrentFile == nil || w.CurrentSize+totalSize > w.SegmentSize {
		err := w.openNewSegment()
		if err != nil {
			return fmt.Errorf("greska pri otvaranju novog WAL segmenta: %v", err)
		}
	}

	// Alociramo memorijski bafer u koji pakujemo sve podatke
	buf := make([]byte, totalSize)
	offset := 0

	// 1. Timestamp (int64)
	binary.LittleEndian.PutUint64(buf[offset:], uint64(record.Timestamp.UnixNano()))
	offset += 8

	// 2. Tombstone (bool) -> 1 ako je true, 0 ako je false
	if record.Tombstone {
		buf[offset] = 1
	} else {
		buf[offset] = 0
	}
	offset += 1

	// velicina kljuca (uint64)
	binary.LittleEndian.PutUint64(buf[offset:], keySize)
	offset += 8

	// velicina vrednosti (uint64)
	binary.LittleEndian.PutUint64(buf[offset:], valSize)
	offset += 8

	//	kljuc
	copy(buf[offset:], keyBytes)
	offset += int(keySize)

	// vrednost
	copy(buf[offset:], record.Value)

	// Na kraju upisujemo ceo zapakovan bafer u fajl jednim potezom
	_, err := w.CurrentFile.Write(buf)
	if err != nil {
		return fmt.Errorf("greska pri upisu u WAL fajl: %v", err)
	}

	// Azuriramo trenutnu velicinu fajla
	w.CurrentSize += totalSize

	return nil
}

// ReadAll prolazi kroz sve WAL fajlove u zadatom folderu i vraca sve zapise.
// Ovo se poziva samo prilikom pokretanja sistema.
func ReadAll(directory string) ([]model.Record, error) {
	var records []model.Record

	// 1. Citamo sve fajlove iz foldera
	files, err := os.ReadDir(directory)
	if err != nil {
		return nil, fmt.Errorf("greska pri citanju WAL foldera: %v", err)
	}

	// 2. Prolazimo kroz svaki fajl
	for _, fileInfo := range files {
		if fileInfo.IsDir() {
			continue // Preskacemo foldere ako ih ima
		}

		filePath := filepath.Join(directory, fileInfo.Name())
		file, err := os.Open(filePath)
		if err != nil {
			return nil, err
		}

		// 3. Citamo zapise iz fajla sve dok ne dodjemo do kraja (EOF)
		for {
			// Naše "zaglavlje" (header) ima tacno 25 bajtova:
			// 8 (timestamp) + 1 (tombstone) + 8 (keySize) + 8 (valSize) = 25
			header := make([]byte, 25)
			_, err := io.ReadFull(file, header)
			if err == io.EOF {
				break // Stigli smo do kraja fajla, idemo na sledeci
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
