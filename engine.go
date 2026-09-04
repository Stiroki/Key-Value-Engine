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
	"github.com/Stiroki/Key-Value-Engine/ratelimit"
	"github.com/Stiroki/Key-Value-Engine/sstable"
	"github.com/Stiroki/Key-Value-Engine/wal"
)

const tokenBucketKey = "__internal_token_bucket__"

// KVEngine je centralna struktura koja povezuje sve komponente sistema
type KVEngine struct {
	MemtablePool *memtable.MemtablePool
	Wal          *wal.WAL
	Cache        *cache.LRUCache
	Config       *config.Config
	BlockManager *block.BlockManager
	DataDir      string
	SSTableCount int
	Limiter      *ratelimit.TokenBucket
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

	// Inicijalizacija LRU Cache-a i BlockManager-a
	c := cache.NewLRUCache(cfg.CacheSize)
	bm := block.NewBlockManager(cfg.BlockSize, c)

	// Inicijalizacija WAL-a preko BlockManager-a
	w, err := wal.NewWAL(dataDir, cfg.BlockSize, cfg.WalBlockCount, bm)
	if err != nil {
		return nil, fmt.Errorf("greska pri inicijalizaciji WAL-a: %v", err)
	}

	// Inicijalizacija Memtable-a
	pool := memtable.NewMemtablePool(cfg.MemtableInstances, cfg.MemtableCapacity, cfg.MemtableType, w)

	// Recovery iz WAL-a
	recoveredRecords, err := w.ReadAll(dataDir)
	if err == nil {
		for i := range recoveredRecords {
			pool.PutDirect(&recoveredRecords[i])
		}
	} else {
		fmt.Printf("[UPOZORENJE] Greška pri citanju WAL-a: %v\n", err)
	}

	// KREIRANJE ENGINE OBJEKTA
	engine := &KVEngine{
		MemtablePool: pool,
		Wal:          w,
		Cache:        c,
		Config:       cfg,
		BlockManager: bm,
		DataDir:      dataDir,
		SSTableCount: maxTableNum,
	}

	// Povezujemo flush callback Memtable-a sa funkcijom za kreiranje SSTable-a u Engine-u
	engine.MemtablePool.FlushCallback = engine.BuildSSTable

	refillInterval := time.Duration(cfg.TokenBucketRefillMs) * time.Millisecond
	var limiter *ratelimit.TokenBucket

	if rec, found := pool.Get(tokenBucketKey); found && !rec.Tombstone {
		limiter = ratelimit.Deserialize(rec.Value)
	} else {
		loaded := false
		for i := maxTableNum; i > 0; i-- {
			tableName := fmt.Sprintf("%s/usertable-%d", dataDir, i)
			reader, err := sstable.NewSSTableReader(tableName, bm)
			if err != nil {
				continue
			}
			rec, err := reader.Find(tokenBucketKey)
			if err == nil && rec != nil && !rec.Tombstone {
				limiter = ratelimit.Deserialize(rec.Value)
				loaded = true
				break
			}
		}
		if !loaded {
			limiter = ratelimit.NewTokenBucket(cfg.TokenBucketCapacity, refillInterval)
		}
	}

	engine.Limiter = limiter

	return engine, nil
}

// Put kreira zapis i prosledjuje ga Memtable-u
func (e *KVEngine) Put(key string, value []byte) error {
	if key == tokenBucketKey {
		return fmt.Errorf("Kljuc je rezervisan za internu upotrebu.")
	}

	if !e.Limiter.Allow() {
		return fmt.Errorf("Previse zahteva, pokusajte ponovo kasnije.")
	}

	e.Cache.Remove(key)

	record := &model.Record{
		Key:       key,
		Value:     value,
		Tombstone: false,
		Timestamp: time.Now(),
	}

	return e.MemtablePool.Put(record)
}

// Get pronalazi podatak prateci Read Path (Memtable -> Cache -> SSTable)
func (e *KVEngine) Get(key string) ([]byte, bool, error) {
	if key == tokenBucketKey {
		return nil, false, nil
	}

	if !e.Limiter.Allow() {
		return nil, false, fmt.Errorf("Previse zahteva, pokusajte ponovo kasnije.")
	}

	// Pretraga u Memtable-u
	record, found := e.MemtablePool.Get(key)
	if found {
		if record.Tombstone {
			return nil, false, nil
		}
		return record.Value, true, nil
	}

	// Pretraga u cache-u
	if cachedVal, cacheFound := e.Cache.Get(key); cacheFound {
		return cachedVal, true, nil
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
				return nil, false, nil
			}
			// Stavljamo na vrh cache-a
			e.Cache.Put(key, rec.Value)
			return rec.Value, true, nil
		}
	}

	return nil, false, nil
}

// Delete logicko brisanje
func (e *KVEngine) Delete(key string) error {
	if key == tokenBucketKey {
		return fmt.Errorf("Kljuc je rezervisan za internu upotrebu.")
	}

	if !e.Limiter.Allow() {
		return fmt.Errorf("Previse zahteva, pokusajte ponovo kasnije.")
	}

	record := &model.Record{
		Key:       key,
		Value:     nil,
		Tombstone: true,
		Timestamp: time.Now(),
	}

	// Uklanjanje iz cache-a
	e.Cache.Remove(key)

	return e.MemtablePool.Put(record)
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

	// Ako stablo nije prazno, snimamo serijalizovane metapodatke stabla
	if mTree.Root != nil {
		metadataPath := basePath + "-Metadata.txt"
		err = os.WriteFile(metadataPath, mTree.Serialize(), 0644)
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

	metaFile, err := os.Open(metadataPath)
	if err != nil {
		fmt.Printf("[GREŠKA] Ne mogu da pronađem Metadata fajl za tabelu %d. (Da li tabela uopšte postoji?)\n", tableNum)
		return
	}
	defer func(metaFile *os.File) {
		err := metaFile.Close()
		if err != nil {

		}
	}(metaFile)

	savedRootHash, savedLeaves, err := sstable.DeserializeMerkleMetadata(metaFile)
	if err != nil {
		fmt.Printf("[GREŠKA] Neuspešno čitanje Merkle metapodataka: %v\n", err)
		return
	}

	fmt.Println("[SISTEM] Započinjem čitanje Data fajla i rekonstrukciju Merkle stabla...")

	file := sstable.NewBMReader(basePath+"-Data.db", e.BlockManager)
	var valuesForMerkle [][]byte
	var recordKeys []string

	for {
		var keyLen uint64
		err := binary.Read(file, binary.LittleEndian, &keyLen)
		if err != nil {
			break // Kraj fajla ili EOF
		}

		// Ako smo naišli na padding nule na kraju bloka ili nerealnu veličinu ključa
		if keyLen == 0 || keyLen > 65536 {
			break
		}

		var valLen uint64
		if err := binary.Read(file, binary.LittleEndian, &valLen); err != nil {
			break
		}

		if valLen > 100*1024*1024 { // Zaštita od nerealne veličine vrednosti
			break
		}

		keyBytes := make([]byte, keyLen)
		if _, err := io.ReadFull(file, keyBytes); err != nil {
			break
		}

		var valBytes []byte
		if valLen > 0 {
			valBytes = make([]byte, valLen)
			if _, err := io.ReadFull(file, valBytes); err != nil {
				break
			}
		}

		var tombstone bool
		if err := binary.Read(file, binary.LittleEndian, &tombstone); err != nil {
			break
		}

		var timestamp int64
		if err := binary.Read(file, binary.LittleEndian, &timestamp); err != nil {
			break
		}

		if !tombstone {
			valuesForMerkle = append(valuesForMerkle, valBytes)
			recordKeys = append(recordKeys, string(keyBytes))
		}
	}

	newTree := sstable.NewMerkleTree(valuesForMerkle)
	var newRootHash []byte
	if newTree.Root != nil {
		newRootHash = newTree.Root.Hash
	}

	if bytes.Equal(savedRootHash, newRootHash) {
		fmt.Printf("\n>>> [USPEH] SSTabela %d je NETAKNUTA! Merkle Root se savršeno poklapa. <<<\n\n", tableNum)
	} else {
		fmt.Printf("\n>>> [KORUPCIJA] UPOZORENJE! Podaci u SSTabeli %d su izmenjeni ili oštećeni! <<<\n", tableNum)

		corruptedIndices := newTree.FindCorruptedIndices(savedLeaves)
		fmt.Println("Detektovane izmene na sledećim zapisima:")
		for _, idx := range corruptedIndices {
			if idx < len(recordKeys) {
				fmt.Printf(" - Zapis #%d (Ključ: '%s') je oštećen ili izmenjen.\n", idx+1, recordKeys[idx])
			} else {
				fmt.Printf(" - Zapis #%d (nedostaje ili je višak u odnosu na originalno stablo).\n", idx+1)
			}
		}
		fmt.Println()
	}
}
func (e *KVEngine) SaveTokenBucketState() {
	record := &model.Record{
		Key:       tokenBucketKey,
		Value:     e.Limiter.Serialize(),
		Tombstone: false,
		Timestamp: time.Now(),
	}
	e.MemtablePool.Tables[0].Data.Put(record)
}
