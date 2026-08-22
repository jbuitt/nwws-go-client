package nwwsclient

import (
	"testing"
	"time"
)

func TestNextBackoff(t *testing.T) {
	cases := []struct {
		attempt int
		want    time.Duration
	}{
		{0, 1 * time.Second},
		{1, 2 * time.Second},
		{2, 4 * time.Second},
		{3, 8 * time.Second},
		{4, 16 * time.Second},
		{5, 32 * time.Second},
		{6, 60 * time.Second},
		{7, 60 * time.Second},
		{20, 60 * time.Second},
	}
	for _, c := range cases {
		got := nextBackoff(c.attempt)
		if got != c.want {
			t.Errorf("nextBackoff(%d) = %v, want %v", c.attempt, got, c.want)
		}
	}
}

func TestMucJID(t *testing.T) {
	got := mucJID("nwws-go-client-abc12")
	want := "nwws@conference.nwws-oi.weather.gov/nwws-go-client-abc12"
	if got != want {
		t.Errorf("mucJID(...) = %q, want %q", got, want)
	}
}
