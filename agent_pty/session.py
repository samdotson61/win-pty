from __future__ import annotations

import os
import shutil
import subprocess

import libtmux

PREFIX = "agent-pty-"

# A hidden, never-exiting session whose only job is to keep the tmux server
# alive. Its name deliberately omits PREFIX so it never shows up in
# list_sessions(). See _ensure_server() for why this exists.
_KEEPALIVE = "agentpty_srv_keepalive"


class SessionExistsError(Exception):
    pass


class SessionNotFoundError(Exception):
    pass


_server: libtmux.Server | None = None


def _tmux_bin() -> str:
    return shutil.which("tmux") or "tmux"


def _run(
    args: list[str],
    *,
    capture: bool = False,
    check: bool = False,
    timeout: float = 30.0,
) -> subprocess.CompletedProcess[str]:
    """Run a tmux command via subprocess with Windows-safe stream handling.

    libtmux drives tmux with stdout/stderr=PIPE and an *inherited* stdin. On
    native Windows talking to the MSYS2 (cygwin) tmux, that combination hangs:
    the cygwin client/pane keeps the pipe handles open so communicate() never
    sees EOF. Pinning stdin to DEVNULL — and using DEVNULL for output on
    commands we don't need to read — sidesteps it entirely. This helper is the
    single choke point through which every tmux invocation flows.
    """
    stream = subprocess.PIPE if capture else subprocess.DEVNULL
    return subprocess.run(
        [_tmux_bin(), *args],
        stdin=subprocess.DEVNULL,
        stdout=stream,
        stderr=stream,
        text=True,
        encoding="utf-8",
        errors="replace",
        check=check,
        timeout=timeout,
        env=_tmux_env(),
    )


def _tmux_env() -> dict[str, str]:
    """Environment for tmux subprocesses.

    The MSYS2 (cygwin) runtime expands glob and brace patterns in a native
    program's argv before the program sees them, so a tmux format like
    ``#{session_name}`` arrives as ``#session_name`` and a keystroke payload
    like ``${var}`` or ``@{}`` gets silently mangled. ``MSYS=noglob`` turns that
    off so arguments reach tmux verbatim. Harmless on POSIX (the variable is
    simply ignored).
    """
    env = dict(os.environ)
    existing = env.get("MSYS", "")
    if "noglob" not in existing.split():
        env["MSYS"] = (existing + " noglob").strip()
    return env


def _ensure_server() -> None:
    """Guarantee the tmux server is already running before libtmux talks to it.

    On native Windows driving the MSYS2 tmux, any libtmux command that *starts*
    the server (the first new-session) leaks libtmux's captured stdout/stderr
    pipes into the daemon, so subprocess.communicate() blocks forever. We avoid
    that by starting the server ourselves with no inherited pipes (DEVNULL on
    every std stream), holding it open with a durable keepalive session that
    never exits. libtmux then only ever *connects* to a live server, which
    never hangs. Idempotent and harmless on POSIX (has-session is a no-op when
    the keepalive already exists, and does not itself start a server).
    """
    alive = _run(["has-session", "-t", _KEEPALIVE]).returncode == 0
    if not alive:
        _run(["new-session", "-d", "-s", _KEEPALIVE, "sleep", "infinity"])


def _get_server() -> libtmux.Server:
    global _server
    _ensure_server()
    if _server is None:
        _server = libtmux.Server()
    return _server


def _full(name: str) -> str:
    return f"{PREFIX}{name}"


def _strip(full: str) -> str:
    return full[len(PREFIX):] if full.startswith(PREFIX) else full


def _has(server: libtmux.Server | None, full_name: str) -> bool:
    # `server` is accepted for backward compatibility (callers in mesh.py pass
    # _get_server()), but existence is checked via a direct, non-hanging
    # has-session call rather than libtmux's session listing.
    return _run(["has-session", "-t", full_name]).returncode == 0


def spawn(
    name: str,
    cmd: str | None = None,
    cwd: str | None = None,
    cols: int = 80,
    rows: int = 24,
) -> str:
    server = _get_server()
    full = _full(name)
    if _has(server, full):
        raise SessionExistsError(f"Session {name!r} already exists")
    args = ["new-session", "-d", "-s", full, "-x", str(cols), "-y", str(rows)]
    if cwd:
        args += ["-c", cwd]
    if cmd:
        args.append(cmd)
    _run(args, check=True)
    return name


def kill(name: str) -> None:
    server = _get_server()
    full = _full(name)
    if not _has(server, full):
        raise SessionNotFoundError(f"Session {name!r} not found")
    _run(["kill-session", "-t", full], check=True)


def list_sessions() -> list[str]:
    _get_server()  # ensure the server is up before listing
    result = _run(["list-sessions", "-F", "#{session_name}"], capture=True)
    if result.returncode != 0:
        return []
    names = [line for line in result.stdout.splitlines() if line]
    return sorted(
        _strip(n)
        for n in names
        if n.startswith(PREFIX) and n != _KEEPALIVE
    )
