package zalo

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"io"
	"net/http"
	"time"
)

// WebhookPath is where Zalo posts events. Only this path should be exposed to the internet.
const WebhookPath = "/api/zalo/webhook"

// Message is the part of a Zalo update the bot uses.
type Message struct {
	MessageID string
	Text      string
	From      struct{ ID, DisplayName string }
	Chat      struct{ ID, Type string }
}

type update struct {
	OK     bool `json:"ok"`
	Result struct {
		EventName string `json:"event_name"`
		Message   struct {
			MessageID string `json:"message_id"`
			Text      string `json:"text"`
			From      struct {
				ID          string `json:"id"`
				DisplayName string `json:"display_name"`
				IsBot       bool   `json:"is_bot"`
			} `json:"from"`
			Chat struct {
				ID   string `json:"id"`
				Type string `json:"chat_type"`
			} `json:"chat"`
		} `json:"message"`
	} `json:"result"`
}

// Handler authenticates a webhook call by the X-Bot-Api-Secret-Token header, answers 2xx at once (Zalo
// reports slow endpoints as unreachable) and processes the message in the background.
func (s *Service) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		got := r.Header.Get("X-Bot-Api-Secret-Token")
		if subtle.ConstantTimeCompare([]byte(got), []byte(s.cfg.Secret)) != 1 {
			http.Error(w, `{"message":"Unauthorized"}`, http.StatusForbidden)
			return
		}
		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
		if err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		var u update
		if json.Unmarshal(body, &u) == nil && u.Result.EventName != "" && !u.Result.Message.From.IsBot {
			msg := Message{MessageID: u.Result.Message.MessageID}
			msg.From.ID, msg.From.DisplayName = u.Result.Message.From.ID, u.Result.Message.From.DisplayName
			msg.Chat.ID, msg.Chat.Type = u.Result.Message.Chat.ID, u.Result.Message.Chat.Type
			if u.Result.EventName == "message.text.received" {
				msg.Text = u.Result.Message.Text
			}
			go func() {
				ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
				defer cancel()
				s.Handle(ctx, msg)
			}()
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	})
}
