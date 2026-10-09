package nas

import (
	"fmt"
	"strings"
	"time"
)

// QuietHours is a daily window, such as the night, in the machine's local time. The zero value is never active.
type QuietHours struct{ start, end int } // minutes after midnight; start == end means never

// ParseQuietHours reads "HH:MM-HH:MM"; a window may cross midnight ("22:00-06:00"). Empty means no quiet hours.
func ParseQuietHours(s string) (QuietHours, error) {
	if strings.TrimSpace(s) == "" {
		return QuietHours{}, nil
	}
	from, to, ok := strings.Cut(strings.TrimSpace(s), "-")
	if !ok {
		return QuietHours{}, fmt.Errorf("quiet hours %q: want HH:MM-HH:MM", s)
	}
	start, err := clockMinutes(from)
	if err != nil {
		return QuietHours{}, err
	}
	end, err := clockMinutes(to)
	if err != nil {
		return QuietHours{}, err
	}
	return QuietHours{start, end}, nil
}

func clockMinutes(s string) (int, error) {
	t, err := time.Parse("15:04", strings.TrimSpace(s))
	if err != nil {
		return 0, fmt.Errorf("quiet hours: %q is not HH:MM", s)
	}
	return t.Hour()*60 + t.Minute(), nil
}

// Contains reports whether t falls inside the window (start inclusive, end exclusive).
func (q QuietHours) Contains(t time.Time) bool {
	m := t.Hour()*60 + t.Minute()
	switch {
	case q.start == q.end:
		return false
	case q.start < q.end:
		return m >= q.start && m < q.end
	}
	return m >= q.start || m < q.end // crosses midnight
}
