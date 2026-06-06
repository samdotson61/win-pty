package main

// win-pty — native-Windows fork of agent-pty, in Go. Single static binary.
//   win-pty mcp              run the MCP server over stdio
//   win-pty spawn <name> [--cmd C] [--cwd D] [--cols N] [--rows N]
//   win-pty send <name> <text>
//   win-pty snapshot <name>
//   win-pty wait-for <name> <pattern> [--timeout S]
//   win-pty list
//   win-pty kill <name>

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, usage)
		os.Exit(2)
	}
	sub := os.Args[1]
	args := os.Args[2:]
	var err error
	switch sub {
	case "mcp", "mcp-serve":
		err = runMCP()
	case "spawn":
		err = cliSpawn(args)
	case "send":
		err = needN(args, 2, "send <name> <text>", func() error { return Send(args[0], args[1]) })
	case "snapshot":
		err = needN(args, 1, "snapshot <name>", func() error { s, e := Snapshot(args[0]); if e == nil { fmt.Println(s) }; return e })
	case "wait-for", "wait":
		err = cliWaitFor(args)
	case "list", "ls":
		var names []string
		names, err = List()
		for _, n := range names {
			fmt.Println(n)
		}
	case "kill":
		err = needN(args, 1, "kill <name>", func() error { return Kill(args[0]) })
	case "split":
		err = cliSplit(args)
	case "panes", "pane-list":
		var out string
		out, err = PaneList(firstOr(args))
		if err == nil {
			fmt.Println(out)
		}
	case "pane-send", "psend":
		err = needN(args, 2, "pane-send <pane> <text>", func() error { return PaneSend(args[0], args[1]) })
	case "pane-capture", "pcap":
		err = needN(args, 1, "pane-capture <pane>", func() error {
			s, e := PaneCapture(args[0])
			if e == nil {
				fmt.Println(s)
			}
			return e
		})
	case "pane-kill", "pkill":
		err = needN(args, 1, "pane-kill <pane>", func() error { return PaneKill(args[0]) })
	case "pane-select", "pselect":
		err = needN(args, 1, "pane-select <pane>", func() error { return PaneSelect(args[0]) })
	case "pane-resize", "presize":
		err = needN(args, 2, "pane-resize <pane> <U|D|L|R> [amount]", func() error {
			amt := 5
			if len(args) >= 3 {
				fmt.Sscanf(args[2], "%d", &amt)
			}
			return PaneResize(args[0], args[1], amt)
		})
	case "pane-layout", "playout":
		layout := ""
		if len(args) >= 1 {
			layout = args[0]
		}
		err = PaneLayout("", layout)
	case "-h", "--help", "help":
		fmt.Println(usage)
		return
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n%s\n", sub, usage)
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

const usage = `win-pty — persistent PTY tool for LLM agents (native Windows, tmux-backed)

 Sessions (background, addressable by name):
  win-pty mcp                          run the MCP server (stdio)
  win-pty spawn <name> [--cmd C] [--cwd D] [--cols N] [--rows N]
  win-pty send <name> <text>           keys: <Enter> <Esc> <Tab> <C-c> <Up> ... ; << = literal <
  win-pty snapshot <name>
  win-pty wait-for <name> <pattern> [--timeout S]
  win-pty list
  win-pty kill <name>

 Panes (split & drive the agent's CURRENT wmux window; the human sees it live):
  win-pty split [h|v] [--target P] [--cmd C] [--cwd D] [--percent N]   -> new pane id
  win-pty panes [all]                  list panes: id active WxH command title
  win-pty pane-send <pane> <text>      send keys to a pane (same key syntax as send)
  win-pty pane-capture <pane>          rendered text of a pane
  win-pty pane-select <pane>           focus a pane
  win-pty pane-resize <pane> <U|D|L|R> [amount]
  win-pty pane-layout [even-horizontal|even-vertical|main-vertical|tiled]
  win-pty pane-kill <pane>`

func needN(args []string, n int, sig string, fn func() error) error {
	if len(args) < n {
		return fmt.Errorf("usage: win-pty %s", sig)
	}
	return fn()
}

// parseOpts pulls --flag value pairs out of args (order-independent, so flags
// may follow the positional name — Go's flag package can't do that). Returns
// the option map and the leftover positionals.
func parseOpts(args []string, valueFlags map[string]bool) (map[string]string, []string) {
	opts := map[string]string{}
	var pos []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		key := strings.TrimLeft(a, "-")
		if strings.HasPrefix(a, "-") && valueFlags[key] && i+1 < len(args) {
			opts[key] = args[i+1]
			i++
			continue
		}
		pos = append(pos, a)
	}
	return opts, pos
}

func cliSpawn(args []string) error {
	opts, pos := parseOpts(args, map[string]bool{"cmd": true, "cwd": true, "cols": true, "rows": true})
	if len(pos) < 1 {
		return fmt.Errorf("usage: win-pty spawn <name> [--cmd C] [--cwd D] [--cols N] [--rows N]")
	}
	cols, rows := 80, 24
	if v := opts["cols"]; v != "" {
		fmt.Sscanf(v, "%d", &cols)
	}
	if v := opts["rows"]; v != "" {
		fmt.Sscanf(v, "%d", &rows)
	}
	if err := Spawn(pos[0], opts["cmd"], opts["cwd"], cols, rows); err != nil {
		return err
	}
	fmt.Println(pos[0])
	return nil
}

func firstOr(args []string) string {
	if len(args) > 0 {
		return args[0]
	}
	return ""
}

// cliSplit: win-pty split [h|v] [--target P] [--cmd C] [--cwd D] [--percent N]
func cliSplit(args []string) error {
	opts, pos := parseOpts(args, map[string]bool{"dir": true, "target": true, "cmd": true, "cwd": true, "percent": true})
	dir := opts["dir"]
	if dir == "" && len(pos) >= 1 {
		dir = pos[0] // allow positional direction: win-pty split h
	}
	pct := 0
	if v := opts["percent"]; v != "" {
		fmt.Sscanf(v, "%d", &pct)
	}
	id, err := Split(opts["target"], dir, opts["cmd"], opts["cwd"], pct)
	if err != nil {
		return err
	}
	fmt.Println(id)
	return nil
}

func cliWaitFor(args []string) error {
	opts, pos := parseOpts(args, map[string]bool{"timeout": true})
	if len(pos) < 2 {
		return fmt.Errorf("usage: win-pty wait-for <name> <pattern> [--timeout S]")
	}
	timeout := 10.0
	if v := opts["timeout"]; v != "" {
		fmt.Sscanf(v, "%f", &timeout)
	}
	snap, err := WaitFor(pos[0], pos[1], timeout)
	if err != nil {
		return err
	}
	fmt.Println(snap)
	return nil
}

// WaitFor polls the pane until pattern (regex or substring) appears or timeout.
func WaitFor(name, pattern string, timeoutSec float64) (string, error) {
	var re *regexp.Regexp
	if r, e := regexp.Compile(pattern); e == nil {
		re = r
	}
	deadline := time.Now().Add(time.Duration(timeoutSec * float64(time.Second)))
	for {
		snap, err := Snapshot(name)
		if err != nil {
			return "", err
		}
		if strings.Contains(snap, pattern) || (re != nil && re.MatchString(snap)) {
			return snap, nil
		}
		if time.Now().After(deadline) {
			return "", fmt.Errorf("timeout: pattern %q not found in session %q within %.1fs", pattern, name, timeoutSec)
		}
		time.Sleep(300 * time.Millisecond)
	}
}

// ---- MCP server ----------------------------------------------------------

func textResult(s string) *mcp.CallToolResult {
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: s}}}
}

type spawnIn struct {
	Name string `json:"name" jsonschema:"session identifier"`
	Cmd  string `json:"cmd,omitempty" jsonschema:"command to run; empty opens the default shell"`
	Cwd  string `json:"cwd,omitempty" jsonschema:"working directory"`
	Cols int    `json:"cols,omitempty" jsonschema:"terminal columns (default 80)"`
	Rows int    `json:"rows,omitempty" jsonschema:"terminal rows (default 24)"`
}
type nameIn struct {
	Name string `json:"name" jsonschema:"session identifier"`
}
type sendIn struct {
	Name string `json:"name" jsonschema:"session identifier"`
	Text string `json:"text" jsonschema:"literal text and named keys, e.g. echo hi<Enter>"`
}
type waitIn struct {
	Name    string  `json:"name" jsonschema:"session identifier"`
	Pattern string  `json:"pattern" jsonschema:"substring or regex to wait for"`
	Timeout float64 `json:"timeout,omitempty" jsonschema:"seconds (default 10)"`
}
type emptyIn struct{}

// --- pane control (the agent's current wmux window) ---
type splitIn struct {
	Dir     string `json:"dir,omitempty" jsonschema:"'h' = side-by-side (left|right), 'v' = stacked (top/bottom). default h"`
	Target  string `json:"target,omitempty" jsonschema:"pane id to split; default = the agent's current pane ($TMUX_PANE)"`
	Cmd     string `json:"cmd,omitempty" jsonschema:"command for the new pane; empty = default shell (PowerShell 7)"`
	Cwd     string `json:"cwd,omitempty" jsonschema:"working directory for the new pane"`
	Percent int    `json:"percent,omitempty" jsonschema:"new pane size as a percent of the split dimension, e.g. 30"`
}
type paneSendIn struct {
	Pane string `json:"pane" jsonschema:"target pane id, e.g. %5 (from pane_split or pane_list)"`
	Text string `json:"text" jsonschema:"literal text + named keys, e.g. ls<Enter> or <C-c>"`
}
type paneIn struct {
	Pane string `json:"pane" jsonschema:"target pane id, e.g. %5"`
}
type paneListIn struct {
	Target string `json:"target,omitempty" jsonschema:"window/pane to list; default = the agent's current window. 'all' = every pane on the server"`
}
type paneResizeIn struct {
	Pane   string `json:"pane" jsonschema:"target pane id"`
	Dir    string `json:"dir" jsonschema:"U, D, L or R (up/down/left/right)"`
	Amount int    `json:"amount,omitempty" jsonschema:"cells to resize by (default 5)"`
}
type paneLayoutIn struct {
	Target string `json:"target,omitempty" jsonschema:"window to re-tile; default = current"`
	Layout string `json:"layout,omitempty" jsonschema:"even-horizontal, even-vertical, main-horizontal, main-vertical, tiled (default tiled)"`
}

func runMCP() error {
	s := mcp.NewServer(&mcp.Implementation{Name: "win-pty", Version: "1.0.0"}, nil)

	mcp.AddTool(s, &mcp.Tool{Name: "pty_spawn", Description: "Create a persistent tmux-backed terminal session. cmd empty = default shell (PowerShell 7)."},
		func(ctx context.Context, req *mcp.CallToolRequest, in spawnIn) (*mcp.CallToolResult, any, error) {
			cols, rows := in.Cols, in.Rows
			if cols == 0 {
				cols = 80
			}
			if rows == 0 {
				rows = 24
			}
			if err := Spawn(in.Name, in.Cmd, in.Cwd, cols, rows); err != nil {
				return nil, nil, err
			}
			return textResult(in.Name), nil, nil
		})

	mcp.AddTool(s, &mcp.Tool{Name: "pty_send", Description: "Send keystrokes (literal text + named keys like <Enter>, <C-c>, <Up>) to a session."},
		func(ctx context.Context, req *mcp.CallToolRequest, in sendIn) (*mcp.CallToolResult, any, error) {
			if err := Send(in.Name, in.Text); err != nil {
				return nil, nil, err
			}
			return textResult("ok"), nil, nil
		})

	mcp.AddTool(s, &mcp.Tool{Name: "pty_snapshot", Description: "Return the current rendered screen of a session as plain text."},
		func(ctx context.Context, req *mcp.CallToolRequest, in nameIn) (*mcp.CallToolResult, any, error) {
			out, err := Snapshot(in.Name)
			if err != nil {
				return nil, nil, err
			}
			return textResult(out), nil, nil
		})

	mcp.AddTool(s, &mcp.Tool{Name: "pty_wait_for", Description: "Block until a substring/regex appears in the session buffer; returns the snapshot."},
		func(ctx context.Context, req *mcp.CallToolRequest, in waitIn) (*mcp.CallToolResult, any, error) {
			to := in.Timeout
			if to == 0 {
				to = 10
			}
			out, err := WaitFor(in.Name, in.Pattern, to)
			if err != nil {
				return nil, nil, err
			}
			return textResult(out), nil, nil
		})

	mcp.AddTool(s, &mcp.Tool{Name: "pty_list", Description: "List currently-managed session names."},
		func(ctx context.Context, req *mcp.CallToolRequest, in emptyIn) (*mcp.CallToolResult, any, error) {
			names, err := List()
			if err != nil {
				return nil, nil, err
			}
			return textResult(strings.Join(names, "\n")), nil, nil
		})

	mcp.AddTool(s, &mcp.Tool{Name: "pty_kill", Description: "Kill a session and clean up its tmux state."},
		func(ctx context.Context, req *mcp.CallToolRequest, in nameIn) (*mcp.CallToolResult, any, error) {
			if err := Kill(in.Name); err != nil {
				return nil, nil, err
			}
			return textResult("ok"), nil, nil
		})

	// ---- pane control: split & drive the agent's CURRENT wmux window --------

	mcp.AddTool(s, &mcp.Tool{Name: "pane_split", Description: "Split a pane in the agent's CURRENT wmux window, creating a new pane the human sees appear live. dir 'h'=side-by-side, 'v'=stacked; default target is the agent's own pane. Returns the new pane id (e.g. %5). Focus stays on the agent's pane."},
		func(ctx context.Context, req *mcp.CallToolRequest, in splitIn) (*mcp.CallToolResult, any, error) {
			id, err := Split(in.Target, in.Dir, in.Cmd, in.Cwd, in.Percent)
			if err != nil {
				return nil, nil, err
			}
			return textResult(id), nil, nil
		})

	mcp.AddTool(s, &mcp.Tool{Name: "pane_send", Description: "Send keystrokes (literal text + named keys like <Enter>, <C-c>, <Up>) to a specific pane by id."},
		func(ctx context.Context, req *mcp.CallToolRequest, in paneSendIn) (*mcp.CallToolResult, any, error) {
			if err := PaneSend(in.Pane, in.Text); err != nil {
				return nil, nil, err
			}
			return textResult("ok"), nil, nil
		})

	mcp.AddTool(s, &mcp.Tool{Name: "pane_capture", Description: "Return the current rendered text of a pane by id."},
		func(ctx context.Context, req *mcp.CallToolRequest, in paneIn) (*mcp.CallToolResult, any, error) {
			out, err := PaneCapture(in.Pane)
			if err != nil {
				return nil, nil, err
			}
			return textResult(out), nil, nil
		})

	mcp.AddTool(s, &mcp.Tool{Name: "pane_list", Description: "List panes in the agent's current window (or target='all' for every pane). Columns: pane_id active(1/0) WxH command title."},
		func(ctx context.Context, req *mcp.CallToolRequest, in paneListIn) (*mcp.CallToolResult, any, error) {
			out, err := PaneList(in.Target)
			if err != nil {
				return nil, nil, err
			}
			return textResult(out), nil, nil
		})

	mcp.AddTool(s, &mcp.Tool{Name: "pane_kill", Description: "Kill a pane by id (closes its window if it was the last pane)."},
		func(ctx context.Context, req *mcp.CallToolRequest, in paneIn) (*mcp.CallToolResult, any, error) {
			if err := PaneKill(in.Pane); err != nil {
				return nil, nil, err
			}
			return textResult("ok"), nil, nil
		})

	mcp.AddTool(s, &mcp.Tool{Name: "pane_select", Description: "Focus a pane by id — the human's cursor/keyboard moves to it."},
		func(ctx context.Context, req *mcp.CallToolRequest, in paneIn) (*mcp.CallToolResult, any, error) {
			if err := PaneSelect(in.Pane); err != nil {
				return nil, nil, err
			}
			return textResult("ok"), nil, nil
		})

	mcp.AddTool(s, &mcp.Tool{Name: "pane_resize", Description: "Resize a pane by id: dir U/D/L/R, amount cells (default 5)."},
		func(ctx context.Context, req *mcp.CallToolRequest, in paneResizeIn) (*mcp.CallToolResult, any, error) {
			if err := PaneResize(in.Pane, in.Dir, in.Amount); err != nil {
				return nil, nil, err
			}
			return textResult("ok"), nil, nil
		})

	mcp.AddTool(s, &mcp.Tool{Name: "pane_layout", Description: "Re-tile the current window with a layout: even-horizontal, even-vertical, main-horizontal, main-vertical, tiled (default tiled)."},
		func(ctx context.Context, req *mcp.CallToolRequest, in paneLayoutIn) (*mcp.CallToolResult, any, error) {
			if err := PaneLayout(in.Target, in.Layout); err != nil {
				return nil, nil, err
			}
			return textResult("ok"), nil, nil
		})

	return s.Run(context.Background(), &mcp.StdioTransport{})
}
