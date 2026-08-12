# AgentBox on Windows

**Read this first: the Windows build is experimental.** It is built and smoke-tested
on a Windows runner for every release, so it is not a guess - but it has never been
used for real work by anybody, and two known defects are listed below. The Linux
build is the supported one.

## Install

There is no installer. Put `agentbox.exe` somewhere on your `PATH`, for example:

```powershell
$dir = "$env:LOCALAPPDATA\Programs\agentbox"
New-Item -ItemType Directory -Force -Path $dir | Out-Null
Copy-Item agentbox.exe $dir
[Environment]::SetEnvironmentVariable('Path', "$env:Path;$dir", 'User')
```

Open a new terminal, then register it with your coding agent:

```powershell
claude mcp add --scope user agentbox -- "$env:LOCALAPPDATA\Programs\agentbox\agentbox.exe" mcp
```

Check it:

```powershell
agentbox version
agentbox notify --title hello --body "it works"
agentbox status
```

The daemon starts itself on the first call and needs no service registration. There
is no autostart entry yet; the Linux package ships a systemd unit and the Windows
equivalent (a Startup shortcut or a logon task) is not written.

## What it needs

The **WebView2 runtime**, because every surface is a webview. Windows 11 and Windows
Server 2022 and newer ship it. On older Windows, install the Evergreen runtime from
Microsoft. Without it the CLI still works and the windows do not.

## Known defects

- **The tray icon does not load.** `systray error: unable to set icon: The operation
  completed successfully.` on every attempt. The tray is a convenience, not a
  message path, so nothing is lost except the icon.
- **`agentbox secret` does not protect its output file the way it does on Linux.**
  The file is created `-rw-rw-rw-` instead of `0600`, because Windows does not honour
  Unix permission bits. **Treat a secret written on Windows as readable by any
  account on the machine**, and prefer passing secrets another way until this is
  fixed.

## What is not X11, and therefore not here

Two capabilities are X11-only and say so rather than failing quietly: the **global
hotkey** for the drop-down panel, and **`drive_desktop`**. On Windows the daemon
reports `hotkey: no X11 display` at startup and carries on; use `agentbox panel`
instead. Card placement is left to the window manager, so a card appears where
Windows puts it rather than dead centre of the monitor you are looking at.
