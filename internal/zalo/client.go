package zalo

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// maxText is the documented sendMessage limit (1-2000 characters); stay a little under it.
const maxText = 1900

// Client calls the Zalo Bot API. The token is part of the URL path, so errors are scrubbed before they
// are returned or logged.
type Client struct {
	token string
	base  string
	http  *http.Client
}

func NewClient(cfg Config) *Client {
	return &Client{token: cfg.Token, base: cfg.APIBase, http: &http.Client{Timeout: 20 * time.Second}}
}

// SendText sends text to a chat, split into several messages when longer than the API allows.
func (c *Client) SendText(ctx context.Context, chatID, text string) error {
	var firstErr error
	for _, part := range splitText(text, maxText) {
		if err := c.call(ctx, "sendMessage", map[string]any{"chat_id": chatID, "text": part}); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func (c *Client) call(ctx context.Context, method string, body any) error {
	raw, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+"/bot"+c.token+"/"+method, bytes.NewReader(raw))
	if err != nil {
		return c.scrub(err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return c.scrub(err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	var out struct {
		OK          bool   `json:"ok"`
		Description string `json:"description"`
		ErrorCode   int    `json:"error_code"`
	}
	_ = json.Unmarshal(data, &out)
	if resp.StatusCode/100 != 2 || !out.OK {
		return fmt.Errorf("zalo %s: http %d, ok=%v, code=%d %s", method, resp.StatusCode, out.OK, out.ErrorCode, out.Description)
	}
	return nil
}

func (c *Client) scrub(err error) error {
	return errors.New(strings.ReplaceAll(err.Error(), c.token, "<token>"))
}

// splitText cuts s into pieces of at most n runes, preferring to break at a newline or space.
func splitText(s string, n int) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	var parts []string
	r := []rune(s)
	for len(r) > n {
		cut := n
		for i := n; i > n/2; i-- {
			if r[i-1] == '\n' {
				cut = i
				break
			}
			if r[i-1] == ' ' && cut == n {
				cut = i
			}
		}
		if p := strings.TrimSpace(string(r[:cut])); p != "" {
			parts = append(parts, p)
		}
		r = r[cut:]
	}
	if p := strings.TrimSpace(string(r)); p != "" {
		parts = append(parts, p)
	}
	return parts
}
