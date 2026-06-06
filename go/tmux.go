package main

// tmux.go — the tmux driver. Ports the proven win-pty fixes for native-Windows
// MSYS2 tmux:
//   - every tmux call gets a null-device stdin and (for session-creating calls)
//     null stdout/stderr, so a forked pane never inherits a pipe we'd block on;
//   - MSYS=noglob so the cygwin runtime doesn't brace-expand format strings /
//     keystroke payloads;
//   - a UTF-8 locale + `tmux -u` so block-art glyphs aren't mangled;
//   - a hidden keepalive session so commands only ever connect to a live server;
//   - literal keystrokes injected via load-buffer(stdin)+paste-buffer, never as
//     send-keys args (Windows->cygwin quoting corrupts " into \).

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const (
	prefix    = "agent-pty-"
	keepalive = "agentpty_srv_keepalive"
	pasteBuf  = "agentpty_io"
)

// tmuxBin locates the MSYS2 tmux. Override with WMUX_TMUX.
func tmuxBin() string {
	if v := os.Getenv("WMUX_TMUX"); v != "" {
		return v
	}
	if p, err := exec.LookPath("tmux"); err == nil {
		return p
	}
	root := os.Getenv("MSYS2_ROOT")
	if root == "" {
		root = `C:\msys64`
	}
	return filepath.Join(root, `usr\bin\tmux.exe`)
}

// tmuxEnv returns the environment tmux needs: MSYS=noglob and a UTF-8 locale.
func tmuxEnv() []string {
	env := os.Environ()
	set := func(k, v string) {
		pre := k + "="
		for i, e := range env {
			if strings.HasPrefix(e, pre) {
				env[i] = pre + v
				return
			}
		}
		env = append(env, pre+v)
	}
	set("MSYS", "noglob")
	lang := os.Getenv("LANG")
	if !strings.Contains(strings.ToUpper(lang), "UTF-8") && !strings.Contains(strings.ToUpper(lang), "UTF8") {
		lang = "C.UTF-8"
		set("LANG", lang)
	}
	set("LC_CTYPE", lang)
	return env
}

// runQuiet runs a tmux command with no captured output (null device on all std
// streams), so a forked pane can't hold a pipe open. Used for session-creating
// and fire-and-forget commands.
func runQuiet(args ...string) error {
	c := exec.Command(tmuxBin(), append([]string{"-u"}, args...)...)
	c.Env = tmuxEnv()
	// Stdin/Stdout/Stderr left nil -> Go connects them to the null device.
	return c.Run()
}

// runCapture runs a tmux command and returns stdout. These commands (list,
// capture-pane, has-session, display) never fork a pane, so capturing is safe.
func runCapture(args ...string) (string, int, error) {
	var out, errb bytes.Buffer
	c := exec.Command(tmuxBin(), append([]string{"-u"}, args...)...)
	c.Env = tmuxEnv()
	c.Stdout = &out
	c.Stderr = &errb
	// Stdin nil -> null device.
	err := c.Run()
	code := 0
	if ee, ok := err.(*exec.ExitError); ok {
		code = ee.ExitCode()
	} else if err != nil {
		code = -1
	}
	return out.String(), code, err
}

func full(name string) string  { return prefix + name }
func strip(s string) string    { return strings.TrimPrefix(s, prefix) }

// ensureServer guarantees a live tmux server via a durable keepalive session,
// so libtmux-style "first command starts the server" pipe hangs can't occur.
func ensureServer() {
	if _, code, _ := runCapture("has-session", "-t", keepalive); code == 0 {
		return
	}
	_ = runQuiet("new-session", "-d", "-s", keepalive, "sleep", "infinity")
}

func has(name string) bool {
	_, code, _ := runCapture("has-session", "-t", full(name))
	return code == 0
}

// Spawn creates a detached session. cmd=="" uses the default-command shell.
func Spawn(name, cmd, cwd string, cols, rows int) error {
	ensureServer()
	if has(name) {
		return fmt.Errorf("session %q already exists", name)
	}
	args := []string{"new-session", "-d", "-s", full(name), "-x", itoa(cols), "-y", itoa(rows)}
	if cwd != "" {
		args = append(args, "-c", cwd)
	}
	if cmd != "" {
		args = append(args, cmd)
	}
	return runQuiet(args...)
}

func Kill(name string) error {
	ensureServer()
	if !has(name) {
		return fmt.Errorf("session %q not found", name)
	}
	return runQuiet("kill-session", "-t", full(name))
}

func List() ([]string, error) {
	ensureServer()
	out, code, _ := runCapture("list-sessions", "-F", "#{session_name}")
	if code != 0 {
		return []string{}, nil
	}
	var names []string
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || !strings.HasPrefix(line, prefix) || line == keepalive {
			continue
		}
		names = append(names, strip(line))
	}
	return names, nil
}

func Snapshot(name string) (string, error) {
	ensureServer()
	if !has(name) {
		return "", fmt.Errorf("session %q not found", name)
	}
	out, code, err := runCapture("capture-pane", "-p", "-t", full(name))
	if code != 0 {
		return "", fmt.Errorf("capture-pane failed: %v", err)
	}
	return strings.TrimRight(out, "\n"), nil
}

// Send types text and named keys into a session. Literal runs go through a
// paste buffer loaded from stdin (immune to Windows->cygwin arg mangling);
// named keys go through send-keys.
func Send(name, text string) error {
	ensureServer()
	if !has(name) {
		return fmt.Errorf("session %q not found", name)
	}
	return sendTo(full(name), text)
}

// sendTo sends a parsed key/text payload to any tmux target (a session like
// "agent-pty-foo" or a pane id like "%5"). Shared by Send and PaneSend.
func sendTo(target, text string) error {
	segs, err := parseKeys(text)
	if err != nil {
		return err
	}
	for _, s := range segs {
		if s.isKey {
			if e := runQuiet("send-keys", "-t", target, s.val); e != nil {
				return e
			}
		} else {
			if e := pasteLiteral(target, s.val); e != nil {
				return e
			}
		}
	}
	return nil
}

func pasteLiteral(target, text string) error {
	if text == "" {
		return nil
	}
	c := exec.Command(tmuxBin(), "-u", "load-buffer", "-b", pasteBuf, "-")
	c.Env = tmuxEnv()
	c.Stdin = strings.NewReader(text)
	if err := c.Run(); err != nil {
		return err
	}
	return runQuiet("paste-buffer", "-b", pasteBuf, "-t", target, "-d")
}

func itoa(n int) string { return fmt.Sprintf("%d", n) }
