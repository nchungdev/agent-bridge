package nas

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

// Service is something installed on the machine that can be listed and controlled.
type Service struct {
	Name        string
	Description string
}

// Actions ServiceAction accepts.
const (
	ActionStart   = "start"
	ActionStop    = "stop"
	ActionRestart = "restart"
	ActionUpdate  = "update"
)

type kind int

const (
	kindContainer kind = iota
	kindUnit
)

// target is a service resolved against what is really installed.
type target struct {
	kind kind
	name string
}

// Services lists the Docker containers and the allow-listed systemd units, each with a one-line description.
// A failing Docker is reported through the error while the units are still returned.
func (h *Host) Services(ctx context.Context) ([]Service, error) {
	list, err := h.containers(ctx)
	services := make([]Service, 0, len(list)+len(h.units))
	for _, c := range list {
		services = append(services, Service{Name: c.name, Description: stateIcon(c.state) + " " + c.description()})
	}
	for _, u := range h.units {
		state, desc := h.unitSummary(ctx, u)
		services = append(services, Service{Name: u, Description: stateIcon(state) + " (systemd) " + desc})
	}
	sort.SliceStable(services, func(i, j int) bool { return services[i].Name < services[j].Name })
	return services, err
}

// ServiceStatus reports the state of one service.
func (h *Host) ServiceStatus(ctx context.Context, name string) (string, error) {
	t, err := h.resolve(ctx, name)
	if err != nil {
		return "", err
	}
	if t.kind == kindUnit {
		return h.unitStatus(ctx, t.name)
	}
	return h.containerStatus(ctx, t.name)
}

// ServiceAction starts, stops, restarts or updates a service and reports the result.
func (h *Host) ServiceAction(ctx context.Context, name, action string) (string, error) {
	t, err := h.resolve(ctx, name)
	if err != nil {
		return "", err
	}
	switch action {
	case ActionStart, ActionStop, ActionRestart:
	case ActionUpdate:
		if t.kind == kindUnit {
			return "", fmt.Errorf("%w: update chỉ dùng cho container Docker", ErrUnsupported)
		}
		return h.updateContainer(ctx, t.name)
	default:
		return "", fmt.Errorf("%w: %q", ErrUnsupported, action)
	}
	if t.kind == kindUnit {
		if _, err := h.sudo(ctx, actionTimeout, "systemctl", action, t.name); err != nil {
			return "", err
		}
		return h.unitStatus(ctx, t.name)
	}
	if _, err := h.exec(ctx, actionTimeout, "docker", action, t.name); err != nil {
		return "", err
	}
	return h.containerStatus(ctx, t.name)
}

// resolve matches name against the installed services, ignoring case. Only names that really exist get through,
// which is what keeps user text away from the command line.
func (h *Host) resolve(ctx context.Context, name string) (target, error) {
	list, _ := h.containers(ctx)
	for _, c := range list {
		if strings.EqualFold(c.name, name) {
			return target{kindContainer, c.name}, nil
		}
	}
	for _, u := range h.units {
		if strings.EqualFold(u, name) {
			return target{kindUnit, u}, nil
		}
	}
	return target{}, ErrUnknownService
}

// ---- Docker ----

type container struct{ name, image, state, status, label string }

// description is the first sentence of the image's own description, or the image name.
func (c container) description() string {
	if d := shortDescription(c.label); d != "" {
		return d
	}
	return c.image
}

var markdownLink = regexp.MustCompile(`\[([^\]]*)\]\([^)]*\)`)

// shortDescription drops markdown links and keeps the first sentence, at most 80 characters.
func shortDescription(label string) string {
	s := strings.TrimSpace(markdownLink.ReplaceAllString(label, "$1"))
	if i := strings.Index(s, ". "); i > 0 {
		s = s[:i]
	}
	if utf8.RuneCountInString(s) > 80 {
		s = string([]rune(s)[:80]) + "…"
	}
	return s
}

func (h *Host) containers(ctx context.Context) ([]container, error) {
	out, err := h.exec(ctx, quickTimeout, "docker", "ps", "-a", "--format", "{{.Names}}|{{.Image}}|{{.State}}|{{.Status}}")
	if err != nil {
		return nil, err
	}
	var list []container
	var names []string
	for _, line := range strings.Split(out, "\n") {
		if f := strings.SplitN(line, "|", 4); len(f) == 4 {
			list = append(list, container{name: f[0], image: f[1], state: f[2], status: f[3]})
			names = append(names, f[0])
		}
	}
	if labels := h.imageDescriptions(ctx, names); len(labels) > 0 {
		for i := range list {
			list[i].label = labels[list[i].name]
		}
	}
	return list, nil
}

// imageDescriptions reads each container's org.opencontainers.image.description label in one call.
func (h *Host) imageDescriptions(ctx context.Context, names []string) map[string]string {
	if len(names) == 0 {
		return nil
	}
	args := append([]string{"inspect", "-f", `{{.Name}}|{{index .Config.Labels "org.opencontainers.image.description"}}`}, names...)
	out, _ := h.exec(ctx, quickTimeout, "docker", args...)
	labels := map[string]string{}
	for _, line := range strings.Split(out, "\n") {
		if name, label, ok := strings.Cut(line, "|"); ok {
			labels[strings.TrimPrefix(name, "/")] = label
		}
	}
	return labels
}

// inspected is the part of `docker inspect` the status report uses.
type inspected struct {
	Name         string
	Image        string // image id
	RestartCount int
	Config       struct {
		Image  string
		Labels map[string]string
	}
	State struct {
		Status    string
		StartedAt string
		ExitCode  int
		Health    *struct{ Status string }
	}
	NetworkSettings struct {
		Ports map[string][]struct{ HostPort string }
	}
}

func (h *Host) inspect(ctx context.Context, name string) (inspected, error) {
	out, err := h.exec(ctx, quickTimeout, "docker", "inspect", name)
	if err != nil {
		return inspected{}, err
	}
	var list []inspected
	if err := json.Unmarshal([]byte(out), &list); err != nil || len(list) == 0 {
		return inspected{}, fmt.Errorf("không đọc được thông tin container %s", name)
	}
	return list[0], nil
}

func (h *Host) containerStatus(ctx context.Context, name string) (string, error) {
	in, err := h.inspect(ctx, name)
	if err != nil {
		return "", err
	}
	state := in.State.Status
	if in.State.Health != nil && in.State.Health.Status != "" {
		state += " (" + in.State.Health.Status + ")"
	}
	lines := []string{stateIcon(in.State.Status) + " " + name + " — " + state, "Image: " + in.Config.Image}
	if started, err := time.Parse(time.RFC3339Nano, in.State.StartedAt); err == nil && in.State.Status == "running" {
		lines = append(lines, fmt.Sprintf("Chạy được %s • khởi động lại %d lần", humanDuration(time.Since(started)), in.RestartCount))
	} else if in.State.Status == "exited" {
		lines = append(lines, fmt.Sprintf("Đã dừng, mã thoát %d", in.State.ExitCode))
	}
	if ports := publishedPorts(in); ports != "" {
		lines = append(lines, "Cổng: "+ports)
	}
	if in.State.Status == "running" {
		if use, err := h.exec(ctx, quickTimeout, "docker", "stats", "--no-stream", "--format", "{{.CPUPerc}} CPU • {{.MemUsage}}", name); err == nil && strings.TrimSpace(use) != "" {
			lines = append(lines, "Tài nguyên: "+strings.TrimSpace(use))
		}
	}
	return strings.Join(lines, "\n"), nil
}

func publishedPorts(in inspected) string {
	var ports []string
	for container, bindings := range in.NetworkSettings.Ports {
		for _, b := range bindings {
			ports = append(ports, b.HostPort+"→"+container)
		}
	}
	sort.Strings(ports)
	return strings.Join(dedupe(ports), ", ")
}

func dedupe(in []string) []string {
	var out []string
	for i, v := range in {
		if i == 0 || v != in[i-1] {
			out = append(out, v)
		}
	}
	return out
}

// updateContainer pulls a newer image and recreates the container through its own docker compose project.
// It refuses when the compose labels are missing or name another service, so it can never update the wrong one.
func (h *Host) updateContainer(ctx context.Context, name string) (string, error) {
	in, err := h.inspect(ctx, name)
	if err != nil {
		return "", err
	}
	l := in.Config.Labels
	dir, project, svc := l["com.docker.compose.project.working_dir"], l["com.docker.compose.project"], l["com.docker.compose.service"]
	if dir == "" || svc == "" {
		return "", fmt.Errorf("%w: container không do docker compose quản lý, cập nhật thủ công", ErrUnsupported)
	}
	if svc != name {
		return "", fmt.Errorf("%w: nhãn compose trỏ tới service %q, không khớp tên container %q, cập nhật thủ công", ErrUnsupported, svc, name)
	}
	compose := []string{"compose", "--project-directory", dir, "--project-name", project}
	for _, f := range strings.Split(l["com.docker.compose.project.config_files"], ",") {
		if f != "" {
			compose = append(compose, "-f", f)
		}
	}
	if _, err := h.exec(ctx, pullTimeout, "docker", append(compose, "pull", svc)...); err != nil {
		return "", fmt.Errorf("không kéo được image mới: %w", err)
	}
	newID, err := h.exec(ctx, quickTimeout, "docker", "image", "inspect", "-f", "{{.Id}}", in.Config.Image)
	if err == nil && strings.TrimSpace(newID) == in.Image {
		return "✅ " + name + " đã là bản mới nhất, không thay đổi gì.", nil
	}
	if _, err := h.exec(ctx, actionTimeout, "docker", append(compose, "up", "-d", "--no-deps", svc)...); err != nil {
		return "", fmt.Errorf("đã kéo image mới nhưng không tạo lại được container: %w", err)
	}
	status, err := h.containerStatus(ctx, name)
	if err != nil {
		return "✅ Đã cập nhật " + name + ".", nil
	}
	return "✅ Đã cập nhật " + name + ".\n" + status, nil
}

// ---- systemd ----

// unitProps reads `systemctl show` output (Key=Value lines).
func unitProps(out string) map[string]string {
	props := map[string]string{}
	for _, line := range strings.Split(out, "\n") {
		if k, v, ok := strings.Cut(line, "="); ok {
			props[k] = strings.TrimSpace(v)
		}
	}
	return props
}

func (h *Host) unitShow(ctx context.Context, unit string) map[string]string {
	out, _ := h.exec(ctx, quickTimeout, "systemctl", "show", unit, "--no-pager",
		"-p", "Description", "-p", "ActiveState", "-p", "SubState", "-p", "ActiveEnterTimestamp", "-p", "NRestarts", "-p", "MainPID")
	return unitProps(out)
}

func (h *Host) unitSummary(ctx context.Context, unit string) (state, description string) {
	p := h.unitShow(ctx, unit)
	return p["ActiveState"], orElse(p["Description"], unit)
}

func (h *Host) unitStatus(ctx context.Context, unit string) (string, error) {
	p := h.unitShow(ctx, unit)
	if p["ActiveState"] == "" {
		return "", fmt.Errorf("không đọc được trạng thái %s", unit)
	}
	lines := []string{stateIcon(p["ActiveState"]) + " " + unit + " — " + p["ActiveState"] + " (" + p["SubState"] + ")", p["Description"]}
	if p["ActiveState"] == "active" && p["ActiveEnterTimestamp"] != "" {
		lines = append(lines, "Từ: "+p["ActiveEnterTimestamp"])
	}
	if p["NRestarts"] != "" && p["NRestarts"] != "0" {
		lines = append(lines, "Khởi động lại: "+p["NRestarts"]+" lần")
	}
	return strings.Join(lines, "\n"), nil
}

func stateIcon(state string) string {
	switch state {
	case "running", "active":
		return "🟢"
	case "restarting", "activating", "reloading":
		return "🟡"
	case "created", "paused", "inactive", "":
		return "⚪"
	}
	return "🔴" // exited, dead, failed, ...
}

func orElse(v, def string) string {
	if v == "" {
		return def
	}
	return v
}
