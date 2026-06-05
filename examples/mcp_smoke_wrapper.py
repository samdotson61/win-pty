"""Verify the production launch path: cmd.exe /c agent-pty-mcp.cmd, exactly as
configured in .claude.json. Confirms stdio works through the wrapper and that
pwsh panes inherit a full PATH (System32 reachable)."""
import asyncio

from mcp import ClientSession, StdioServerParameters
from mcp.client.stdio import stdio_client


async def main():
    params = StdioServerParameters(
        command="cmd.exe",
        args=["/c", r"C:\msys64\home\Sam\agent-pty\agent-pty-mcp.cmd"],
    )
    async with stdio_client(params) as (read, write):
        async with ClientSession(read, write) as sess:
            await sess.initialize()
            tools = await sess.list_tools()
            print(f"tools: {len(tools.tools)}")

            await sess.call_tool("pty_spawn", {"name": "wrap", "cols": 110, "rows": 30})
            await asyncio.sleep(6.0)  # pwsh cold start through cmd.exe wrapper
            # Prove the pwsh pane has a real, full PATH (System32 cmds resolve).
            await sess.call_tool("pty_send", {
                "name": "wrap",
                "text": "Write-Output \"WHOAMI=$(whoami); EDITION=$($PSVersionTable.PSEdition)\"<Enter>",
            })
            r = await sess.call_tool("pty_wait_for", {"name": "wrap", "pattern": "EDITION=Core", "timeout": 8.0})
            txt = r.content[0].text
            assert "EDITION=Core" in txt, txt
            assert "WHOAMI=" in txt and "\\" in txt, txt  # whoami printed DOMAIN\user -> System32 on PATH
            print("pwsh pane has full PATH + is PowerShell 7")

            await sess.call_tool("pty_kill", {"name": "wrap"})
            print("WRAPPER LAUNCH OK")


asyncio.run(main())
