// Package wal implements an append-only write-ahead log with CRC-framed records.
package wal

import (
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"io"
	"os"
	"sync"
)

// RecordType identifies a mutation stored in the log.
type RecordType uint8

const (
	RecPut    RecordType = 1
	RecDelete RecordType = 2
)

const headerSize = 13 // crc32 + type + klen + vlen

// Record is one durable mutation.
type Record struct {
	Type  RecordType
	Key   []byte
	Value []byte
}

// WAL is a single-writer append-only log file.
type WAL struct {
	mu   sync.Mutex
	f    *os.File
	path string
	sync bool
}

// Open creates or appends to the log at path. If sync is true, every Append fsyncs.
func Open(path string, sync bool) (*WAL, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, err
	}
	if _, err := f.Seek(0, io.SeekEnd); err != nil {
		_ = f.Close()
		return nil, err
	}
	return &WAL{f: f, path: path, sync: sync}, nil
}

// Append writes one record. The MemTable must not be updated until this returns nil.
func (w *WAL) Append(rec Record) error {
	if len(rec.Key) == 0 {
		return fmt.Errorf("wal: empty key")
	}
	if rec.Type != RecPut && rec.Type != RecDelete {
		return fmt.Errorf("wal: unknown record type %d", rec.Type)
	}
	if rec.Type == RecDelete {
		rec.Value = nil
	}

	payload := encodePayload(rec)
	crc := crc32.ChecksumIEEE(payload)

	buf := make([]byte, 4+len(payload))
	binary.LittleEndian.PutUint32(buf[0:4], crc)
	copy(buf[4:], payload)

	w.mu.Lock()
	defer w.mu.Unlock()

	if _, err := w.f.Write(buf); err != nil {
		return err
	}
	if w.sync {
		return w.f.Sync()
	}
	return nil
}

// Sync flushes dirty pages to disk.
func (w *WAL) Sync() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.f.Sync()
}

// Close syncs (if the file is still open) and closes the log.
func (w *WAL) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.f == nil {
		return nil
	}
	err := w.f.Sync()
	cerr := w.f.Close()
	w.f = nil
	if err != nil {
		return err
	}
	return cerr
}

func encodePayload(rec Record) []byte {
	klen := uint32(len(rec.Key))
	vlen := uint32(len(rec.Value))
	buf := make([]byte, 1+4+4+klen+vlen)
	buf[0] = byte(rec.Type)
	binary.LittleEndian.PutUint32(buf[1:5], klen)
	binary.LittleEndian.PutUint32(buf[5:9], vlen)
	copy(buf[9:], rec.Key)
	copy(buf[9+klen:], rec.Value)
	return buf
}

// Replay reads all complete records from path. A torn final record is truncated.
// A CRC mismatch is a hard error (ADR-005).
func Replay(path string, fn func(Record) error) error {
	f, err := os.OpenFile(path, os.O_RDWR, 0o644)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer f.Close()

	var offset int64
	hdr := make([]byte, headerSize)
	for {
		n, err := io.ReadFull(f, hdr)
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			if n > 0 {
				if terr := f.Truncate(offset); terr != nil {
					return terr
				}
			}
			return nil
		}
		if err != nil {
			return err
		}

		wantCRC := binary.LittleEndian.Uint32(hdr[0:4])
		typ := RecordType(hdr[4])
		klen := binary.LittleEndian.Uint32(hdr[5:9])
		vlen := binary.LittleEndian.Uint32(hdr[9:13])

		body := make([]byte, klen+vlen)
		if klen+vlen > 0 {
			if _, err := io.ReadFull(f, body); err != nil {
				if err == io.EOF || err == io.ErrUnexpectedEOF {
					if terr := f.Truncate(offset); terr != nil {
						return terr
					}
					return nil
				}
				return err
			}
		}

		payload := make([]byte, 1+4+4+len(body))
		payload[0] = byte(typ)
		binary.LittleEndian.PutUint32(payload[1:5], klen)
		binary.LittleEndian.PutUint32(payload[5:9], vlen)
		copy(payload[9:], body)

		gotCRC := crc32.ChecksumIEEE(payload)
		if gotCRC != wantCRC {
			return fmt.Errorf("wal: crc mismatch at offset %d", offset)
		}
		if typ != RecPut && typ != RecDelete {
			return fmt.Errorf("wal: unknown type %d at offset %d", typ, offset)
		}

		rec := Record{Type: typ, Key: body[:klen]}
		if vlen > 0 {
			rec.Value = body[klen:]
		}
		if err := fn(rec); err != nil {
			return err
		}
		offset += int64(headerSize) + int64(klen) + int64(vlen)
	}
}
