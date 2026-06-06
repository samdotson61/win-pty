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
  win-pty mcp                          run the MCP server (stdio)
  win-pty spawn <name> [--cmd C] [--cwd D] [--cols N] [--rows N]
  win-pty send <name> <text>           keys: <Enter> <Esc> <Tab> <C-c> <Up> ... ; << = literal <
  win-pty snapshot <name>
  win-pty wait-for <name> <pattern> [--timeout S]
  win-pty list
  win-pty kill <name>`

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

	return s.Run(context.Background(), &mcp.StdioTransport{})
}
