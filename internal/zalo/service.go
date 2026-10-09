package zalo

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/nchungdev/agent-bridge/internal/core"
	"github.com/nchungdev/agent-bridge/internal/store"
)

// Agent is the part of manager.Manager the bot needs.
type Agent interface {
	Subscribe(conv string) (<-chan core.Event, func())
	Send(ctx context.Context, conv, engineID string, in core.UserInput) error
	SetConv(c store.ConvSettings) error
	Decide(conv, approvalID string, d core.Decision) error
	Cancel(conv string) error
}

// Settings persists the chat -> conversation mapping (store.Store satisfies it).
type Settings interface {
	GetSetting(key string) string
	SetSetting(key, value string) error
}

// Sender delivers text to a Zalo chat (*Client satisfies it).
type Sender interface {
	SendText(ctx context.Context, chatID, text string) error
}

// Service turns Zalo messages into agent turns.
type Service struct {
	cfg         Config
	agent       Agent
	settings    Settings
	send        Sender
	newConv     func(name, workspace string) (string, error)
	pathOK      func(string) bool
	turnTimeout time.Duration

	mu    sync.Mutex
	chats map[string]*chat
	seen  map[string]time.Time // message_id -> first seen, to drop redelivered webhooks
}

type chat struct {
	busy    sync.Mutex // held for the whole turn: one running turn per chat
	mu      sync.Mutex
	approve string // id of the approval the agent is waiting on, "" when none
}

// New wires the service. newConv creates a conversation and returns its id; pathOK validates the workspace.
func New(cfg Config, agent Agent, settings Settings, send Sender, newConv func(name, workspace string) (string, error), pathOK func(string) bool) *Service {
	return &Service{cfg: cfg, agent: agent, settings: settings, send: send, newConv: newConv, pathOK: pathOK,
		turnTimeout: 20 * time.Minute, chats: map[string]*chat{}, seen: map[string]time.Time{}}
}

const helpText = "Lệnh: /new bắt đầu hội thoại mới, /stop dừng lượt đang chạy, /ok hoặc /no trả lời yêu cầu cấp quyền của agent, /help xem trợ giúp. Tin nhắn khác được gửi cho agent."

// Handle processes one received message. It is safe to call from many goroutines.
func (s *Service) Handle(ctx context.Context, m Message) {
	if !s.cfg.Allowed[m.From.ID] {
		// Never answer strangers; the id in the log lets the owner add themselves to ZALO_ALLOWED_IDS.
		log.Printf("zalo: ignored message from user %q (not in ZALO_ALLOWED_IDS)", m.From.ID)
		return
	}
	if m.Chat.Type != "PRIVATE" || m.Chat.ID == "" {
		return
	}
	if s.duplicate(m.MessageID) {
		return
	}
	text := strings.TrimSpace(m.Text)
	if text == "" {
		s.reply(ctx, m.Chat.ID, "Mình chỉ đọc được tin nhắn văn bản.")
		return
	}
	c := s.chat(m.Chat.ID)
	switch strings.ToLower(strings.Fields(text)[0]) {
	case "/help", "/start":
		s.reply(ctx, m.Chat.ID, helpText)
	case "/stop":
		if conv := s.settings.GetSetting(convKey(m.Chat.ID)); conv != "" {
			_ = s.agent.Cancel(conv)
		}
		s.reply(ctx, m.Chat.ID, "Đã gửi lệnh dừng.")
	case "/ok", "/no":
		s.decide(ctx, c, m.Chat.ID, strings.EqualFold(strings.Fields(text)[0], "/ok"))
	case "/new":
		if !c.busy.TryLock() {
			s.reply(ctx, m.Chat.ID, "Agent đang chạy, gửi /stop trước.")
			return
		}
		defer c.busy.Unlock()
		if _, err := s.createConv(m.Chat.ID, m.From.DisplayName); err != nil {
			s.reply(ctx, m.Chat.ID, "Không tạo được hội thoại mới: "+err.Error())
			return
		}
		s.reply(ctx, m.Chat.ID, "Đã bắt đầu hội thoại mới.")
	default:
		if !c.busy.TryLock() {
			s.reply(ctx, m.Chat.ID, "Agent đang xử lý tin trước, gửi /stop để dừng.")
			return
		}
		defer c.busy.Unlock()
		s.turn(ctx, c, m, text)
	}
}

func (s *Service) turn(ctx context.Context, c *chat, m Message, text string) {
	conv := s.settings.GetSetting(convKey(m.Chat.ID))
	if conv == "" {
		var err error
		if conv, err = s.createConv(m.Chat.ID, m.From.DisplayName); err != nil {
			s.reply(ctx, m.Chat.ID, "Không tạo được hội thoại: "+err.Error())
			return
		}
	}
	// Subscribe before sending so no event of this turn is missed.
	events, cancel := s.agent.Subscribe(conv)
	defer cancel()
	if err := s.agent.Send(ctx, conv, s.cfg.Engine, core.UserInput{Text: text}); err != nil {
		s.reply(ctx, m.Chat.ID, "Không gửi được cho agent: "+err.Error())
		return
	}
	var out strings.Builder
	timeout := time.After(s.turnTimeout)
	for {
		select {
		case ev, ok := <-events:
			if !ok {
				s.finish(ctx, m.Chat.ID, &out)
				return
			}
			switch ev.Type {
			case core.EvTextDelta:
				out.WriteString(ev.Text)
			case core.EvApprovalRequest:
				if ev.Approval != nil {
					c.mu.Lock()
					c.approve = ev.Approval.ID
					c.mu.Unlock()
					s.reply(ctx, m.Chat.ID, fmt.Sprintf("Agent xin quyền chạy %s (%s). Trả lời /ok để cho phép hoặc /no để từ chối.", ev.Approval.Tool, ev.Approval.Title))
				}
			case core.EvError:
				if ev.Err != nil {
					out.WriteString("\n[Lỗi] " + ev.Err.Message)
				}
				s.finish(ctx, m.Chat.ID, &out)
				return
			case core.EvTurnDone:
				s.finish(ctx, m.Chat.ID, &out)
				return
			}
		case <-timeout:
			_ = s.agent.Cancel(conv)
			s.reply(ctx, m.Chat.ID, "Quá thời gian chờ agent, đã dừng lượt này.")
			return
		case <-ctx.Done():
			return
		}
	}
}

func (s *Service) finish(ctx context.Context, chatID string, out *strings.Builder) {
	text := strings.TrimSpace(out.String())
	if text == "" {
		text = "(agent không trả lời bằng văn bản)"
	}
	s.reply(ctx, chatID, text)
}

func (s *Service) decide(ctx context.Context, c *chat, chatID string, allow bool) {
	conv := s.settings.GetSetting(convKey(chatID))
	c.mu.Lock()
	id := c.approve
	c.approve = ""
	c.mu.Unlock()
	if conv == "" || id == "" {
		s.reply(ctx, chatID, "Không có yêu cầu cấp quyền nào đang chờ.")
		return
	}
	if err := s.agent.Decide(conv, id, core.Decision{Allow: allow, Scope: "once", DecidedBy: "zalo"}); err != nil {
		s.reply(ctx, chatID, "Không áp dụng được quyết định: "+err.Error())
	}
}

func (s *Service) createConv(chatID, name string) (string, error) {
	ws := s.cfg.Workspace
	if ws != "" && !s.pathOK(ws) {
		return "", fmt.Errorf("workspace %q không được phép", ws)
	}
	if name == "" {
		name = chatID
	}
	conv, err := s.newConv("Zalo: "+name, ws)
	if err != nil {
		return "", err
	}
	if err := s.agent.SetConv(store.ConvSettings{ConvID: conv, ActiveEngine: s.cfg.Engine, Mode: s.cfg.Mode, Workspace: ws}); err != nil {
		return "", err
	}
	if err := s.settings.SetSetting(convKey(chatID), conv); err != nil {
		return "", err
	}
	return conv, nil
}

func (s *Service) reply(ctx context.Context, chatID, text string) {
	if err := s.send.SendText(ctx, chatID, text); err != nil {
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
