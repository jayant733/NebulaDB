// Package btree is an in-memory B+ tree (order ≥ 3) with leaf sibling scans.
package btree

import "bytes"

type node struct {
	leaf     bool
	keys     [][]byte
	vals     [][]byte
	children []*node
	next     *node
}

// Tree maps keys to values with ordered range iteration.
type Tree struct {
	root  *node
	order int
}

// New returns an empty tree. order is the maximum number of keys per node.
func New(order int) *Tree {
	if order < 3 {
		order = 3
	}
	return &Tree{root: &node{leaf: true}, order: order}
}

func (n *node) search(key []byte) int {
	i := 0
	for i < len(n.keys) && bytes.Compare(n.keys[i], key) < 0 {
		i++
	}
	return i
}

func (n *node) childIndex(key []byte) int {
	i := 0
	for i < len(n.keys) && bytes.Compare(key, n.keys[i]) >= 0 {
		i++
	}
	return i
}

// Get returns a copy of the value for key.
func (t *Tree) Get(key []byte) ([]byte, bool) {
	n := t.root
	for !n.leaf {
		n = n.children[n.childIndex(key)]
	}
	i := n.search(key)
	if i < len(n.keys) && bytes.Equal(n.keys[i], key) {
		return append([]byte(nil), n.vals[i]...), true
	}
	return nil, false
}

// Put inserts or replaces key.
func (t *Tree) Put(key, val []byte) {
	key = append([]byte(nil), key...)
	val = append([]byte(nil), val...)
	if len(t.root.keys) >= t.order {
		mid, right := t.split(t.root)
		t.root = &node{keys: [][]byte{mid}, children: []*node{t.root, right}}
	}
	t.insertNonFull(t.root, key, val)
}

func (t *Tree) insertNonFull(n *node, key, val []byte) {
	if n.leaf {
		i := n.search(key)
		if i < len(n.keys) && bytes.Equal(n.keys[i], key) {
			n.vals[i] = val
			return
		}
		n.keys = insertBytes(n.keys, i, key)
		n.vals = insertBytes(n.vals, i, val)
		return
	}
	i := n.childIndex(key)
	ch := n.children[i]
	if len(ch.keys) >= t.order {
		mid, right := t.split(ch)
		n.keys = insertBytes(n.keys, i, mid)
		n.children = insertNode(n.children, i+1, right)
		if bytes.Compare(key, n.keys[i]) >= 0 {
			i++
		}
		ch = n.children[i]
	}
	t.insertNonFull(ch, key, val)
}

func (t *Tree) split(n *node) (mid []byte, right *node) {
	midIdx := len(n.keys) / 2
	right = &node{leaf: n.leaf}
	if n.leaf {
		mid = append([]byte(nil), n.keys[midIdx]...)
		right.keys = append([][]byte(nil), n.keys[midIdx:]...)
		right.vals = append([][]byte(nil), n.vals[midIdx:]...)
		n.keys = append([][]byte(nil), n.keys[:midIdx]...)
		n.vals = append([][]byte(nil), n.vals[:midIdx]...)
		right.next = n.next
		n.next = right
		return mid, right
	}
	mid = append([]byte(nil), n.keys[midIdx]...)
	right.keys = append([][]byte(nil), n.keys[midIdx+1:]...)
	right.children = append([]*node(nil), n.children[midIdx+1:]...)
	n.keys = append([][]byte(nil), n.keys[:midIdx]...)
	n.children = append([]*node(nil), n.children[:midIdx+1]...)
	return mid, right
}

// Range calls fn for keys in [lo, hi]. Empty lo starts at the minimum key.
// Empty hi means no upper bound. hiInclusive includes hi when hi is non-empty.
func (t *Tree) Range(lo, hi []byte, hiInclusive bool, fn func(key, val []byte) bool) {
	n := t.root
	if lo == nil {
		for !n.leaf {
			n = n.children[0]
		}
	} else {
		for !n.leaf {
			n = n.children[n.childIndex(lo)]
		}
	}
	for n != nil {
		for i, k := range n.keys {
			if lo != nil && bytes.Compare(k, lo) < 0 {
				continue
			}
			if hi != nil {
				cmp := bytes.Compare(k, hi)
				if hiInclusive {
					if cmp > 0 {
						return
					}
				} else if cmp >= 0 {
					return
				}
			}
			if !fn(k, n.vals[i]) {
				return
			}
		}
		n = n.next
	}
}

func insertBytes(s [][]byte, i int, v []byte) [][]byte {
	s = append(s, nil)
	copy(s[i+1:], s[i:])
	s[i] = v
	return s
}

func insertNode(s []*node, i int, v *node) []*node {
	s = append(s, nil)
	copy(s[i+1:], s[i:])
	s[i] = v
	return s
}
