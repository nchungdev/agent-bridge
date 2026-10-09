package nas

import (
	"context"
	"fmt"
	"strings"
)

// Overview reports the machine: uptime, CPU, memory, drives with their temperatures, and disk space.
func (h *Host) Overview(ctx context.Context) (string, error) {
	var b strings.Builder
	host, _ := h.sys.hostname()
	b.WriteString("🖥 " + host)
	if up, ok := h.uptime(); ok {
		b.WriteString(" — chạy " + humanDuration(up))
	}
	b.WriteString("\n" + h.cpuLine())
	if m, ok := h.memory(); ok {
		used := m.total - m.available
		fmt.Fprintf(&b, "\nRAM: %s/%s (%d%%) • swap %s/%s", humanBytes(used), humanBytes(m.total), used*100/m.total,
			humanBytes(m.swapTotal-m.swapFree), humanBytes(m.swapTotal))
	}
	b.WriteString("\n\nỔ cứng:")
	drives, err := h.drives(ctx)
	if err != nil {
		b.WriteString("\n⚪ không liệt kê được ổ: " + err.Error())
	}
	for _, d := range drives {
		b.WriteString("\n" + d.line())
	}
	if lines := h.mounts(); len(lines) > 0 {
		b.WriteString("\n\nDung lượng:\n" + strings.Join(lines, "\n"))
	}
	return b.String(), nil
}

func (h *Host) cpuLine() string {
	parts := []string{}
	if use, ok := h.cpuUsage(); ok {
		parts = append(parts, fmt.Sprintf("%.0f%%", use))
	}
	if load := h.loadAvg(); load != "" {
		parts = append(parts, fmt.Sprintf("tải %s (%d nhân)", load, h.sys.cores()))
	}
	if t, ok := h.cpuTemp(); ok {
		parts = append(parts, fmt.Sprintf("%d°C", t))
	}
	return "CPU: " + strings.Join(parts, " • ")
}
