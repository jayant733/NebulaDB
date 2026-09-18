package index

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"sync"

	"github.com/jayant/nebuladb/internal/row"
	"github.com/jayant/nebuladb/internal/storage"
)

// Spec is a secondary index on one tuple field.
type Spec struct {
	Name  string
	Field int
}

// Store maintains primary rows and LSM secondary indexes on an Engine.
type Store struct {
	mu    sync.Mutex
	eng   *storage.Engine
	specs []Spec
}

// Wrap loads the index catalog from eng.
func Wrap(eng *storage.Engine) (*Store, error) {
	s := &Store{eng: eng}
	raw, ok, err := eng.Get(catalogKey)
	if err != nil {
		return nil, err
	}
	if ok {
		specs, err := decodeCatalog(raw)
		if err != nil {
			return nil, err
		}
		s.specs = specs
	}
	return s, nil
}

// Engine returns the underlying KV engine.
func (s *Store) Engine() *storage.Engine { return s.eng }

// Indexes returns a copy of the catalog.
func (s *Store) Indexes() []Spec {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Spec, len(s.specs))
	copy(out, s.specs)
	return out
}

// CreateIndex records spec and backfills from existing primary rows.
func (s *Store) CreateIndex(name string, field int) error {
	if name == "" || field < 0 {
		return fmt.Errorf("index: invalid spec")
	}
	if bytesContainsNUL(name) {
		return fmt.Errorf("index: name must not contain NUL")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, sp := range s.specs {
		if sp.Name == name {
			return fmt.Errorf("index: %q exists", name)
		}
	}
	s.specs = append(s.specs, Spec{Name: name, Field: field})
	if err := s.persistCatalog(); err != nil {
		s.specs = s.specs[:len(s.specs)-1]
		return err
	}
	idx := []byte(name)
	type pair struct {
		pk     []byte
		fields [][]byte
	}
	var (
		rows        []pair
		backfillErr error
	)
	s.eng.ScanPrefix([]byte{tagPrimary}, func(k, v []byte) bool {
		pk, ok := parsePrimary(k)
		if !ok {
			return true
		}
		fields, err := row.Decode(v)
		if err != nil {
			backfillErr = err
			return false
		}
		rows = append(rows, pair{pk: append([]byte(nil), pk...), fields: fields})
		return true
	})
	if backfillErr != nil {
		return backfillErr
	}
	for _, r := range rows {
		if err := s.addSecondaryLocked(idx, field, r.pk, r.fields); err != nil {
			return err
		}
	}
	return nil
}

// DropIndex removes secondary keys and the catalog entry.
func (s *Store) DropIndex(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	i := -1
	for j, sp := range s.specs {
		if sp.Name == name {
			i = j
			break
		}
	}
	if i < 0 {
		return fmt.Errorf("index: %q not found", name)
	}
	var keys [][]byte
	s.eng.ScanPrefix(SecondaryPrefixAll([]byte(name)), func(k, v []byte) bool {
		keys = append(keys, append([]byte(nil), k...))
		return true
	})
	for _, k := range keys {
		if err := s.eng.Delete(k); err != nil {
			return err
		}
	}
	s.specs = append(s.specs[:i], s.specs[i+1:]...)
	return s.persistCatalog()
}

// PutRow writes a primary tuple and maintains indexes.
func (s *Store) PutRow(pk []byte, fields [][]byte) error {
	if len(pk) == 0 {
		return fmt.Errorf("index: empty primary key")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	old, ok, err := s.eng.Get(PrimaryKey(pk))
	if err != nil {
		return err
	}
	if ok {
		oldFields, err := row.Decode(old)
		if err != nil {
			return err
		}
		if err := s.removeAllSecondaryLocked(pk, oldFields); err != nil {
			return err
		}
	}
	if err := s.eng.Set(PrimaryKey(pk), row.Encode(fields)); err != nil {
		return err
	}
	return s.addAllSecondaryLocked(pk, fields)
}

// GetRow decodes the primary tuple.
func (s *Store) GetRow(pk []byte) ([][]byte, bool, error) {
	v, ok, err := s.eng.Get(PrimaryKey(pk))
	if err != nil || !ok {
		return nil, ok, err
	}
	fields, err := row.Decode(v)
	if err != nil {
		return nil, false, err
	}
	return fields, true, nil
}

// DeleteRow removes the primary row and secondary keys.
func (s *Store) DeleteRow(pk []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	old, ok, err := s.eng.Get(PrimaryKey(pk))
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}
	fields, err := row.Decode(old)
	if err != nil {
		return err
	}
	if err := s.removeAllSecondaryLocked(pk, fields); err != nil {
		return err
	}
	return s.eng.Delete(PrimaryKey(pk))
}

// Find returns primary keys with indexed field equal to sk (SK order, then pk).
func (s *Store) Find(name string, sk []byte) ([][]byte, error) {
	if _, err := s.spec(name); err != nil {
		return nil, err
	}
	var pks [][]byte
	idx := []byte(name)
	s.eng.ScanPrefix(SecondaryPrefixSK(idx, sk), func(k, v []byte) bool {
		_, pk, ok := parseSecondary(k, idx)
		if ok {
			pks = append(pks, append([]byte(nil), pk...))
		}
		return true
	})
	return pks, nil
}

// RangeFind returns pks with lo ≤ sk ≤ hi.
func (s *Store) RangeFind(name string, lo, hi []byte) ([][]byte, error) {
	if _, err := s.spec(name); err != nil {
		return nil, err
	}
	idx := []byte(name)
	var pks [][]byte
	s.eng.ScanPrefix(SecondaryPrefixAll(idx), func(k, v []byte) bool {
		sk, pk, ok := parseSecondary(k, idx)
		if !ok {
			return true
		}
		if lo != nil && bytes.Compare(sk, lo) < 0 {
			return true
		}
		if hi != nil && bytes.Compare(sk, hi) > 0 {
			return false
		}
		pks = append(pks, append([]byte(nil), pk...))
		return true
	})
	return pks, nil
}

func (s *Store) spec(name string) (Spec, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, sp := range s.specs {
		if sp.Name == name {
			return sp, nil
		}
	}
	return Spec{}, fmt.Errorf("index: %q not found", name)
}

func (s *Store) persistCatalog() error {
	return s.eng.Set(catalogKey, encodeCatalog(s.specs))
}

func (s *Store) addAllSecondaryLocked(pk []byte, fields [][]byte) error {
	for _, sp := range s.specs {
		if err := s.addSecondaryLocked([]byte(sp.Name), sp.Field, pk, fields); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) removeAllSecondaryLocked(pk []byte, fields [][]byte) error {
	for _, sp := range s.specs {
		sk, ok := row.Field(fields, sp.Field)
		if !ok {
			continue
		}
		if err := s.eng.Delete(SecondaryKey([]byte(sp.Name), sk, pk)); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) addSecondaryLocked(idx []byte, field int, pk []byte, fields [][]byte) error {
	sk, ok := row.Field(fields, field)
	if !ok {
		return nil
	}
	return s.eng.Set(SecondaryKey(idx, sk, pk), nil)
}

func bytesContainsNUL(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] == 0 {
			return true
		}
	}
	return false
}

func encodeCatalog(specs []Spec) []byte {
	size := 4
	for _, sp := range specs {
		size += 4 + len(sp.Name) + 4
	}
	buf := make([]byte, size)
	binary.LittleEndian.PutUint32(buf[0:4], uint32(len(specs)))
	p := 4
	for _, sp := range specs {
		binary.LittleEndian.PutUint32(buf[p:p+4], uint32(len(sp.Name)))
		p += 4
		copy(buf[p:], sp.Name)
		p += len(sp.Name)
		binary.LittleEndian.PutUint32(buf[p:p+4], uint32(sp.Field))
		p += 4
	}
	return buf
}

func decodeCatalog(buf []byte) ([]Spec, error) {
	if len(buf) < 4 {
		return nil, fmt.Errorf("index: short catalog")
	}
	n := binary.LittleEndian.Uint32(buf[0:4])
	p := 4
	out := make([]Spec, 0, n)
	for i := uint32(0); i < n; i++ {
		if p+4 > len(buf) {
			return nil, fmt.Errorf("index: truncated catalog name")
		}
		ln := binary.LittleEndian.Uint32(buf[p : p+4])
		p += 4
		if p+int(ln)+4 > len(buf) {
			return nil, fmt.Errorf("index: truncated catalog entry")
		}
		name := string(buf[p : p+int(ln)])
		p += int(ln)
		field := int(binary.LittleEndian.Uint32(buf[p : p+4]))
		p += 4
		out = append(out, Spec{Name: name, Field: field})
	}
	return out, nil
}
