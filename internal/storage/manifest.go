package storage

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	manifestName  = "MANIFEST"
	manifestMagic = "NEBULAMF1"
	sstDirName    = "sst"
)

func sstPath(dir string, id uint64) string {
	return filepath.Join(dir, sstDirName, fmt.Sprintf("%06d.sst", id))
}

func loadManifest(dir string) ([]uint64, error) {
	path := filepath.Join(dir, manifestName)
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	if !sc.Scan() {
		return nil, fmt.Errorf("storage: empty manifest")
	}
	if strings.TrimSpace(sc.Text()) != manifestMagic {
		return nil, fmt.Errorf("storage: bad manifest magic")
	}
	var ids []uint64
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) != 2 || fields[0] != "sst" {
			return nil, fmt.Errorf("storage: bad manifest line %q", line)
		}
		id, err := strconv.ParseUint(fields[1], 10, 64)
		if err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, sc.Err()
}

func saveManifest(dir string, ids []uint64) error {
	path := filepath.Join(dir, manifestName)
	tmp := path + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(f, "%s\n", manifestMagic); err != nil {
		_ = f.Close()
		return err
	}
	for _, id := range ids {
		if _, err := fmt.Fprintf(f, "sst %06d\n", id); err != nil {
			_ = f.Close()
			return err
		}
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	_ = os.Remove(path)
	return os.Rename(tmp, path)
}
