// Copyright 2026 Glassbox Users
// SPDX-License-Identifier: Apache-2.0

package compare

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"os"
)

const (
	wasmMagic = "\x00asm"
)

// WASMSection represents a single section parsed from a WASM binary.
type WASMSection struct {
	// ID is the WASM section type byte (0–12 for standard sections).
	ID byte
	// Name is the human-readable section type name.
	Name string
	// Size is the byte-length of the section payload.
	Size uint32
}

// WASMInfo holds metadata extracted from a WASM binary.
type WASMInfo struct {
	// Hash is the hex-encoded SHA-256 digest of the entire binary.
	Hash string
	// Size is the total byte length of the binary.
	Size int
	// IsValidWASM is true when the binary starts with the WASM magic bytes.
	IsValidWASM bool
	// SectionCount is the number of sections parsed from the binary.
	SectionCount int
	// Sections is the ordered list of section descriptors.
	Sections []WASMSection
}

// WASMDiffResult holds the output of comparing two WASM binaries.
type WASMDiffResult struct {
	// Local is the metadata for the first (local) binary.
	Local WASMInfo
	// Remote is the metadata for the second (remote/on-chain) binary.
	Remote WASMInfo
	// HashMatch is true when both binaries are bit-for-bit identical.
	HashMatch bool
	// SizeMatch is true when both binaries have the same total byte count.
	SizeMatch bool
	// SectionMatch is true when both binaries contain the same number of sections.
	SectionMatch bool
	// HasDivergence is true when the binaries differ in any way.
	HasDivergence bool
	// Summary is a human-readable one-line result description.
	Summary string
}

// sectionNames maps the standard WASM section type IDs to readable names.
var sectionNames = map[byte]string{
	0:  "Custom",
	1:  "Type",
	2:  "Import",
	3:  "Function",
	4:  "Table",
	5:  "Memory",
	6:  "Global",
	7:  "Export",
	8:  "Start",
	9:  "Element",
	10: "Code",
	11: "Data",
	12: "DataCount",
}

// InspectWASM analyses a WASM binary in memory and returns its metadata.
func InspectWASM(data []byte) WASMInfo {
	info := WASMInfo{Size: len(data)}

	sum := sha256.Sum256(data)
	info.Hash = hex.EncodeToString(sum[:])

	if len(data) < 8 || string(data[:4]) != wasmMagic {
		return info
	}
	info.IsValidWASM = true
	info.Sections = parseSections(data[8:])
	info.SectionCount = len(info.Sections)

	return info
}

// InspectWASMFile reads a WASM binary from disk and returns its metadata.
func InspectWASMFile(path string) (WASMInfo, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return WASMInfo{}, fmt.Errorf("failed to read WASM file %s: %w", path, err)
	}
	return InspectWASM(data), nil
}

// DiffWASM compares two in-memory WASM binaries and returns a structured result.
func DiffWASM(local, remote []byte) *WASMDiffResult {
	r := &WASMDiffResult{
		Local:  InspectWASM(local),
		Remote: InspectWASM(remote),
	}

	r.HashMatch = r.Local.Hash == r.Remote.Hash
	r.SizeMatch = r.Local.Size == r.Remote.Size
	r.SectionMatch = r.Local.SectionCount == r.Remote.SectionCount
	r.HasDivergence = !r.HashMatch

	switch {
	case r.HashMatch:
		r.Summary = "Binaries are identical (SHA-256 match)"
	case r.SectionMatch:
		r.Summary = fmt.Sprintf(
			"Binaries differ — same section count (%d) but different content",
			r.Local.SectionCount,
		)
	default:
		r.Summary = fmt.Sprintf(
			"Binaries differ — local has %d section(s), remote has %d section(s)",
			r.Local.SectionCount, r.Remote.SectionCount,
		)
	}

	return r
}

// DiffWASMFiles reads two WASM files from disk and returns a structured diff.
func DiffWASMFiles(localPath, remotePath string) (*WASMDiffResult, error) {
	localData, err := os.ReadFile(localPath)
	if err != nil {
		return nil, fmt.Errorf("cannot read local WASM %s: %w", localPath, err)
	}
	remoteData, err := os.ReadFile(remotePath)
	if err != nil {
		return nil, fmt.Errorf("cannot read remote WASM %s: %w", remotePath, err)
	}
	return DiffWASM(localData, remoteData), nil
}

// parseSections decodes WASM section headers from the payload that follows the
// 8-byte WASM file header (magic + version).
func parseSections(payload []byte) []WASMSection {
	var sections []WASMSection
	offset := 0

	for offset < len(payload) {
		// Section ID byte
		if offset >= len(payload) {
			break
		}
		sectionID := payload[offset]
		offset++

		// LEB128-encoded payload size
		size, n := binary.Uvarint(payload[offset:])
		if n <= 0 {
			break
		}
		offset += n

		name, ok := sectionNames[sectionID]
		if !ok {
			name = fmt.Sprintf("Unknown(%d)", sectionID)
		}

		sections = append(sections, WASMSection{
			ID:   sectionID,
			Name: name,
			Size: uint32(size),
		})

		// Advance past the section payload.
		offset += int(size)
		if offset > len(payload) {
			break
		}
	}

	return sections
}
