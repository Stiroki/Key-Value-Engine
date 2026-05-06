package main

import (
	"fmt"
	"log"
	"time"

	"github.com/Stiroki/Key-Value-Engine/config"
	"github.com/Stiroki/Key-Value-Engine/memtable"
	"github.com/Stiroki/Key-Value-Engine/model"
	"github.com/Stiroki/Key-Value-Engine/wal"
)

func main() {
	// 1. Učitavanje konfiguracije
	cfg, err := config.LoadConfig("config/config.json")
	if err != nil {
		log.Fatalf("Greska pri ucitavanju konfiguracije: %v", err)
	}

	// 2. Inicijalizacija WAL-a
	myWal, err := wal.NewWAL("data/wal", cfg.WalSegmentSize)
	if err != nil {
		log.Fatalf("Greska pri kreiranju WAL-a: %v", err)
	}

	// 3. Inicijalizacija Memtable-a
	// Prosleđujemo kapacitet iz configa, tip strukture i naš WAL
	mt := memtable.NewMemtable(cfg.MemtableCapacity, cfg.MemtableType, myWal)

	fmt.Printf("Kapacitet memtable-a je %d. Krećemo sa upisom...\n", cfg.MemtableCapacity)
	fmt.Println("--------------------------------------------------")

	// 4. Namerno ubacujemo VIŠE zapisa nego što može da stane (npr. 12 zapisa)
	// Ako je u config.json capacity 10, na 10. zapisu mora da se desi Flush!
	for i := 1; i <= 12; i++ {
		key := fmt.Sprintf("kljuc_%d", i)
		val := []byte(fmt.Sprintf("Vrednost za kljuc %d", i))

		record := &model.Record{
			Key:       key,
			Value:     val,
			Tombstone: false,
			Timestamp: time.Now(),
		}

		err := mt.Put(record)
		if err != nil {
			log.Fatalf("Greska pri upisu: %v", err)
		}
		fmt.Printf("Upisan '%s'. Trenutna veličina Memtable-a: %d\n", key, mt.Data.Size())

		// Mala pauza da bi fajlovi dobili jedinstvena imena po vremenu
		time.Sleep(5 * time.Millisecond)
	}

	fmt.Println("--------------------------------------------------")

	// 5. Pokušaj čitanja ključa 5
	fmt.Println("Pokušavam da pročitam 'kljuc_5' iz Memtable-a...")
	rec, found := mt.Get("kljuc_5")
	if found {
		fmt.Printf("Pronadjen: %s -> %s\n", rec.Key, string(rec.Value))
	} else {
		fmt.Println("Kljuc 5 NIJE PRONADJEN u memoriji!")
		fmt.Println("(Ovo je ispravno! Svi podaci od 1 do 10 su prebačeni na disk tokom Flusha, a memorija je ispražnjena. Trenutno su u memoriji samo ključevi 11 i 12.)")
	}
}
