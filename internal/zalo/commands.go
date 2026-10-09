package zalo

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/nchungdev/agent-bridge/internal/core"
	"github.com/nchungdev/agent-bridge/internal/store"
)

const (
	maxListed = 8 // conversations shown by /convs
	helpText  = `Gửi tin nhắn thường để nói chuyện với agent.
/new — hội thoại mới   /stop — dừng lượt đang chạy
/ok, /no — trả lời yêu cầu cấp quyền
/status — xem engine, quyền, thư mục
/engine [tên] — xem/đổi engine   /mode [tên] — xem/đổi quyền
/ws <thư mục> — hội thoại mới ở thư mục khác
/convs — hội thoại gần đây   /use <số> — làm tiếp một hội thoại
/nas — trạng thái máy chủ (gửi /nas help để xem lệnh)`
)

// invocation is how a command was called: "/nas:sonarr restart" has Qualifier "sonarr" and Arg "restart".
type invocation struct{ Qualifier, Arg string }

// commandFunc handles one slash command.
type commandFunc func(s *Service, ctx context.Context, c *chat, m Message, in invocation)

// command is an entry of the command table. Adding a command means adding an entry, nothing else.
type command struct {
	run commandFunc
	// qualified commands accept a ":<qualifier>" after the name, as in /nas:sonarr.
	qualified bool
	// exclusive commands change the conversation and so wait until no turn is running; the others work at any time.
	exclusive bool
}

var commands = map[string]command{
	"/help":  {run: (*Service).cmdHelp},
	"/start": {run: (*Service).cmdHelp},
	"/stop":  {run: (*Service).cmdStop},
	"/ok": {run: func(s *Service, ctx context.Context, c *chat, m Message, _ invocation) {
		s.decide(ctx, c, m.Chat.ID, true)
	}},
	"/no": {run: func(s *Service, ctx context.Context, c *chat, m Message, _ invocation) {
		s.decide(ctx, c, m.Chat.ID, false)
	}},
	"/status": {run: (*Service).cmdStatus, exclusive: true},
	"/engine": {run: (*Service).cmdEngine, exclusive: true},
	"/mode":   {run: (*Service).cmdMode, exclusive: true},
	"/ws":     {run: (*Service).cmdWorkspace, exclusive: true},
	"/new":    {run: (*Service).cmdNew, exclusive: true},
	"/convs":  {run: (*Service).cmdConvs, exclusive: true},
	"/use":    {run: (*Service).cmdUse, exclusive: true},
	"/nas":    {run: (*Service).cmdNAS, qualified: true},
}

// command dispatches a slash command through the table.
func (s *Service) command(ctx context.Context, c *chat, m Message, text string) {
	name, arg, _ := strings.Cut(text, " ")
	base, qualifier, _ := strings.Cut(name, ":")
	cmd, ok := commands[strings.ToLower(base)]
	if !ok || (qualifier != "" && !cmd.qualified) {
		s.reply(ctx, m.Chat.ID, "Lệnh không có. Gửi /help để xem danh sách.")
		return
	}
	if cmd.exclusive {
		if !c.busy.TryLock() {
			s.reply(ctx, m.Chat.ID, "Agent đang chạy, gửi /stop trước.")
			return
		}
		defer c.busy.Unlock()
	}
	cmd.run(s, ctx, c, m, invocation{Qualifier: qualifier, Arg: strings.TrimSpace(arg)})
}

func (s *Service) cmdHelp(ctx context.Context, _ *chat, m Message, _ invocation) {
	s.reply(ctx, m.Chat.ID, helpText)
}

func (s *Service) cmdStop(ctx context.Context, _ *chat, m Message, _ invocation) {
	if conv := s.d.Settings.GetSetting(convKey(m.Chat.ID)); conv != "" {
		_ = s.d.Agent.Cancel(conv)
	}
	s.reply(ctx, m.Chat.ID, "Đã gửi lệnh dừng.")
}

func (s *Service) cmdNew(ctx context.Context, _ *chat, m Message, _ invocation) {
	if _, err := s.createConv(m.Chat.ID, m.From.DisplayName, ""); err != nil {
		s.reply(ctx, m.Chat.ID, "Không tạo được hội thoại mới: "+err.Error())
		return
	}
	s.reply(ctx, m.Chat.ID, "Đã bắt đầu hội thoại mới.")
}

func (s *Service) cmdStatus(ctx context.Context, _ *chat, m Message, _ invocation) {
	conv := s.d.Settings.GetSetting(convKey(m.Chat.ID))
	if conv == "" {
		s.reply(ctx, m.Chat.ID, fmt.Sprintf("Chưa có hội thoại. Mặc định: engine %s, quyền %s. Gửi tin nhắn để bắt đầu.", s.cfg.Engine, s.cfg.Mode))
		return
	}
	engine, mode, ws := s.cfg.Engine, s.cfg.Mode, s.cfg.Workspace
	if cs, _ := s.d.Settings.GetConv(conv); cs != nil {
		engine, mode, ws = orDefault(cs.ActiveEngine, engine), orDefault(cs.Mode, mode), orDefault(cs.Workspace, ws)
	}
	state := orDefault(string(s.d.Agent.State(conv, engine)), "nghỉ")
	s.reply(ctx, m.Chat.ID, fmt.Sprintf("Hội thoại %s\nEngine: %s\nQuyền: %s (cho phép đổi: %s)\nThư mục: %s\nTrạng thái: %s",
		short(conv), engine, mode, strings.Join(s.cfg.Modes, ", "), ws, state))
}

func (s *Service) cmdEngine(ctx context.Context, _ *chat, m Message, in invocation) {
	arg := in.Arg
	chatID := m.Chat.ID
	engines := s.d.Engines()
	if arg == "" {
		ids := make([]string, 0, len(engines))
		for _, e := range engines {
			ids = append(ids, e.ID)
		}
		s.reply(ctx, chatID, "Engine có sẵn: "+strings.Join(ids, ", ")+"\nĐổi bằng /engine <tên>.")
		return
	}
	i := slices.IndexFunc(engines, func(e EngineInfo) bool { return e.ID == arg })
	if i < 0 {
		s.reply(ctx, chatID, fmt.Sprintf("Không có engine %q. Gửi /engine để xem danh sách.", arg))
		return
	}
	target := engines[i]
	conv, err := s.ensureConv(chatID, m.From.DisplayName)
	if err != nil {
		s.reply(ctx, chatID, "Lỗi: "+err.Error())
		return
	}
	mode := s.cfg.Mode
	if cs, _ := s.d.Settings.GetConv(conv); cs != nil && supports(target, cs.Mode) {
		mode = cs.Mode
	}
	if !supports(target, mode) {
		s.reply(ctx, chatID, fmt.Sprintf("Engine %s không hỗ trợ quyền %s.", target.ID, mode))
		return
	}
	if err := s.d.Agent.SwitchEngine(conv, target.ID); err != nil {
		s.reply(ctx, chatID, "Không đổi được engine: "+err.Error())
		return
	}
	if err := s.d.Agent.SetConv(store.ConvSettings{ConvID: conv, ActiveEngine: target.ID, Mode: mode}); err != nil {
		s.reply(ctx, chatID, "Không lưu được engine: "+err.Error())
		return
	}
	s.reply(ctx, chatID, fmt.Sprintf("Đã đổi sang %s (quyền %s).", target.ID, mode))
}

func (s *Service) cmdMode(ctx context.Context, _ *chat, m Message, in invocation) {
	arg := in.Arg
	chatID := m.Chat.ID
	if arg == "" {
		s.reply(ctx, chatID, "Quyền cho phép đổi từ Zalo: "+strings.Join(s.cfg.Modes, ", ")+"\nĐổi bằng /mode <tên>. plan = chỉ đọc, ask = hỏi lại từng việc.")
		return
	}
	if !slices.Contains(s.cfg.Modes, arg) {
		s.reply(ctx, chatID, fmt.Sprintf("Quyền %q không được đổi từ Zalo (cho phép: %s). Đổi trên giao diện web hoặc ZALO_MODES.", arg, strings.Join(s.cfg.Modes, ", ")))
		return
	}
	conv, err := s.ensureConv(chatID, m.From.DisplayName)
	if err != nil {
		s.reply(ctx, chatID, "Lỗi: "+err.Error())
		return
	}
	engine := s.engineOf(conv)
	for _, e := range s.d.Engines() {
		if e.ID == engine && !supports(e, arg) {
			s.reply(ctx, chatID, fmt.Sprintf("Engine %s không hỗ trợ quyền %s.", engine, arg))
			return
		}
	}
	if err := s.d.Agent.SetConv(store.ConvSettings{ConvID: conv, Mode: arg}); err != nil {
		s.reply(ctx, chatID, "Không đổi được quyền: "+err.Error())
		return
	}
	s.reply(ctx, chatID, "Đã đổi quyền sang "+arg+".")
}

// cmdWorkspace starts a new conversation in another folder: a running agent keeps the folder it started in.
func (s *Service) cmdWorkspace(ctx context.Context, _ *chat, m Message, in invocation) {
	arg := in.Arg
	chatID := m.Chat.ID
	if arg == "" {
		s.reply(ctx, chatID, "Dùng: /ws <đường dẫn thư mục tuyệt đối>. Mình sẽ mở hội thoại mới ở đó.")
		return
	}
	path := filepath.Clean(arg)
	switch {
	case !filepath.IsAbs(path):
		s.reply(ctx, chatID, "Cần đường dẫn tuyệt đối, ví dụ /ws /home/chungnh/projects/app.")
		return
	case !isDir(path):
		s.reply(ctx, chatID, "Không thấy thư mục đó.")
		return
	case !s.d.PathOK(path):
		s.reply(ctx, chatID, "Thư mục này không nằm trong vùng được phép (AGENT_BRIDGE_WORKSPACE_ROOTS).")
		return
	}
	if _, err := s.createConv(chatID, m.From.DisplayName, path); err != nil {
		s.reply(ctx, chatID, "Không mở được hội thoại: "+err.Error())
		return
	}
	s.reply(ctx, chatID, "Đã mở hội thoại mới ở "+path+".")
}

func (s *Service) cmdConvs(ctx context.Context, c *chat, m Message, _ invocation) {
	list, err := s.recent()
	if err != nil {
		s.reply(ctx, m.Chat.ID, "Không đọc được danh sách: "+err.Error())
		return
	}
	if len(list) == 0 {
		s.reply(ctx, m.Chat.ID, "Chưa có hội thoại nào.")
		return
	}
	ids := make([]string, 0, len(list))
	var b strings.Builder
	b.WriteString("Hội thoại gần đây (gửi /use <số> để làm tiếp):\n")
	for i, ci := range list {
		ids = append(ids, ci.ID)
		fmt.Fprintf(&b, "%d. %s — %s, %s\n", i+1, clip(ci.Title, 50), filepath.Base(ci.Workspace), ago(ci.Updated))
	}
	c.mu.Lock()
	c.listed = ids
	c.mu.Unlock()
	s.reply(ctx, m.Chat.ID, b.String())
}

func (s *Service) cmdUse(ctx context.Context, c *chat, m Message, in invocation) {
	arg := in.Arg
	chatID := m.Chat.ID
	n, err := strconv.Atoi(arg)
	c.mu.Lock()
	ids := c.listed
	c.mu.Unlock()
	if err != nil || n < 1 || n > len(ids) {
		s.reply(ctx, chatID, "Gửi /convs trước, rồi /use <số trong danh sách>.")
		return
	}
	conv := ids[n-1]
	cs, _ := s.d.Settings.GetConv(conv)
	if cs != nil && cs.Workspace != "" && !s.d.PathOK(cs.Workspace) {
		s.reply(ctx, chatID, "Hội thoại này nằm ngoài vùng được phép.")
		return
	}
	// A conversation opened on the web may run with broader rights than Zalo may grant: bring it down.
	note := ""
	if cs != nil && cs.Mode != "" && !slices.Contains(s.cfg.Modes, cs.Mode) {
		if err := s.d.Agent.SetConv(store.ConvSettings{ConvID: conv, Mode: s.cfg.Mode}); err != nil {
			s.reply(ctx, chatID, "Không hạ được quyền của hội thoại: "+err.Error())
			return
		}
		note = fmt.Sprintf(" Quyền %s đã được hạ xuống %s vì Zalo không được dùng quyền đó.", cs.Mode, s.cfg.Mode)
	}
	if err := s.attach(chatID, conv); err != nil {
		s.reply(ctx, chatID, "Không gắn được: "+err.Error())
		return
	}
	s.reply(ctx, chatID, "Đã gắn vào hội thoại "+short(conv)+"."+note)
}

// recent lists the newest conversations the bot may open, skipping archived ones and forbidden folders.
func (s *Service) recent() ([]ConvInfo, error) {
	all, err := s.d.Convs()
	if err != nil {
		return nil, err
	}
	sort.SliceStable(all, func(i, j int) bool { return all[i].Updated.After(all[j].Updated) })
	var out []ConvInfo
	for _, ci := range all {
		if ci.Archived || (ci.Workspace != "" && !s.d.PathOK(ci.Workspace)) {
			continue
		}
		ci.Title = orDefault(ci.Title, "(chưa đặt tên)")
		if out = append(out, ci); len(out) == maxListed {
			break
		}
	}
	return out, nil
}

// decide answers the approval request the agent is waiting on.
func (s *Service) decide(ctx context.Context, c *chat, chatID string, allow bool) {
	conv := s.d.Settings.GetSetting(convKey(chatID))
	c.mu.Lock()
	id := c.approve
	c.approve = ""
	c.mu.Unlock()
	if conv == "" || id == "" {
		s.reply(ctx, chatID, "Không có yêu cầu cấp quyền nào đang chờ.")
		return
	}
	if err := s.d.Agent.Decide(conv, id, core.Decision{Allow: allow, Scope: "once", DecidedBy: "zalo"}); err != nil {
		s.reply(ctx, chatID, "Không áp dụng được quyết định: "+err.Error())
	}
}

func supports(e EngineInfo, mode string) bool {
	return len(e.Modes) == 0 || slices.Contains(e.Modes, mode)
}

func isDir(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && fi.IsDir()
}

func orDefault(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

func short(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}

func clip(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n]) + "…"
	}
	return s
}

func ago(t time.Time) string {
	d := time.Since(t)
	switch {
	case t.IsZero():
		return "?"
	case d < time.Minute:
		return "vừa xong"
	case d < time.Hour:
		return fmt.Sprintf("%d phút trước", int(d.Minutes()))
	case d < 48*time.Hour:
		return fmt.Sprintf("%d giờ trước", int(d.Hours()))
	}
	return fmt.Sprintf("%d ngày trước", int(d.Hours()/24))
}
