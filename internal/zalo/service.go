package zalo

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/nchungdev/agent-bridge/internal/core"
	"github.com/nchungdev/agent-bridge/internal/store"
)

// Service routes Zalo messages: slash commands go to the command table, anything else becomes an agent turn.
type Service struct {
	cfg  Config
	d    Deps
	turn time.Duration // longest a single turn may run

	nasBusy sync.Mutex // one /nas service action at a time

	mu    sync.Mutex
	chats map[string]*chat
	seen  map[string]time.Time // message_id -> first seen, to drop redelivered webhooks
	ctx   context.Context      // lifetime of the background watchers (set by Start)
}

// chat is the state of one Zalo chat.
type chat struct {
	busy      sync.Mutex  // held for the whole turn: one running turn per chat
	fromZalo  atomic.Bool // the turn now running was started from Zalo (the watcher must not announce it again)
	mu        sync.Mutex  // guards the fields below
	approve   string      // id of the approval the agent is waiting on, "" when none
	listed    []string    // conversation ids of the last /convs, for /use N
	stopWatch context.CancelFunc
}

// New wires the service.
func New(cfg Config, d Deps) *Service {
	return &Service{cfg: cfg, d: d, turn: 20 * time.Minute, chats: map[string]*chat{}, seen: map[string]time.Time{}, ctx: context.Background()}
}

// Handle processes one received message. It is safe to call from many goroutines.
func (s *Service) Handle(ctx context.Context, m Message) {
	if !s.cfg.Allowed[m.From.ID] {
		// Never answer strangers; the id in the log lets the owner add themselves to ZALO_ALLOWED_IDS.
		log.Printf("zalo: ignored message from user %q (not in ZALO_ALLOWED_IDS)", m.From.ID)
		return
	}
	if m.Chat.Type != "PRIVATE" || m.Chat.ID == "" || s.duplicate(m.MessageID) {
		return
	}
	text := strings.TrimSpace(m.Text)
	if text == "" && m.Photo == "" {
		s.reply(ctx, m.Chat.ID, "Mình chỉ đọc được tin nhắn văn bản và ảnh.")
		return
	}
	c := s.chat(m.Chat.ID)
	if m.Photo == "" && strings.HasPrefix(text, "/") {
		s.command(ctx, c, m, text)
		return
	}
	if !c.busy.TryLock() {
		s.reply(ctx, m.Chat.ID, "Agent đang xử lý tin trước, gửi /stop để dừng.")
		return
	}
	defer c.busy.Unlock()
	if m.Photo != "" {
		var err error
		if text, err = s.withImage(ctx, text, m.Photo); err != nil {
			s.reply(ctx, m.Chat.ID, "Không tải được ảnh: "+err.Error())
			return
		}
	}
	s.runTurn(ctx, c, m, text)
}

// withImage saves the image and returns the prompt with its path attached.
func (s *Service) withImage(ctx context.Context, caption, photo string) (string, error) {
	path, err := s.d.Images.Save(ctx, photo)
	if err != nil {
		return "", err
	}
	if caption == "" {
		caption = "Xem ảnh này."
	}
	return caption + "\n\nAttached media:\n- " + path, nil
}

// runTurn sends text to the agent and relays what it does until the turn ends, with typing and progress.
func (s *Service) runTurn(ctx context.Context, c *chat, m Message, text string) {
	conv, err := s.ensureConv(m.Chat.ID, m.From.DisplayName)
	if err != nil {
		s.reply(ctx, m.Chat.ID, "Không tạo được hội thoại: "+err.Error())
		return
	}
	// Subscribe before sending so no event of this turn is missed.
	events, unsubscribe := s.d.Agent.Subscribe(conv)
	defer unsubscribe()
	c.fromZalo.Store(true)
	if err := s.d.Agent.Send(ctx, conv, s.engineOf(conv), core.UserInput{Text: text}); err != nil {
		c.fromZalo.Store(false)
		s.reply(ctx, m.Chat.ID, "Không gửi được cho agent: "+err.Error())
		return
	}
	_ = s.d.Messenger.Typing(ctx, m.Chat.ID)
	r := newRelay(s, c, m.Chat.ID)
	typing := time.NewTicker(5 * time.Second)
	defer typing.Stop()
	timeout := time.After(s.turn)
	for {
		select {
		case ev, ok := <-events:
			if !ok {
				r.finish(ctx)
				return
			}
			if r.handle(ctx, ev) {
				return
			}
		case <-typing.C:
			_ = s.d.Messenger.Typing(ctx, m.Chat.ID)
			r.reportProgress(ctx, time.Now())
		case <-timeout:
			c.fromZalo.Store(false)
			_ = s.d.Agent.Cancel(conv)
			s.reply(ctx, m.Chat.ID, "Quá thời gian chờ agent, đã dừng lượt này.")
			return
		case <-ctx.Done():
			return
		}
	}
}

// engineOf is the engine a conversation runs on, falling back to the configured one.
func (s *Service) engineOf(conv string) string {
	if cs, _ := s.d.Settings.GetConv(conv); cs != nil && cs.ActiveEngine != "" {
		return cs.ActiveEngine
	}
	return s.cfg.Engine
}

// ensureConv returns the chat's conversation, creating it on first use.
func (s *Service) ensureConv(chatID, name string) (string, error) {
	if conv := s.d.Settings.GetSetting(convKey(chatID)); conv != "" {
		return conv, nil
	}
	return s.createConv(chatID, name, "")
}

// createConv starts a conversation for the chat (in workspace ws, or the configured one) and attaches the chat to it.
func (s *Service) createConv(chatID, name, ws string) (string, error) {
	if ws == "" {
		ws = s.cfg.Workspace
	}
	if ws != "" && !s.d.PathOK(ws) {
		return "", fmt.Errorf("workspace %q không được phép", ws)
	}
	if name == "" {
		name = chatID
	}
	conv, err := s.d.NewConv("Zalo: "+name, ws)
	if err != nil {
		return "", err
	}
	if err := s.d.Agent.SetConv(store.ConvSettings{ConvID: conv, ActiveEngine: s.cfg.Engine, Mode: s.cfg.Mode, Workspace: ws}); err != nil {
		return "", err
	}
	if err := s.attach(chatID, conv); err != nil {
		return "", err
	}
	return conv, nil
}

// attach points the chat at a conversation and starts announcing when that conversation finishes work.
func (s *Service) attach(chatID, conv string) error {
	if err := s.d.Settings.SetSetting(convKey(chatID), conv); err != nil {
		return err
	}
	s.rememberChat(chatID)
	s.watch(chatID)
	return nil
}

func (s *Service) reply(ctx context.Context, chatID, text string) {
	if err := s.d.Messenger.SendText(ctx, chatID, text); err != nil {
		log.Printf("zalo: send failed: %v", err)
	}
}

func (s *Service) chat(id string) *chat {
	s.mu.Lock()
	defer s.mu.Unlock()
	c := s.chats[id]
	if c == nil {
		c = &chat{}
		s.chats[id] = c
	}
	return c
}

// duplicate reports whether this message id was already handled (Zalo may redeliver) and records it.
func (s *Service) duplicate(id string) bool {
	if id == "" {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	for k, t := range s.seen {
		if now.Sub(t) > 30*time.Minute {
			delete(s.seen, k)
		}
	}
	if _, ok := s.seen[id]; ok {
		return true
	}
	s.seen[id] = now
	return false
}

func convKey(chatID string) string { return "zalo.conv." + chatID }
