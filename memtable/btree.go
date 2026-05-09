package memtable

import (
	"strings"

	"github.com/Stiroki/Key-Value-Engine/model"
)

type BTreeNode struct {
	Records  []*model.Record // Niz zapisa
	Children []*BTreeNode    // Pokazivaci na decu
	IsLeaf   bool
}

type BTree struct {
	Root *BTreeNode
	M    int // Red stabla
	size int
}

func NewBTree(order int) *BTree {
	if order < 3 {
		order = 3
	}
	return &BTree{
		Root: &BTreeNode{IsLeaf: true},
		M:    order,
		size: 0,
	}
}

func (t *BTree) Get(key string) (*model.Record, bool) {
	return t.search(t.Root, key)
}

func (t *BTree) search(node *BTreeNode, key string) (*model.Record, bool) {
	i := 0
	for i < len(node.Records) && strings.Compare(key, node.Records[i].Key) > 0 {
		i++
	}

	if i < len(node.Records) && node.Records[i].Key == key {
		return node.Records[i], true
	}

	if node.IsLeaf {
		return nil, false
	}

	return t.search(node.Children[i], key)
}

func (t *BTree) Put(record *model.Record) {
	root := t.Root

	if len(root.Records) == t.M-1 {
		newRoot := &BTreeNode{IsLeaf: false}
		newRoot.Children = append(newRoot.Children, root)
		t.Root = newRoot

		t.splitChild(newRoot, 0, root)
		t.insertNonFull(newRoot, record)
	} else {
		t.insertNonFull(root, record)
	}
}

func (t *BTree) splitChild(parent *BTreeNode, index int, child *BTreeNode) {
	midIndex := (t.M - 1) / 2

	newNode := &BTreeNode{IsLeaf: child.IsLeaf}

	newNode.Records = append(newNode.Records, child.Records[midIndex+1:]...)

	if !child.IsLeaf {
		newNode.Children = append(newNode.Children, child.Children[midIndex+1:]...)
	}

	midRecord := child.Records[midIndex]

	child.Records = child.Records[:midIndex]
	if !child.IsLeaf {
		child.Children = child.Children[:midIndex+1]
	}

	parent.Children = append(parent.Children, nil)
	copy(parent.Children[index+2:], parent.Children[index+1:])
	parent.Children[index+1] = newNode

	parent.Records = append(parent.Records, nil)
	copy(parent.Records[index+1:], parent.Records[index:])
	parent.Records[index] = midRecord
}

// insertNonFull dodaje element putujuci odozgo na dole, rasciscavajuci pune cvorove na putu
func (t *BTree) insertNonFull(node *BTreeNode, record *model.Record) {
	i := len(node.Records) - 1

	if node.IsLeaf {
		node.Records = append(node.Records, nil)
		for i >= 0 && strings.Compare(record.Key, node.Records[i].Key) < 0 {
			node.Records[i+1] = node.Records[i]
			i--
		}
		// Ako kljuc vec postoji, samo ga update-ujemo
		if i >= 0 && node.Records[i].Key == record.Key {
			node.Records[i] = record
			node.Records = node.Records[:len(node.Records)-1]
			return
		}
		node.Records[i+1] = record
		t.size++
	} else {
		for i >= 0 && strings.Compare(record.Key, node.Records[i].Key) < 0 {
			i--
		}
		i++

		if len(node.Children[i].Records) == t.M-1 {
			t.splitChild(node, i, node.Children[i])
			if strings.Compare(record.Key, node.Records[i].Key) > 0 {
				i++
			} else if record.Key == node.Records[i].Key {
				node.Records[i] = record
				return
			}
		}
		t.insertNonFull(node.Children[i], record)
	}
}

// GetAll vraca sve zapise iz stabla sortirane po kljucu
func (t *BTree) GetAll() []*model.Record {
	var results []*model.Record
	t.inOrder(t.Root, &results)
	return results
}

func (t *BTree) inOrder(node *BTreeNode, results *[]*model.Record) {
	if node == nil {
		return
	}
	for i := 0; i < len(node.Records); i++ {
		if !node.IsLeaf {
			t.inOrder(node.Children[i], results)
		}
		*results = append(*results, node.Records[i])
	}
	if !node.IsLeaf {
		t.inOrder(node.Children[len(node.Records)], results)
	}
}

func (t *BTree) Size() int {
	return t.size
}

func (t *BTree) Clear() {
	t.Root = &BTreeNode{IsLeaf: true}
	t.size = 0
}
