package config

import (
	"encoding/json"
	"os"
)

// tldr citanje i preslikavanje JSON
type Config struct {
	MemtableCapacity int    `json:"memtable_capacity"`
	MemtableType     string `json:"memtable_type"`
	WalSegmentSize   int64  `json:"wal_segment_size"`
	SummarySparsity  int    `json:"summary_sparsity"`
	CacheSize        int    `json:"cache_size"`
}

// LoadConfig cita JSON fajl i vraca Config objekat
func LoadConfig(filename string) (*Config, error) {
	// Otvaramo fajl
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close() // Zatvaramo fajl na kraju

	// MORA!!!! pravimo prazan config objekat
	var config Config

	// Dekodiramo JSON u našu strukturu
	decoder := json.NewDecoder(file)
	err = decoder.Decode(&config)
	if err != nil {
		return nil, err
	}

	return &config, nil
}
