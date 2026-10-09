package nas

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// Limits that turn a reading into an alert.
const (
	hddWarnTemp     = 53 // °C, hard disks
	hddCriticalTemp = 60 // °C, above the safe limit most makers give
	nvmeWarnTemp    = 72 // °C
	cpuWarnTemp     = 85 // °C
	fullPercent     = 95 // % of a filesystem in use
)

// How soon the same alert may be sent again while it keeps firing. A changed text is always sent at once.
const (
	repeatTemp     = 30 * time.Minute
	repeatCritical = 10 * time.Minute
	repeatHealth   = 6 * time.Hour
	repeatSectors  = 24 * time.Hour
	repeatFull     = 12 * time.Hour
)

// Alert is something that needs attention. Key identifies the condition across runs and Text says what is
// wrong; the same Key with the same Text is the same alert.
type Alert struct {
	Key    string
	Text   string
	Repeat time.Duration
	// Urgent alerts (heat, a failing drive) are worth waking someone for; the others wait for quiet hours to end.
	Urgent bool
}

// Alerts lists what is wrong with the machine right now: hot drives or processor, SMART failures and bad
// sectors, and filesystems that are nearly full. An empty list means all is well.
func (h *Host) Alerts(ctx context.Context) ([]Alert, error) {
	var alerts []Alert
	drives, err := h.drives(ctx)
	for _, d := range drives {
		alerts = append(alerts, d.alerts()...)
	}
	if t, ok := h.cpuTemp(); ok && t >= cpuWarnTemp {
		alerts = append(alerts, Alert{
			Key:    "temp:cpu",
			Text:   fmt.Sprintf("🔥 CPU: %d°C (ngưỡng %d°C)", t, cpuWarnTemp),
			Repeat: repeatTemp,
			Urgent: true,
		})
	}
	for _, m := range h.usages() {
		if m.percent() >= fullPercent {
			alerts = append(alerts, Alert{
				Key:    "full:" + m.path,
				Text:   fmt.Sprintf("💾 %s đầy %d%% (%s/%s)", m.path, m.percent(), humanBytes(m.used), humanBytes(m.total)),
				Repeat: repeatFull,
			})
		}
	}
	return alerts, err
}

// alerts turns one drive's readings into alerts.
func (r diskReport) alerts() []Alert {
	title := strings.TrimSpace(r.name + " " + r.size + " " + r.model)
	var out []Alert
	if r.smart.hasTemp {
		if a, ok := tempAlert(r.name, title, r.smart.temp); ok {
			out = append(out, a)
		}
	}
	if r.smart.health != "" && !strings.EqualFold(r.smart.health, "PASSED") {
		out = append(out, Alert{
			Key:    "smart:" + r.name,
			Text:   "🚨 " + title + ": SMART báo " + r.smart.health,
			Repeat: repeatHealth,
			Urgent: true,
		})
	}
	if s := r.smart; s.pending > 0 || s.reallocated > 0 || s.uncorrectable > 0 {
		// The counts are in the text, so a growing number is a new alert while a stable one repeats daily.
		out = append(out, Alert{
			Key:    "sectors:" + r.name,
			Text:   "⚠️ " + title + ": " + strings.Join(sectorProblems(s), ", "),
			Repeat: repeatSectors,
		})
	}
	return out
}

// sectorProblems is problems() without the health verdict, which has an alert of its own.
func sectorProblems(s smartInfo) []string {
	s.health = ""
	return s.problems()
}

// tempAlert turns a temperature into an alert when it is over the limit for that kind of drive.
func tempAlert(name, title string, temp int) (Alert, bool) {
	hot := func(text string, repeat time.Duration) (Alert, bool) {
		return Alert{Key: "temp:" + name, Text: text, Repeat: repeat, Urgent: true}, true
	}
	switch nvme := strings.HasPrefix(name, "nvme"); {
	case !nvme && temp >= hddCriticalTemp:
		return hot(fmt.Sprintf("🚨 %s: %d°C — vượt %d°C, quá nóng!", title, temp, hddCriticalTemp), repeatCritical)
	case !nvme && temp >= hddWarnTemp:
		return hot(fmt.Sprintf("🔥 %s: %d°C (ngưỡng %d°C)", title, temp, hddWarnTemp), repeatTemp)
	case nvme && temp >= nvmeWarnTemp:
		return hot(fmt.Sprintf("🔥 %s: %d°C (ngưỡng %d°C)", title, temp, nvmeWarnTemp), repeatTemp)
	}
	return Alert{}, false
}
