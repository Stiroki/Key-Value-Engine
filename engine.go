package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/Stiroki/Key-Value-Engine/block"
	"github.com/Stiroki/Key-Value-Engine/cache"
	"github.com/Stiroki/Key-Value-Engine/config"
	"github.com/Stiroki/Key-Value-Engine/memtable"
	"github.com/Stiroki/Key-Value-Engine/model"
	"github.com/Stiroki/Key-Value-Engine/sstable"
	"github.com/Stiroki/Key-Value-Engine/wal"
)

// KVEngine je centralna struktura koja povezuje sve komponente sistema
type KVEngine struct {
	Memtable     *memtable.Memtable
	Wal          *wal.WAL
	Cache        *cache.LRUCache
	Config       *config.Config
	BlockManager *block.BlockManager
	DataDir      string
	SSTableCount int
}

// NewKVEngine inicijalizuje bazu, ucitava konfiguraciju i radi oporavak iz WAL-a
func NewKVEngine(dataDir string, configPath string) (*KVEngine, error) {
	err := os.MkdirAll(dataDir, 0755)
	if err != nil {
		return nil, fmt.Errorf("greska pri kreiranju data foldera: %v", err)
	}

	// Trazimo trenutni broj SSTabela kako bismo znali indeks sledece
	files, _ := os.ReadDir(dataDir)
	maxTableNum := 0
	for _, f := range files {
		if strings.HasPrefix(f.Name(), "usertable-") && strings.HasSuffix(f.Name(), "-Data.db") {
			parts := strings.Split(f.Name(), "-")
			if len(parts) >= 2 {
				num, _ := strconv.Atoi(parts[1])
				if num > maxTableNum {
					maxTableNum = num
				}
			}
		}
	}

	// Ucitavanje konfiguracije
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		return nil, fmt.Errorf("greska pri ucitavanju konfiguracije: %v", err)
	}

	// Inicijalizacija WAL-a
	w, err := wal.NewWAL(dataDir, cfg.WalSegmentSize)
	if err != nil {
		return nil, fmt.Errorf("greska pri inicijalizaciji WAL-a: %v", err)
	}

	// Inicijalizacija Memtable-a
	mt := memtable.NewMemtable(cfg.MemtableCapacity, cfg.MemtableType, w)

	// Recovery iz WAL-a
	recoveredRecords, err := w.ReadAll(dataDir)
	if err == nil {
		for i := range recoveredRecords {
			mt.Put(&recoveredRecords[i])
		}
	} else {
		fmt.Printf("[UPOZORENJE] Greška pri citanju WAL-a: %v\n", err)
	}

	// Inicijalizacija LRU Cache-a
	c := cache.NewLRUCache(cfg.CacheSize)

	bm := block.NewBlockManager(cfg.BlockSize, c)

	// KREIRANJE ENGINE OBJEKTA
	engine := &KVEngine{
		Memtable:     mt,
		Wal:          w,
		Cache:        c,
		Config:       cfg,
		BlockManager: bm,
		DataDir:      dataDir,
		SSTableCount: maxTableNum,
	}

	// Povezujemo flush callback Memtable-a sa funkcijom za kreiranje SSTable-a u Engine-u
	engine.Memtable.FlushCallback = engine.BuildSSTable

	return engine, nil
}

// Put kreira zapis i prosledjuje ga Memtable-u
func (e *KVEngine) Put(key string, value []byte) error {
	record := &model.Record{
		Key:       key,
		Value:     value,
		Tombstone: false,
		Timestamp: time.Now(),
	}

	return e.Memtable.Put(record)
}

// Get pronalazi podatak prateci Read Path (Memtable -> Cache -> SSTable)
func (e *KVEngine) Get(key string) ([]byte, bool) {
	// Pretraga u Memtable-u
	record, found := e.Memtable.Get(key)
	if found {
		if record.Tombstone {
			return nil, false
		}
		return record.Value, true
	}

	// Pretraga u cache-u
	if cachedVal, cacheFound := e.Cache.Get(key); cacheFound {
		return cachedVal, true
	}

	// Pretraga u SSTabelama (od najnovije ka najstarijoj)
	for i := e.SSTableCount; i > 0; i-- {
		tableName := fmt.Sprintf("%s/usertable-%d", e.DataDir, i)
		reader, err := sstable.NewSSTableReader(tableName, e.BlockManager)
		if err != nil {
			continue
		}

		rec, err := reader.Find(key)
		if err == nil && rec != nil {
			if rec.Tombstone {
				return nil, false
			}
			// Stavljamo na vrh cache-a
			e.Cache.Put(key, rec.Value)
			return rec.Value, true
		}
	}

	return nil, false
}

// Delete logicko brisanje
func (e *KVEngine) Delete(key string) error {
	record := &model.Record{
		Key:       key,
		Value:     nil,
		Tombstone: true,
		Timestamp: time.Now(),
	}

	// Uklanjanje iz cache-a
	e.Cache.Remove(key)

	return e.Memtable.Put(record)
}

func (e *KVEngine) BuildSSTable(records []*model.Record) error {
	if len(records) == 0 {
		return nil
	}

	e.SSTableCount++
	basePath := fmt.Sprintf("%s/usertable-%d", e.DataDir, e.SSTableCount)

	fmt.Printf("[ENGINE] Započinjem kreiranje SSTabele: %s\n", basePath)

	builder, err := sstable.NewSSTableBuilder(
		basePath,
		e.Config.SummarySparsity,
		len(records),
		0.01,
		e.BlockManager,
	)
	if err != nil {
		return err
	}

	var valuesForMerkle [][]byte

	for _, rec := range records {
		sstableRec := &sstable.Record{
			Key:       rec.Key,
			Value:     rec.Value,
			Tombstone: rec.Tombstone,
			Timestamp: rec.Timestamp.UnixNano(),
		}

		if !rec.Tombstone {
			valuesForMerkle = append(valuesForMerkle, rec.Value)
		}

		if err := builder.WriteRecord(sstableRec); err != nil {
			return err
		}
	}

	if err := builder.Finish(); err != nil {
		return err
	}

	mTree := sstable.NewMerkleTree(valuesForMerkle)

	// Ako stablo nije prazno, snimamo Root Hash u poseban fajl
	if mTree.Root != nil {
		metadataPath := basePath + "-Metadata.txt"
		err = os.WriteFile(metadataPath, mTree.Root.Hash, 0644)
		if err != nil {
			fmt.Printf("[UPOZORENJE] Greška pri snimanju Merkle stabla: %v\n", err)
		}
	}

	fmt.Println("[ENGINE] Kreiranje SSTabele i Merkle stabla uspesno zavrseno!")
	return nil
}

// ValidateSSTable proverava integritet SSTabele koristeci Merkle stablo
func (e *KVEngine) ValidateSSTable(tableNum int) {
	basePath := fmt.Sprintf("%s/usertable-%d", e.DataDir, tableNum)
	metadataPath := basePath + "-Metadata.txt"

	savedHash, err := os.ReadFile(metadataPath)
	if err != nil {
		fmt.Printf("[GREŠKA] Ne mogu da pronađem Metadata fajl za tabelu %d. (Da li tabela uopšte postoji?)\n", tableNum)
		return
	}

	fmt.Println("[SISTEM] Započinjem čitanje fajla i rekonstrukciju Merkle stabla...")

	// Otvaramo fajl preko Block Managera
	file := sstable.NewBMReader(basePath+"-Data.db", e.BlockManager)
	var valuesForMerkle [][]byte

	// Citamo sve podatke sekvencijalno
	for {
		var keyLen, valLen uint64

		err := binary.Read(file, binary.LittleEndian, &keyLen)
		if err == io.EOF {
			break
		}
		if err != nil {
			fmt.Printf("[GREŠKA] Problem pri čitanju Data fajla: %v\n", err)
			return
		}

		binary.Read(file, binary.LittleEndian, &valLen)

		keyBytes := make([]byte, keyLen)
		io.ReadFull(file, keyBytes)

		valBytes := make([]byte, valLen)
		io.ReadFull(file, valBytes)

		var tombstone bool
		binary.Read(file, binary.LittleEndian, &tombstone)

		var timestamp int64
		binary.Read(file, binary.LittleEndian, &timestamp)

		if !tombstone {
			valuesForMerkle = append(valuesForMerkle, valBytes)
		}
	}

	// Pravimo novo Merkle stablo sa procitanim vrednostima
	newTree := sstable.NewMerkleTree(valuesForMerkle)
	var newHash []byte
	if newTree.Root != nil {
		newHash = newTree.Root.Hash
	}

	// Poredjenje hash-eva
	if bytes.Equal(savedHash, newHash) {
		fmt.Printf("\n>>> [USPEH] SSTabela %d je NETAKNUTA! Merkle Root se savršeno poklapa. <<<\n\n", tableNum)
	} else {
		fmt.Printf("\n>>> [KORUPCIJA] UPOZORENJE! Podaci u SSTabeli %d su izmenjeni ili oštećeni! <<<\n\n", tableNum)
	}
}
