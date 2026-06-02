// Copyright 2026 Glassbox Users
// SPDX-License-Identifier: Apache-2.0

package snapshot

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// DedupStore manages content-addressed snapshot storage with deduplication.
// Snapshots are stored by their content hash in a flat directory structure
// to avoid duplicate copies of identical session snapshots.
//
// Storage layout:
//   <baseDir>/
//     snapshots/
//       <hash[:8]>_<hash[8:]>.snap.json    # Actual snapshot file (content-addressed)
//       <hash[:8]>_<hash[8:]>.snap.json    # Multiple identical snapshots deduplicated
//       index.json                          # Index of all stored hashes
//
// Each file is named as: <hash8>_<hash56>.snap.json where hash is the
// SHA-256 hex digest of the PersistedSnapshot content.
type DedupStore struct {
	baseDir string
}

// ContentHash computes the SHA-256 hash of the Snapshot content only.
// This hash is stable - identical snapshots always produce the same hash,
// regardless of creation time or transaction context.
//
// The hash is computed over the normalized snapshot JSON representation
// (ledger entries + linear memory) to ensure deterministic results.
// Metadata is NOT included - two different transactions can have the same
// content hash if their snapshots are identical.
func ContentHash(ps *PersistedSnapshot) (string, error) {
	if ps == nil {
		ps = &PersistedSnapshot{
			Metadata: &ReplayMetadata{},
			Snapshot: FromMap(nil),
		}
	}

	// Only hash the snapshot content, not metadata
	// This ensures identical ledger states deduplicate regardless of tx/network
	snapshot := normalizeSnapshotForHashing(ps.Snapshot)

	// Marshal to JSON without extra whitespace for consistent hashing
	data, err := json.Marshal(snapshot)
	if err != nil {
		return "", fmt.Errorf("failed to marshal snapshot for hashing: %w", err)
	}

	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

// normalizeSnapshotForHashing creates a canonical snapshot representation for hashing.
// This ensures identical logical snapshots always produce the same hash.
func normalizeSnapshotForHashing(snap *Snapshot) *Snapshot {
	if snap == nil {
		return FromMap(nil)
	}

	// Normalize snapshot (sorts entries, computes fingerprint)
	return normalizedForSave(snap)
}

// NewDedupStore creates a new deduplication store with the given base directory.
func NewDedupStore(baseDir string) *DedupStore {
	return &DedupStore{
		baseDir: baseDir,
	}
}

// hashToFilename converts a content hash to a snapshot filename.
// Uses the first 8 hex digits and remaining 56 hex digits for better readability
// and shell-friendly naming.
func hashToFilename(hash string) string {
	if len(hash) < 8 {
		hash = "00000000" + hash
	}
	if len(hash) < 64 {
		hash = hash + strings.Repeat("0", 64-len(hash))
	}
	return hash[:8] + "_" + hash[8:] + ".snap.json"
}

// filenameToHash extracts the content hash from a snapshot filename.
func filenameToHash(filename string) string {
	if !strings.HasSuffix(filename, ".snap.json") {
		return ""
	}
	name := strings.TrimSuffix(filename, ".snap.json")
	parts := strings.Split(name, "_")
	if len(parts) == 2 {
		return parts[0] + parts[1]
	}
	return ""
}

// SaveWithDedup saves a snapshot using content-addressed storage.
// If an identical snapshot already exists (same content hash), the existing
// file is returned and no write occurs. Otherwise, a new file is created.
//
// Returns the hash of the stored snapshot and whether it was newly created.
func (ds *DedupStore) SaveWithDedup(meta *ReplayMetadata, snap *Snapshot) (hash string, isNew bool, err error) {
	if meta == nil {
		return "", false, fmt.Errorf("metadata must not be nil")
	}

	// Normalize snapshot metadata for consistent hashing
	meta.SchemaVersion = PersistSchemaVersion
	if meta.SavedAt.IsZero() {
		meta.SavedAt = meta.SavedAt.UTC()
	}

	ps := &PersistedSnapshot{
		Metadata: meta,
		Snapshot: normalizedForSave(snap),
	}

	// Compute content hash
	hash, err = ContentHash(ps)
	if err != nil {
		return "", false, err
	}

	// Ensure snapshots directory exists
	snapshotDir := filepath.Join(ds.baseDir, "snapshots")
	if err := os.MkdirAll(snapshotDir, 0755); err != nil {
		return "", false, fmt.Errorf("failed to create snapshots directory: %w", err)
	}

	// Build target path
	filename := hashToFilename(hash)
	targetPath := filepath.Join(snapshotDir, filename)

	// Check if file already exists (deduplication)
	if _, err := os.Stat(targetPath); err == nil {
		// File exists, no need to write
		return hash, false, nil
	} else if !os.IsNotExist(err) {
		// Unexpected error
		return "", false, err
	}

	// Write new file
	data, err := json.MarshalIndent(ps, "", "  ")
	if err != nil {
		return "", false, fmt.Errorf("failed to marshal snapshot: %w", err)
	}

	// Atomic write using temp file
	tmp := targetPath + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return "", false, fmt.Errorf("failed to write snapshot temp file: %w", err)
	}
	if err := os.Rename(tmp, targetPath); err != nil {
		_ = os.Remove(tmp)
		return "", false, fmt.Errorf("failed to rename snapshot file: %w", err)
	}

	// Update index
	if err := ds.updateIndex(hash, true); err != nil {
		// Log but don't fail on index errors
		fmt.Fprintf(os.Stderr, "warning: failed to update dedup index: %v\n", err)
	}

	return hash, true, nil
}

// LoadByHash loads a snapshot by its content hash.
func (ds *DedupStore) LoadByHash(hash string) (*PersistedSnapshot, error) {
	if hash == "" {
		return nil, fmt.Errorf("hash must not be empty")
	}

	filename := hashToFilename(hash)
	targetPath := filepath.Join(ds.baseDir, "snapshots", filename)

	return LoadPersisted(targetPath)
}

// GetStoredHashes returns all content hashes currently stored in the dedup store.
func (ds *DedupStore) GetStoredHashes() ([]string, error) {
	snapshotDir := filepath.Join(ds.baseDir, "snapshots")

	entries, err := os.ReadDir(snapshotDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, fmt.Errorf("failed to read snapshots directory: %w", err)
	}

	var hashes []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if strings.HasSuffix(entry.Name(), ".snap.json") {
			hash := filenameToHash(entry.Name())
			if hash != "" {
				hashes = append(hashes, hash)
			}
		}
	}

	sort.Strings(hashes)
	return hashes, nil
}

// QueryDedup reports deduplication statistics for the store.
// Returns the number of unique hashes and total logical snapshots.
func (ds *DedupStore) QueryDedup() (uniqueCount int, err error) {
	hashes, err := ds.GetStoredHashes()
	if err != nil {
		return 0, err
	}
	return len(hashes), nil
}

// DeleteByHash removes a snapshot from storage by its content hash.
// Returns an error if the file doesn't exist.
func (ds *DedupStore) DeleteByHash(hash string) error {
	if hash == "" {
		return fmt.Errorf("hash must not be empty")
	}

	filename := hashToFilename(hash)
	targetPath := filepath.Join(ds.baseDir, "snapshots", filename)

	if err := os.Remove(targetPath); err != nil {
		return fmt.Errorf("failed to delete snapshot: %w", err)
	}

	// Update index
	if err := ds.updateIndex(hash, false); err != nil {
		// Log but don't fail on index errors
		fmt.Fprintf(os.Stderr, "warning: failed to update dedup index: %v\n", err)
	}

	return nil
}

// DedupIndex tracks all stored content hashes for quick lookup.
type DedupIndex struct {
	Hashes    []string `json:"hashes"`
	CreatedAt string   `json:"created_at"`
	UpdatedAt string   `json:"updated_at"`
}

// updateIndex maintains an index file of all stored hashes.
func (ds *DedupStore) updateIndex(hash string, add bool) error {
	indexPath := filepath.Join(ds.baseDir, "snapshots", "index.json")

	// Read existing index
	var index DedupIndex
	if data, err := os.ReadFile(indexPath); err == nil {
		if err := json.Unmarshal(data, &index); err != nil {
			// If index is corrupted, rebuild it
			hashes, err := ds.GetStoredHashes()
			if err != nil {
				return fmt.Errorf("failed to rebuild index: %w", err)
			}
			index.Hashes = hashes
		}
	}

	// Update hashes
	if add {
		// Add if not present
		found := false
		for _, h := range index.Hashes {
			if h == hash {
				found = true
				break
			}
		}
		if !found {
			index.Hashes = append(index.Hashes, hash)
			sort.Strings(index.Hashes)
		}
	} else {
		// Remove if present
		newHashes := make([]string, 0, len(index.Hashes))
		for _, h := range index.Hashes {
			if h != hash {
				newHashes = append(newHashes, h)
			}
		}
		index.Hashes = newHashes
	}

	// Write updated index
	data, err := json.MarshalIndent(index, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal index: %w", err)
	}

	if err := os.WriteFile(indexPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write index: %w", err)
	}

	return nil
}

// RecoverFromCorruption attempts to recover from corrupted snapshot files.
// It scans the directory for valid snapshot files and rebuilds the index.
func (ds *DedupStore) RecoverFromCorruption() (recoveredCount int, err error) {
	snapshotDir := filepath.Join(ds.baseDir, "snapshots")

	entries, err := os.ReadDir(snapshotDir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, fmt.Errorf("failed to read snapshots directory: %w", err)
	}

	var validHashes []string
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".snap.json") {
			continue
		}

		path := filepath.Join(snapshotDir, entry.Name())
		if _, err := LoadPersisted(path); err == nil {
			hash := filenameToHash(entry.Name())
			if hash != "" {
				validHashes = append(validHashes, hash)
			}
		}
	}

	// Write clean index
	sort.Strings(validHashes)
	index := DedupIndex{Hashes: validHashes}
	data, _ := json.MarshalIndent(index, "", "  ")
	indexPath := filepath.Join(snapshotDir, "index.json")
	_ = os.WriteFile(indexPath, data, 0644)

	return len(validHashes), nil
}

// GetDiskUsage returns the total disk space used by stored snapshots.
func (ds *DedupStore) GetDiskUsage() (totalBytes int64, err error) {
	snapshotDir := filepath.Join(ds.baseDir, "snapshots")

	return calculateDirSize(snapshotDir)
}

// calculateDirSize computes the total size of files in a directory.
func calculateDirSize(dir string) (int64, error) {
	var totalSize int64

	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			info, err := d.Info()
			if err != nil {
				return err
			}
			totalSize += info.Size()
		}
		return nil
	})

	if err != nil && !os.IsNotExist(err) {
		return 0, err
	}

	return totalSize, nil
}

// IsDeduplicatedWith checks if another snapshot has the same content hash.
func (ds *DedupStore) IsDeduplicatedWith(hash1, hash2 string) bool {
	return hash1 == hash2
}

// FindDuplicateHashes returns all hashes that have identical content.
// This is useful for identifying which transactions produced the same snapshot.
func (ds *DedupStore) FindDuplicateHashes() (map[string][]string, error) {
	hashes, err := ds.GetStoredHashes()
	if err != nil {
		return nil, err
	}

	// Build map of content to transaction info
	contentToHashes := make(map[string][]string)

	for _, hash := range hashes {
		ps, err := ds.LoadByHash(hash)
		if err != nil {
			continue // Skip corrupted files
		}

		contentHash, _ := ContentHash(ps)
		contentToHashes[contentHash] = append(contentToHashes[contentHash], hash)
	}

	// Filter to only duplicates
	duplicates := make(map[string][]string)
	for content, hashes := range contentToHashes {
		if len(hashes) > 1 {
			duplicates[content] = hashes
		}
	}

	return duplicates, nil
}
