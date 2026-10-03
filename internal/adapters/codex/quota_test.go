package codex_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/nchungdev/agent-hub/internal/adapters/codex"
)

func TestQuotaFromRateLimits(t *testing.T) {
	abs, _ := filepath.Abs("testdata/fakecodex.py")
	q, err := codex.New(abs).Quota(context.Background())
	if err != nil || len(q.Groups) != 1 || q.Plan != "plus" {
		t.Fatalf("q=%+v err=%v", q, err)
	}
	ws := q.Groups[0].Windows
	if len(ws) != 2 || ws[0].Label != "5-hour" || *ws[0].UsedPercent != 42 || ws[1].Label != "1-week" || ws[1].ResetsAt == nil {
		t.Fatalf("windows=%+v", ws)
	}
}
