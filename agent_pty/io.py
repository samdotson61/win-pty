from __future__ import annotations

import subprocess

from agent_pty.keys import parse as _parse_keys
from agent_pty.session import (
    SessionNotFoundError,
    _full,
    _get_server,
    _has,
    _run,
    _tmux_bin,
    _tmux_env,
)


def _require(name: str) -> str:
    """Resolve a session to its full tmux name, erroring if it is gone."""
    full = _full(name)
    if not _has(_get_server(), full):
        raise SessionNotFoundError(f"Session {name!r} not found")
    return full


def _paste_literal(full: str, value: str) -> None:
    """Type a run of literal text into a pane without command-line mangling.

    Passing the text as a `send-keys -l` argument would route it through the
    Windows command line, where Python's MSVC-style quoting (`"` -> `\\"`) and
    cygwin tmux's own parsing disagree, corrupting quotes and backslashes. We
    instead pipe the bytes into a tmux paste buffer via stdin (which no parser
    touches) and paste it into the pane. `-d` discards the buffer afterwards;
    no `-p`, so bracketed-paste markers are not injected.
    """
    if not value:
        return
    buf = "agentpty_io"
    subprocess.run(
        [_tmux_bin(), "load-buffer", "-b", buf, "-"],
        input=value.encode("utf-8"),  # implies a stdin pipe; no separate stdin=
        stdout=subprocess.DEVNULL,
        stderr=subprocess.DEVNULL,
        env=_tmux_env(),
        check=True,
        timeout=30.0,
    )
    _run(["paste-buffer", "-b", buf, "-t", full, "-d"], check=True)


def send(name: str, text: str) -> None:
    full = _require(name)
    for kind, value in _parse_keys(text):
        if kind == "text":
            _paste_literal(full, value)
        else:
            # Key names (Enter, C-c, Tab, arrows...) are plain ASCII tokens
            # that survive the command line untouched.
            _run(["send-keys", "-t", full, value], check=True)


def snapshot(name: str) -> str:
    full = _require(name)
    result = _run(["capture-pane", "-p", "-t", full], capture=True, check=True)
    # capture-pane appends a trailing newline; drop only trailing blank lines so
    # the returned buffer matches the rendered screen without a dangling line.
    return result.stdout.rstrip("\n")
