package quality

import "time"

// Due reports whether a date has reached its interval. A zero date is
// always due (first run).
func Due(last, now time.Time, interval time.Duration) bool {
	if last.IsZero() {
		return true
	}
	return !now.Before(last.Add(interval))
}
