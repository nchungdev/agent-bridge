package nas

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func alertHost(smart map[string]string, usedPercent uint64) *Host {
	r := &fakeRunner{out: map[string]string{
		"lsblk -dn -o NAME,SIZE,ROTA,MODEL": "sdb 3.6T 1 EC-7352\nsdc 3.6T 1 EC-7352\nnvme0n1 238.5G 0 WD\n",
	}}
	for dev, out := range smart {
		r.out["sudo -n smartctl -A -H /dev/"+dev] = out
	}
	h := newHost(r)
	h.sys.statfs = func(string) (uint64, uint64, error) { return 100 << 30, (100 - usedPercent) << 30, nil }
	return h
}

func keys(alerts []Alert) string {
	var k []string
	for _, a := range alerts {
		k = append(k, a.Key)
	}
	return strings.Join(k, ",")
}

func TestAlertsStayQuietWhenEverythingIsFine(t *testing.T) {
	h := alertHost(map[string]string{"sdb": smartHDD, "sdc": smartHDD, "nvme0n1": smartNVMe}, 50)
	// smartHDD is 51°C: under the 53°C limit. NVMe 59°C is under 72°C. CPU is 58°C.
	got, err := h.Alerts(context.Background())
	if err != nil || len(got) != 0 {
		t.Fatalf("alerts = %v err = %v", got, err)
	}
}

func TestAlertsFireOnHeatBadSectorsFailingSmartAndFullDisks(t *testing.T) {
	hot := strings.Replace(smartHDD, "51 (Min/Max 29/59)", "54 (Min/Max 29/59)", 1)
	critical := strings.Replace(smartHDD, "51 (Min/Max 29/59)", "61 (Min/Max 29/61)", 1)
	failing := strings.Replace(smartNVMe, "PASSED", "FAILED!", 1)
	h := alertHost(map[string]string{"sdb": hot, "sdc": critical, "nvme0n1": failing}, 96)
	got, _ := h.Alerts(context.Background())
	byKey := map[string]Alert{}
	for _, a := range got {
		byKey[a.Key] = a
	}
	if a := byKey["temp:sdb"]; !strings.Contains(a.Text, "54°C") || a.Repeat != repeatTemp {
		t.Errorf("sdb = %+v", a)
	}
	if a := byKey["temp:sdc"]; !strings.Contains(a.Text, "quá nóng") || a.Repeat != repeatCritical {
		t.Errorf("sdc = %+v", a)
	}
	if a := byKey["smart:nvme0n1"]; !strings.Contains(a.Text, "FAILED") {
		t.Errorf("nvme smart = %+v", a)
	}
	if _, ok := byKey["full:/srv/pool"]; !ok {
		t.Errorf("no full-disk alert in %s", keys(got))
	}
}

func TestBadSectorsAlertCarriesTheCountSoGrowthIsANewAlert(t *testing.T) {
	h := alertHost(map[string]string{"sdb": smartUSB, "sdc": smartHDD, "nvme0n1": smartNVMe}, 50)
	got, _ := h.Alerts(context.Background())
	if len(got) != 1 || got[0].Key != "sectors:sdb" || !strings.Contains(got[0].Text, "1808") || got[0].Repeat != repeatSectors {
		t.Fatalf("alerts = %+v", got)
	}
	if strings.Contains(got[0].Text, "SMART báo") {
		t.Fatalf("health verdict leaked into the sector alert: %q", got[0].Text)
	}
}

func TestThrottleSendsNewChangedAndOverdueAlertsOnly(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state", "alerts.json")
	now := time.Date(2026, 10, 9, 3, 0, 0, 0, time.UTC)
	th := &Throttle{path: path, now: func() time.Time { return now }}
	a := Alert{Key: "temp:sdb", Text: "sdb 54°C", Repeat: 30 * time.Minute}

	due, commit := th.Select([]Alert{a}, nil)
	if len(due) != 1 {
		t.Fatalf("first run due = %v", due)
	}
	if err := commit(); err != nil {
		t.Fatal(err)
	}

	now = now.Add(15 * time.Minute)
	if due, _ := th.Select([]Alert{a}, nil); len(due) != 0 {
		t.Fatalf("repeated after 15 min: %v", due)
	}
	changed := a
	changed.Text = "sdb 57°C"
	if due, _ := th.Select([]Alert{changed}, nil); len(due) != 1 {
		t.Fatalf("changed text not sent: %v", due)
	}
	now = now.Add(20 * time.Minute) // 35 min since the first send
	if due, _ := th.Select([]Alert{a}, nil); len(due) != 1 {
		t.Fatalf("overdue alert not repeated: %v", due)
	}
}

func TestThrottleRetriesWhenDeliveryFailedAndForgetsRecoveredAlerts(t *testing.T) {
	path := filepath.Join(t.TempDir(), "alerts.json")
	now := time.Date(2026, 10, 9, 3, 0, 0, 0, time.UTC)
	th := &Throttle{path: path, now: func() time.Time { return now }}
	a := Alert{Key: "temp:sdb", Text: "sdb 54°C", Repeat: time.Hour}

	if due, _ := th.Select([]Alert{a}, nil); len(due) != 1 { // delivery fails: commit is never called
		t.Fatal("not due")
	}
	due, commit := th.Select([]Alert{a}, nil)
	if len(due) != 1 {
		t.Fatalf("not retried after a failed delivery: %v", due)
	}
	_ = commit()

	now = now.Add(5 * time.Minute)
	if _, commit := th.Select(nil, nil); commit() != nil { // the problem went away: forgotten
		t.Fatal("commit failed")
	}
	if due, _ := th.Select([]Alert{a}, nil); len(due) != 1 {
		t.Fatalf("a recurring problem was not announced again: %v", due)
	}
}

func TestThrottleTreatsADamagedStateFileAsEmpty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "alerts.json")
	if err := writeFile(path, "{not json"); err != nil {
		t.Fatal(err)
	}
	th := NewThrottle(path)
	if due, _ := th.Select([]Alert{{Key: "k", Text: "t", Repeat: time.Hour}}, nil); len(due) != 1 {
		t.Fatalf("due = %v", due)
	}
}

func writeFile(path, content string) error { return os.WriteFile(path, []byte(content), 0o600) }

func TestOnlyHeatAndFailingDrivesAreUrgent(t *testing.T) {
	hot := strings.Replace(smartHDD, "51 (Min/Max 29/59)", "54 (Min/Max 29/59)", 1)
	h := alertHost(map[string]string{"sdb": hot, "sdc": smartUSB, "nvme0n1": strings.Replace(smartNVMe, "PASSED", "FAILED!", 1)}, 97)
	got, _ := h.Alerts(context.Background())
	urgent := map[string]bool{}
	for _, a := range got {
		urgent[a.Key] = a.Urgent
	}
	for key, want := range map[string]bool{"temp:sdb": true, "smart:nvme0n1": true, "sectors:sdc": false, "full:/srv/pool": false} {
		if got, ok := urgent[key]; !ok || got != want {
			t.Errorf("%s: urgent=%v present=%v, want urgent=%v", key, got, ok, want)
		}
	}
}

func TestMutedAlertsAreHeldBackThenSentWhenTheMuteEnds(t *testing.T) {
	path := filepath.Join(t.TempDir(), "alerts.json")
	now := time.Date(2026, 10, 9, 2, 0, 0, 0, time.UTC) // the middle of the night
	th := &Throttle{path: path, now: func() time.Time { return now }}
	night := QuietHours{0, 6 * 60}
	mute := func(a Alert) bool { return night.Contains(th.now()) && !a.Urgent }
	routine := Alert{Key: "full:/", Text: "/ đầy 97%", Repeat: 12 * time.Hour}
	urgent := Alert{Key: "temp:sdb", Text: "sdb 61°C", Repeat: 10 * time.Minute, Urgent: true}

	due, commit := th.Select([]Alert{routine, urgent}, mute)
	if len(due) != 1 || due[0].Key != "temp:sdb" {
		t.Fatalf("at night due = %v, want only the urgent alert", due)
	}
	_ = commit()

	now = time.Date(2026, 10, 9, 6, 0, 0, 0, time.UTC) // quiet hours are over
	due, commit = th.Select([]Alert{routine, urgent}, mute)
	if len(due) != 2 {
		t.Fatalf("in the morning due = %v, want the routine alert and the urgent one again", due)
	}
	_ = commit()
	now = now.Add(time.Hour)
	if due, _ := th.Select([]Alert{routine}, mute); len(due) != 0 {
		t.Fatalf("routine alert repeated within 12h: %v", due)
	}
}

func TestQuietHoursWindows(t *testing.T) {
	at := func(h, m int) time.Time { return time.Date(2026, 10, 9, h, m, 0, 0, time.UTC) }
	night, err := ParseQuietHours("00:00-06:00")
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		h, m int
		in   bool
	}{{0, 0, true}, {3, 30, true}, {5, 59, true}, {6, 0, false}, {12, 0, false}, {23, 59, false}} {
		if got := night.Contains(at(c.h, c.m)); got != c.in {
			t.Errorf("%02d:%02d in night = %v, want %v", c.h, c.m, got, c.in)
		}
	}
	wrap, _ := ParseQuietHours("22:00-06:00")
	if !wrap.Contains(at(23, 0)) || !wrap.Contains(at(1, 0)) || wrap.Contains(at(7, 0)) {
		t.Error("a window crossing midnight is wrong")
	}
	none, _ := ParseQuietHours("")
	if none.Contains(at(3, 0)) {
		t.Error("empty window must never be active")
	}
	for _, bad := range []string{"6", "0-6", "25:00-06:00", "00:00 06:00"} {
		if _, err := ParseQuietHours(bad); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
}
