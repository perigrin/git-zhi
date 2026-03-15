// ABOUTME: Shared UUID slice utilities used across graph and CLI packages.
// ABOUTME: Provides containsUUID and removeUUID helpers to avoid duplication.
package uuids

import "github.com/gofrs/uuid/v5"

// ContainsUUID reports whether slice contains the given UUID.
func ContainsUUID(slice []uuid.UUID, target uuid.UUID) bool {
	for _, u := range slice {
		if u == target {
			return true
		}
	}
	return false
}

// RemoveUUID returns a new slice with all occurrences of target removed.
func RemoveUUID(slice []uuid.UUID, target uuid.UUID) []uuid.UUID {
	result := slice[:0:0]
	for _, u := range slice {
		if u != target {
			result = append(result, u)
		}
	}
	return result
}
