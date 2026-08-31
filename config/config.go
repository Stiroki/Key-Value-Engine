package config

import (
	"encoding/json"
	"os"
)

// citanje i preslikavanje JSON konfiguracije u Go strukturu
type Config struct {
	MemtableCapacity    int    `json:"memtable_capacity"`
	MemtableType        string `json:"memtable_type"`
	WalSegmentSize      int64  `json:"wal_segment_size"`
	SummarySparsity     int    `json:"summary_sparsity"`
	CacheSize           int    `json:"cache_size"`
	BlockSize           int    `json:"block_size"`
	TokenBucketCapacity int    `json:"token_bucket_capacity"`
	TokenBucketRefillMs int64  `json:"token_bucket_refill_ms"`
	MemtableInstances   int    `json:"memtable_instances"`
}

// LoadConfig cita JSON fajl i vraca Config objekat
func LoadConfig(filename string) (*Config, error) {
	config := Config{
		MemtableCapacity:    5,
		MemtableType:        "btree",
		WalSegmentSize:      1048576,
		SummarySparsity:     3,
		CacheSize:           10,
		BlockSize:           4096,
		TokenBucketCapacity: 10,
		TokenBucketRefillMs: 1000,
		MemtableInstances:   3,
	}

	file, err := os.Open(filename)
	if err != nil {
		return &config, nil
	}
	defer file.Close()

	// Dekodiramo JSON
	decoder := json.NewDecoder(file)
	err = decoder.Decode(&config)
	if err != nil {
		return nil, err
	}

	return &config, nil
}
