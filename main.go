package main

import (
	"fmt"
	"log"

	"github.com/Stiroki/Key-Value-Engine/wal"
)

func main() {
	fmt.Println("Pokusavam da procitam prethodno sacuvane WAL fajlove...")

	// Pozivamo funkciju za citanje svih zapisa
	records, err := wal.ReadAll("data/wal")
	if err != nil {
		log.Fatalf("Greska pri citanju WAL-a: %v", err)
	}

	if len(records) == 0 {
		fmt.Println("Nije pronadjen nijedan zapis. (Da nisi obrisala folder?)")
		return
	}

	fmt.Printf("Uspesno procitano %d zapisa!\n\n", len(records))

	// Ispisujemo svaki zapis da se uverimo da podaci nisu osteceni
	for i, rec := range records {
		fmt.Printf("Zapis %d: Kljuc='%s', Vrednost='%s', Obrisano=%t\n",
			i+1, rec.Key, string(rec.Value), rec.Tombstone)
	}
}
