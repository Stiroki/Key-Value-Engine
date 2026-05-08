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
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var config Config

	// Dekodiramo JSON
	decoder := json.NewDecoder(file)
	err = decoder.Decode(&config)
	if err != nil {
		return nil, err
	}

	return &config, nil
}
