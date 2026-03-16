// ABOUTME: Tests for the multi-strategy recovery manager.
// ABOUTME: Covers failure diagnosis, recovery context creation, and strategy coverage.

package updater

import (
	"errors"
	"testing"
)

func TestDiagnoseFailure(t *testing.T) {
	rm := NewRecoveryManager()

	tests := []struct {
		name     string
		err      error
		expected RecoveryScenario
	}{
		{
			name:     "permission denied",
			err:      errors.New("permission denied"),
			expected: ScenarioPermissionDenied,
		},
		{
			name:     "access denied",
			err:      errors.New("access denied to directory"),
			expected: ScenarioPermissionDenied,
		},
		{
			name:     "disk full",
			err:      errors.New("no space left on device"),
			expected: ScenarioFileSystemFull,
		},
		{
			name:     "disk full - file too large",
			err:      errors.New("file too large"),
			expected: ScenarioFileSystemFull,
		},
		{
			name:     "checksum mismatch",
			err:      errors.New("checksum verification failed"),
			expected: ScenarioChecksumMismatch,
		},
		{
			name:     "hash mismatch",
			err:      errors.New("hash does not match"),
			expected: ScenarioChecksumMismatch,
		},
		{
			name:     "network failure",
			err:      errors.New("network connection refused"),
			expected: ScenarioNetworkFailure,
		},
		{
			name:     "connection timeout",
			err:      errors.New("connection timeout"),
			expected: ScenarioNetworkFailure,
		},
		{
			name:     "dns failure",
			err:      errors.New("dns lookup failed"),
			expected: ScenarioNetworkFailure,
		},
		{
			name:     "incompatible binary",
			err:      errors.New("exec format error"),
			expected: ScenarioIncompatibleBinary,
		},
		{
			name:     "cannot execute",
			err:      errors.New("cannot execute binary"),
			expected: ScenarioIncompatibleBinary,
		},
		{
			name:     "partial update",
			err:      errors.New("interrupted download"),
			expected: ScenarioPartialUpdate,
		},
		{
			name:     "incomplete update",
			err:      errors.New("incomplete file received"),
			expected: ScenarioPartialUpdate,
		},
		{
			name:     "unknown error",
			err:      errors.New("some unknown error occurred"),
			expected: ScenarioUnknownFailure,
		},
		{
			name:     "nil error returns unknown",
			err:      nil,
			expected: ScenarioUnknownFailure,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			scenario := rm.DiagnoseFailure("", tc.err)
			if scenario != tc.expected {
				t.Errorf("DiagnoseFailure(%q) = %d, want %d", tc.err, scenario, tc.expected)
			}
		})
	}
}

func TestCreateRecoveryContext(t *testing.T) {
	rm := NewRecoveryManager()

	targetPath := "/usr/local/bin/git-zhi"
	failedVersion := "1.2.3"
	err := errors.New("permission denied")

	ctx := rm.CreateRecoveryContext(targetPath, failedVersion, err)

	if ctx == nil {
		t.Fatal("CreateRecoveryContext returned nil")
	}

	if ctx.Scenario != ScenarioPermissionDenied {
		t.Errorf("expected ScenarioPermissionDenied, got %d", ctx.Scenario)
	}

	if ctx.TargetPath != targetPath {
		t.Errorf("expected TargetPath %q, got %q", targetPath, ctx.TargetPath)
	}

	if ctx.FailedVersion != failedVersion {
		t.Errorf("expected FailedVersion %q, got %q", failedVersion, ctx.FailedVersion)
	}

	if ctx.MaxAttempts != 3 {
		t.Errorf("expected MaxAttempts 3, got %d", ctx.MaxAttempts)
	}

	if ctx.AttemptCount != 0 {
		t.Errorf("expected AttemptCount 0, got %d", ctx.AttemptCount)
	}

	if ctx.ErrorDetails != err {
		t.Errorf("expected ErrorDetails to be the original error")
	}
}

func TestRecoveryStrategiesExistForAllScenarios(t *testing.T) {
	rm := NewRecoveryManager()

	scenarios := []RecoveryScenario{
		ScenarioCorruptedBinary,
		ScenarioPartialUpdate,
		ScenarioPermissionDenied,
		ScenarioFileSystemFull,
		ScenarioNetworkFailure,
		ScenarioChecksumMismatch,
		ScenarioIncompatibleBinary,
		ScenarioUnknownFailure,
	}

	for _, scenario := range scenarios {
		strategies := rm.getRecoveryStrategies(scenario)
		if len(strategies) == 0 {
			t.Errorf("scenario %d has no recovery strategies", scenario)
		}
	}
}
