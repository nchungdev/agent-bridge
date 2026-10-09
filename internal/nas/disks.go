package nas

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
)

// disk is a physical drive as lsblk lists it.
type disk struct{ name, size, model string }

// parseLsblk reads `lsblk -dn -o NAME,SIZE,ROTA,MODEL`; the model may contain spaces or be empty.
func parseLsblk(out string) []disk {
	var disks []disk
	for _, line := range strings.Split(out, "\n") {
		f := strings.Fields(line)
		if len(f) < 3 {
			continue
		}
		disks = append(disks, disk{name: f[0], size: f[1], model: strings.Join(f[3:], " ")})
	}
	return disks
}

// smartInfo is what smartctl says about a drive.
type smartInfo struct {
	temp                                int
	hasTemp                             bool
	health                              string
	reallocated, pending, uncorrectable int
}

// parseSmart reads the output of `smartctl -A -H` for SATA and NVMe drives.
func parseSmart(out string) smartInfo {
	var s smartInfo
	for _, line := range strings.Split(out, "\n") {
		if _, v, ok := strings.Cut(line, "overall-health self-assessment test result:"); ok {
			s.health = strings.TrimSpace(v)
			continue
		}
		f := strings.Fields(line)
		if len(f) >= 3 && f[0] == "Temperature:" { // NVMe: "Temperature:   59 Celsius"
			if n, err := strconv.Atoi(f[1]); err == nil {
				s.temp, s.hasTemp = n, true
			}
			continue
		}
		if len(f) < 10 { // a SATA attribute row has ten columns, the tenth being the raw value
			continue
		}
		raw, _ := strconv.Atoi(f[9])
		switch f[1] {
		case "Temperature_Celsius":
			s.temp, s.hasTemp = raw, true
		case "Airflow_Temperature_Cel":
			if !s.hasTemp {
				s.temp, s.hasTemp = raw, true
			}
		case "Reallocated_Sector_Ct":
			s.reallocated = raw
		case "Current_Pending_Sector":
			s.pending = raw
		case "Offline_Uncorrectable":
			s.uncorrectable = raw
		}
	}
	return s
}

// problems lists what is wrong with a drive in words, empty when it looks healthy.
func (s smartInfo) problems() []string {
	var p []string
	if s.health != "" && !strings.EqualFold(s.health, "PASSED") {
		p = append(p, "SMART báo "+s.health)
	}
	if s.pending > 0 {
		p = append(p, fmt.Sprintf("%d sector chờ xử lý (Current_Pending_Sector)", s.pending))
	}
	if s.reallocated > 0 {
		p = append(p, fmt.Sprintf("%d sector đã thay thế (Reallocated_Sector_Ct)", s.reallocated))
	}
	if s.uncorrectable > 0 {
		p = append(p, fmt.Sprintf("%d sector không sửa được (Offline_Uncorrectable)", s.uncorrectable))
	}
	return p
}

// tempIcon colours a drive temperature. NVMe drives run hotter by design, so they have their own limits.
func tempIcon(temp int, nvme bool) string {
	green, yellow, orange := 45, 50, 55
	if nvme {
		green, yellow, orange = 65, 70, 72
	}
	switch {
	case temp < green:
		return "🟢"
	case temp < yellow:
		return "🟡"
	case temp < orange:
		return "🟠"
	}
	return "🔴"
}

// diskReport is one drive ready to print.
type diskReport struct {
	disk
	smart smartInfo
	err   error
}

// drives lists the drives and reads their SMART data in parallel.
func (h *Host) drives(ctx context.Context) ([]diskReport, error) {
	out, err := h.exec(ctx, quickTimeout, "lsblk", "-dn", "-o", "NAME,SIZE,ROTA,MODEL")
	if err != nil && out == "" {
		return nil, err
	}
	reports := make([]diskReport, 0, 8)
	for _, d := range parseLsblk(out) {
		if strings.HasPrefix(d.name, "loop") || strings.HasPrefix(d.name, "zram") || strings.HasPrefix(d.name, "sr") {
			continue
		}
		reports = append(reports, diskReport{disk: d})
	}
	var wg sync.WaitGroup
	for i := range reports {
		wg.Add(1)
		go func(r *diskReport) {
			defer wg.Done()
			// smartctl reports findings through its exit status, so use the output whenever there is some.
			o, err := h.sudo(ctx, quickTimeout, "smartctl", "-A", "-H", "/dev/"+r.name)
			if o == "" {
				r.err = err
				return
			}
			r.smart = parseSmart(o)
		}(&reports[i])
	}
	wg.Wait()
	return reports, nil
}

func (r diskReport) line() string {
	title := strings.TrimSpace(r.name + " " + r.size + " " + r.model)
	if !r.smart.hasTemp && r.smart.health == "" {
		return "⚪ " + title + " — không đọc được SMART"
	}
	icon, parts := "⚪", []string{}
	if r.smart.hasTemp {
		icon = tempIcon(r.smart.temp, strings.HasPrefix(r.name, "nvme"))
		parts = append(parts, fmt.Sprintf("%d°C", r.smart.temp))
	}
	if r.smart.health != "" {
		parts = append(parts, "SMART "+r.smart.health)
	}
	line := icon + " " + title + " — " + strings.Join(parts, ", ")
	for _, p := range r.smart.problems() {
		line += "\n   ⚠️ " + p
	}
	return line
}

// usedFilesystems are the filesystems worth reporting; pseudo and container ones are left out.
var usedFilesystems = map[string]bool{"ext4": true, "xfs": true, "btrfs": true, "zfs": true, "exfat": true, "fuseblk": true, "fuse.mergerfs": true, "ntfs3": true}

// mounts reports the used space of every real filesystem.
func (h *Host) mounts() []string {
	data, err := h.sys.readFile("/proc/mounts")
	if err != nil {
		return nil
	}
	seen := map[string]bool{}
	var lines []string
	for _, line := range strings.Split(data, "\n") {
		f := strings.Fields(line)
		if len(f) < 3 || !usedFilesystems[f[2]] || seen[f[1]] {
			continue
		}
		seen[f[1]] = true
		total, free, err := h.sys.statfs(f[1])
		if err != nil || total == 0 {
			continue
		}
		used := total - free
		lines = append(lines, fmt.Sprintf("%s — %s/%s (%d%%)", f[1], humanBytes(used), humanBytes(total), used*100/total))
	}
	return lines
}
