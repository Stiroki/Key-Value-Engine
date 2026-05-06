package sstable

import (
	"crypto/sha1"
)

// MerkleNode predstavlja jedan čvor u Merkle stablu
type MerkleNode struct {
	Hash  []byte
	Left  *MerkleNode
	Right *MerkleNode
}

// MerkleTree predstavlja celokupno stablo
type MerkleTree struct {
	Root *MerkleNode
}

// hashData je pomoćna funkcija koja računa SHA-1 heš vrednost
func hashData(data []byte) []byte {
	h := sha1.New()
	h.Write(data)
	return h.Sum(nil)
}

// NewMerkleTree kreira novo stablo na osnovu liste svih vrednosti iz Data bloka
func NewMerkleTree(values [][]byte) *MerkleTree {
	if len(values) == 0 {
		return &MerkleTree{}
	}

	var nodes []*MerkleNode

	// 1. Kreiramo listove (leaf nodes) - svaki list je heš jedne vrednosti
	for _, val := range values {
		nodes = append(nodes, &MerkleNode{
			Hash:  hashData(val),
			Left:  nil,
			Right: nil,
		})
	}

	// 2. Gradimo stablo odozdo prema gore spajanjem po dva čvora
	for len(nodes) > 1 {
		var nextLevel []*MerkleNode

		for i := 0; i < len(nodes); i += 2 {
			if i+1 < len(nodes) {
				// Ako imamo par, spajamo levi i desni heš i heširamo rezultat
				combinedHash := append(nodes[i].Hash, nodes[i+1].Hash...)
				nextLevel = append(nextLevel, &MerkleNode{
					Hash:  hashData(combinedHash),
					Left:  nodes[i],
					Right: nodes[i+1],
				})
			} else {
				// Ako imamo neparan broj čvorova, poslednji se samo prebacuje na sledeći nivo
				nextLevel = append(nextLevel, nodes[i])
			}
		}
		nodes = nextLevel
	}

	// Poslednji preostali čvor je koren stabla (Root)
	return &MerkleTree{Root: nodes[0]}
}
