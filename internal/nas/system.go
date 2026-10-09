package nas

import (
	"fmt"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
)

func globFiles(pattern string) ([]string, error) { return filepath.Glob(pattern) }
func numCPU() int                                { return runtime.NumCPU() }

func statfs(path string) (total, free uint64, err error) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		return 0, 0, err
	}
	return st.Blocks * uint64(st.Bsize), st.Bavail * uint64(st.Bsize), nil
}

// cpuSample is the cumulative CPU time of one /proc/stat reading.
type cpuSample struct{ idle, total uint64 }

func parseCPUSample(stat string) (cpuSample, bool) {
	line, _, _ := strings.Cut(stat, "\n")
	f := strings.Fields(line)
	if len(f) < 6 || f[0] != "cpu" {
		return cpuSample{}, false
	}
	var s cpuSample
	for i, v := range f[1:] {
		n, err := strconv.ParseUint(v, 10, 64)
		if err != nil {
			return cpuSample{}, false
		}
		s.total += n
		if i == 3 || i == 4 { // idle, iowait
			s.idle += n
		}
	}
	return s, true
}

// cpuUsage measures the share of CPU time in use over a short interval, in percent.
func (h *Host) cpuUsage() (float64, bool) {
	a, ok := h.sampleCPU()
	if !ok {
		return 0, false
	}
	h.sys.sleep(500 * time.Millisecond)
	b, ok := h.sampleCPU()
	if !ok || b.total <= a.total {
		return 0, false
	}
	return 100 * (1 - float64(b.idle-a.idle)/float64(b.total-a.total)), true
}

func (h *Host) sampleCPU() (cpuSample, bool) {
	stat, err := h.sys.readFile("/proc/stat")
	if err != nil {
		return cpuSample{}, false
	}
	return parseCPUSample(stat)
}

// memory is memory and swap in bytes.
type memory struct{ total, available, swapTotal, swapFree uint64 }

func parseMeminfo(s string) memory {
	vals := map[string]uint64{}
	for _, line := range strings.Split(s, "\n") {
		key, rest, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		if f := strings.Fields(rest); len(f) > 0 {
			if n, err := strconv.ParseUint(f[0], 10, 64); err == nil {
				vals[key] = n * 1024 // /proc/meminfo is in kB
			}
		}
	}
	return memory{vals["MemTotal"], vals["MemAvailable"], vals["SwapTotal"], vals["SwapFree"]}
}

func (h *Host) memory() (memory, bool) {
	s, err := h.sys.readFile("/proc/meminfo")
	if err != nil {
		return memory{}, false
	}
	m := parseMeminfo(s)
	return m, m.total > 0
}

func (h *Host) loadAvg() string {
	s, err := h.sys.readFile("/proc/loadavg")
	if err != nil {
		return ""
	}
	f := strings.Fields(s)
	if len(f) < 3 {
		return ""
	}
	return strings.Join(f[:3], "/")
}

func (h *Host) uptime() (time.Duration, bool) {
	s, err := h.sys.readFile("/proc/uptime")
	if err != nil {
		return 0, false
	}
	f := strings.Fields(s)
	if len(f) == 0 {
		return 0, false
	}
	secs, err := strconv.ParseFloat(f[0], 64)
	return time.Duration(secs) * time.Second, err == nil
}

// cpuTemp reads the processor package temperature from hwmon, in degrees C.
func (h *Host) cpuTemp() (int, bool) {
	names, _ := h.sys.glob("/sys/class/hwmon/hwmon*/name")
	for _, namePath := range names {
		name, err := h.sys.readFile(namePath)
		if err != nil || !isCPUSensor(strings.TrimSpace(name)) {
			continue
		}
		raw, err := h.sys.readFile(filepath.Join(filepath.Dir(namePath), "temp1_input"))
		if err != nil {
			continue
		}
		if milli, err := strconv.Atoi(strings.TrimSpace(raw)); err == nil {
			return milli / 1000, true
		}
	}
	return 0, false
}

func isCPUSensor(name string) bool {
	return name == "coretemp" || name == "k10temp" || name == "zenpower"
}

func humanBytes(n uint64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%dB", n)
	}
	div, exp := uint64(unit), 0
	for v := n / unit; v >= unit; v /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f%c", float64(n)/float64(div), "KMGTPE"[exp])
}

func humanDuration(d time.Duration) string {
	switch {
	case d < time.Minute:
		return "vừa xong"
	case d < time.Hour:
		return fmt.Sprintf("%d phút", int(d.Minutes()))
	case d < 48*time.Hour:
		return fmt.Sprintf("%d giờ %d phút", int(d.Hours()), int(d.Minutes())%60)
	}
	return fmt.Sprintf("%d ngày %d giờ", int(d.Hours())/24, int(d.Hours())%24)
}
