package agy_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/nchungdev/agent-bridge/internal/adapters/agy"
)

func TestQuotaParsesTheCliTable(t *testing.T) {
	abs, _ := filepath.Abs("testdata/fakeagy.py")
	q, err := agy.New(abs).Quota(context.Background())
	if err != nil || len(q.Groups) != 2 {
		t.Fatalf("q=%+v err=%v", q, err)
	}
	g := q.Groups[0]
	if g.Name != "Gemini Models" || len(g.Windows) != 2 {
		t.Fatalf("group=%+v", g)
	}
	w := g.Windows[0]
	if w.Label != "Weekly Limit" || w.UsedPercent == nil || *w.UsedPercent != 100 || w.ResetsAt == nil || w.ResetsAt.Year() != 2026 {
		t.Fatalf("weekly=%+v", w)
	}
	if !g.Windows[1].Disabled || g.Windows[1].UsedPercent != nil {
		t.Fatalf("disabled window=%+v", g.Windows[1])
	}
	if p := q.Groups[1].Windows[0].UsedPercent; p == nil || *p != 0 {
		t.Fatalf("100%% remaining should be 0%% used, got %v", p)
	}
}
