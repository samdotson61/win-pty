# win-pty (agent-pty for Windows, in Go)

A persistent PTY tool for LLM coding agents. Closes the gap between stateless `Bash` and full computer-use, giving agents a real terminal session they can drive — and a human can attach to.

> **win-pty** is a native-Windows, **single-binary Go** implementation of the agent-pty idea by **[AakeshF](https://github.com/AakeshF/agent-pty)** (whose original Python design and API this follows). It drives a real MSYS2 `tmux.exe` (no WSL) with **PowerShell 7 panes by default**, and ships as one static `win-pty.exe` — no Python, venv, or pip. The original Python fork is retained under [`agent_pty/`](agent_pty/) for lineage/credit; see [`NOTICE`](NOTICE). Go rewrite + Windows port by Sam Dotson.

## Install

One-click. Needs the MSYS2 tmux (install [winmux](https://github.com/samdotson61/winmux) first) and Go (the installer fetches it via winget if missing):

```powershell
./install.ps1          # or double-click install.cmd
```

This checks for Go, compiles a single static `win-pty.exe` (tool + all dependencies in one executable), puts it on your PATH, and prints the MCP registration. Idempotent.

> The Go source is in [`go/`](go/). The retained Python implementation ([`agent_pty/`](agent_pty/), `pip install -e .`) still works and is the upstream-compatible API, but the Go binary is what the installer builds and what the MCP/CLI run.

## Quickstart

```python
from agent_pty import Pty

Pty.spawn("demo", cmd="python3 -q")
Pty.wait_for("demo", ">>>", timeout=5.0)
Pty.send("demo", "x = 21; print(x * 2)\n")
print(Pty.wait_for("demo", "42", timeout=3.0))
Pty.kill("demo")
```

The session lives on the default tmux socket as `agent-pty-demo`. While it's alive you can `tmux attach -t agent-pty-demo` to watch the agent work or take over.

A working example end-to-end: [`examples/drive_python_repl.py`](examples/drive_python_repl.py).

## API

| Method | Description |
|---|---|
| `Pty.spawn(name, cmd=None, cwd=None, cols=80, rows=24)` | Create a new session. `cmd=None` opens the user's default shell. |
| `Pty.send(name, text)` | Send keys. Supports literal text plus `<Enter>`, `<Esc>`, `<Tab>`, `<BS>`, `<Up>`/`<Down>`/`<Left>`/`<Right>`, `<Home>`, `<End>`, `<PgUp>`, `<PgDn>`, `<Del>`, `<F1>`–`<F12>`, `<C-x>`, `<S-x>`, `<M-x>`. `<<` produces a literal `<`. |
| `Pty.snapshot(name)` | Return the current rendered screen as plain text (no escape codes). |
| `Pty.wait_for(name, pattern, timeout=10.0)` | Block until `pattern` (string substring or compiled regex) appears in the buffer. Returns the matching snapshot. Raises `TimeoutError` on timeout. |
| `Pty.list()` | Return names of currently-managed sessions. |
| `Pty.kill(name)` | Kill a session. |

Errors: `SessionExistsError`, `SessionNotFoundError`, `KeyParseError`, plus stdlib `TimeoutError`.

## CLI

`install.ps1` puts `win-pty.exe` on your PATH:

Sessions — background terminals addressed by name:

```powershell
win-pty spawn demo                 # default pane = PowerShell 7
win-pty spawn demo --cmd "bash -l" # or an explicit shell/command
win-pty wait-for demo "PS "
win-pty send demo "Write-Output (2+2)<Enter>"
win-pty snapshot demo
win-pty list
win-pty kill demo
```

Panes — split and drive the **current** wmux window (run from inside a wmux
session; the human watching sees the panes appear live):

```powershell
win-pty split h                    # split current pane left|right -> prints new pane id (e.g. %5)
win-pty split v --cmd "bash -l"    # split top/bottom, running bash
win-pty panes                      # list: id [session:win.pane] active attached WxH command
win-pty pane-info %5               # session/window, pane count, attached clients
win-pty pane-send %5 "make<Enter>" # send keys to pane %5 (same key syntax as `send`)
win-pty pane-capture %5            # read pane %5's rendered screen
win-pty pane-layout tiled          # re-tile (even-horizontal | even-vertical | main-vertical | tiled)
win-pty pane-select %5             # move focus to a pane
win-pty pane-kill %5               # close a pane
```

`win-pty --help` shows all subcommands. Override the tmux location with
`WMUX_TMUX`, or the MSYS2 root with `MSYS2_ROOT`.

**`split` reports the attached-client count** (also via `pane-info`): if it's
`0`, no human is viewing that session, so the pane is invisible — the signal
that you split a detached session instead of the window you're in. Splitting
your own pane (the default) inside a session a human is attached to always shows
≥1 client. This is what makes "I added panes to your window" verifiable rather
than assumed.

## MCP server (for Claude Code and other agents)

The same binary runs the MCP server over stdio with `win-pty mcp`. No
launcher/wrapper or env block is needed — the binary finds tmux itself and sets
the cygwin env (`MSYS=noglob`, UTF-8) internally. Tools registered:

- **Sessions** (background, addressable by name): `pty_spawn`, `pty_send`, `pty_snapshot`, `pty_wait_for`, `pty_list`, `pty_kill`.
- **Panes** (split & drive the agent's *current* wmux window — the human sees it live): `pane_split`, `pane_send`, `pane_capture`, `pane_list`, `pane_kill`, `pane_select`, `pane_resize`, `pane_layout`. `pane_split` defaults its target to the agent's own pane via `$TMUX_PANE`, so "split my window" just works when the agent runs inside wmux.

Register in `~/.claude.json` (the installer prints this with your real path):

```json
"win-pty": {
  "type": "stdio",
  "command": "C:\\path\\to\\win-pty\\win-pty.exe",
  "args": ["mcp"]
}
```

Restart Claude Code (or reconnect via `/mcp`). Validate the server over stdio
with any MCP client pointed at `win-pty.exe mcp` (e.g. `examples/mcp_smoke_wrapper.py`).

## Roadmap

The core (M1–M5) is shipped and frozen. **M6 — mesh** adds an opt-in orchestration layer for the [Captain Kirk pattern](docs/captain-kirk-pattern.md): one agent driving N agents in other panes, with done-detection, push-event subscriptions, blocked-on-prompt detection, incremental snapshots, cross-pane piping, and lifecycle notifications. Lives in `agent_pty/mesh.py` with parallel `mesh_*` MCP tools; core API unchanged. See [docs/build-plan.md](docs/build-plan.md#m6--mesh-orchestration-across-sessions) for the full milestone with acceptance tests.

## Problem

LLM coding agents operate terminals as if terminals were stateless and non-interactive. They aren't. A terminal is a persistent, stateful, bidirectional interactive medium with a real PTY, ANSI redraws, a live cursor, and programs that expect to be talked to in real time. The current "send a shell command, get stdout back" model — what every agent uses — is a degenerate projection of that medium. It works for ~90% of one-shot tasks and falls apart the moment something asks a question back, redraws its screen, or expects the same shell to remember anything about the last command.

Visible symptoms:

- Can't drive `python`, `psql`, `gdb`, or any REPL — every call is a fresh shell
- Can't operate `vim`, `htop`, `lazygit`, `k9s` — they need a PTY and live keyboard
- Can't react to surprise prompts (`sudo` password, `Are you sure? [y/N]`, auth flows) — has to bounce them to the human
- Can't share state with the human — `cd`, `source`, env vars are lost between calls; the human can't see what the agent sees
- "Just background it" doesn't help — backgrounded processes have no TTY, can't be sent further input, and capture line-buffered stdout instead of screen state

The deeper framing: the missing primitive between "fire-and-forget shell" and "control the whole computer with a mouse" is a persistent, addressable PTY session.

## The three primitives

| | What it is | Strengths | Weaknesses |
|---|---|---|---|
| **A. Stateless exec** (Bash today) | One-shot non-interactive shell, return stdout/stderr/exit | Cheap, predictable, sandbox-friendly, fine for 90% of tasks | No TTY, no state across calls, can't answer prompts, breaks TUIs |
| **B. Computer use** (screenshots + input) | Vision-driven control of any GUI | Universal — works with anything visible | Slow loop, pixel-based reasoning over text content, expensive in tokens, fragile, semantically blind |
| **C. Persistent PTY session** (this project) | Long-lived shell with a real PTY, addressable by handle, with screen-buffer reads and keystroke sends | Right-sized for terminal work: text-native, fast, stateful, shareable with the human | Requires real session management; needs careful API around timing/waiting |

A is for "run a script." B is for "use Photoshop." C is for "use a terminal." Reading terminal state as pixels is a category error — like OCR'ing a CSV.

## Proposal: a `Pty` tool, backed by tmux

A new tool, separate from `Bash`, that exposes a persistent PTY session as a first-class resource. tmux is the natural backend — it already handles PTY lifecycle, screen capture (`capture-pane`), keystroke injection (`send-keys`), persistence, and human attach. The tool is a thin, opinionated wrapper.

### API (minimum viable)

| Operation | Purpose |
|---|---|
| `Pty.spawn(name, cmd?, cwd?, cols?, rows?)` | Create a named session. If `cmd` omitted, opens a shell. Returns handle. |
| `Pty.send(name, keys)` | Send keystrokes. Supports literal text + named keys (`<Enter>`, `<C-c>`, `<Up>`, `<Esc>`, `<Tab>`). |
| `Pty.snapshot(name)` | Return the current rendered screen buffer (post-redraw, like `tmux capture-pane -p`), not raw stdout. |
| `Pty.wait_for(name, pattern, timeout)` | Efficiently block until a regex appears in the buffer. Returns buffer snapshot. Avoids polling-via-snapshot. |
| `Pty.list()` / `Pty.kill(name)` | Lifecycle. |

### Why each piece matters

- **`snapshot` returns rendered screen, not stdout stream** — the whole game. A curses program's state lives in the screen buffer after redraws; raw stdout is a soup of escape codes that says nothing about what's actually on screen. tmux's `capture-pane` already produces exactly this.
- **`send` understands named keys** — REPLs and TUIs need `<Enter>`, `<C-c>`, arrows, `<Esc>`. Stringly-typed text-only is a footgun.
- **`wait_for` is a primitive, not a polling pattern** — every interactive flow is "do thing → wait for prompt → do next thing." If the agent has to poll-then-snapshot in a loop, every interaction costs N tool calls. Native `wait_for` collapses it to one.
- **Sessions are real tmux sessions** — the human can `tmux attach -t <name>` and watch, take over, or hand back. Free shared-state, no extra plumbing.

## Non-goals

- **Not a replacement for Bash.** Bash stays as the cheap stateless workhorse for the 90% case.
- **Not computer use.** Vision is for GUI apps without a text equivalent.
- **Not a full multiplexer feature set.** No window/pane management surface for the agent. tmux can do all that under the hood; the agent gets sessions and treats each as one screen.

## Win condition

Drive `python`, `psql`, `vim`, `lazygit`, `gdb`, `htop`, an `ssh` session through 2FA, a `sudo` password, a Cargo `(y/n)` confirmation — without bouncing any of it to the human. The human can `tmux attach` at any time to watch, intervene, or take over. One primitive solves "agent needs a REPL," "agent needs to operate a TUI," "agent needs to react to a prompt," and "human and agent need to share a terminal."

## Build order

Smallest useful slice first:

1. `spawn` + `send` + `snapshot` + `kill` — covers REPLs and basic TUI driving
2. `wait_for` — collapses the polling tax, makes longer flows tractable
3. `list` + named-key parser polish — quality-of-life

Roughly a few hundred lines of glue around tmux, plus tool schema work. Conceptual lift is the bigger half.
