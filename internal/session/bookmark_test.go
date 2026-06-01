// Copyright 2026 Glassbox Users
// SPDX-License-Identifier: Apache-2.0

package session

import (
	"context"
	"testing"
	"time"
)

func TestStore_LoadByName(t *testing.T) {
	home := t.TempDir()
	t.Setenv("USERPROFILE", home)
	t.Setenv("HOME", home)

	store, err := NewStore()
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	ctx := context.Background()
	data := &Data{
		ID:            "session-1",
		Name:          "payroll-bug",
		CreatedAt:     time.Now(),
		LastAccessAt:  time.Now(),
		Status:        "saved",
		Network:       "testnet",
		HorizonURL:    "https://example.test",
		TxHash:        "txhash",
		SchemaVersion: SchemaVersion,
	}
	if err := store.Save(ctx, data); err != nil {
		t.Fatal(err)
	}

	loaded, err := store.LoadByName(ctx, "payroll-bug")
	if err != nil {
		t.Fatal(err)
	}
	if loaded.ID != data.ID {
		t.Fatalf("expected %s, got %s", data.ID, loaded.ID)
	}
	if loaded.Name != "payroll-bug" {
		t.Fatalf("expected bookmark name to round trip, got %q", loaded.Name)
	}
}
