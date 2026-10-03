package claude_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/nchungdev/agent-hub/internal/adapters/claude"
)

func TestQuotaFromRateLimitEvent(t *testing.T) {
	abs, _ := filepath.Abs("testdata/fakeclaude.py")
	e := claude.New(abs)
	q, err := e.Quota(context.Background())
	if err != nil || len(q.Groups) != 1 {
		t.Fatalf("q=%+v err=%v", q, err)
	}
	ws := q.Groups[0].Windows
	if len(ws) != 2 || ws[0].Label != "5-hour" || int(*ws[0].UsedPercent+0.5) != 62 || ws[1].Label != "Weekly" || int(*ws[1].UsedPercent+0.5) != 8 || ws[0].ResetsAt == nil {
		t.Fatalf("windows=%+v", ws)
	}
	// a second call within the freshness window must reuse the captured value (no new spawn)
	q2, err := claude.New("/nonexistent").Quota(context.Background())
	if err == nil {
		t.Fatalf("expected an error without a binary or cache, got %+v", q2)
	}
}
