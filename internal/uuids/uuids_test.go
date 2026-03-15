// ABOUTME: Tests for the shared UUID slice utilities: ContainsUUID and RemoveUUID.
// ABOUTME: Covers empty slices, single elements, and multi-element cases.
package uuids_test

import (
	"testing"

	"github.com/gofrs/uuid/v5"

	"github.com/perigrin/git-chain/internal/uuids"
)

func TestContainsUUID_Empty(t *testing.T) {
	id, _ := uuid.NewV7()
	if uuids.ContainsUUID(nil, id) {
		t.Fatal("expected ContainsUUID on nil slice to return false")
	}
	if uuids.ContainsUUID([]uuid.UUID{}, id) {
		t.Fatal("expected ContainsUUID on empty slice to return false")
	}
}

func TestContainsUUID_Found(t *testing.T) {
	gen := uuid.NewGen()
	id1, _ := gen.NewV7()
	id2, _ := gen.NewV7()
	id3, _ := gen.NewV7()

	slice := []uuid.UUID{id1, id2, id3}
	if !uuids.ContainsUUID(slice, id2) {
		t.Fatal("expected ContainsUUID to find id2 in slice")
	}
}

func TestContainsUUID_NotFound(t *testing.T) {
	gen := uuid.NewGen()
	id1, _ := gen.NewV7()
	id2, _ := gen.NewV7()
	other, _ := gen.NewV7()

	slice := []uuid.UUID{id1, id2}
	if uuids.ContainsUUID(slice, other) {
		t.Fatal("expected ContainsUUID to return false for absent UUID")
	}
}

func TestRemoveUUID_Empty(t *testing.T) {
	id, _ := uuid.NewV7()
	result := uuids.RemoveUUID(nil, id)
	if len(result) != 0 {
		t.Fatalf("expected empty slice, got len %d", len(result))
	}
	result2 := uuids.RemoveUUID([]uuid.UUID{}, id)
	if len(result2) != 0 {
		t.Fatalf("expected empty slice, got len %d", len(result2))
	}
}

func TestRemoveUUID_RemovesTarget(t *testing.T) {
	gen := uuid.NewGen()
	id1, _ := gen.NewV7()
	id2, _ := gen.NewV7()
	id3, _ := gen.NewV7()

	slice := []uuid.UUID{id1, id2, id3}
	result := uuids.RemoveUUID(slice, id2)

	if len(result) != 2 {
		t.Fatalf("expected slice of length 2, got %d", len(result))
	}
	for _, u := range result {
		if u == id2 {
			t.Fatal("expected id2 to be removed from slice")
		}
	}
}

func TestRemoveUUID_RemovesAllOccurrences(t *testing.T) {
	gen := uuid.NewGen()
	id1, _ := gen.NewV7()
	id2, _ := gen.NewV7()

	slice := []uuid.UUID{id1, id2, id1, id2}
	result := uuids.RemoveUUID(slice, id1)

	if len(result) != 2 {
		t.Fatalf("expected 2 elements after removing both id1 occurrences, got %d", len(result))
	}
	for _, u := range result {
		if u == id1 {
			t.Fatal("expected all occurrences of id1 to be removed")
		}
	}
}

func TestRemoveUUID_NotPresent(t *testing.T) {
	gen := uuid.NewGen()
	id1, _ := gen.NewV7()
	id2, _ := gen.NewV7()
	other, _ := gen.NewV7()

	slice := []uuid.UUID{id1, id2}
	result := uuids.RemoveUUID(slice, other)

	if len(result) != 2 {
		t.Fatalf("expected unchanged length 2 when removing absent UUID, got %d", len(result))
	}
}
