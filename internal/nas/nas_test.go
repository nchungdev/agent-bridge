package nas

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeRunner answers by the command line, and records what was run.
type fakeRunner struct {
	mu    sync.Mutex
	out   map[string]string // full command line -> stdout
	err   map[string]error
	calls []string
}

func (f *fakeRunner) Run(_ context.Context, name string, args ...string) (string, error) {
	line := strings.Join(append([]string{name}, args...), " ")
	f.mu.Lock()
	f.calls = append(f.calls, line)
	f.mu.Unlock()
	return f.out[line], f.err[line]
}

func (f *fakeRunner) ran(prefix string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, c := range f.calls {
		if strings.HasPrefix(c, prefix) {
			return true
		}
	}
	return false
}

func fakeSystem(files map[string]string) system {
	return system{
		readFile: func(p string) (string, error) {
			if v, ok := files[p]; ok {
				return v, nil
			}
			return "", errors.New("missing " + p)
		},
		glob: func(pattern string) ([]string, error) {
			if pattern == "/sys/class/hwmon/hwmon*/name" {
				return []string{"/sys/class/hwmon/hwmon0/name", "/sys/class/hwmon/hwmon1/name"}, nil
			}
			return nil, nil
		},
		statfs: func(path string) (uint64, uint64, error) {
			if path == "/srv/pool" {
				return 4000 << 30, 1000 << 30, nil
			}
			return 100 << 30, 90 << 30, nil
		},
		hostname: func() (string, error) { return "duinch", nil },
		sleep:    func(time.Duration) {},
		cores:    func() int { return 4 },
	}
}

// Real smartctl output seen on the NAS.
const (
	smartUSB = `SMART overall-health self-assessment test result: PASSED
  5 Reallocated_Sector_Ct   0x0033   100   100   005    Pre-fail  Always       -       0
  9 Power_On_Hours          0x0012   040   040   000    Old_age   Always       -       26310
194 Temperature_Celsius     0x0002   142   142   000    Old_age   Always       -       42 (Min/Max 24/52)
197 Current_Pending_Sector  0x0022   030   030   000    Old_age   Always       -       1808
198 Offline_Uncorrectable   0x0008   100   100   000    Old_age   Offline      -       0
`
	smartHDD = `SMART overall-health self-assessment test result: PASSED
  5 Reallocated_Sector_Ct   0x0033   100   100   050    Pre-fail  Always       -       0
194 Temperature_Celsius     0x0022   100   100   000    Old_age   Always       -       51 (Min/Max 29/59)
197 Current_Pending_Sector  0x0032   200   200   000    Old_age   Always       -       0
`
	smartNVMe = `SMART overall-health self-assessment test result: PASSED
Temperature:                        59 Celsius
Temperature Sensor 1:               81 Celsius
`
)

func newHost(r *fakeRunner) *Host {
	h := New(r, []string{"nginx"})
	h.sys = fakeSystem(map[string]string{
		"/proc/stat":                          "cpu  100 0 100 800 0 0 0 0\n",
		"/proc/loadavg":                       "0.35 0.40 0.38 1/200 123\n",
		"/proc/uptime":                        "11520.5 40000.1\n",
		"/proc/meminfo":                       "MemTotal: 16000000 kB\nMemAvailable: 12800000 kB\nSwapTotal: 4000000 kB\nSwapFree: 4000000 kB\n",
		"/proc/mounts":                        "/dev/sdb1 /srv/pool ext4 rw 0 0\ntmpfs /run tmpfs rw 0 0\n/dev/sda1 / ext4 rw 0 0\nover /var/lib/docker/overlay2/x overlay rw 0 0\n",
		"/sys/class/hwmon/hwmon0/name":        "nvme\n",
		"/sys/class/hwmon/hwmon1/name":        "coretemp\n",
		"/sys/class/hwmon/hwmon1/temp1_input": "58850\n",
	})
	return h
}

func TestParseSmartReadsTemperatureAndFindsBadSectors(t *testing.T) {
	usb := parseSmart(smartUSB)
	if !usb.hasTemp || usb.temp != 42 || usb.pending != 1808 || usb.health != "PASSED" {
		t.Fatalf("usb = %+v", usb)
	}
	if p := usb.problems(); len(p) != 1 || !strings.Contains(p[0], "1808") {
		t.Fatalf("problems = %v", p)
	}
	if hdd := parseSmart(smartHDD); hdd.temp != 51 || len(hdd.problems()) != 0 {
		t.Fatalf("hdd = %+v", hdd)
	}
	if nv := parseSmart(smartNVMe); nv.temp != 59 {
		t.Fatalf("nvme temp = %d, want 59 (not the 81 of sensor 1)", nv.temp)
	}
}

func TestTempIconUsesSeparateLimitsForNVMe(t *testing.T) {
	if tempIcon(51, false) != "🟠" || tempIcon(59, true) != "🟢" || tempIcon(60, false) != "🔴" || tempIcon(73, true) != "🔴" {
		t.Fatal("wrong icons")
	}
}

func TestOverviewReportsMachineDisksAndSpace(t *testing.T) {
	r := &fakeRunner{out: map[string]string{
		"lsblk -dn -o NAME,SIZE,ROTA,MODEL":   "sda     931.5G 1 nal USB 3.0\nsdb       3.6T 1 EC-7352\nnvme0n1 238.5G 0 WD PC SN740\nloop0 10M 0\n",
		"sudo -n smartctl -A -H /dev/sda":     smartUSB,
		"sudo -n smartctl -A -H /dev/sdb":     smartHDD,
		"sudo -n smartctl -A -H /dev/nvme0n1": smartNVMe,
	}}
	out, err := newHost(r).Overview(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"duinch", "chạy 3 giờ 12 phút", "CPU:", "tải 0.35/0.40/0.38 (4 nhân)", "58°C",
		"RAM: 3.1G/15.3G (20%)", "sda 931.5G nal USB 3.0 — 42°C", "1808 sector chờ xử lý",
		"🟠 sdb 3.6T EC-7352 — 51°C", "🟢 nvme0n1", "/srv/pool — 2.9T/3.9T (75%)",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
	if strings.Contains(out, "loop0") || strings.Contains(out, "/run") || strings.Contains(out, "overlay") {
		t.Errorf("pseudo devices leaked into:\n%s", out)
	}
}

const psOut = "sonarr|lscr.io/linuxserver/sonarr:latest|running|Up 3 hours\nbazarr|lscr.io/linuxserver/bazarr:latest|exited|Exited (0) 1 hour ago\n"

func dockerRunner() *fakeRunner {
	return &fakeRunner{out: map[string]string{
		"docker ps -a --format {{.Names}}|{{.Image}}|{{.State}}|{{.Status}}":                                                        psOut,
		`docker inspect -f {{.Name}}|{{index .Config.Labels "org.opencontainers.image.description"}} sonarr bazarr`:                 "/sonarr|[Sonarr](https://sonarr.tv/) is a PVR for usenet. It can monitor feeds.\n/bazarr|\n",
		"systemctl show nginx --no-pager -p Description -p ActiveState -p SubState -p ActiveEnterTimestamp -p NRestarts -p MainPID": "Description=A high performance web server\nActiveState=active\nSubState=running\nNRestarts=0\n",
	}}
}

func TestServicesListsContainersAndUnitsWithShortDescriptions(t *testing.T) {
	list, err := newHost(dockerRunner()).Services(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, s := range list {
		got[s.Name] = s.Description
	}
	if got["sonarr"] != "🟢 Sonarr is a PVR for usenet" {
		t.Errorf("sonarr = %q", got["sonarr"])
	}
	if got["bazarr"] != "🔴 lscr.io/linuxserver/bazarr:latest" {
		t.Errorf("bazarr = %q", got["bazarr"])
	}
	if got["nginx"] != "🟢 (systemd) A high performance web server" {
		t.Errorf("nginx = %q", got["nginx"])
	}
}

func TestOnlyInstalledServicesCanBeControlled(t *testing.T) {
	r := dockerRunner()
	h := newHost(r)
	for _, name := range []string{"; rm -rf /", "sonarr --all", "../etc", "unknown"} {
		if _, err := h.ServiceAction(context.Background(), name, ActionRestart); !errors.Is(err, ErrUnknownService) {
			t.Errorf("%q: err = %v", name, err)
		}
	}
	if r.ran("docker restart") || r.ran("sudo") {
		t.Fatalf("a command was run for an unknown name: %v", r.calls)
	}
}

func TestActionMapsToTheRightCommand(t *testing.T) {
	r := dockerRunner()
	r.out["docker inspect sonarr"] = `[{"Name":"/sonarr","Image":"sha256:old","Config":{"Image":"lscr.io/linuxserver/sonarr:latest","Labels":{}},"State":{"Status":"running","StartedAt":"2026-10-09T01:00:00Z"}}]`
	h := newHost(r)
	if _, err := h.ServiceAction(context.Background(), "SONARR", ActionRestart); err != nil {
		t.Fatal(err)
	}
	if !r.ran("docker restart sonarr") {
		t.Fatalf("calls = %v", r.calls)
	}
	if _, err := h.ServiceAction(context.Background(), "nginx", ActionStop); err != nil {
		t.Fatal(err)
	}
	if !r.ran("sudo -n systemctl stop nginx") {
		t.Fatalf("calls = %v", r.calls)
	}
	if _, err := h.ServiceAction(context.Background(), "nginx", ActionUpdate); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("systemd update: err = %v", err)
	}
	if _, err := h.ServiceAction(context.Background(), "sonarr", "delete"); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("unknown action: err = %v", err)
	}
}

func TestUpdateRefusesMissingOrMismatchedComposeLabels(t *testing.T) {
	r := dockerRunner()
	// sonarr is labelled as compose service "radarr": the real NAS has exactly this mismatch.
	r.out["docker inspect sonarr"] = `[{"Name":"/sonarr","Image":"sha256:a","Config":{"Image":"x","Labels":{"com.docker.compose.project.working_dir":"/docker-files/sonarr","com.docker.compose.project":"sonarr","com.docker.compose.service":"radarr"}},"State":{"Status":"running"}}]`
	r.out["docker inspect bazarr"] = `[{"Name":"/bazarr","Image":"sha256:b","Config":{"Image":"y","Labels":{}},"State":{"Status":"exited"}}]`
	h := newHost(r)
	for _, name := range []string{"sonarr", "bazarr"} {
		if _, err := h.ServiceAction(context.Background(), name, ActionUpdate); !errors.Is(err, ErrUnsupported) {
			t.Errorf("%s: err = %v", name, err)
		}
	}
	if r.ran("docker compose") {
		t.Fatalf("compose ran despite bad labels: %v", r.calls)
	}
}

func TestUpdatePullsAndRecreatesOnlyWhenTheImageChanged(t *testing.T) {
	labels := `"com.docker.compose.project.working_dir":"/docker-files/sonarr","com.docker.compose.project":"sonarr","com.docker.compose.service":"sonarr","com.docker.compose.project.config_files":"/docker-files/sonarr/docker-compose.yml"`
	r := dockerRunner()
	r.out["docker inspect sonarr"] = `[{"Name":"/sonarr","Image":"sha256:old","Config":{"Image":"img:latest","Labels":{` + labels + `}},"State":{"Status":"running"}}]`
	r.out["docker image inspect -f {{.Id}} img:latest"] = "sha256:old\n"
	h := newHost(r)
	out, err := h.ServiceAction(context.Background(), "sonarr", ActionUpdate)
	if err != nil || !strings.Contains(out, "đã là bản mới nhất") || r.ran("docker compose --project-directory /docker-files/sonarr --project-name sonarr -f /docker-files/sonarr/docker-compose.yml up") {
		t.Fatalf("unchanged image: out=%q err=%v calls=%v", out, err, r.calls)
	}
	r.out["docker image inspect -f {{.Id}} img:latest"] = "sha256:new\n"
	if _, err := h.ServiceAction(context.Background(), "sonarr", ActionUpdate); err != nil {
		t.Fatal(err)
	}
	if !r.ran("docker compose --project-directory /docker-files/sonarr --project-name sonarr -f /docker-files/sonarr/docker-compose.yml up -d --no-deps sonarr") {
		t.Fatalf("no recreate: %v", r.calls)
	}
}

func TestShortDescription(t *testing.T) {
	if got := shortDescription("[Sonarr](https://sonarr.tv/) (formerly NZBdrone) is a PVR. It can monitor."); got != "Sonarr (formerly NZBdrone) is a PVR" {
		t.Fatalf("got %q", got)
	}
	if got := shortDescription(strings.Repeat("x", 200)); len([]rune(got)) != 81 {
		t.Fatalf("len = %d", len([]rune(got)))
	}
}
