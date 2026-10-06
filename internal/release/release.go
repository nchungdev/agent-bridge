// Package release names builds. A build is a release tag pushed on purpose:
//
//	v1.0.1       version 1.0.1, build 01
//	v1.0.1-b02   version 1.0.1, build 02 (a hotfix build), ... up to -b99
//
// After build 99 the next patch version (v1.0.2) must be tagged, which restarts the numbering at 01.
package release

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// MaxBuild is the highest build number of one version.
const MaxBuild = 99

var tagRe = regexp.MustCompile(`^(v\d+\.\d+\.\d+)(?:-b(\d{2}))?$`)

// Parse splits a release tag into its version ("v1.0.1") and build number (1 for the plain tag).
// Anything else (dev, v1.0.3-rc1, -b00, -b01, a build over MaxBuild) is not a release tag.
func Parse(tag string) (version string, build int, ok bool) {
	m := tagRe.FindStringSubmatch(strings.TrimSpace(tag))
	if m == nil {
		return "", 0, false
	}
	if m[2] == "" {
		return m[1], 1, true
	}
	b, _ := strconv.Atoi(m[2])
	if b < 2 || b > MaxBuild { // -b01 would be the plain tag, and there is no build 0
		return "", 0, false
	}
	return m[1], b, true
}

// Key orders releases: version numerically, then build.
func Key(version string, build int) [4]int {
	var k [4]int
	for i, p := range strings.Split(strings.TrimPrefix(version, "v"), ".") {
		if i < 3 {
			k[i], _ = strconv.Atoi(p)
		}
	}
	k[3] = build
	return k
}

// Less reports whether key a sorts below key b.
func Less(a, b [4]int) bool {
	for i := range a {
		if a[i] != b[i] {
			return a[i] < b[i]
		}
	}
	return false
}

// Newer reports whether tag a is a newer release than tag b. A tag that is not a release (such as "dev") is
// older than any release, and a release is never newer than a non-release.
func Newer(a, b string) bool {
	va, ba, okA := Parse(a)
	if !okA {
		return false
	}
	vb, bb, okB := Parse(b)
	if !okB {
		return true
	}
	return Less(Key(vb, bb), Key(va, ba))
}

// Format shows a version and build the way the app displays them: "v1.0.1 (build 02)".
func Format(version string, build int) string {
	if build > 0 {
		return fmt.Sprintf("%s (build %02d)", version, build)
	}
	return version
}

// Display shows a release tag as "v1.0.1 (build 02)", and anything else (such as "dev") unchanged.
func Display(tag string) string {
	if v, b, ok := Parse(tag); ok {
		return Format(v, b)
	}
	return tag
}
