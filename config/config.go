package config

import (
	"encoding/json"
	"os"
)

// citanje i preslikavanje JSON konfiguracije u Go strukturu
type Config struct {
	MemtableCapacity int    `json:"memtable_capacity"`
	MemtableType     string `json:"memtable_type"`
	WalSegmentSize   int64  `json:"wal_segment_size"`
	SummarySparsity  int    `json:"summary_sparsity"`
	CacheSize        int    `json:"cache_size"`
	BlockSize        int    `json:"block_size"`
}

// LoadConfig cita JSON fajl i vraca Config objekat
func LoadConfig(filename string) (*Config, error) {
	config := Config{
		MemtableCapacity: 5,
		MemtableType:     "btree",
		WalSegmentSize:   1048576,
		SummarySparsity:  3,
		CacheSize:        10,
		BlockSize:        4096,
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
