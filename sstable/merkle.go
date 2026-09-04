package sstable

import (
	"bytes"
	"crypto/sha1"
	"encoding/binary"
	"io"
)

type MerkleNode struct {
	Hash  []byte
	Left  *MerkleNode
	Right *MerkleNode
}

type MerkleTree struct {
	Root   *MerkleNode
	Leaves []*MerkleNode
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

	var leaves []*MerkleNode
	for _, val := range values {
		node := &MerkleNode{
			Hash:  hashData(val),
			Left:  nil,
			Right: nil,
		}
		leaves = append(leaves, node)
	}

	nodes := make([]*MerkleNode, len(leaves))
	copy(nodes, leaves)

	// Gradjenje stabla dok ne ostane samo root
	for len(nodes) > 1 {
		var nextLevel []*MerkleNode

		for i := 0; i < len(nodes); i += 2 {
			if i+1 < len(nodes) {
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

	return &MerkleTree{
		Root:   nodes[0],
		Leaves: leaves,
	}
}

// Serialize čuva broj listova i heš svakog lista u bajtove
func (m *MerkleTree) Serialize() []byte {
	buf := new(bytes.Buffer)
	if m.Root == nil {
		return buf.Bytes()
	}

	// 1. Zapisujemo Root Hash (20 bajtova za SHA-1)
	buf.Write(m.Root.Hash)

	// 2. Broj listova
	leafCount := uint32(len(m.Leaves))
	err := binary.Write(buf, binary.LittleEndian, leafCount)
	if err != nil {
		return nil
	}

	// 3. Heš svakog lista redom
	for _, leaf := range m.Leaves {
		buf.Write(leaf.Hash)
	}

	return buf.Bytes()
}

// DeserializeMerkleMetadata učitava Root Hash i listove iz sačuvanih metapodataka
func DeserializeMerkleMetadata(r io.Reader) (rootHash []byte, leafHashes [][]byte, err error) {
	rootHash = make([]byte, 20) // SHA-1 je 20 bajtova
	if _, err := io.ReadFull(r, rootHash); err != nil {
		return nil, nil, err
	}

	var leafCount uint32
	if err := binary.Read(r, binary.LittleEndian, &leafCount); err != nil {
		return nil, nil, err
	}

	leafHashes = make([][]byte, leafCount)
	for i := uint32(0); i < leafCount; i++ {
		leaf := make([]byte, 20)
		if _, err := io.ReadFull(r, leaf); err != nil {
			return nil, nil, err
		}
		leafHashes[i] = leaf
	}

	return rootHash, leafHashes, nil
}

// CompareTrees upoređuje sačuvane listove sa novim stablom i vraća indekse oštećenih zapisa
func (m *MerkleTree) FindCorruptedIndices(savedLeafHashes [][]byte) []int {
	var corrupted []int

	maxLen := len(savedLeafHashes)
	if len(m.Leaves) > maxLen {
		maxLen = len(m.Leaves)
	}

	for i := 0; i < maxLen; i++ {
		if i >= len(savedLeafHashes) || i >= len(m.Leaves) {
			corrupted = append(corrupted, i)
			continue
		}
		if !bytes.Equal(savedLeafHashes[i], m.Leaves[i].Hash) {
			corrupted = append(corrupted, i)
		}
	}

	return corrupted
}
