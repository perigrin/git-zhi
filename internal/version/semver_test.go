// ABOUTME: Tests for semantic version parsing and comparison utilities.
// ABOUTME: Covers ParseVersion, Compare, IsNewer/IsOlder/IsEqual, and CompareVersionStrings.

package version

import (
	"testing"
)

func TestParseVersion_Valid(t *testing.T) {
	tests := []struct {
		input string
		major int
		minor int
		patch int
	}{
		{"1.2.3", 1, 2, 3},
		{"v1.2.3", 1, 2, 3},
		{"0.1.0", 0, 1, 0},
		{"10.20.30", 10, 20, 30},
		{"1.0", 1, 0, 0},
		{"1", 1, 0, 0},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			sv, err := ParseVersion(tt.input)
			if err != nil {
				t.Fatalf("ParseVersion(%q) unexpected error: %v", tt.input, err)
			}
			if sv.Major != tt.major {
				t.Errorf("Major: got %d, want %d", sv.Major, tt.major)
			}
			if sv.Minor != tt.minor {
				t.Errorf("Minor: got %d, want %d", sv.Minor, tt.minor)
			}
			if sv.Patch != tt.patch {
				t.Errorf("Patch: got %d, want %d", sv.Patch, tt.patch)
			}
		})
	}
}

func TestParseVersion_Invalid(t *testing.T) {
	tests := []string{
		"",
		"not-a-version",
		"1.2.3.4.5",
		"abc.def.ghi",
	}

	for _, input := range tests {
		t.Run(input, func(t *testing.T) {
			_, err := ParseVersion(input)
			if err == nil {
				t.Errorf("ParseVersion(%q) expected error, got nil", input)
			}
		})
	}
}

func TestParseVersion_Prerelease(t *testing.T) {
	tests := []struct {
		input      string
		prerelease string
	}{
		{"1.2.3-alpha", "alpha"},
		{"1.2.3-alpha.1", "alpha.1"},
		{"1.2.3-0.3.7", "0.3.7"},
		{"v2.0.0-rc.1", "rc.1"},
		{"1.0.0-beta", "beta"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			sv, err := ParseVersion(tt.input)
			if err != nil {
				t.Fatalf("ParseVersion(%q) unexpected error: %v", tt.input, err)
			}
			if sv.Prerelease != tt.prerelease {
				t.Errorf("Prerelease: got %q, want %q", sv.Prerelease, tt.prerelease)
			}
			if !sv.IsPrerelease() {
				t.Errorf("IsPrerelease() should return true for %q", tt.input)
			}
		})
	}
}

func TestParseVersion_BuildMetadata(t *testing.T) {
	tests := []struct {
		input string
		build string
	}{
		{"1.2.3+build.1", "build.1"},
		{"1.2.3+20230101", "20230101"},
		{"1.2.3-alpha+001", "001"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			sv, err := ParseVersion(tt.input)
			if err != nil {
				t.Fatalf("ParseVersion(%q) unexpected error: %v", tt.input, err)
			}
			if sv.Build != tt.build {
				t.Errorf("Build: got %q, want %q", sv.Build, tt.build)
			}
		})
	}
}

func TestCompare(t *testing.T) {
	tests := []struct {
		a    string
		b    string
		want int
	}{
		{"1.2.3", "1.2.3", 0},
		{"2.0.0", "1.0.0", 1},
		{"1.0.0", "2.0.0", -1},
		{"1.3.0", "1.2.0", 1},
		{"1.2.0", "1.3.0", -1},
		{"1.2.4", "1.2.3", 1},
		{"1.2.3", "1.2.4", -1},
		// Prerelease is less than release
		{"1.0.0", "1.0.0-alpha", 1},
		{"1.0.0-alpha", "1.0.0", -1},
		// Prerelease ordering
		{"1.0.0-alpha.1", "1.0.0-alpha", 1},
		{"1.0.0-alpha", "1.0.0-alpha.1", -1},
		{"1.0.0-beta", "1.0.0-alpha", 1},
		{"1.0.0-alpha", "1.0.0-beta", -1},
		// Numeric prerelease parts compare numerically
		{"1.0.0-alpha.10", "1.0.0-alpha.2", 1},
		{"1.0.0-alpha.2", "1.0.0-alpha.10", -1},
	}

	for _, tt := range tests {
		t.Run(tt.a+"_vs_"+tt.b, func(t *testing.T) {
			a, err := ParseVersion(tt.a)
			if err != nil {
				t.Fatalf("ParseVersion(%q): %v", tt.a, err)
			}
			b, err := ParseVersion(tt.b)
			if err != nil {
				t.Fatalf("ParseVersion(%q): %v", tt.b, err)
			}
			got := a.Compare(b)
			if got != tt.want {
				t.Errorf("Compare(%q, %q) = %d, want %d", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

func TestIsNewer(t *testing.T) {
	a, _ := ParseVersion("2.0.0")
	b, _ := ParseVersion("1.0.0")
	c, _ := ParseVersion("2.0.0")

	if !a.IsNewer(b) {
		t.Error("2.0.0 should be newer than 1.0.0")
	}
	if b.IsNewer(a) {
		t.Error("1.0.0 should not be newer than 2.0.0")
	}
	if a.IsNewer(c) {
		t.Error("2.0.0 should not be newer than 2.0.0")
	}
}

func TestIsOlder(t *testing.T) {
	a, _ := ParseVersion("1.0.0")
	b, _ := ParseVersion("2.0.0")
	c, _ := ParseVersion("1.0.0")

	if !a.IsOlder(b) {
		t.Error("1.0.0 should be older than 2.0.0")
	}
	if b.IsOlder(a) {
		t.Error("2.0.0 should not be older than 1.0.0")
	}
	if a.IsOlder(c) {
		t.Error("1.0.0 should not be older than 1.0.0")
	}
}

func TestIsEqual(t *testing.T) {
	a, _ := ParseVersion("1.2.3")
	b, _ := ParseVersion("v1.2.3")
	c, _ := ParseVersion("2.0.0")

	if !a.IsEqual(b) {
		t.Error("1.2.3 and v1.2.3 should be equal")
	}
	if a.IsEqual(c) {
		t.Error("1.2.3 and 2.0.0 should not be equal")
	}
}

func TestCompareVersionStrings(t *testing.T) {
	tests := []struct {
		a    string
		b    string
		want int
	}{
		{"1.2.3", "1.2.3", 0},
		{"2.0.0", "1.9.9", 1},
		{"1.0.0", "1.0.1", -1},
	}

	for _, tt := range tests {
		t.Run(tt.a+"_vs_"+tt.b, func(t *testing.T) {
			got, err := CompareVersionStrings(tt.a, tt.b)
			if err != nil {
				t.Fatalf("CompareVersionStrings(%q, %q) unexpected error: %v", tt.a, tt.b, err)
			}
			if got != tt.want {
				t.Errorf("CompareVersionStrings(%q, %q) = %d, want %d", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

func TestCompareVersionStrings_InvalidInput(t *testing.T) {
	_, err := CompareVersionStrings("not-valid", "1.0.0")
	if err == nil {
		t.Error("expected error for invalid version string, got nil")
	}

	_, err = CompareVersionStrings("1.0.0", "not-valid")
	if err == nil {
		t.Error("expected error for invalid version string, got nil")
	}
}

func TestNormalizeVersionString(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"1.2.3", "v1.2.3"},
		{"v1.2.3", "v1.2.3"},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := NormalizeVersionString(tt.input)
			if got != tt.want {
				t.Errorf("NormalizeVersionString(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestStripVersionPrefix(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"v1.2.3", "1.2.3"},
		{"1.2.3", "1.2.3"},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := StripVersionPrefix(tt.input)
			if got != tt.want {
				t.Errorf("StripVersionPrefix(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestString(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"1.2.3", "1.2.3"},
		{"v1.2.3", "1.2.3"},
		{"1.2.3-alpha", "1.2.3-alpha"},
		{"1.2.3+build", "1.2.3+build"},
		{"1.2.3-alpha+build", "1.2.3-alpha+build"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			sv, err := ParseVersion(tt.input)
			if err != nil {
				t.Fatalf("ParseVersion(%q): %v", tt.input, err)
			}
			got := sv.String()
			if got != tt.want {
				t.Errorf("String() for %q = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
