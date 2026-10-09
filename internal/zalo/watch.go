package zalo

import (
	"context"
	"slices"
	"strings"

	"github.com/nchungdev/agent-bridge/internal/core"
)

const (
	chatsKey    = "zalo.chats" // comma separated chat ids that have a conversation to watch
	maxAnnounce = 1500         // runes of a finished turn sent as a notification
)

// Start resumes the watchers of every chat that was attached before a restart. ctx ends them all.
func (s *Service) Start(ctx context.Context) {
	s.mu.Lock()
	s.ctx = ctx
	s.mu.Unlock()
	for _, id := range s.rememberedChats() {
		s.watch(id)
	}
}

func (s *Service) rememberedChats() []string {
	return strings.FieldsFunc(s.d.Settings.GetSetting(chatsKey), func(r rune) bool { return r == ',' })
}

func (s *Service) rememberChat(chatID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ids := s.rememberedChats()
	if slices.Contains(ids, chatID) {
		return
	}
	_ = s.d.Settings.SetSetting(chatsKey, strings.Join(append(ids, chatID), ","))
}

// watch announces on Zalo when the chat's conversation finishes a turn that did not start from Zalo, for
// example work begun in the web UI. Only the conversation attached to the chat is watched, and attaching
// the chat to another conversation replaces the watcher.
func (s *Service) watch(chatID string) {
	conv := s.d.Settings.GetSetting(convKey(chatID))
	c := s.chat(chatID)
	s.mu.Lock()
	parent := s.ctx
	s.mu.Unlock()

	c.mu.Lock()
	if c.stopWatch != nil {
		c.stopWatch()
		c.stopWatch = nil
	}
	if conv == "" {
		c.mu.Unlock()
		return
	}
	ctx, stop := context.WithCancel(parent)
	c.stopWatch = stop
	c.mu.Unlock()

	events, unsubscribe := s.d.Agent.Subscribe(conv)
	go func() {
		defer unsubscribe()
		s.announce(ctx, c, chatID, events)
	}()
}

// announce relays the end of turns that did not start from Zalo until ctx ends or the stream closes.
func (s *Service) announce(ctx context.Context, c *chat, chatID string, events <-chan core.Event) {
	var answer strings.Builder
	for {
		select {
		case <-ctx.Done():
			return
		case ev, ok := <-events:
			if !ok {
				return
			}
			switch ev.Type {
			case core.EvUserMessage:
				answer.Reset()
			case core.EvTextDelta:
				answer.WriteString(ev.Text)
			case core.EvError:
				if !c.fromZalo.Load() && ev.Err != nil {
					s.reply(ctx, chatID, "⚠️ Hội thoại gặp lỗi: "+clip(ev.Err.Message, 300))
				}
			case core.EvTurnDone:
				text := strings.TrimSpace(answer.String())
				answer.Reset()
				if c.fromZalo.Swap(false) {
					continue // started from Zalo: its answer is already sent there
				}
				if text == "" {
					text = "(không có phản hồi bằng văn bản)"
				}
				s.reply(ctx, chatID, "✅ Hội thoại đã xong việc:\n"+clip(text, maxAnnounce))
			}
		}
	}
}
