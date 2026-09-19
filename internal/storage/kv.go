package storage

// KV is the subset of Engine used by indexes and transactions.
type KV interface {
	Get(key []byte) ([]byte, bool, error)
	Set(key, value []byte) error
	Delete(key []byte) error
	ScanPrefix(prefix []byte, fn func(key, value []byte) bool)
}
