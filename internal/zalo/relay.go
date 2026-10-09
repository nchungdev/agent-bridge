package zalo

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/nchungdev/agent-bridge/internal/core"
)

const (
	progressAfter = 20 * time.Second // a turn must run this long before a "still working" message
	progressEvery = 30 * time.Second // and such messages are at least this far apart
)

// relay turns the events of one agent turn into Zalo messages: it collects the answer, forwards approval
// requests and reports progress while the agent works.
type relay struct {
	s      *Service
	c      *chat
	chatID string
	answer strings.Builder
	tools  *progress
}

func newRelay(s *Service, c *chat, chatID string) *relay {
	return &relay{s: s, c: c, chatID: chatID, tools: newProgress()}
}

// handle consumes one event and reports whether the turn is over.
func (r *relay) handle(ctx context.Context, ev core.Event) (done bool) {
	switch ev.Type {
	case core.EvTextDelta:
		r.answer.WriteString(ev.Text)
	case core.EvToolCall:
		if ev.Tool != nil {
			r.tools.add(ev.Tool.Name)
		}
	case core.EvApprovalRequest:
		if ev.Approval != nil {
			r.c.mu.Lock()
			r.c.approve = ev.Approval.ID
			r.c.mu.Unlock()
			r.s.reply(ctx, r.chatID, fmt.Sprintf("Agent xin quyền chạy %s (%s). Trả lời /ok để cho phép hoặc /no để từ chối.", ev.Approval.Tool, ev.Approval.Title))
		}
	case core.EvError:
		if ev.Err != nil {
			r.answer.WriteString("\n[Lỗi] " + ev.Err.Message)
		}
		r.finish(ctx)
		return true
	case core.EvTurnDone:
		r.finish(ctx)
		return true
	}
	return false
}

// finish sends the collected answer.
func (r *relay) finish(ctx context.Context) {
	text := strings.TrimSpace(r.answer.String())
	if text == "" {
		text = "(agent không trả lời bằng văn bản)"
	}
	r.s.reply(ctx, r.chatID, text)
}

// reportProgress sends a "still working" message when one is due.
func (r *relay) reportProgress(ctx context.Context, now time.Time) {
	if msg := r.tools.due(now); msg != "" {
		r.s.reply(ctx, r.chatID, msg)
	}
}

// progress turns the tools an agent used into an occasional "still working" message.
type progress struct {
	start, last time.Time
	names       []string
	seen        map[string]bool
	reported    int
}

func newProgress() *progress { return &progress{start: time.Now(), seen: map[string]bool{}} }

func (p *progress) add(name string) {
	if name != "" && !p.seen[name] {
		p.seen[name] = true
		p.names = append(p.names, name)
	}
}

// due returns a message when the turn has run a while and used something new since the last message.
func (p *progress) due(now time.Time) string {
	if now.Sub(p.start) < progressAfter || len(p.names) == p.reported {
		return ""
	}
	if !p.last.IsZero() && now.Sub(p.last) < progressEvery {
		return ""
	}
	p.last, p.reported = now, len(p.names)
	return "⏳ Agent vẫn đang làm việc (đã dùng: " + strings.Join(p.names, ", ") + ")"
}
