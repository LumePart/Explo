package playlist

import (
	"testing"
	"time"
)

// BUG-31: the refresh gate used LastFetched captured at registration,
// so once a weekly playlist was past its interval it refreshed on
// every daily tick. The gate must evaluate the freshly loaded state.
func TestDueForRefresh(t *testing.T) {
	now := time.Now()
	weekAgo := now.Add(-7 * 24 * time.Hour)
	sixDaysAgo := now.Add(-6 * 24 * time.Hour)

	tests := []struct {
		name        string
		lastFetched time.Time
		days        int
		want        bool
	}{
		{"weekly, fetched a week ago: due", weekAgo, 7, true},
		{"weekly, fetched six days ago: not due", sixDaysAgo, 7, false},
		{"weekly, fetched just now: not due", now, 7, false},
		{"daily, fetched yesterday: due", now.Add(-24 * time.Hour), 1, true},
		{"zero cadence refreshes every tick", sixDaysAgo, 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := dueForRefresh(tt.lastFetched, tt.days, now); got != tt.want {
				t.Errorf("dueForRefresh(%v, %d) = %v, want %v", tt.lastFetched, tt.days, got, tt.want)
			}
		})
	}
}
