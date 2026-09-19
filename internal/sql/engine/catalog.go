package engine

import (
	"encoding/binary"
	"fmt"
	"sync"

	"github.com/jayant/nebuladb/internal/sql/ast"
	"github.com/jayant/nebuladb/internal/storage"
)

var schemaKey = []byte("x\x00schema")

// Column is a persisted column definition.
type Column struct {
	Name string
	Type ast.Type
}

// Table is a persisted table definition.
type Table struct {
	Name string
	Cols []Column
}

// Catalog is the on-disk table list.
type Catalog struct {
	mu     sync.Mutex
	kv    storage.KV
	tables map[string]*Table
}

func loadCatalog(kv storage.KV) (*Catalog, error) {
	c := &Catalog{kv: kv, tables: map[string]*Table{}}
	raw, ok, err := kv.Get(schemaKey)
	if err != nil {
		return nil, err
	}
	if !ok {
		return c, nil
	}
	tabs, err := decodeSchema(raw)
	if err != nil {
		return nil, err
	}
	for _, t := range tabs {
		tt := t
		c.tables[t.Name] = &tt
	}
	return c, nil
}

func (c *Catalog) refreshLocked() {
	raw, ok, err := c.kv.Get(schemaKey)
	if err != nil || !ok {
		return
	}
	tabs, err := decodeSchema(raw)
	if err != nil {
		return
	}
	c.tables = map[string]*Table{}
	for _, t := range tabs {
		tt := t
		c.tables[t.Name] = &tt
	}
}

func (c *Catalog) list() []*Table {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.refreshLocked()
	out := make([]*Table, 0, len(c.tables))
	for _, t := range c.tables {
		out = append(out, t)
	}
	return out
}

func (c *Catalog) get(name string) (*Table, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.refreshLocked()
	t, ok := c.tables[name]
	if !ok {
		return nil, fmt.Errorf("sql: table %q does not exist", name)
	}
	return t, nil
}

func (c *Catalog) create(t *Table) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.refreshLocked()
	if _, ok := c.tables[t.Name]; ok {
		return fmt.Errorf("sql: table %q already exists", t.Name)
	}
	c.tables[t.Name] = t
	return c.persist()
}

func (c *Catalog) persist() error {
	tabs := make([]Table, 0, len(c.tables))
	for _, t := range c.tables {
		tabs = append(tabs, *t)
	}
	return c.kv.Set(schemaKey, encodeSchema(tabs))
}

func (c *Catalog) col(table, col string) (int, ast.Type, error) {
	t, err := c.get(table)
	if err != nil {
		return 0, 0, err
	}
	for i, cl := range t.Cols {
		if cl.Name == col {
			return i, cl.Type, nil
		}
	}
	return 0, 0, fmt.Errorf("sql: column %q not in %s", col, table)
}

func encodeSchema(tabs []Table) []byte {
	size := 4
	for _, t := range tabs {
		size += 4 + len(t.Name) + 4
		for _, cl := range t.Cols {
			size += 4 + len(cl.Name) + 1
		}
	}
	buf := make([]byte, size)
	binary.LittleEndian.PutUint32(buf[0:4], uint32(len(tabs)))
	p := 4
	for _, t := range tabs {
		binary.LittleEndian.PutUint32(buf[p:p+4], uint32(len(t.Name)))
		p += 4
		copy(buf[p:], t.Name)
		p += len(t.Name)
		binary.LittleEndian.PutUint32(buf[p:p+4], uint32(len(t.Cols)))
		p += 4
		for _, cl := range t.Cols {
			binary.LittleEndian.PutUint32(buf[p:p+4], uint32(len(cl.Name)))
			p += 4
			copy(buf[p:], cl.Name)
			p += len(cl.Name)
			buf[p] = byte(cl.Type)
			p++
		}
	}
	return buf
}

func decodeSchema(buf []byte) ([]Table, error) {
	if len(buf) < 4 {
		return nil, fmt.Errorf("sql: short schema")
	}
	n := binary.LittleEndian.Uint32(buf[0:4])
	p := 4
	var tabs []Table
	for i := uint32(0); i < n; i++ {
		if p+4 > len(buf) {
			return nil, fmt.Errorf("sql: truncated schema")
		}
		ln := binary.LittleEndian.Uint32(buf[p : p+4])
		p += 4
		if p+int(ln)+4 > len(buf) {
			return nil, fmt.Errorf("sql: truncated table")
		}
		name := string(buf[p : p+int(ln)])
		p += int(ln)
		nc := binary.LittleEndian.Uint32(buf[p : p+4])
		p += 4
		t := Table{Name: name}
		for j := uint32(0); j < nc; j++ {
			if p+4 > len(buf) {
				return nil, fmt.Errorf("sql: truncated column")
			}
			cln := binary.LittleEndian.Uint32(buf[p : p+4])
			p += 4
			if p+int(cln)+1 > len(buf) {
				return nil, fmt.Errorf("sql: truncated column name")
			}
			cname := string(buf[p : p+int(cln)])
			p += int(cln)
			typ := ast.Type(buf[p])
			p++
			t.Cols = append(t.Cols, Column{Name: cname, Type: typ})
		}
		tabs = append(tabs, t)
	}
	return tabs, nil
}
