package nas

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"
)

// Throttle remembers, in a file, which alerts were already sent, so a condition that keeps firing is not
// repeated on every run: an alert is due when it is new, when its text changed, or when its Repeat time has
// passed. An alert that stops firing is forgotten, so it is sent again if the problem comes back.
type Throttle struct {
	path string
	now  func() time.Time
}

// NewThrottle keeps its state in the file at path.
func NewThrottle(path string) *Throttle { return &Throttle{path: path, now: time.Now} }

type sentAlert struct {
	Text string    `json:"text"`
	At   time.Time `json:"at"`
}

// Select returns the alerts due now. Call commit once they were delivered: nothing is remembered before that,
// so a failed delivery is retried by the next run. Alerts for which muted returns true (nil mutes none) are
// held back: they are not due, and what was remembered about them is kept, so they are sent when the mute ends.
func (t *Throttle) Select(alerts []Alert, muted func(Alert) bool) (due []Alert, commit func() error) {
	sent := t.load()
	now := t.now()
	next := make(map[string]sentAlert, len(alerts))
	for _, a := range alerts {
		prev, known := sent[a.Key]
		if muted != nil && muted(a) {
			if known {
				next[a.Key] = prev
			}
			continue
		}
		if known && prev.Text == a.Text && now.Sub(prev.At) < a.Repeat {
			next[a.Key] = prev // sent recently: stay quiet and keep the original time
			continue
		}
		due = append(due, a)
		next[a.Key] = sentAlert{Text: a.Text, At: now}
	}
	return due, func() error { return t.save(next) }
}

// load reads the state; a missing or damaged file means nothing was sent yet.
func (t *Throttle) load() map[string]sentAlert {
	sent := map[string]sentAlert{}
	if data, err := os.ReadFile(t.path); err == nil {
		if json.Unmarshal(data, &sent) != nil {
			return map[string]sentAlert{}
		}
	}
	return sent
}

// save replaces the state file atomically.
func (t *Throttle) save(state map[string]sentAlert) error {
	data, err := json.Marshal(state)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(t.path), 0o755); err != nil {
		return err
	}
	tmp := t.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, t.path); err != nil {
		return errors.Join(err, os.Remove(tmp))
	}
	return nil
}
