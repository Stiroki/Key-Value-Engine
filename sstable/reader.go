package sstable

import (
	"bytes"
	"encoding/binary"
	"io"
	"os"
)

// LoadBloomFilter učitava filter sa diska
func LoadBloomFilter(path string) (*BloomFilter, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var m, k uint64
	// Čitamo M i K
	if err := binary.Read(file, binary.LittleEndian, &m); err != nil {
		return nil, err
	}
	if err := binary.Read(file, binary.LittleEndian, &k); err != nil {
		return nil, err
	}

	// Čitamo niz bitova (BitSet)
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
}

func NewSSTableReader(basePath string) (*SSTableReader, error) {
	bloom, err := LoadBloomFilter(basePath + "-Filter.db")
	if err != nil {
		return nil, err
	}
	return &SSTableReader{
		BasePath: basePath,
		Bloom:    bloom,
	}, nil
}

// Find traži zapis po ključu u ovoj SSTabeli
func (r *SSTableReader) Find(searchKey string) (*Record, error) {
	keyBytes := []byte(searchKey)

	// 1. KORAK: Provera Bloom Filtera
	if !r.Bloom.Has(keyBytes) {
		return nil, nil // Sigurno nije ovde! Vraćamo nil bez greške.
	}

	// Upozorenje: Bloom Filter može dati "False Positive" (kaže da postoji, a zapravo ga nema)
	// Zato moramo da prođemo kroz ostale fajlove da potvrdimo.

	// 2. KORAK: Čitanje Summary fajla
	// Tražimo offset u Index fajlu od kog treba da počnemo pretragu.
	indexOffset, err := r.searchSummary(keyBytes)
	if err != nil {
		return nil, err
	}
	if indexOffset == -1 {
		return nil, nil // Nije nađen
	}

	// 3. KORAK: Čitanje Index fajla
	// Tražimo tačan offset u Data fajlu.
	dataOffset, err := r.searchIndex(indexOffset, keyBytes)
	if err != nil {
		return nil, err
	}
	if dataOffset == -1 {
		return nil, nil // Nije nađen
	}

	// 4. KORAK: Čitanje Data fajla
	// (U pravoj integraciji ovde bi koristila onaj tvoj BlockManager, ali za sada
	// čitamo direktno iz fajla da potvrdimo logiku)
	return r.readDataRecord(dataOffset)
}

// searchSummary traži početni offset u Index fajlu
func (r *SSTableReader) searchSummary(searchKey []byte) (int64, error) {
	file, err := os.Open(r.BasePath + "-Summary.db")
	if err != nil {
		return -1, err
	}
	defer file.Close()

	var lastValidIndexOffset int64 = 0 // Počinjemo od nule po defaultu

	for {
		var keyLen uint64
		// Čitamo dužinu ključa
		err := binary.Read(file, binary.LittleEndian, &keyLen)
		if err == io.EOF {
			break // Stigli smo do kraja fajla
		}
		if err != nil {
			return -1, err
		}

		// Čitamo ključ
		key := make([]byte, keyLen)
		if _, err := io.ReadFull(file, key); err != nil {
			return -1, err
		}

		// Čitamo offset u Indexu
		var indexOffset int64
		if err := binary.Read(file, binary.LittleEndian, &indexOffset); err != nil {
			return -1, err
		}

		// Poredimo ključeve leksikografski (abecedno)
		cmp := bytes.Compare(key, searchKey)
		if cmp == 0 {
			// Našli smo tačan ključ u Summary-u! Vraćamo njegov offset.
			return indexOffset, nil
		} else if cmp > 0 {
			// Pročitali smo ključ koji je veći (abecedno posle) od našeg.
			// To znači da se naš ključ sigurno nalazi u prethodnom bloku!
			break
		}

		// Ako je ključ manji, pamtimo njegov offset i nastavljamo dalje
		lastValidIndexOffset = indexOffset
	}

	return lastValidIndexOffset, nil
}

// searchIndex traži tačan offset u Data fajlu
func (r *SSTableReader) searchIndex(startIndexOffset int64, searchKey []byte) (int64, error) {
	file, err := os.Open(r.BasePath + "-Index.db")
	if err != nil {
		return -1, err
	}
	defer file.Close()

	// Preskačemo sve do offseta koji nam je dao Summary
	_, err = file.Seek(startIndexOffset, io.SeekStart)
	if err != nil {
		return -1, err
	}

	for {
		var keyLen uint64
		err := binary.Read(file, binary.LittleEndian, &keyLen)
		if err == io.EOF {
			break // Kraj fajla, ključ nije pronađen
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
			// BINGO! Našli smo tačan ključ u Indeksu.
			return dataOffset, nil
		} else if cmp > 0 {
			// Pošto su ključevi sortirani, ako naiđemo na "veći" ključ,
			// znači da smo preskočili mesto gde je naš trebao da bude. Nema ga.
			return -1, nil
		}
	}

	return -1, nil // Nije nađen
}

// readDataRecord čita kompletan zapis sa zadatog offseta u Data fajlu
func (r *SSTableReader) readDataRecord(dataOffset int64) (*Record, error) {
	file, err := os.Open(r.BasePath + "-Data.db")
	if err != nil {
		return nil, err
	}
	defer file.Close()

	// Preskačemo tačno na mesto gde podatak počinje
	_, err = file.Seek(dataOffset, io.SeekStart)
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

	// Kreiramo i vraćamo pronađeni Record!
	return &Record{
		Key:       string(keyBytes),
		Value:     valBytes,
		Tombstone: tombstone,
		Timestamp: timestamp,
	}, nil
}
