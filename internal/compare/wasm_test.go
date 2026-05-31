// Copyright 2026 Glassbox Users
// SPDX-License-Identifier: Apache-2.0

package compare

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ─── WASM test helpers ────────────────────────────────────────────────────────

// wasmHeader returns a minimal valid WASM header (magic + version).
var wasmHeader = []byte{0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00}

// makeWASM builds a minimal WASM binary with a single custom section.
func makeWASM(payload []byte) []byte {
	bin := make([]byte, len(wasmHeader))
	copy(bin, wasmHeader)
	if len(payload) == 0 {
		return bin
	}
	// Section ID 0 = Custom, followed by LEB128 length, then payload.
	bin = append(bin, 0x00)
	bin = appendUvarint(bin, uint64(len(payload)))
	bin = append(bin, payload...)
	return bin
}

func appendUvarint(buf []byte, v uint64) []byte {
	for v >= 0x80 {
		buf = append(buf, byte(v)|0x80)
		v >>= 7
	}
	return append(buf, byte(v))
}

// ─── InspectWASM ─────────────────────────────────────────────────────────────

func TestInspectWASM_ValidEmpty(t *testing.T) {
	data := wasmHeader
	info := InspectWASM(data)
	assert.True(t, info.IsValidWASM)
	assert.Equal(t, 0, info.SectionCount)
	assert.Equal(t, len(data), info.Size)
	assert.NotEmpty(t, info.Hash)
}

func TestInspectWASM_NotWASM(t *testing.T) {
	data := []byte("ELF binary not WASM")
	info := InspectWASM(data)
	assert.False(t, info.IsValidWASM)
	assert.Equal(t, len(data), info.Size)
	assert.NotEmpty(t, info.Hash)
}

func TestInspectWASM_EmptySlice(t *testing.T) {
	info := InspectWASM([]byte{})
	assert.False(t, info.IsValidWASM)
	assert.Equal(t, 0, info.Size)
}

func TestInspectWASM_Hash(t *testing.T) {
	data := append(wasmHeader, 0x42)
	info := InspectWASM(data)
	expected := sha256.Sum256(data)
	assert.Equal(t, hex.EncodeToString(expected[:]), info.Hash)
}

func TestInspectWASM_SingleSection(t *testing.T) {
	data := makeWASM([]byte{0x01, 0x02, 0x03})
	info := InspectWASM(data)
	assert.True(t, info.IsValidWASM)
	assert.Equal(t, 1, info.SectionCount)
	assert.Equal(t, "Custom", info.Sections[0].Name)
}

// ─── InspectWASMFile ─────────────────────────────────────────────────────────

func TestInspectWASMFile_Valid(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "contract.wasm")
	data := makeWASM([]byte{0xAA, 0xBB})
	require.NoError(t, os.WriteFile(path, data, 0644))

	info, err := InspectWASMFile(path)
	require.NoError(t, err)
	assert.True(t, info.IsValidWASM)
}

func TestInspectWASMFile_Missing(t *testing.T) {
	_, err := InspectWASMFile("/nonexistent/path/contract.wasm")
	assert.Error(t, err)
}

// ─── DiffWASM ────────────────────────────────────────────────────────────────

func TestDiffWASM_Identical(t *testing.T) {
	data := makeWASM([]byte{0x01})
	result := DiffWASM(data, data)
	assert.True(t, result.HashMatch)
	assert.True(t, result.SizeMatch)
	assert.True(t, result.SectionMatch)
	assert.False(t, result.HasDivergence)
	assert.Contains(t, result.Summary, "identical")
}

func TestDiffWASM_DifferentContent_SameSections(t *testing.T) {
	local := makeWASM([]byte{0x01, 0x02})
	remote := makeWASM([]byte{0x03, 0x04})
	result := DiffWASM(local, remote)
	assert.False(t, result.HashMatch)
	assert.True(t, result.HasDivergence)
	assert.True(t, result.SectionMatch, "both have one custom section")
	assert.Contains(t, result.Summary, "same section count")
}

func TestDiffWASM_DifferentSectionCounts(t *testing.T) {
	// local: WASM header only (0 sections)
	local := make([]byte, len(wasmHeader))
	copy(local, wasmHeader)

	// remote: WASM with one section
	remote := makeWASM([]byte{0xDE, 0xAD})

	result := DiffWASM(local, remote)
	assert.False(t, result.HashMatch)
	assert.False(t, result.SectionMatch)
	assert.True(t, result.HasDivergence)
	assert.Equal(t, 0, result.Local.SectionCount)
	assert.Equal(t, 1, result.Remote.SectionCount)
}

func TestDiffWASM_LocalNotWASM(t *testing.T) {
	local := []byte("not a wasm binary")
	remote := makeWASM([]byte{0x01})
	result := DiffWASM(local, remote)
	assert.False(t, result.Local.IsValidWASM)
	assert.True(t, result.Remote.IsValidWASM)
	assert.True(t, result.HasDivergence)
}

func TestDiffWASM_BothNotWASM(t *testing.T) {
	a := []byte("file a")
	b := []byte("file b")
	result := DiffWASM(a, b)
	assert.False(t, result.Local.IsValidWASM)
	assert.False(t, result.Remote.IsValidWASM)
	assert.True(t, result.HasDivergence)
}

// ─── DiffWASMFiles ────────────────────────────────────────────────────────────

func TestDiffWASMFiles_Identical(t *testing.T) {
	dir := t.TempDir()
	data := makeWASM([]byte{0x11, 0x22})
	p1 := filepath.Join(dir, "a.wasm")
	p2 := filepath.Join(dir, "b.wasm")
	require.NoError(t, os.WriteFile(p1, data, 0644))
	require.NoError(t, os.WriteFile(p2, data, 0644))

	result, err := DiffWASMFiles(p1, p2)
	require.NoError(t, err)
	assert.True(t, result.HashMatch)
}

func TestDiffWASMFiles_MissingLocal(t *testing.T) {
	dir := t.TempDir()
	remote := filepath.Join(dir, "remote.wasm")
	require.NoError(t, os.WriteFile(remote, makeWASM(nil), 0644))

	_, err := DiffWASMFiles("/nonexistent.wasm", remote)
	assert.Error(t, err)
}

func TestDiffWASMFiles_MissingRemote(t *testing.T) {
	dir := t.TempDir()
	local := filepath.Join(dir, "local.wasm")
	require.NoError(t, os.WriteFile(local, makeWASM(nil), 0644))

	_, err := DiffWASMFiles(local, "/nonexistent.wasm")
	assert.Error(t, err)
}

// ─── parseSections ───────────────────────────────────────────────────────────

func TestParseSections_Empty(t *testing.T) {
	sections := parseSections([]byte{})
	assert.Empty(t, sections)
}

func TestParseSections_KnownSectionID(t *testing.T) {
	// Build a minimal Type section (ID=1) with a 3-byte payload.
	payload := []byte{0x01}                            // section ID = Type
	payload = appendUvarint(payload, 3)                // size = 3
	payload = append(payload, 0xAA, 0xBB, 0xCC)       // arbitrary payload

	sections := parseSections(payload)
	require.Len(t, sections, 1)
	assert.Equal(t, byte(1), sections[0].ID)
	assert.Equal(t, "Type", sections[0].Name)
	assert.Equal(t, uint32(3), sections[0].Size)
}

func TestParseSections_UnknownSectionID(t *testing.T) {
	payload := []byte{0xFF}
	payload = appendUvarint(payload, 1)
	payload = append(payload, 0x00)

	sections := parseSections(payload)
	require.Len(t, sections, 1)
	assert.Contains(t, sections[0].Name, "Unknown")
}
