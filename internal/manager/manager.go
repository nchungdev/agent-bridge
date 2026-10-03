// Package manager owns live engine sessions: one process per (conversation,
// engine) binding, a bounded pool with LRU suspension, idle reaping, per-binding
// turn queue, policy-driven approvals and restart recovery.
package manager

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/nchungdev/agent-hub/internal/core"
	"github.com/nchungdev/agent-hub/internal/store"
)

type Config struct {
	MaxLive     int           // max simultaneously live engine processes
	IdleTimeout time.Duration // suspend idle bindings after this long
	Policy      Policy
	DefaultMode string
}

type key struct{ conv, engine string }

type live struct {
	k        key
	sess     core.Session
	state    core.State
	last     time.Time
	queue    []core.UserInput
	preamble string
	closing  bool
	model    string
	effort   string
	pending  map[string]bool
	allowed  map[string]bool // tools approved for the rest of this session
	shellSeq int64           // last event seq already reported to this engine as shell context
	oneShot  map[string]bool // approvals that must never be widened to a session scope (sudo)
}

type Manager struct {
	st      *store.Store
	engines map[string]core.Engine
	cfg     Config

	mu   sync.Mutex
	live map[key]*live
	subs map[string]map[int]chan core.Event
	next int
}

func New(st *store.Store, engines []core.Engine, cfg Config) *Manager {
	if cfg.MaxLive <= 0 {
		cfg.MaxLive = 2
	}
	if cfg.IdleTimeout <= 0 {
		cfg.IdleTimeout = 20 * time.Minute
	}
	if cfg.Policy == nil {
		cfg.Policy = NewDefaultPolicy()
	}
	if cfg.DefaultMode == "" {
		cfg.DefaultMode = "ask"
	}
	m := &Manager{st: st, engines: map[string]core.Engine{}, cfg: cfg, live: map[key]*live{}, subs: map[string]map[int]chan core.Event{}}
	for _, e := range engines {
		m.engines[e.ID()] = e
	}
	return m
}

// Recover must be called once at startup: no child process survives a restart.
func (m *Manager) Recover() (int64, error) { return m.st.ResetLiveBindings() }

// Run reaps idle sessions until ctx is done.
func (m *Manager) Run(ctx context.Context) {
	iv := m.cfg.IdleTimeout / 4
	if iv < time.Second {
		iv = time.Second
	}
	if iv > time.Minute {
		iv = time.Minute
	}
	t := time.NewTicker(iv)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-t.C:
			m.ReapIdle(now)
		}
	}
}

func (m *Manager) ReapIdle(now time.Time) int {
	m.mu.Lock()
	var victims []*live
	for _, l := range m.live {
		if l.state == core.StateIdle && now.Sub(l.last) >= m.cfg.IdleTimeout {
			victims = append(victims, l)
		}
	}
	m.mu.Unlock()
	for _, l := range victims {
		m.suspend(l)
	}
	return len(victims)
}

// Subscribe returns a channel of live events for a conversation.
func (m *Manager) Subscribe(conv string) (<-chan core.Event, func()) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.subs[conv] == nil {
		m.subs[conv] = map[int]chan core.Event{}
	}
	m.next++
	id := m.next
	ch := make(chan core.Event, 512)
	m.subs[conv][id] = ch
	return ch, func() {
		m.mu.Lock()
		defer m.mu.Unlock()
		if c, ok := m.subs[conv][id]; ok {
			delete(m.subs[conv], id)
			close(c)
		}
	}
}

func (m *Manager) publishLocked(ev core.Event) {
	for _, ch := range m.subs[ev.ConvID] {
		select {
		case ch <- ev:
		default: // slow consumer: drop; it can resync from the store via Since()
		}
	}
}

func (m *Manager) record(ev core.Event) core.Event {
	out, err := m.st.Append(ev)
	if err != nil {
		log.Printf("[manager] append: %v", err)
		out = ev
	}
	if err := m.st.ApplyEvent(out); err != nil {
		log.Printf("[manager] apply: %v", err)
	}
	return out
}

func (m *Manager) emit(ev core.Event) {
	ev = m.record(ev)
	m.mu.Lock()
	m.publishLocked(ev)
	m.mu.Unlock()
}

// setStateLocked validates and applies a transition, persists it and notifies subscribers.
func (m *Manager) setStateLocked(l *live, to core.State) {
	if !core.CanTransition(l.state, to) {
		log.Printf("[manager] illegal transition %s -> %s for %s/%s", l.state, to, l.k.conv, l.k.engine)
		return
	}
	if l.state == to {
		return
	}
	l.state = to
	b, _ := m.st.GetBinding(l.k.conv, l.k.engine)
	nb := store.Binding{ConvID: l.k.conv, Engine: l.k.engine, State: to}
	if b != nil {
		nb.LastSyncedSeq = b.LastSyncedSeq
		nb.EngineSessionID = b.EngineSessionID
	}
	if l.sess != nil && l.sess.EngineSessionID() != "" {
		nb.EngineSessionID = l.sess.EngineSessionID()
	}
	_ = m.st.UpsertBinding(nb)
	m.publishLocked(core.Event{ConvID: l.k.conv, Engine: l.k.engine, Type: core.EvStateChange, Time: time.Now(), Data: map[string]any{"state": string(to)}})
}

// State returns the live state of a binding ("" when not live).
func (m *Manager) State(conv, engine string) core.State {
	m.mu.Lock()
	defer m.mu.Unlock()
	if l := m.live[key{conv, engine}]; l != nil {
		return l.state
	}
	return ""
}

func (m *Manager) LiveCount() int { m.mu.Lock(); defer m.mu.Unlock(); return len(m.live) }

func (m *Manager) modeFor(conv string) string {
	if c, _ := m.st.GetConv(conv); c != nil && c.Mode != "" {
		return c.Mode
	}
	return m.cfg.DefaultMode
}

// SetConv configures workspace/mode for a conversation (and the live session's mode).
func (m *Manager) SetConv(c store.ConvSettings) error {
	old, _ := m.st.GetConv(c.ConvID)
	if old != nil {
		if c.ActiveEngine == "" {
			c.ActiveEngine = old.ActiveEngine
		}
		if c.Workspace == "" {
			c.Workspace = old.Workspace
		}
		if c.Mode == "" {
			c.Mode = old.Mode
		}
		if c.Model == "" {
			c.Model = old.Model
		}
		if c.Effort == "" {
			c.Effort = old.Effort
		}
	}
	if err := m.st.SetConv(c); err != nil {
		return err
	}
	m.mu.Lock()
	var sessions []core.Session
	for k, l := range m.live {
		if k.conv == c.ConvID && l.sess != nil {
			sessions = append(sessions, l.sess)
		}
	}
	m.mu.Unlock()
	for _, s := range sessions {
		_ = s.SetMode(c.Mode)
	}
	if old == nil || old.Mode != c.Mode {
		m.restartForMode(c.ConvID)
	}
	return nil
}

// restartForMode suspends idle bindings whose engine fixes the permission mode
// at process start; the next Send resumes them with the new mode.
func (m *Manager) restartForMode(conv string) {
	m.mu.Lock()
	var victims []*live
	for k, l := range m.live {
		if k.conv != conv || l.state != core.StateIdle {
			continue
		}
		if e, ok := m.engines[k.engine]; ok && e.Capabilities().ModeRequiresRestart {
			victims = append(victims, l)
		}
	}
	m.mu.Unlock()
	for _, l := range victims {
		m.suspend(l)
	}
}

func (m *Manager) ensureLive(ctx context.Context, conv, engineID string) (*live, error) {
	eng, ok := m.engines[engineID]
	if !ok {
		return nil, core.ErrNoSuchEngine
	}
	k := key{conv, engineID}
	m.mu.Lock()
	if l := m.live[k]; l != nil {
		m.mu.Unlock()
		return l, nil
	}
	m.mu.Unlock()
	// Refuse to spawn a process that is known to be unauthenticated.
	if sp, ok := eng.(core.StatusProvider); ok {
		if st := sp.Status(ctx); st.Known && (!st.Installed || !st.LoggedIn) {
			msg := engineID + " is not available: " + st.Detail
			if st.LoginHint != "" {
				msg += " — " + st.LoginHint
			}
			return nil, fmt.Errorf("%w: %s", core.ErrNotLoggedIn, msg)
		}
	}
	m.mu.Lock()
	// make room
	for len(m.live) >= m.cfg.MaxLive {
		var victim *live
		for _, l := range m.live {
			if l.state == core.StateIdle && (victim == nil || l.last.Before(victim.last)) {
				victim = l
			}
		}
		if victim == nil {
			m.mu.Unlock()
			return nil, fmt.Errorf("%w: pool full (%d live, none idle)", core.ErrBusy, len(m.live))
		}
		m.mu.Unlock()
		m.suspend(victim)
		m.mu.Lock()
	}
	if l := m.live[k]; l != nil { // raced
		m.mu.Unlock()
		return l, nil
	}
	l := &live{k: k, state: core.StateCreated, last: time.Now(), pending: map[string]bool{}, allowed: map[string]bool{}, oneShot: map[string]bool{}}
	m.live[k] = l
	m.mu.Unlock()

	b, _ := m.st.GetBinding(conv, engineID)
	conf, _ := m.st.GetConv(conv)
	opts := core.StartOpts{ConvID: conv, Mode: m.modeFor(conv)}
	if conf != nil {
		opts.Workspace, opts.Model, opts.Effort = conf.Workspace, conf.Model, conf.Effort
	}
	resumed := false
	if b != nil && b.EngineSessionID != "" && eng.Capabilities().Resume {
		opts.ResumeID = b.EngineSessionID
		resumed = true
	}
	m.mu.Lock()
	m.setStateLocked(l, core.StateStarting)
	m.mu.Unlock()
	sess, err := eng.Start(context.Background(), opts) // process lifetime is not tied to the request ctx
	if err != nil {
		m.mu.Lock()
		m.setStateLocked(l, core.StateFailed)
		delete(m.live, k)
		m.mu.Unlock()
		return nil, fmt.Errorf("start %s: %w", engineID, err)
	}
	// Build handoff for what this engine has not seen yet.
	var since int64
	if b != nil {
		since = b.LastSyncedSeq
	}
	pre := m.handoff(conv, engineID, since, resumed)
	m.mu.Lock()
	l.sess = sess
	l.model, l.effort = opts.Model, opts.Effort
	if tail, _ := m.st.Tail(conv, 1); len(tail) > 0 {
		l.shellSeq = tail[0].Seq // the handoff above already covered everything up to here
	}
	l.preamble = pre
	m.setStateLocked(l, core.StateIdle)
	m.mu.Unlock()
	go m.pump(l)
	return l, nil
}

// handoff summarises everything the engine has not seen (other engines' turns,
// working state). Empty when the engine is fully in sync.
// shellNote renders a user-run "!cmd" and its output as context text.
func shellNote(e core.Event) string {
	if e.Tool == nil {
		return ""
	}
	cmd, _ := e.Tool.Args["command"].(string)
	out := e.Tool.Output
	if r := []rune(out); len(r) > 4000 {
		out = string(r[:2000]) + "\n…[truncated]…\n" + string(r[len(r)-2000:])
	}
	code := e.Data["exit_code"]
	return fmt.Sprintf("The user ran this shell command directly (not you): $ %s\n%s\n(exit code %v)\n", cmd, out, code)
}

// shellNotesLocked returns shell commands the user ran since this binding last saw them.
func (m *Manager) shellNotesLocked(l *live) string {
	evs, err := m.st.Since(l.k.conv, l.shellSeq, 200)
	if err != nil || len(evs) == 0 {
		return ""
	}
	var sb strings.Builder
	for _, e := range evs {
		if e.Seq > l.shellSeq {
			l.shellSeq = e.Seq
		}
		if e.Type == core.EvShell {
			sb.WriteString(shellNote(e))
		}
	}
	return sb.String()
}

// Emit records an event on a conversation and publishes it to subscribers.
func (m *Manager) Emit(ev core.Event) { m.emit(ev) }

func (m *Manager) handoff(conv, engine string, since int64, resumed bool) string {
	evs, err := m.st.Since(conv, since, 400)
	if err != nil {
		return ""
	}
	var sb strings.Builder
	n := 0
	for _, e := range evs {
		switch e.Type {
		case core.EvUserMessage:
			sb.WriteString("User: " + e.Text + "\n")
			n++
		case core.EvTextDelta:
			if e.Engine != engine {
				sb.WriteString(fmt.Sprintf("[%s]: %s", e.Engine, e.Text))
				n++
			}
		case core.EvShell:
			sb.WriteString(shellNote(e))
			n++
		}
	}
	if n == 0 {
		return ""
	}
	ws, _ := m.st.GetState(conv)
	head := "Context from earlier in this conversation (handled by other engines or while you were suspended):\n"
	if len(ws.FilesChanged) > 0 {
		head += "Files changed so far: " + strings.Join(ws.FilesChanged, ", ") + "\n"
	}
	return head + sb.String()
}

// Send delivers a user message to the conversation's engine; queues if busy.
func (m *Manager) Send(ctx context.Context, conv, engineID string, in core.UserInput) error {
	l, err := m.ensureLive(ctx, conv, engineID)
	if err != nil {
		return err
	}
	// A changed model/effort needs a new process: restart an idle one (resumes the same engine session).
	if conf, _ := m.st.GetConv(conv); conf != nil {
		m.mu.Lock()
		stale := l.state == core.StateIdle && ((conf.Model != "" && conf.Model != l.model) || (conf.Effort != "" && conf.Effort != l.effort))
		m.mu.Unlock()
		if stale {
			m.suspend(l)
			if l, err = m.ensureLive(ctx, conv, engineID); err != nil {
				return err
			}
		}
	}
	m.mu.Lock()
	if l.sess == nil {
		m.mu.Unlock()
		return fmt.Errorf("%w: engine still starting", core.ErrBusy)
	}
	if l.state == core.StateRunning || l.state == core.StateAwaitingApproval {
		l.queue = append(l.queue, in)
		m.mu.Unlock()
		return nil
	}
	m.mu.Unlock()
	return m.dispatch(ctx, l, in)
}

func (m *Manager) dispatch(ctx context.Context, l *live, in core.UserInput) error {
	m.mu.Lock()
	if l.preamble != "" && in.Preamble == "" {
		in.Preamble = l.preamble
	}
	l.preamble = ""
	if notes := m.shellNotesLocked(l); notes != "" {
		in.Preamble += notes
	}
	l.last = time.Now()
	m.setStateLocked(l, core.StateRunning)
	sess := l.sess
	m.mu.Unlock()
	m.emit(core.Event{ConvID: l.k.conv, Engine: l.k.engine, Type: core.EvUserMessage, Text: in.Text})
	if err := sess.Send(ctx, in); err != nil {
		m.mu.Lock()
		m.setStateLocked(l, core.StateFailed)
		m.mu.Unlock()
		return err
	}
	return nil
}

func (m *Manager) pump(l *live) {
	for ev := range l.sess.Events() {
		ev.ConvID, ev.Engine = l.k.conv, l.k.engine
		if ev.Time.IsZero() {
			ev.Time = time.Now()
		}
		switch ev.Type {
		case core.EvApprovalRequest:
			if ev.Approval != nil {
				m.handleApproval(l, ev)
				continue
			}
		case core.EvError:
			if ev.Err != nil {
				switch ev.Err.Kind {
				case "quota_exceeded", "auth", "crashed":
					rec := m.record(ev)
					m.mu.Lock()
					m.publishLocked(rec)
					m.setStateLocked(l, core.StateFailed)
					m.mu.Unlock()
					go m.teardown(l) // free the pool slot; the next Send restarts/resumes
					continue
				}
			}
		case core.EvTurnDone:
			rec := m.record(ev)
			m.mu.Lock()
			m.publishLocked(rec)
			b, _ := m.st.GetBinding(l.k.conv, l.k.engine)
			nb := store.Binding{ConvID: l.k.conv, Engine: l.k.engine, State: l.state, EngineSessionID: l.sess.EngineSessionID(), LastSyncedSeq: rec.Seq}
			if b != nil && nb.EngineSessionID == "" {
				nb.EngineSessionID = b.EngineSessionID
			}
			_ = m.st.UpsertBinding(nb)
			l.last = time.Now()
			m.setStateLocked(l, core.StateIdle)
			var next *core.UserInput
			if len(l.queue) > 0 {
				n := l.queue[0]
				l.queue = l.queue[1:]
				next = &n
			}
			m.mu.Unlock()
			if next != nil {
				if err := m.dispatch(context.Background(), l, *next); err != nil {
					log.Printf("[manager] queued dispatch: %v", err)
				}
			}
			continue
		}
		m.emit(ev)
	}
	// process ended
	m.mu.Lock()
	wasClosing := l.closing
	if !wasClosing && l.state != core.StateFailed && l.state != core.StateStopped {
		m.setStateLocked(l, core.StateFailed)
		m.mu.Unlock()
		m.emit(core.Event{ConvID: l.k.conv, Engine: l.k.engine, Type: core.EvError, Err: &core.ErrInfo{Kind: "crashed", Message: "engine process ended unexpectedly"}})
		m.mu.Lock()
	}
	if m.live[l.k] == l {
		delete(m.live, l.k)
	}
	m.mu.Unlock()
}

// teardown closes a failed binding's process and drops it from the pool.
func (m *Manager) teardown(l *live) {
	m.mu.Lock()
	if l.closing {
		m.mu.Unlock()
		return
	}
	l.closing = true
	if m.live[l.k] == l {
		delete(m.live, l.k)
	}
	sess := l.sess
	m.mu.Unlock()
	if sess != nil {
		_ = sess.Close()
	}
}

func (m *Manager) handleApproval(l *live, ev core.Event) {
	a := ev.Approval
	if err := m.st.AddApproval(l.k.conv, l.k.engine, *a); err != nil {
		log.Printf("[manager] add approval: %v", err)
	}
	rec := m.record(ev)
	m.mu.Lock()
	mode := m.modeFor(l.k.conv)
	verdict := m.cfg.Policy.Evaluate(mode, *a)
	if verdict == AskAlways {
		l.oneShot[a.ID] = true
		verdict = Ask
	} else if verdict == Ask && l.allowed[a.Tool] {
		verdict = Allow
	}
	if verdict == Ask {
		// Register as pending BEFORE publishing so an immediate GUI reply cannot race it.
		l.pending[a.ID] = true
		m.setStateLocked(l, core.StateAwaitingApproval)
	}
	m.publishLocked(rec)
	m.mu.Unlock()
	switch verdict {
	case Allow:
		_ = m.resolve(l, a.ID, core.Decision{Allow: true, Scope: "once", DecidedBy: "policy"})
	case Deny:
		_ = m.resolve(l, a.ID, core.Decision{Allow: false, Scope: "once", DecidedBy: "policy", Reason: "denied by policy (" + mode + ")"})
	}
}

func (m *Manager) resolve(l *live, id string, d core.Decision) error {
	if err := m.st.ResolveApproval(id, d); err != nil {
		return err
	}
	if err := l.sess.Decide(id, d); err != nil {
		return err
	}
	m.mu.Lock()
	if d.Allow && d.Scope == "session" && !l.oneShot[id] {
		if pend, _ := m.st.ApprovalTool(id); pend != "" {
			l.allowed[pend] = true
		}
	}
	delete(l.pending, id)
	if len(l.pending) == 0 && l.state == core.StateAwaitingApproval {
		m.setStateLocked(l, core.StateRunning)
	}
	m.mu.Unlock()
	m.emit(core.Event{ConvID: l.k.conv, Engine: l.k.engine, Type: core.EvApprovalResolved,
		Data: map[string]any{"id": id, "allow": d.Allow, "scope": d.Scope, "by": d.DecidedBy}})
	return nil
}

// Decide answers a pending approval (from the GUI).
func (m *Manager) Decide(conv, approvalID string, d core.Decision) error {
	m.mu.Lock()
	var target *live
	for k, l := range m.live {
		if k.conv == conv && l.pending[approvalID] {
			target = l
		}
	}
	m.mu.Unlock()
	if target == nil {
		return core.ErrNoApproval
	}
	if d.DecidedBy == "" {
		d.DecidedBy = "user"
	}
	return m.resolve(target, approvalID, d)
}

// Cancel aborts the current turn and clears queued input; the process stays alive.
func (m *Manager) Cancel(conv string) error {
	m.mu.Lock()
	var sessions []core.Session
	for k, l := range m.live {
		if k.conv == conv {
			l.queue = nil
			sessions = append(sessions, l.sess)
		}
	}
	m.mu.Unlock()
	for _, s := range sessions {
		_ = s.Cancel()
	}
	return nil
}

func (m *Manager) suspend(l *live) {
	m.mu.Lock()
	if l.closing {
		m.mu.Unlock()
		return
	}
	l.closing = true
	sess := l.sess
	id := ""
	if sess != nil {
		id = sess.EngineSessionID()
	}
	m.setStateLocked(l, core.StateSuspended)
	b, _ := m.st.GetBinding(l.k.conv, l.k.engine)
	nb := store.Binding{ConvID: l.k.conv, Engine: l.k.engine, State: core.StateSuspended, EngineSessionID: id}
	if b != nil {
		nb.LastSyncedSeq = b.LastSyncedSeq
	}
	_ = m.st.UpsertBinding(nb)
	delete(m.live, l.k)
	m.mu.Unlock()
	if sess != nil {
		_ = sess.Close()
	}
}

// Stop ends a binding for good (process closed, state stopped).
func (m *Manager) Stop(conv, engine string) {
	m.mu.Lock()
	l := m.live[key{conv, engine}]
	m.mu.Unlock()
	if l != nil {
		m.suspend(l)
	}
	if b, _ := m.st.GetBinding(conv, engine); b != nil {
		b.State = core.StateStopped
		_ = m.st.UpsertBinding(*b)
	}
}

// SwitchEngine makes `to` the active engine. The previous engine must be idle
// (or not live); it is suspended and `to` will receive a handoff on its next turn.
func (m *Manager) SwitchEngine(conv, to string) error {
	if _, ok := m.engines[to]; !ok {
		return core.ErrNoSuchEngine
	}
	c, _ := m.st.GetConv(conv)
	from := ""
	if c != nil {
		from = c.ActiveEngine
	}
	if from == to {
		return nil
	}
	m.mu.Lock()
	var old *live
	if from != "" {
		old = m.live[key{conv, from}]
	}
	if old != nil && old.state != core.StateIdle {
		m.mu.Unlock()
		return fmt.Errorf("%w: %s is %s; cancel or wait", core.ErrBusy, from, old.state)
	}
	m.mu.Unlock()
	if old != nil {
		m.suspend(old)
	}
	cs := store.ConvSettings{ConvID: conv, ActiveEngine: to}
	if c != nil {
		cs.Mode, cs.Workspace = c.Mode, c.Workspace
	}
	if err := m.st.SetConv(cs); err != nil {
		return err
	}
	m.emit(core.Event{ConvID: conv, Engine: to, Type: core.EvEngineSwitch, Data: map[string]any{"from": from, "to": to}})
	return nil
}

// Shutdown suspends every live session (graceful service stop).
func (m *Manager) Shutdown() {
	m.mu.Lock()
	var all []*live
	for _, l := range m.live {
		all = append(all, l)
	}
	m.mu.Unlock()
	for _, l := range all {
		m.suspend(l)
	}
}
