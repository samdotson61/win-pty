"""Windows MCP smoke: launches agent-pty-mcp the same way Claude Code does
(native venv python + msys/pwsh on PATH), drives it over stdio JSON-RPC, and
verifies a DEFAULT pane is PowerShell 7."""
import asyncio
import os
from pathlib import Path

from mcp import ClientSession, StdioServerParameters
from mcp.client.stdio import stdio_client

# Resolve the venv python relative to this checkout so the test is portable.
VENV_PY = str(Path(__file__).resolve().parent.parent / ".venv-win" / "Scripts" / "python.exe")
PATH = r"C:\msys64\usr\bin;C:\Program Files\PowerShell\7;" + os.environ.get("PATH", "")


async def main():
    params = StdioServerParameters(
        command=VENV_PY,
        args=["-m", "agent_pty.mcp"],
        env={"PATH": PATH, "MSYS": "noglob", "SYSTEMROOT": os.environ.get("SYSTEMROOT", r"C:\Windows")},
    )
    async with stdio_client(params) as (read, write):
        async with ClientSession(read, write) as sess:
            await sess.initialize()
            tools = await sess.list_tools()
            print(f"server reports {len(tools.tools)} tools")

            r = await sess.call_tool("pty_spawn", {"name": "smoke", "cols": 100, "rows": 28})
            print(f"spawn(default) -> {r.content[0].text!r}")
            await asyncio.sleep(3.0)

            await sess.call_tool("pty_send", {
                "name": "smoke",
                "text": "Write-Output \"DEFAULT=$($PSVersionTable.PSEdition)/$($PSVersionTable.PSVersion)\"<Enter>",
            })
            r = await sess.call_tool("pty_wait_for", {"name": "smoke", "pattern": "DEFAULT=Core", "timeout": 8.0})
            assert "DEFAULT=Core" in r.content[0].text, r.content[0].text
            print("default pane is PowerShell 7 (Core)")

            r = await sess.call_tool("pty_spawn", {"name": "shell", "cmd": "bash -l", "cols": 100, "rows": 28})
            await asyncio.sleep(1.5)
            await sess.call_tool("pty_send", {"name": "shell", "text": "echo BASHOK=$BASH_VERSION<Enter>"})
            r = await sess.call_tool("pty_wait_for", {"name": "shell", "pattern": "BASHOK=", "timeout": 5.0})
            assert "BASHOK=" in r.content[0].text
            print("explicit bash pane works")

            r = await sess.call_tool("pty_list", {})
            print(f"list -> {r.content[0].text!r}")

            await sess.call_tool("pty_kill", {"name": "smoke"})
            await sess.call_tool("pty_kill", {"name": "shell"})
            print("kill\n\nMCP server smoke test PASSED.")


asyncio.run(main())
