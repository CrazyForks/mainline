package hooks

import "testing"

func TestGoVersionAtLeast(t *testing.T) {
	tests := []struct {
		version string
		want    bool
	}{
		{"go1.19.8", false},
		{"go1.21.13", false},
		{"go1.22.0", true},
		{"go1.22.5", true},
		{"go1.23.1", true},
		{"go version go1.22.5 linux/amd64", true},
		{"devel go1.24", true},
		{"unknown", true},
	}
	for _, tt := range tests {
		t.Run(tt.version, func(t *testing.T) {
			if got := goVersionAtLeast(tt.version, 1, 22); got != tt.want {
				t.Fatalf("goVersionAtLeast(%q, 1, 22) = %v, want %v", tt.version, got, tt.want)
			}
		})
	}
}
