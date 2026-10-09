package zalo

import (
	"context"
	"fmt"
	"slices"
	"strings"
)

const nasHelp = `Lệnh máy chủ, chạy trực tiếp trên máy (không qua agent):
/nas — trạng thái máy, ổ cứng, nhiệt độ, CPU, RAM
/nas service-list — các service cài trên máy
/nas:<service> — trạng thái một service
/nas:<service> start|stop|restart|update — điều khiển service
/nas help — danh sách này`

// nasActions are the verbs /nas:<service> accepts.
var nasActions = []string{"start", "stop", "restart", "update"}

// cmdNAS serves the /nas family. None of it goes through an agent.
func (s *Service) cmdNAS(ctx context.Context, _ *chat, m Message, in invocation) {
	chatID := m.Chat.ID
	if s.d.Host == nil {
		s.reply(ctx, chatID, "Lệnh máy chủ chưa được bật.")
		return
	}
	sub := strings.ToLower(in.Qualifier)
	if sub == "" {
		sub = strings.ToLower(in.Arg)
	}
	switch {
	case in.Qualifier == "" && in.Arg == "":
		s.nasOverview(ctx, chatID)
	case sub == "help" && (in.Qualifier == "" || in.Arg == ""):
		s.reply(ctx, chatID, nasHelp)
	case sub == "service-list" && (in.Qualifier == "" || in.Arg == ""):
		s.nasServiceList(ctx, chatID)
	case in.Qualifier == "":
		s.reply(ctx, chatID, "Không hiểu \"/nas "+in.Arg+"\". Gửi /nas help để xem lệnh.")
	default:
		s.nasService(ctx, chatID, in.Qualifier, strings.ToLower(in.Arg))
	}
}

func (s *Service) nasOverview(ctx context.Context, chatID string) {
	_ = s.d.Messenger.Typing(ctx, chatID)
	out, err := s.d.Host.Overview(ctx)
	if err != nil {
		s.reply(ctx, chatID, "❌ Không đọc được trạng thái máy: "+err.Error())
		return
	}
	s.reply(ctx, chatID, out)
}

func (s *Service) nasServiceList(ctx context.Context, chatID string) {
	list, err := s.d.Host.Services(ctx)
	var b strings.Builder
	if err != nil {
		b.WriteString("⚠️ Không liệt kê được một phần: " + err.Error() + "\n")
	}
	if len(list) == 0 {
		s.reply(ctx, chatID, b.String()+"Không có service nào.")
		return
	}
	fmt.Fprintf(&b, "%d service:\n", len(list))
	for _, sv := range list {
		fmt.Fprintf(&b, "%s — %s\n", sv.Name, sv.Description)
	}
	s.reply(ctx, chatID, strings.TrimSpace(b.String()))
}

// nasService reports on a service, or starts/stops/restarts/updates it when an action is given.
func (s *Service) nasService(ctx context.Context, chatID, name, action string) {
	if action == "" {
		out, err := s.d.Host.ServiceStatus(ctx, name)
		s.replyResult(ctx, chatID, out, err)
		return
	}
	if !slices.Contains(nasActions, action) {
		s.reply(ctx, chatID, fmt.Sprintf("Thao tác %q không có. Dùng: %s.", action, strings.Join(nasActions, ", ")))
		return
	}
	if !s.cfg.NASControl {
		s.reply(ctx, chatID, "Điều khiển service đang tắt. Bật bằng ZALO_NAS_CONTROL=1 rồi restart Agent Bridge.")
		return
	}
	if action != "start" && s.protected(name) {
		s.reply(ctx, chatID, fmt.Sprintf("%s là service bot cần để hoạt động nên không %s từ Zalo được. Hãy dùng SSH.", name, action))
		return
	}
	if !s.nasBusy.TryLock() {
		s.reply(ctx, chatID, "Đang có một thao tác service khác chạy, đợi nó xong.")
		return
	}
	defer s.nasBusy.Unlock()
	s.reply(ctx, chatID, "⏳ "+nasProgress(action, name))
	out, err := s.d.Host.ServiceAction(ctx, name, action)
	s.replyResult(ctx, chatID, out, err)
}

func nasProgress(action, name string) string {
	if action == "update" {
		return "Đang kéo image mới và cập nhật " + name + ", có thể mất vài phút…"
	}
	return "Đang " + action + " " + name + "…"
}

// protected reports whether name is one of the services Zalo may not take down.
func (s *Service) protected(name string) bool {
	lower := strings.ToLower(name)
	return slices.ContainsFunc(s.cfg.NASProtected, func(p string) bool { return strings.Contains(lower, strings.ToLower(p)) })
}

// replyResult sends a command's output, or its error.
func (s *Service) replyResult(ctx context.Context, chatID, out string, err error) {
	if err != nil {
		s.reply(ctx, chatID, "❌ "+err.Error())
		return
	}
	s.reply(ctx, chatID, out)
}
