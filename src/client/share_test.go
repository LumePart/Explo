package client

import "testing"

func TestShareTitle(t *testing.T) {
	tests := []struct{ owner, name, want string }{
		{"Gabe", "Weekly-Exploration", "Gabe's Weekly-Exploration"},
		{"O'Brien", "X", "O'Brien's X"},
		{"  ", "X", "X"},
		{"", "X", "X"},
	}
	for _, tt := range tests {
		if got := ShareTitle(tt.owner, tt.name); got != tt.want {
			t.Errorf("ShareTitle(%q, %q) = %q, want %q", tt.owner, tt.name, got, tt.want)
		}
	}
}
