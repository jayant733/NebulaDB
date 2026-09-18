# Storage Engine (Phase 1)

## Key-value API

```go
Set(key, value []byte) error
Get(key []byte) (value []byte, ok bool, err error)
Delete(key []byte) error
```

Keys and values are opaque bytes. SQL will later map rows onto this API (for example `table\x1fpk` → encoded tuple).

Empty keys are rejected. `nil` and empty values are stored as zero-length values (distinct from a missing key). Delete is a **tombstone** in the MemTable so Phase 2 compaction can drop older versions.

## WAL on-disk format

Little-endian. One file, append-only.

```
record:
  crc32  uint32   // IEEE CRC of [type..payload], not including crc itself
  type   uint8    // 1 = Put, 2 = Delete
  klen   uint32
  vlen   uint32   // 0 for Delete
  key    [klen]byte
  value  [vlen]byte
```

Why CRC: detect torn writes and corruption. Why length prefixes: streaming replay without delimiters in user data.

### Durability

`Engine.Options.Sync`:

| Mode | When fsync runs | Use |
|------|-----------------|-----|
| `always` (default) | After every record | Correctness, demos, recovery tests |
| `none` | Never (OS page cache only) | Benchmarks only; not crash-safe |

`fsync` is the difference between "I wrote a file" and "the disk has the bytes after power loss." Group commit (batch many records, one fsync) is a Phase 2/11 performance item.

### Recovery

1. Open WAL at start of file.
2. Read records until EOF or a short read.
3. If a record header/payload is incomplete → **truncate** back to last good offset (torn write).
4. If CRC fails → return error (do not skip silently in Phase 1).
5. Apply Put/Delete to a new MemTable.

## MemTable

In-memory **skip list**, ordered by `bytes.Compare`.

- Expected height ~ log₄(n).
- Tombstones: `deleted=true`, value empty.
- `Get` returns `(nil, false)` for missing or tombstoned keys.
- Iterator (scan) skips tombstones for the REPL; compaction in Phase 2 will emit them to SSTables.

A `map[string][]byte` would be simpler but would not teach ordered flush to SSTables. The skip list is the LSM-ready structure.

## Invariants

1. **WAL-then-MemTable:** MemTable is updated only after a successful WAL append.
2. **Single-writer log:** concurrent `Set`/`Delete` serialize on the WAL mutex.
3. **Idempotent replay:** replaying the same WAL twice yields the same MemTable state (last write wins per key).
4. **Copy-out on Get:** returned slices are copies so callers cannot mutate internal nodes.

## LSM (Phase 2)

### Read path

```
Get(key) → MemTable → SSTables (newest → oldest)
         Bloom negative → skip file
         tombstone      → not found
```

### SSTable layout (little-endian)

```
data:  repeating
  type   uint8     // 1 = put, 2 = tombstone
  klen   uint32
  vlen   uint32
  key, value

index: (offsets of restart keys, every ~4KiB of data)
  count  uint32
  repeating: offset uint64, klen uint32, key

bloom:
  k      uint8     // hash functions
  nbits  uint32    // bit array length
  bits   [(nbits+7)/8]byte

footer (24 bytes at EOF):
  index_off  uint64
  bloom_off  uint64
  magic      uint32   // 0x4C535431  "LST1"
  crc32      uint32   // IEEE of the 20 bytes above
```

### MANIFEST

Text file `MANIFEST` in the data dir:

```
NEBULAMF1
sst 000001
sst 000002
```

Later lines are **newer**. Reads search from the bottom.

### Compaction

Size-tiered L0: when live SST count ≥ `CompactN` (default 4), merge all SSTables newest-wins into one file. With a single level, tombstones can be dropped in the output.
