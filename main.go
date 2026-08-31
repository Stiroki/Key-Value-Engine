package main

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

func main() {
	fmt.Println("==================================================")
	fmt.Println("        POKRETANJE KEY-VALUE ENGINE-a...          ")
	fmt.Println("==================================================")

	engine, err := NewKVEngine("data", "config/config.json")
	if err != nil {
		fmt.Printf("[KRITIČNA GREŠKA] Sistem ne može da se pokrene: %v\n", err)
		return
	}
	fmt.Println("[USPEH] Sistem uspešno inicijalizovan i spreman za rad!")

	scanner := bufio.NewScanner(os.Stdin)

	for {
		fmt.Println("\n---------------- GLAVNI MENI ----------------")
		fmt.Println("1. Dodaj novi podatak (PUT)")
		fmt.Println("2. Pronađi podatak (GET)")
		fmt.Println("3. Obriši podatak (DELETE)")
		fmt.Println("4. Validacija SSTabele (Merkle Tree)")
		fmt.Println("5. Izlaz")
		fmt.Print("\nIzaberite opciju (1-5): ")

		scanner.Scan()
		opcija := strings.TrimSpace(scanner.Text())

		switch opcija {
		case "1":
			fmt.Print("Unesite ključ (Key): ")
			scanner.Scan()
			kljuc := strings.TrimSpace(scanner.Text())

			fmt.Print("Unesite vrednost (Value): ")
			scanner.Scan()
			vrednost := []byte(strings.TrimSpace(scanner.Text()))

			err := engine.Put(kljuc, vrednost)
			if err != nil {
				fmt.Printf("[GREŠKA] Upis nije uspeo: %v\n", err)
			} else {
				fmt.Println("[USPEH] Podatak uspesno dodat!")
			}

		case "2":
			fmt.Print("Unesite ključ za pretragu: ")
			scanner.Scan()
			kljuc := strings.TrimSpace(scanner.Text())

			vrednost, pronadjen, greska := engine.Get(kljuc)
			if greska != nil {
				fmt.Printf("[GRESKA]  %v\n", greska)
			} else if pronadjen {
				fmt.Printf("[REZULTAT] Pronađeno: %s -> %s\n", kljuc, string(vrednost))
			} else {
				fmt.Println("[REZULTAT] Podatak ne postoji u bazi ili je obrisan.")
			}

		case "3":
			fmt.Print("Unesite ključ koji želite da obrišete: ")
			scanner.Scan()
			kljuc := strings.TrimSpace(scanner.Text())

			err := engine.Delete(kljuc)
			if err != nil {
				fmt.Printf("[GREŠKA] Brisanje nije uspelo: %v\n", err)
			} else {
				fmt.Println("[USPEH] Podatak uspesno (logicki) obrisan!")
			}

		case "4":
			fmt.Print("Unesite redni broj SSTabele za validaciju (npr. 1): ")
			scanner.Scan()
			unos := strings.TrimSpace(scanner.Text())
			brojTabele, err := strconv.Atoi(unos)
			if err != nil {
				fmt.Println("[GREŠKA] Morate uneti validan broj!")
				continue
			}

			engine.ValidateSSTable(brojTabele)

		case "5":
			engine.SaveTokenBucketState()
			fmt.Println("Gašenje sistema...")
			return

		default:
			fmt.Println("[GREŠKA] Nepostojeća opcija. Molimo pokušajte ponovo.")
		}
	}
}
