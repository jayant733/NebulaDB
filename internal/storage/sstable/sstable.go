// Package sstable implements immutable sorted string tables.
package sstable

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"io"
	"os"

	"github.com/jayant/nebuladb/internal/storage/bloom"
)

const (
	TypePut       uint8 = 1
	TypeTombstone uint8 = 2
	magic               = 0x4C535431 // LST1
	footerSize          = 24
	restartEvery        = 4096
)

// Entry is one sorted record.
type Entry struct {
	Key       []byte
	Value     []byte
	Tombstone bool
}

// Write creates path with sorted entries. entries must be strictly increasing by key.
func Write(path string, entries []Entry) error {
	for i := 1; i < len(entries); i++ {
		if bytes.Compare(entries[i-1].Key, entries[i].Key) >= 0 {
			return fmt.Errorf("sstable: keys not strictly increasing at %d", i)
		}
	}

	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer func() {
		if f != nil {
			_ = f.Close()
		}
	}()

	bf := bloom.New(len(entries), 0.01)
	var (
		indexOffs []uint64
		indexKeys [][]byte
		off       uint64
		since     int
	)
	for i, e := range entries {
		if i == 0 || since >= restartEvery {
			indexOffs = append(indexOffs, off)
			indexKeys = append(indexKeys, e.Key)
			since = 0
		}
		bf.Add(e.Key)
		n, err := writeRecord(f, e)
		if err != nil {
			return err
		}
		off += uint64(n)
		since += n
	}

	indexOff := off
	if err := binary.Write(f, binary.LittleEndian, uint32(len(indexOffs))); err != nil {
		return err
	}
	off += 4
	for i := range indexOffs {
		if err := binary.Write(f, binary.LittleEndian, indexOffs[i]); err != nil {
			return err
		}
		k := indexKeys[i]
		if err := binary.Write(f, binary.LittleEndian, uint32(len(k))); err != nil {
			return err
		}
		if _, err := f.Write(k); err != nil {
			return err
		}
		off += 8 + 4 + uint64(len(k))
	}

	bloomOff := off
	enc := bf.Encode()
	if _, err := f.Write(enc); err != nil {
		return err
	}

	var foot [20]byte
	binary.LittleEndian.PutUint64(foot[0:8], indexOff)
	binary.LittleEndian.PutUint64(foot[8:16], bloomOff)
	binary.LittleEndian.PutUint32(foot[16:20], magic)
	crc := crc32.ChecksumIEEE(foot[:])
	if _, err := f.Write(foot[:]); err != nil {
		return err
	}
	if err := binary.Write(f, binary.LittleEndian, crc); err != nil {
		return err
	}
	if err := f.Sync(); err != nil {
		return err
	}
	err = f.Close()
	f = nil
	return err
}

func writeRecord(w io.Writer, e Entry) (int, error) {
	typ := TypePut
	if e.Tombstone {
		typ = TypeTombstone
		e.Value = nil
	}
	buf := make([]byte, 1+4+4+len(e.Key)+len(e.Value))
	buf[0] = typ
	binary.LittleEndian.PutUint32(buf[1:5], uint32(len(e.Key)))
	binary.LittleEndian.PutUint32(buf[5:9], uint32(len(e.Value)))
	copy(buf[9:], e.Key)
	copy(buf[9+len(e.Key):], e.Value)
	n, err := w.Write(buf)
	return n, err
}

// Reader is an opened SSTable.
type Reader struct {
	path     string
	f        *os.File
	filter   *bloom.Filter
	idxOff   []uint64
	idxKey   [][]byte
	dataEnd  int64
	indexOff uint64
	bloomN   uint64 // Get probes that consulted the filter
	bloomNeg uint64 // Get probes skipped by a negative Bloom
}

// Open reads footer, index, and Bloom filter.
func Open(path string) (*Reader, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	st, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, err
	}
	if st.Size() < footerSize {
		_ = f.Close()
		return nil, fmt.Errorf("sstable: file too small")
	}
	foot := make([]byte, footerSize)
	if _, err := f.ReadAt(foot, st.Size()-footerSize); err != nil {
		_ = f.Close()
		return nil, err
	}
	want := binary.LittleEndian.Uint32(foot[20:24])
	if crc32.ChecksumIEEE(foot[:20]) != want {
		_ = f.Close()
		return nil, fmt.Errorf("sstable: footer crc mismatch")
	}
	indexOff := binary.LittleEndian.Uint64(foot[0:8])
	bloomOff := binary.LittleEndian.Uint64(foot[8:16])
	if binary.LittleEndian.Uint32(foot[16:20]) != magic {
		_ = f.Close()
		return nil, fmt.Errorf("sstable: bad magic")
	}

	bloomLen := int(st.Size()-footerSize) - int(bloomOff)
	if bloomLen < 0 {
		_ = f.Close()
		return nil, fmt.Errorf("sstable: bad bloom offset")
	}
	braw := make([]byte, bloomLen)
	if _, err := f.ReadAt(braw, int64(bloomOff)); err != nil {
		_ = f.Close()
		return nil, err
	}
	filter, err := bloom.Decode(braw)
	if err != nil {
		_ = f.Close()
		return nil, err
	}

	indexLen := int(bloomOff - indexOff)
	iraw := make([]byte, indexLen)
	if _, err := f.ReadAt(iraw, int64(indexOff)); err != nil {
		_ = f.Close()
		return nil, err
	}
	if len(iraw) < 4 {
		_ = f.Close()
		return nil, fmt.Errorf("sstable: short index")
	}
	nidx := binary.LittleEndian.Uint32(iraw[0:4])
	p := 4
	r := &Reader{
		path:     path,
		f:        f,
		filter:   filter,
		dataEnd:  int64(indexOff),
		indexOff: indexOff,
	}
	for i := uint32(0); i < nidx; i++ {
		if p+12 > len(iraw) {
			_ = f.Close()
			return nil, fmt.Errorf("sstable: truncated index")
		}
		off := binary.LittleEndian.Uint64(iraw[p : p+8])
		p += 8
		klen := binary.LittleEndian.Uint32(iraw[p : p+4])
		p += 4
		if p+int(klen) > len(iraw) {
			_ = f.Close()
			return nil, fmt.Errorf("sstable: truncated index key")
		}
		k := make([]byte, klen)
		copy(k, iraw[p:p+int(klen)])
		p += int(klen)
		r.idxOff = append(r.idxOff, off)
		r.idxKey = append(r.idxKey, k)
	}
	return r, nil
}

// Close closes the file.
func (r *Reader) Close() error {
	if r.f == nil {
		return nil
	}
	err := r.f.Close()
	r.f = nil
	return err
}

// Path returns the file path.
func (r *Reader) Path() string { return r.path }

// Get looks up key. ok=false means absent in this file (or Bloom miss).
func (r *Reader) Get(key []byte) (value []byte, tombstone bool, ok bool, err error) {
	if r.filter != nil {
		r.bloomN++
		if !r.filter.MayContain(key) {
			r.bloomNeg++
			return nil, false, false, nil
		}
	}
	start := r.restartFor(key)
	off := int64(start)
	for off < r.dataEnd {
		e, n, err := r.readRecord(off)
		if err != nil {
			return nil, false, false, err
		}
		cmp := bytes.Compare(e.Key, key)
		if cmp == 0 {
			return e.Value, e.Tombstone, true, nil
		}
		if cmp > 0 {
			return nil, false, false, nil
		}
		off += int64(n)
	}
	return nil, false, false, nil
}

func (r *Reader) restartFor(key []byte) uint64 {
	if len(r.idxKey) == 0 {
		return 0
	}
	i := 0
	for i+1 < len(r.idxKey) && bytes.Compare(r.idxKey[i+1], key) <= 0 {
		i++
	}
	return r.idxOff[i]
}

func (r *Reader) readRecord(off int64) (Entry, int, error) {
	hdr := make([]byte, 9)
	if _, err := r.f.ReadAt(hdr, off); err != nil {
		return Entry{}, 0, err
	}
	typ := hdr[0]
	klen := binary.LittleEndian.Uint32(hdr[1:5])
	vlen := binary.LittleEndian.Uint32(hdr[5:9])
	body := make([]byte, klen+vlen)
	if len(body) > 0 {
		if _, err := r.f.ReadAt(body, off+9); err != nil {
			return Entry{}, 0, err
		}
	}
	e := Entry{Key: body[:klen], Value: body[klen:], Tombstone: typ == TypeTombstone}
	if e.Tombstone {
		e.Value = nil
	} else {
		e.Value = bytes.Clone(e.Value)
	}
	e.Key = bytes.Clone(e.Key)
	return e, 9 + int(klen) + int(vlen), nil
}

// Iter walks records in key order.
func (r *Reader) Iter() *Iter {
	return &Iter{r: r, off: 0}
}

// Iter is a sequential SSTable cursor.
type Iter struct {
	r   *Reader
	off int64
	cur Entry
	ok  bool
	err error
}

// Next advances. False on end or error.
func (it *Iter) Next() bool {
	if it.err != nil || it.off >= it.r.dataEnd {
		it.ok = false
		return false
	}
	e, n, err := it.r.readRecord(it.off)
	if err != nil {
		it.err = err
		it.ok = false
		return false
	}
	it.cur = e
	it.off += int64(n)
	it.ok = true
	return true
}

func (it *Iter) Entry() Entry { return it.cur }
func (it *Iter) Err() error   { return it.err }

// BloomStats returns how many Get probes consulted the filter and how many were skipped.
func (r *Reader) BloomStats() (checked, negative uint64) {
	return r.bloomN, r.bloomNeg
}
