package main

// panes.go — pane-level control of the agent's CURRENT wmux window. This is the
// core of win-pty's purpose: an agent running inside a wmux pane can split its
// own window into horizontal/vertical panes and drive each one, and the human
// attached to that session sees it happen live.
//
// The trick that makes "my current window" work: this MCP server is a child of
// the Claude process running inside the pane, so it inherits $TMUX_PANE (the
// agent's pane) and $TMUX (the agent's server). tmux commands then target the
// same server the human is looking at. Splits use -d so focus stays on the
// agent's pane (the human keeps typing to the agent, not the new empty pane).

import (
	"fmt"
	"os"
	"strings"
)

// currentPane is the pane the agent's process runs in ($TMUX_PANE, inherited
// from the Claude process that launched this server). Empty if not in tmux.
func currentPane() string { return os.Getenv("TMUX_PANE") }

// resolveTarget returns target if non-empty, else the agent's current pane.
func resolveTarget(target string) (string, error) {
	if strings.TrimSpace(target) != "" {
		return target, nil
	}
	if p := currentPane(); p != "" {
		return p, nil
	}
	return "", fmt.Errorf("no target pane: $TMUX_PANE is unset, so I can't tell which pane I'm in. " +
		"Run the agent inside a wmux session, or pass an explicit pane id from pane_list")
}

// splitFlag maps a friendly direction to tmux's split-window flag.
// tmux convention: -h = side-by-side (left|right), -v = stacked (top/bottom).
func splitFlag(dir string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(dir)) {
	case "h", "horizontal", "lr", "left-right", "right", "side", "":
		return "-h", nil // default: side by side
	case "v", "vertical", "tb", "top-bottom", "down", "stack", "stacked":
		return "-v", nil
	}
	return "", fmt.Errorf("dir must be 'h' (side-by-side) or 'v' (stacked), got %q", dir)
}

// Split splits target (default: the agent's current pane) and returns the new
// pane's id (e.g. "%7"). The new pane runs cmd, or the default shell
// (PowerShell 7) if cmd is empty. Created with -d so focus stays put.
func Split(target, dir, cmd, cwd string, percent int) (string, error) {
	ensureServer()
	t, err := resolveTarget(target)
	if err != nil {
		return "", err
	}
	flag, err := splitFlag(dir)
	if err != nil {
		return "", err
	}
	args := []string{"split-window", flag, "-d", "-t", t, "-P", "-F", "#{pane_id}"}
	if percent > 0 && percent < 100 {
		args = append(args, "-l", itoa(percent)+"%")
	}
	if cwd != "" {
		args = append(args, "-c", cwd)
	}
	if cmd != "" {
		args = append(args, cmd)
	}
	// Server is up (ensureServer), so this is a pure client command: it asks the
	// server to fork the pane, prints the new pane id, and exits — the pane's
	// shell runs under the server, not this client, so capturing is safe.
	out, code, err := runCapture(args...)
	if code != 0 || err != nil {
		return "", fmt.Errorf("split-window failed: %v %s", err, strings.TrimSpace(out))
	}
	id := strings.TrimSpace(out)
	if id == "" {
		return "", fmt.Errorf("split-window produced no pane id")
	}
	return id, nil
}

// PaneList lists panes in target's window (default: the agent's current
// window), or every pane on the server when target == "all". Columns:
// "<pane_id> <active 1/0> <WxH> <command> <title>".
func PaneList(target string) (string, error) {
	ensureServer()
	args := []string{"list-panes"}
	if strings.EqualFold(strings.TrimSpace(target), "all") {
		args = append(args, "-a")
	} else {
		t, err := resolveTarget(target)
		if err != nil {
			return "", err
		}
		args = append(args, "-t", t)
	}
	args = append(args, "-F", "#{pane_id} #{pane_active} #{pane_width}x#{pane_height} #{pane_current_command} #{pane_title}")
	out, code, err := runCapture(args...)
	if code != 0 {
		return "", fmt.Errorf("list-panes failed: %v %s", err, strings.TrimSpace(out))
	}
	return strings.TrimRight(out, "\n"), nil
}

// PaneCapture returns the rendered text of a pane.
func PaneCapture(pane string) (string, error) {
	ensureServer()
	if strings.TrimSpace(pane) == "" {
		return "", fmt.Errorf("pane id required")
	}
	out, code, err := runCapture("capture-pane", "-p", "-t", pane)
	if code != 0 {
		return "", fmt.Errorf("capture-pane failed for %q: %v %s", pane, err, strings.TrimSpace(out))
	}
	return strings.TrimRight(out, "\n"), nil
}

// PaneSend sends keystrokes (literal text + named keys) to a specific pane.
func PaneSend(pane, text string) error {
	ensureServer()
	if strings.TrimSpace(pane) == "" {
		return fmt.Errorf("pane id required")
	}
	return sendTo(pane, text)
}

// PaneKill kills a pane (if it's the last pane in a window, the window closes).
func PaneKill(pane string) error {
	ensureServer()
	if strings.TrimSpace(pane) == "" {
		return fmt.Errorf("pane id required")
	}
	return runQuiet("kill-pane", "-t", pane)
}

// PaneSelect focuses a pane (moves the active/focused pane — the human's cursor
// follows). Useful to hand focus to a pane you set up.
func PaneSelect(pane string) error {
	ensureServer()
	if strings.TrimSpace(pane) == "" {
		return fmt.Errorf("pane id required")
	}
	return runQuiet("select-pane", "-t", pane)
}

// PaneResize grows a pane in a direction (U/D/L/R) by amount cells (default 5).
func PaneResize(pane, dir string, amount int) error {
	ensureServer()
	if strings.TrimSpace(pane) == "" {
		return fmt.Errorf("pane id required")
	}
	if amount <= 0 {
		amount = 5
	}
	var flag string
	switch strings.ToUpper(strings.TrimSpace(dir)) {
	case "U", "UP":
		flag = "-U"
	case "D", "DOWN":
		flag = "-D"
	case "L", "LEFT":
		flag = "-L"
	case "R", "RIGHT":
		flag = "-R"
	default:
		return fmt.Errorf("dir must be U, D, L or R, got %q", dir)
	}
	return runQuiet("resize-pane", "-t", pane, flag, itoa(amount))
}

// PaneLayout re-tiles target's window (default: current) with a tmux layout:
// even-horizontal, even-vertical, main-horizontal, main-vertical, tiled.
func PaneLayout(target, layout string) error {
	ensureServer()
	t, err := resolveTarget(target)
	if err != nil {
		return err
	}
	if strings.TrimSpace(layout) == "" {
		layout = "tiled"
	}
	return runQuiet("select-layout", "-t", t, layout)
}
