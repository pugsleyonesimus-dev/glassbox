// Copyright 2026 Glassbox Users
// SPDX-License-Identifier: Apache-2.0

package snapshot

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// --- ContentHash Tests ---

func TestContentHash_Deterministic(t *testing.T) {
	meta := &ReplayMetadata{
		GlassboxVersion: "v1.0.0",
		TxHash:          "abc123",
		Network:         "testnet",
		EnvelopeXdr:     "env",
		ResultMetaXdr:   "result",
	}
	snap := FromMap(map[string]string{"key": "value"})
	ps := &PersistedSnapshot{Metadata: meta, Snapshot: snap}

	hash1, err := ContentHash(ps)
	if err != nil {
		t.Fatalf("ContentHash failed: %v", err)
	}

	hash2, err := ContentHash(ps)
	if err != nil {
		t.Fatalf("ContentHash failed: %v", err)
	}

	if hash1 != hash2 {
		t.Errorf("ContentHash not deterministic: %s vs %s", hash1, hash2)
	}
}

func TestContentHash_IdenticalSnapshots(t *testing.T) {
	meta := &ReplayMetadata{
		GlassboxVersion: "v1.0.0",
		TxHash:          "abc123",
		Network:         "testnet",
	}
	snap := FromMap(map[string]string{"k1": "v1", "k2": "v2"})

	ps1 := &PersistedSnapshot{Metadata: meta, Snapshot: snap}
	ps2 := &PersistedSnapshot{Metadata: meta, Snapshot: snap}

	hash1, _ := ContentHash(ps1)
	hash2, _ := ContentHash(ps2)

	if hash1 != hash2 {
		t.Errorf("Identical snapshots have different hashes: %s vs %s", hash1, hash2)
	}
}

func TestContentHash_DifferentSnapshots(t *testing.T) {
	meta := &ReplayMetadata{
		GlassboxVersion: "v1.0.0",
		TxHash:          "abc123",
		Network:         "testnet",
	}

	snap1 := FromMap(map[string]string{"k": "v1"})
	snap2 := FromMap(map[string]string{"k": "v2"})

	ps1 := &PersistedSnapshot{Metadata: meta, Snapshot: snap1}
	ps2 := &PersistedSnapshot{Metadata: meta, Snapshot: snap2}

	hash1, _ := ContentHash(ps1)
	hash2, _ := ContentHash(ps2)

	if hash1 == hash2 {
		t.Errorf("Different snapshots have same hash: %s", hash1)
	}
}

func TestContentHash_TimestampIgnored(t *testing.T) {
	snap := FromMap(map[string]string{"k": "v"})

	meta1 := &ReplayMetadata{
		GlassboxVersion: "v1.0.0",
		TxHash:          "abc123",
		Network:         "testnet",
		SavedAt:         time.Now(),
	}

	meta2 := &ReplayMetadata{
		GlassboxVersion: "v1.0.0",
		TxHash:          "abc123",
		Network:         "testnet",
		SavedAt:         time.Now().Add(1 * time.Hour),
	}

	ps1 := &PersistedSnapshot{Metadata: meta1, Snapshot: snap}
	ps2 := &PersistedSnapshot{Metadata: meta2, Snapshot: snap}

	hash1, _ := ContentHash(ps1)
	hash2, _ := ContentHash(ps2)

	if hash1 != hash2 {
		t.Errorf("Timestamp should not affect hash: %s vs %s", hash1, hash2)
	}
}

func TestContentHash_NilSnapshot(t *testing.T) {
	hash, err := ContentHash(nil)
	if err != nil {
		t.Fatalf("ContentHash failed on nil: %v", err)
	}
	if hash == "" {
		t.Error("ContentHash returned empty string for nil")
	}

	hash2, _ := ContentHash(nil)
	if hash != hash2 {
		t.Errorf("ContentHash(nil) not deterministic: %s vs %s", hash, hash2)
	}
}

// --- Filename/Hash Conversion Tests ---

func TestHashToFilename_Format(t *testing.T) {
	hash := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	filename := hashToFilename(hash)

	expected := "01234567_89abcdef0123456789abcdef0123456789abcdef0123456789abcdef.snap.json"
	if filename != expected {
		t.Errorf("expected %s, got %s", expected, filename)
	}
}

func TestFilenameToHash_RoundTrip(t *testing.T) {
	hash := "fedcba9876543210fedcba9876543210fedcba9876543210fedcba9876543210"
	filename := hashToFilename(hash)
	recovered := filenameToHash(filename)

	if recovered != hash {
		t.Errorf("hash round-trip failed: %s -> %s -> %s", hash, filename, recovered)
	}
}

// --- DedupStore Tests ---

func TestDedupStore_SaveWithDedup_New(t *testing.T) {
	dir := t.TempDir()
	store := NewDedupStore(dir)

	meta := &ReplayMetadata{
		GlassboxVersion: "v1.0.0",
		TxHash:          "tx1",
		Network:         "testnet",
	}
	snap := FromMap(map[string]string{"key": "value"})

	hash, isNew, err := store.SaveWithDedup(meta, snap)
	if err != nil {
		t.Fatalf("SaveWithDedup failed: %v", err)
	}
	if !isNew {
		t.Error("expected isNew=true for new snapshot")
	}
	if hash == "" {
		t.Error("expected non-empty hash")
	}

	// Verify file was created
	filename := hashToFilename(hash)
	path := filepath.Join(dir, "snapshots", filename)
	if _, err := os.Stat(path); err != nil {
		t.Errorf("snapshot file not created: %v", err)
	}
}

func TestDedupStore_SaveWithDedup_Deduplication(t *testing.T) {
	dir := t.TempDir()
	store := NewDedupStore(dir)

	meta := &ReplayMetadata{
		GlassboxVersion: "v1.0.0",
		TxHash:          "tx1",
		Network:         "testnet",
	}
	snap := FromMap(map[string]string{"key": "value"})

	// Save first snapshot
	hash1, isNew1, err := store.SaveWithDedup(meta, snap)
	if err != nil {
		t.Fatalf("first SaveWithDedup failed: %v", err)
	}
	if !isNew1 {
		t.Error("expected isNew=true for first save")
	}

	// Save identical snapshot (different tx hash but same content)
	meta2 := &ReplayMetadata{
		GlassboxVersion: "v1.0.0",
		TxHash:          "tx2",
		Network:         "testnet",
	}
	hash2, isNew2, err := store.SaveWithDedup(meta2, snap)
	if err != nil {
		t.Fatalf("second SaveWithDedup failed: %v", err)
	}
	if isNew2 {
		t.Error("expected isNew=false for duplicate snapshot")
	}
	if hash1 != hash2 {
		t.Errorf("identical snapshots should have same hash: %s vs %s", hash1, hash2)
	}

	// Verify only one file exists
	snapshots, err := store.GetStoredHashes()
	if err != nil {
		t.Fatalf("GetStoredHashes failed: %v", err)
	}
	if len(snapshots) != 1 {
		t.Errorf("expected 1 stored snapshot, got %d", len(snapshots))
	}
}

func TestDedupStore_SaveWithDedup_Different(t *testing.T) {
	dir := t.TempDir()
	store := NewDedupStore(dir)

	meta := &ReplayMetadata{
		GlassboxVersion: "v1.0.0",
		TxHash:          "tx1",
		Network:         "testnet",
	}

	snap1 := FromMap(map[string]string{"key": "value1"})
	snap2 := FromMap(map[string]string{"key": "value2"})

	hash1, _, err := store.SaveWithDedup(meta, snap1)
	if err != nil {
		t.Fatalf("first SaveWithDedup failed: %v", err)
	}

	hash2, _, err := store.SaveWithDedup(meta, snap2)
	if err != nil {
		t.Fatalf("second SaveWithDedup failed: %v", err)
	}

	if hash1 == hash2 {
		t.Errorf("different snapshots should have different hashes")
	}

	// Verify two files exist
	snapshots, _ := store.GetStoredHashes()
	if len(snapshots) != 2 {
		t.Errorf("expected 2 stored snapshots, got %d", len(snapshots))
	}
}

func TestDedupStore_LoadByHash(t *testing.T) {
	dir := t.TempDir()
	store := NewDedupStore(dir)

	meta := &ReplayMetadata{
		GlassboxVersion: "v1.0.0",
		TxHash:          "tx1",
		Network:         "testnet",
		EnvelopeXdr:     "env",
		ResultMetaXdr:   "result",
	}
	snap := FromMap(map[string]string{"key": "value"})

	hash, _, err := store.SaveWithDedup(meta, snap)
	if err != nil {
		t.Fatalf("SaveWithDedup failed: %v", err)
	}

	// Load by hash
	loaded, err := store.LoadByHash(hash)
	if err != nil {
		t.Fatalf("LoadByHash failed: %v", err)
	}

	if loaded.Metadata.TxHash != "tx1" {
		t.Errorf("expected TxHash tx1, got %s", loaded.Metadata.TxHash)
	}
	if loaded.Snapshot.ToMap()["key"] != "value" {
		t.Error("snapshot data mismatch")
	}
}

func TestDedupStore_GetStoredHashes(t *testing.T) {
	dir := t.TempDir()
	store := NewDedupStore(dir)

	meta := &ReplayMetadata{
		GlassboxVersion: "v1.0.0",
		TxHash:          "tx",
		Network:         "testnet",
	}

	hashes := make([]string, 3)
	for i := 0; i < 3; i++ {
		snap := FromMap(map[string]string{"k": string(rune(i))})
		h, _, _ := store.SaveWithDedup(meta, snap)
		hashes[i] = h
	}

	stored, err := store.GetStoredHashes()
	if err != nil {
		t.Fatalf("GetStoredHashes failed: %v", err)
	}

	if len(stored) != 3 {
		t.Errorf("expected 3 hashes, got %d", len(stored))
	}

	// Verify all saved hashes are present
	for _, h := range hashes {
		found := false
		for _, s := range stored {
			if s == h {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("hash %s not found in stored hashes", h)
		}
	}
}

func TestDedupStore_DeleteByHash(t *testing.T) {
	dir := t.TempDir()
	store := NewDedupStore(dir)

	meta := &ReplayMetadata{
		GlassboxVersion: "v1.0.0",
		TxHash:          "tx1",
		Network:         "testnet",
	}
	snap := FromMap(map[string]string{"key": "value"})

	hash, _, _ := store.SaveWithDedup(meta, snap)

	// Verify it exists
	before, _ := store.GetStoredHashes()
	if len(before) != 1 {
		t.Fatalf("expected 1 stored snapshot before delete")
	}

	// Delete it
	err := store.DeleteByHash(hash)
	if err != nil {
		t.Fatalf("DeleteByHash failed: %v", err)
	}

	// Verify it's gone
	after, _ := store.GetStoredHashes()
	if len(after) != 0 {
		t.Errorf("expected 0 stored snapshots after delete, got %d", len(after))
	}
}

func TestDedupStore_QueryDedup(t *testing.T) {
	dir := t.TempDir()
	store := NewDedupStore(dir)

	meta := &ReplayMetadata{
		GlassboxVersion: "v1.0.0",
		TxHash:          "tx",
		Network:         "testnet",
	}

	// Save 3 different snapshots
	for i := 0; i < 3; i++ {
		snap := FromMap(map[string]string{"k": string(rune(i))})
		store.SaveWithDedup(meta, snap)
	}

	count, err := store.QueryDedup()
	if err != nil {
		t.Fatalf("QueryDedup failed: %v", err)
	}

	if count != 3 {
		t.Errorf("expected 3 unique snapshots, got %d", count)
	}
}

func TestDedupStore_IsDeduplicatedWith(t *testing.T) {
	store := NewDedupStore(t.TempDir())

	hash1 := "aaaa"
	hash2 := "aaaa"
	hash3 := "bbbb"

	if !store.IsDeduplicatedWith(hash1, hash2) {
		t.Error("identical hashes should be deduplicated")
	}

	if store.IsDeduplicatedWith(hash1, hash3) {
		t.Error("different hashes should not be deduplicated")
	}
}

func TestDedupStore_GetDiskUsage(t *testing.T) {
	dir := t.TempDir()
	store := NewDedupStore(dir)

	meta := &ReplayMetadata{
		GlassboxVersion: "v1.0.0",
		TxHash:          "tx",
		Network:         "testnet",
	}
	snap := FromMap(map[string]string{"key": "value"})

	store.SaveWithDedup(meta, snap)

	usage, err := store.GetDiskUsage()
	if err != nil {
		t.Fatalf("GetDiskUsage failed: %v", err)
	}

	if usage <= 0 {
		t.Errorf("expected positive disk usage, got %d", usage)
	}
}

// --- Recovery Tests ---

func TestDedupStore_RecoverFromCorruption(t *testing.T) {
	dir := t.TempDir()
	store := NewDedupStore(dir)

	meta := &ReplayMetadata{
		GlassboxVersion: "v1.0.0",
		TxHash:          "tx",
		Network:         "testnet",
	}

	// Save valid snapshots
	h1, _, _ := store.SaveWithDedup(meta, FromMap(map[string]string{"k": "v1"}))
	h2, _, _ := store.SaveWithDedup(meta, FromMap(map[string]string{"k": "v2"}))

	// Create corrupted file
	snapshotDir := filepath.Join(dir, "snapshots")
	badFile := filepath.Join(snapshotDir, "bad_corrupted.snap.json")
	os.WriteFile(badFile, []byte("{invalid json}"), 0644)

	// Recover
	recovered, err := store.RecoverFromCorruption()
	if err != nil {
		t.Fatalf("RecoverFromCorruption failed: %v", err)
	}

	if recovered != 2 {
		t.Errorf("expected 2 recovered snapshots, got %d", recovered)
	}

	// Verify valid snapshots still accessible
	loaded1, _ := store.LoadByHash(h1)
	loaded2, _ := store.LoadByHash(h2)

	if loaded1 == nil || loaded2 == nil {
		t.Error("failed to recover valid snapshots")
	}

	// Verify index only contains valid hashes
	indexPath := filepath.Join(snapshotDir, "index.json")
	data, _ := os.ReadFile(indexPath)
	var index DedupIndex
	json.Unmarshal(data, &index)
	if len(index.Hashes) != 2 {
		t.Errorf("expected 2 hashes in index after recovery, got %d", len(index.Hashes))
	}
}

// --- Index Tests ---

func TestDedupStore_UpdateIndex(t *testing.T) {
	dir := t.TempDir()
	store := NewDedupStore(dir)

	meta := &ReplayMetadata{
		GlassboxVersion: "v1.0.0",
		TxHash:          "tx",
		Network:         "testnet",
	}

	// Save snapshot
	hash, _, _ := store.SaveWithDedup(meta, FromMap(map[string]string{"k": "v"}))

	// Verify index was created
	indexPath := filepath.Join(dir, "snapshots", "index.json")
	data, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatalf("index file not created: %v", err)
	}

	var index DedupIndex
	if err := json.Unmarshal(data, &index); err != nil {
		t.Fatalf("failed to parse index: %v", err)
	}

	found := false
	for _, h := range index.Hashes {
		if h == hash {
			found = true
			break
		}
	}
	if !found {
		t.Error("hash not found in index")
	}
}

// --- Integration Tests ---

func TestDedupStore_ComplexScenario(t *testing.T) {
	dir := t.TempDir()
	store := NewDedupStore(dir)

	// Simulate multiple transactions with overlapping snapshots
	scenarios := []struct {
		txHash string
		data   map[string]string
	}{
		{"tx1", map[string]string{"state": "initial"}},
		{"tx2", map[string]string{"state": "initial"}},          // duplicate of tx1
		{"tx3", map[string]string{"state": "modified"}},         // different
		{"tx4", map[string]string{"state": "initial"}},          // duplicate of tx1
		{"tx5", map[string]string{"state": "modified"}},         // duplicate of tx3
		{"tx6", map[string]string{"state": "completely_new"}},   // unique
	}

	hashes := make(map[string]string)
	for _, scenario := range scenarios {
		meta := &ReplayMetadata{
			GlassboxVersion: "v1.0.0",
			TxHash:          scenario.txHash,
			Network:         "testnet",
		}
		h, _, _ := store.SaveWithDedup(meta, FromMap(scenario.data))
		hashes[scenario.txHash] = h
	}

	// Verify deduplication
	unique, _ := store.QueryDedup()
	if unique != 3 {
		t.Errorf("expected 3 unique snapshots, got %d", unique)
	}

	// Verify mapping
	if hashes["tx1"] != hashes["tx2"] || hashes["tx1"] != hashes["tx4"] {
		t.Error("identical snapshots should have same hash")
	}

	if hashes["tx3"] != hashes["tx5"] {
		t.Error("identical snapshots should have same hash")
	}

	if hashes["tx1"] == hashes["tx3"] || hashes["tx1"] == hashes["tx6"] || hashes["tx3"] == hashes["tx6"] {
		t.Error("different snapshots should have different hashes")
	}
}

func TestDedupStore_NilMetadata(t *testing.T) {
	dir := t.TempDir()
	store := NewDedupStore(dir)

	_, _, err := store.SaveWithDedup(nil, FromMap(nil))
	if err == nil {
		t.Error("expected error for nil metadata")
	}
}

func TestDedupStore_EmptyHashErrorHandling(t *testing.T) {
	store := NewDedupStore(t.TempDir())

	_, err := store.LoadByHash("")
	if err == nil {
		t.Error("expected error for empty hash")
	}

	err = store.DeleteByHash("")
	if err == nil {
		t.Error("expected error for empty hash")
	}
}

func TestDedupStore_InvalidHashPath(t *testing.T) {
	store := NewDedupStore(t.TempDir())

	_, err := store.LoadByHash("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent hash")
	}
}
