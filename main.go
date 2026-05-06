package main

import (
	"fmt"
	"log"

	"github.com/Stiroki/Key-Value-Engine/config"
	// Prilagodi putanju ako se razlikuje u go.mod
)

func main() {
	// Pokušavamo da učitamo konfiguraciju, test
	cfg, err := config.LoadConfig("config/config.json")
	if err != nil {
		log.Fatalf("Greska pri ucitavanju konfiguracije: %v", err)
	}

	fmt.Println("Konfiguracija uspesno ucitana!")
	fmt.Printf("Kapacitet Memtable-a je: %d\n", cfg.MemtableCapacity)
	fmt.Printf("Tip Memtable-a je: %s\n", cfg.MemtableType)
}
