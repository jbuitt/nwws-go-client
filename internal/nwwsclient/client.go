package nwwsclient

import "time"

const mucRoom = "nwws@conference.nwws-oi.weather.gov"

// mucJID returns the full MUC occupant JID for the given resource/nickname.
func mucJID(resource string) string {
	return mucRoom + "/" + resource
}

const (
	backoffBase = 1 * time.Second
	backoffCap  = 60 * time.Second
)

// nextBackoff returns the delay before reconnect attempt number attempt
// (0-indexed): 1s, 2s, 4s, ... capped at 60s.
func nextBackoff(attempt int) time.Duration {
	d := backoffBase
	for i := 0; i < attempt; i++ {
		d *= 2
		if d >= backoffCap {
			return backoffCap
		}
	}
	return d
}
