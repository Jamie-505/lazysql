package components

import "testing"

func TestConnectionWindowHeight(t *testing.T) {
	tests := []struct {
		name          string
		content       int
		screen        int
		wantWindowRow int
	}{
		{"short content keeps the standard height", 5, 60, connectionWindowMinHeight},
		{"tall content grows the window", 24, 60, 26},
		{"growth is capped at 95% of the screen", 40, 30, 28},
		{"small screen caps the standard height", 5, 16, 15},
		{"tiny screen still gets a row", 5, 1, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := connectionWindowHeight(tt.content, tt.screen); got != tt.wantWindowRow {
				t.Errorf("connectionWindowHeight(%d, %d) = %d, want %d", tt.content, tt.screen, got, tt.wantWindowRow)
			}
		})
	}
}
