package sstable

import (
	"crypto/sha1"
)

type MerkleNode struct {
	Hash  []byte
	Left  *MerkleNode
	Right *MerkleNode
}

type MerkleTree struct {
	Root *MerkleNode
}

// hashData racuna SHA-1 hash vrednost
func hashData(data []byte) []byte {
	h := sha1.New()
	h.Write(data)
	return h.Sum(nil)
}

// NewMerkleTree kreira novo stablo na osnovu liste svih vrednosti iz Data block-a
func NewMerkleTree(values [][]byte) *MerkleTree {
	if len(values) == 0 {
		return &MerkleTree{}
	}

	var nodes []*MerkleNode

	for _, val := range values {
		nodes = append(nodes, &MerkleNode{
			Hash:  hashData(val),
			Left:  nil,
			Right: nil,
		})
	}

	// Gradjenje stabla dok ne ostane samo root
	for len(nodes) > 1 {
		var nextLevel []*MerkleNode

		for i := 0; i < len(nodes); i += 2 {
			if i+1 < len(nodes) {
				// Ako imamo par, spajamo levi i desni hash i ponovno hash-iramo
				combinedHash := append(nodes[i].Hash, nodes[i+1].Hash...)
				nextLevel = append(nextLevel, &MerkleNode{
					Hash:  hashData(combinedHash),
					Left:  nodes[i],
					Right: nodes[i+1],
				})
			} else {
				nextLevel = append(nextLevel, nodes[i])
			}
		}
		nodes = nextLevel
	}

	return &MerkleTree{Root: nodes[0]}
}
