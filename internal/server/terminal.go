package server

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/creack/pty"

	"github.com/han/runic/internal/protocol"
)

const (
	terminalBacklogLimit = 1024 * 1024
	terminalClientBuffer = 256
)

// byteRing is a fixed-capacity ring buffer of raw terminal output. Writes never
// allocate after construction; Bytes() returns the retained tail in order. It
// replaces the previous "re-slice the whole backlog on every read" approach,
// which allocated ~1MB per 32KB read on busy terminals.
type byteRing struct {
	buf  []byte
	w    int
	full bool
}

func newByteRing(size int) *byteRing { return &byteRing{buf: make([]byte, size)} }

func (r *byteRing) write(p []byte) {
	size := len(r.buf)
	if size == 0 {
		return
	}
	if len(p) >= size {
		copy(r.buf, p[len(p)-size:])
		r.w = 0
		r.full = true
		return
	}
	end := r.w + len(p)
	if end <= size {
		copy(r.buf[r.w:], p)
	} else {
		k := size - r.w
		copy(r.buf[r.w:], p[:k])
		copy(r.buf, p[k:])
		r.full = true
	}
	if end >= size {
		r.full = true
	}
	r.w = end % size
}

func (r *byteRing) bytes() []byte {
	if !r.full {
		return append([]byte(nil), r.buf[:r.w]...)
	}
	out := make([]byte, len(r.buf))
	n := copy(out, r.buf[r.w:])
	copy(out[n:], r.buf[:r.w])
	return out
}

type TerminalManager struct {
	mu       sync.Mutex
	sessions map[string]*TerminalPTY
	layouts  map[string][]byte // session -> opaque client layout blob
	nextName uint64            // monotonic source of unique pane names
	// onAttention is handed to every pane so an attention transition can be
	// notified on; see attention.go.
	onAttention func(AttentionEvent)
	// sessionDir resolves the directory a session's panes should start in — its
	// git worktree, when it owns one. Nil (or an empty result) means the pane
	// inherits the daemon's working directory, the behavior without worktrees.
	sessionDir func(session string) string
}

type TerminalPTY struct {
	// pane is immutable for the pane's lifetime and is its real identity.
	// session is a label that moves when the session is renamed, so it is
	// guarded by mu and must be read through sessionName().
	pane  string
	ptmx  *os.File
	cmd   *exec.Cmd
	rcDir string

	session string

	mu      sync.Mutex
	clients map[chan []byte]bool
	backlog *byteRing
	done    bool

	// Self-reported attention state (`ru mark`), guarded by mu.
	attention   protocol.Attention
	note        string
	since       time.Time
	onAttention func(AttentionEvent)
}

// promptMarker is the OSC 133 "prompt starts here" sequence the pane's shell rc
// emits from its precmd hook. Seeing it means the shell is back at a prompt, so
// whatever was running in the pane has exited.
const promptMarker = "\x1b]133;A\a"

func NewTerminalManager() *TerminalManager {
	return &TerminalManager{
		sessions: make(map[string]*TerminalPTY),
		layouts:  make(map[string][]byte),
	}
}

func terminalKey(session, pane string) string {
	if session == "" {
		session = "default"
	}
	if pane == "" {
		pane = "main"
	}
	return session + "\x00" + pane
}

func (m *TerminalManager) GetOrCreate(session, pane string, cols, rows int) (*TerminalPTY, error) {
	key := terminalKey(session, pane)
	m.mu.Lock()
	if t := m.sessions[key]; t != nil && !t.isDone() {
		m.mu.Unlock()
		t.Resize(cols, rows)
		return t, nil
	}
	delete(m.sessions, key)
	resolve := m.sessionDir
	m.mu.Unlock()

	dir := ""
	if resolve != nil {
		dir = resolve(session)
	}
	t, err := startTerminalPTY(session, pane, cols, rows, dir)
	if err != nil {
		return nil, err
	}

	m.mu.Lock()
	if existing := m.sessions[key]; existing != nil && !existing.isDone() {
		m.mu.Unlock()
		t.discard() // lost the create race: kill AND reap (no readLoop will)
		existing.Resize(cols, rows)
		return existing, nil
	}
	t.onAttention = m.onAttention
	m.sessions[key] = t
	m.mu.Unlock()

	go func() {
		t.readLoop()
		m.forget(t)
	}()
	return t, nil
}

// forget drops a pane from the registry by identity rather than by the key it
// was registered under, because a session rename re-keys it (RenameSession). A
// key-based delete would leave the dead pane in the map under its new key.
func (m *TerminalManager) forget(t *TerminalPTY) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for key, cur := range m.sessions {
		if cur == t {
			delete(m.sessions, key)
		}
	}
}

// ListAll returns every live pane across all sessions.
func (m *TerminalManager) ListAll() []protocol.TerminalInfo {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []protocol.TerminalInfo
	for _, t := range m.sessions {
		if !t.isDone() {
			a, note, since := t.Attention()
			out = append(out, protocol.TerminalInfo{
				Session:   t.sessionName(),
				Pane:      t.pane,
				Attention: a,
				Note:      note,
				Since:     since,
			})
		}
	}
	return out
}

func (m *TerminalManager) Kill(session, pane string) {
	key := terminalKey(session, pane)
	m.mu.Lock()
	t := m.sessions[key]
	delete(m.sessions, key)
	m.mu.Unlock()
	if t != nil {
		t.Kill()
	}
}

// Reset kills every pane and forgets all persisted layouts (used by `:reset`).
func (m *TerminalManager) Reset() {
	m.mu.Lock()
	m.layouts = make(map[string][]byte)
	m.mu.Unlock()
	m.KillAll()
}

func (m *TerminalManager) KillAll() {
	m.mu.Lock()
	terms := make([]*TerminalPTY, 0, len(m.sessions))
	for _, t := range m.sessions {
		terms = append(terms, t)
	}
	m.sessions = make(map[string]*TerminalPTY)
	m.mu.Unlock()
	for _, t := range terms {
		t.Kill()
	}
}

// SetSessionDirResolver installs the lookup that maps a session to the
// directory its panes start in (its git worktree, when it has one).
func (m *TerminalManager) SetSessionDirResolver(fn func(session string) string) {
	m.mu.Lock()
	m.sessionDir = fn
	m.mu.Unlock()
}

func startTerminalPTY(session, pane string, cols, rows int, dir string) (*TerminalPTY, error) {
	if !ptySupported {
		return nil, fmt.Errorf("interactive panes need a PTY, which is not available on %s", runtime.GOOS)
	}
	if session == "" {
		session = "default"
	}
	if pane == "" {
		pane = "main"
	}
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/sh"
	}
	cmd, rcDir := terminalShellCommand(shell)
	env := cmd.Env
	if env == nil {
		env = os.Environ()
	}
	// RUNIC_PANE lets a process in the pane report its own attention state back
	// with `ru mark` without having to guess which pane it is in.
	cmd.Env = append(env, "RUNIC_SESSION="+session, "RUNIC_PANE="+pane)
	// A worktree session's panes open in its own checkout. A stale path (the
	// directory removed behind the daemon's back) must not make the pane
	// unstartable, so fall back to the daemon's cwd.
	if dir != "" {
		if st, err := os.Stat(dir); err == nil && st.IsDir() {
			cmd.Dir = dir
		}
	}

	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{Rows: uint16(rows), Cols: uint16(cols)})
	if err != nil {
		if rcDir != "" {
			os.RemoveAll(rcDir)
		}
		return nil, err
	}
	return &TerminalPTY{
		session: session,
		pane:    pane,
		ptmx:    ptmx,
		cmd:     cmd,
		rcDir:   rcDir,
		clients: make(map[chan []byte]bool),
		backlog: newByteRing(terminalBacklogLimit),
	}, nil
}

// Open creates a fresh pane in a session with a unique, restart-stable name
// (server-assigned, never recycled within a daemon lifetime) and returns it.
func (m *TerminalManager) Open(session string, cols, rows int) (string, error) {
	m.mu.Lock()
	m.nextName++
	pane := fmt.Sprintf("p%d", m.nextName)
	m.mu.Unlock()
	if _, err := m.GetOrCreate(session, pane, cols, rows); err != nil {
		return "", err
	}
	return pane, nil
}

// ListLayout returns a session's persisted layout blob plus the names of its
// panes whose PTYs are still alive.
func (m *TerminalManager) ListLayout(session string) ([]byte, []string) {
	if session == "" {
		session = "default"
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	blob := append([]byte(nil), m.layouts[session]...)
	var alive []string
	for _, t := range m.sessions {
		if t.sessionName() == session && !t.isDone() {
			alive = append(alive, t.pane)
		}
	}
	return blob, alive
}

// SetLayout persists a session's layout blob and reaps any pane in that session
// not present in keep (panes the client closed or dropped from the layout).
func (m *TerminalManager) SetLayout(session string, blob []byte, keep []string) {
	if session == "" {
		session = "default"
	}
	keepSet := make(map[string]bool, len(keep))
	for _, p := range keep {
		keepSet[p] = true
	}
	m.mu.Lock()
	m.layouts[session] = append([]byte(nil), blob...)
	var doomed []*TerminalPTY
	for key, t := range m.sessions {
		if t.sessionName() == session && !keepSet[t.pane] {
			doomed = append(doomed, t)
			delete(m.sessions, key)
		}
	}
	m.mu.Unlock()
	for _, t := range doomed {
		t.Kill()
	}
}

// RenameSession migrates everything the manager holds for a session to a new
// name: the persisted layout blob and every live pane, which is re-keyed and
// told its new session.
//
// Moving the panes is what keeps one identity for the session. Leaving them
// under the old name would split it in two: the session's attention roll-up,
// its layout and its delete would look at the new name while the running shells
// still answered to the old one — so an agent's "waiting on you" would vanish
// from the tree and its panes would survive a delete.
//
// The panes' own identity is their name (server-assigned, never recycled), so
// nothing about them has to change but the label. Shells started before the
// rename still have the old RUNIC_SESSION in their environment; `ru mark`
// resolves those by pane name instead — see TerminalManager.Mark.
func (m *TerminalManager) RenameSession(oldName, newName string) {
	if oldName == "" {
		oldName = "default"
	}
	if newName == "" {
		newName = "default"
	}
	if oldName == newName {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if blob, ok := m.layouts[oldName]; ok {
		m.layouts[newName] = blob
		delete(m.layouts, oldName)
	}
	for key, t := range m.sessions {
		if t.sessionName() != oldName {
			continue
		}
		delete(m.sessions, key)
		t.setSessionName(newName)
		m.sessions[terminalKey(newName, t.pane)] = t
	}
}

// DropSession removes all persisted state for a session: it deletes the layout
// blob and kills any live panes. Used when a session is deleted outright.
func (m *TerminalManager) DropSession(session string) {
	if session == "" {
		session = "default"
	}
	m.mu.Lock()
	delete(m.layouts, session)
	var doomed []*TerminalPTY
	for key, t := range m.sessions {
		if t.sessionName() == session {
			doomed = append(doomed, t)
			delete(m.sessions, key)
		}
	}
	m.mu.Unlock()
	for _, t := range doomed {
		t.Kill()
	}
}

func terminalShellCommand(shell string) (*exec.Cmd, string) {
	name := filepath.Base(shell)
	switch name {
	case "zsh":
		if rcDir, err := setupTerminalZshRC(); err == nil {
			cmd := exec.Command(shell)
			cmd.Env = append(os.Environ(), "ZDOTDIR="+rcDir, "RUNIC_ORIG_ZDOTDIR="+origZDOTDIR())
			return cmd, rcDir
		}
	case "bash":
		if rcDir, rcFile, err := setupTerminalBashRC(); err == nil {
			return exec.Command(shell, "--rcfile", rcFile, "-i"), rcDir
		}
	}
	return exec.Command(shell), ""
}

func setupTerminalZshRC() (string, error) {
	rcDir, err := os.MkdirTemp("", "runic-zsh-*")
	if err != nil {
		return "", err
	}
	rc := `if [ -n "$RUNIC_ORIG_ZDOTDIR" ] && [ -r "$RUNIC_ORIG_ZDOTDIR/.zshrc" ]; then
  source "$RUNIC_ORIG_ZDOTDIR/.zshrc"
elif [ -r "$HOME/.zshrc" ]; then
  source "$HOME/.zshrc"
fi
function __runic_prompt_marker() {
  printf '\033]133;A\a'
  printf '\033]7;file://%s%s\a' "${HOST}" "${PWD}"
}
autoload -Uz add-zsh-hook
add-zsh-hook precmd __runic_prompt_marker
`
	if err := os.WriteFile(filepath.Join(rcDir, ".zshrc"), []byte(rc), 0600); err != nil {
		os.RemoveAll(rcDir)
		return "", err
	}
	return rcDir, nil
}

func setupTerminalBashRC() (string, string, error) {
	rcDir, err := os.MkdirTemp("", "runic-bash-*")
	if err != nil {
		return "", "", err
	}
	rcFile := filepath.Join(rcDir, "bashrc")
	rc := `if [ -r "$HOME/.bashrc" ]; then
  source "$HOME/.bashrc"
fi
__runic_prompt_marker() {
  printf '\033]133;A\a'
  printf '\033]7;file://%s%s\a' "${HOSTNAME}" "${PWD}"
}
if [ -n "$PROMPT_COMMAND" ]; then
  PROMPT_COMMAND="__runic_prompt_marker; $PROMPT_COMMAND"
else
  PROMPT_COMMAND="__runic_prompt_marker"
fi
`
	if err := os.WriteFile(rcFile, []byte(rc), 0600); err != nil {
		os.RemoveAll(rcDir)
		return "", "", err
	}
	return rcDir, rcFile, nil
}

func origZDOTDIR() string {
	if zdotdir := os.Getenv("ZDOTDIR"); zdotdir != "" {
		return zdotdir
	}
	return os.Getenv("HOME")
}

// sessionName reports the session the pane currently belongs to. Callers on the
// manager hold m.mu and then take t.mu here; nothing takes m.mu while holding
// t.mu, so that order is the only one and cannot deadlock.
func (t *TerminalPTY) sessionName() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.session
}

func (t *TerminalPTY) setSessionName(name string) {
	t.mu.Lock()
	t.session = name
	t.mu.Unlock()
}

func (t *TerminalPTY) isDone() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.done
}

func (t *TerminalPTY) readLoop() {
	buf := make([]byte, 32*1024)
	for {
		n, err := t.ptmx.Read(buf)
		if n > 0 {
			t.broadcast(buf[:n])
		}
		if err != nil {
			t.finish()
			return
		}
	}
}

func (t *TerminalPTY) broadcast(data []byte) {
	cp := append([]byte(nil), data...)
	t.mu.Lock()
	t.backlog.write(cp)
	ev := t.noteOutputLocked(cp)
	for ch := range t.clients {
		select {
		case ch <- cp:
		default:
			// Subscriber is too far behind to keep up (deeply buffered, so this
			// only happens to a genuinely wedged client). Disconnect it rather
			// than block the PTY reader or silently drop a chunk and corrupt
			// its view: closing the channel ends its attach handler, the client
			// sees the drop and reattaches, replaying the backlog to resync.
			delete(t.clients, ch)
			close(ch)
		}
	}
	t.mu.Unlock()
	t.fire(ev)
}

func (t *TerminalPTY) finish() {
	t.mu.Lock()
	if t.done {
		t.mu.Unlock()
		return
	}
	t.done = true
	for ch := range t.clients {
		close(ch)
	}
	t.clients = nil
	t.mu.Unlock()
	if t.rcDir != "" {
		os.RemoveAll(t.rcDir)
	}
	t.waitProcess()
}

// waitProcess reaps the child, escalating to SIGKILL if it ignores the earlier
// SIGTERM. Without the bound, a shell that traps SIGTERM would block this
// goroutine (and the registry's pane-removal goroutine) forever.
func (t *TerminalPTY) waitProcess() {
	if t.cmd == nil {
		return
	}
	done := make(chan struct{})
	go func() {
		_ = t.cmd.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		if t.cmd.Process != nil {
			_ = forceKillProcessGroup(t.cmd.Process.Pid)
		}
		<-done
	}
}

func (t *TerminalPTY) Attach() (chan []byte, []byte) {
	ch := make(chan []byte, terminalClientBuffer)
	t.mu.Lock()
	backlog := t.backlog.bytes()
	if t.done {
		close(ch)
	} else {
		t.clients[ch] = true
	}
	t.mu.Unlock()
	return ch, backlog
}

func (t *TerminalPTY) Detach(ch chan []byte) {
	t.mu.Lock()
	if t.clients != nil {
		delete(t.clients, ch)
	}
	t.mu.Unlock()
}

func (t *TerminalPTY) Write(data []byte) {
	if len(data) == 0 {
		return
	}
	_, _ = t.ptmx.Write(data)
}

func (t *TerminalPTY) Resize(cols, rows int) {
	if cols < 10 {
		cols = 10
	}
	if rows < 2 {
		rows = 2
	}
	_ = pty.Setsize(t.ptmx, &pty.Winsize{Rows: uint16(rows), Cols: uint16(cols)})
}

func (t *TerminalPTY) Kill() {
	if t.cmd != nil && t.cmd.Process != nil {
		_ = killProcessGroup(t.cmd.Process.Pid)
	}
	if t.ptmx != nil {
		_ = t.ptmx.Close()
	}
}

// discard tears down a terminal that was created but never registered (lost the
// create race in GetOrCreate). Because no readLoop runs for it, finish() will
// never reap the child, so we kill, reap, and clean up synchronously here.
func (t *TerminalPTY) discard() {
	t.Kill()
	t.waitProcess()
	if t.rcDir != "" {
		os.RemoveAll(t.rcDir)
	}
}
