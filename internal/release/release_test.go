package release

import "testing"

func TestParse(t *testing.T) {
	for _, c := range []struct {
		tag     string
		version string
		build   int
		ok      bool
	}{
		{"v1.0.1", "v1.0.1", 1, true},
		{"v1.0.1-b02", "v1.0.1", 2, true},
		{"v1.0.1-b99", "v1.0.1", 99, true},
		{"v10.20.30", "v10.20.30", 1, true},
		{"v1.0.1-b01", "", 0, false},  // build 01 is the plain tag
		{"v1.0.1-b00", "", 0, false},  // there is no build 0
		{"v1.0.1-b100", "", 0, false}, // past MaxBuild: the next patch version is required
		{"v1.0.1-rc1", "", 0, false},
		{"dev", "", 0, false},
		{"1.0.1", "", 0, false},
		{"v1.0", "", 0, false},
		{"", "", 0, false},
	} {
		v, b, ok := Parse(c.tag)
		if v != c.version || b != c.build || ok != c.ok {
			t.Errorf("Parse(%q) = (%q, %d, %v), want (%q, %d, %v)", c.tag, v, b, ok, c.version, c.build, c.ok)
		}
	}
}

func TestNewer(t *testing.T) {
	ascending := []string{"v0.1.0", "v1.0.0", "v1.0.1", "v1.0.1-b02", "v1.0.1-b10", "v1.0.1-b99", "v1.0.2", "v1.0.10", "v1.1.0", "v2.0.0"}
	for i := range ascending {
		for j := range ascending {
			if got, want := Newer(ascending[j], ascending[i]), j > i; got != want {
				t.Errorf("Newer(%q, %q) = %v, want %v", ascending[j], ascending[i], got, want)
			}
		}
	}
	if !Newer("v1.0.0", "dev") {
		t.Error("any release is newer than a development build")
	}
	if Newer("dev", "v1.0.0") || Newer("nightly", "v1.0.0") {
		t.Error("a non-release is never newer than a release")
	}
}

func TestDisplay(t *testing.T) {
	for tag, want := range map[string]string{
		"v1.0.1":     "v1.0.1 (build 01)",
		"v1.0.1-b02": "v1.0.1 (build 02)",
		"v1.0.1-b99": "v1.0.1 (build 99)",
		"dev":        "dev",
	} {
		if got := Display(tag); got != want {
			t.Errorf("Display(%q) = %q, want %q", tag, got, want)
		}
	}
}
