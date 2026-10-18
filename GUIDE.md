# SnipeZilla Server Manager 2: guide

Everything the manager does, in detail. To install it, see [INSTALL.md](INSTALL.md).

- [Servers: install, watch, update, restart](#servers)
- [Crash reports](#crash-reports)
- [The web interface](#the-web-interface)
- [Backups and off-site copies](#backups-and-off-site-copies)
- [Addons](#addons-metamod-counterstrikesharp-sourcemod-amx-mod-x)
- [CS2 Workshop maps](#cs2-workshop-maps)
- [Discord and XenForo](#discord-and-xenforo)
- [Machines: one page for several machines](#machines-one-page-for-several-machines)
- [Users and rights](#users-and-rights)
- [Commands](#commands)
- [Files next to szm.exe](#files-next-to-szmexe)
- [Performance](#performance)
- [Security](#security)

## Servers

- **Install** any server from the catalog with steamcmd: CS2, CS:S, CS 1.6, CS:CZ, TF2, L4D, L4D2, GMod, DoD/DoD:S, HL2DM, HLDM, TFC, Insurgency, NMRiH… GoldSrc (app 90) installs reliably: steamcmd is run again until the install is complete. steamcmd itself is downloaded the first time it is needed.
- **Run** each server in its own folder as its working directory. This is what CS 1.6 / hlds.exe needs to start.
- **Watch** each server two ways: its process, and a Steam query (A2S) every 30 s. If the process ends, or the server stops answering queries, it is restarted after 5 s, 15 s, then 1 min; the third restart first checks its files with steamcmd (validate). One more crash and the manager stops trying and tells you (Settings > Crash watch).
- **Update by itself:** the manager asks Steam whether each game's version is current. When an update is out, it warns the players in-game (`say`, or `sm_csay`…), stops the server, updates it and starts it again. An empty server is updated at once.
  - **Shared copy** mode: the update downloads once for all servers of a game while they keep running, and each server is stopped only for the copy. Only changed files are copied, and your `server.cfg`, `mapcycle.txt`… are never overwritten. The **Shared copies** page (Settings > steamcmd > See their files) shows each copy, the servers that update from it, and its files.
- **Metamod for CS2:** after each CS2 update the manager puts Metamod back into `gameinfo.gi` (when `addons/metamod` exists).
- **Scheduled restarts:** daily at a set time (waiting until the server is empty), after 24 h when empty, or never.
- **Takes servers back:** stopping or updating the manager does not stop the game servers. When it starts again it finds them running and keeps watching them.
- **Hides your Steam login tokens (GSLT)** in logs, the console and every screen.

## Crash reports

When a server crashes, or is stuck and ended, the manager keeps what was known then, and says what most likely caused it. See the server's **Crashes** tab, or the "Crash report" link on each crash in Activity.

- **The crash dump, read** (Windows): szm has Windows save a small dump of each server crash (`crashes\dumps`; the manager must run as an administrator or as its service, else the Servers page and Settings > Crash watch say so) and reads it: the exception (an access violation reading a null pointer…), the module and offset it crashed in (`server.dll+0x1A2B3C`, with its version), and the modules found on the crashed thread's stack. The addons among them (a SourceMod extension, a Metamod or CounterStrikeSharp plugin) are named: **Most likely: SourceMod extension dhooks**. A plugin crashed inside, one running when the game crashed, a module unloaded while still in use, code outside any module (a SourcePawn or .NET plugin) are told apart. **Download** gives the dump for a plugin's author (WinDbg, Visual Studio).
- **Every logs folder**: the end of the logs written during the run (the game's, SourceMod's, AMX Mod X's, CounterStrikeSharp's…), the newest first.
- **What changed in the week before the run**: plugins, configs and game files modified, and what the manager did (updates, addons pushed, files saved). A crash series often starts right after one.
- **Patterns** across the reports kept: the same crash again (by where it crashed), the plugin running in most of them, the map, while changing map, right after starting, with nobody on.
- And as before: the last lines of its console (the end of `console.log`, written as it crashed, included; "Console log" must be on), its exit code in words, Windows' record of the crash in its Application log, the text of an error window it was stuck on, and the map, players, version and use at the time. On Linux, the kernel's segfault or out-of-memory line (szm running as root). The last 30 per server are kept in `crashes/`; **Save as text** gives one to paste. Crashed servers show no Windows "has stopped working" box: they end at once, so the manager restarts them without waiting.

## The web interface

- Dashboard with live status, map, players, CPU and RAM.
- Each server's Overview adds its network in and out and the size of its folder. The network is what Windows counts for the server's process, as Task Manager shows it (ETW, the Microsoft-Windows-Kernel-Network events): szm must run as administrator or as the service; otherwise the Overview shows the server's PID, and its tooltip says why. On Linux, szm reads the headers of the packets (an AF_PACKET socket, as tcpdump does) and gives each one to the server whose processes own its port: it needs root or the right CAP_NET_RAW (the systemd service of [INSTALL.md](INSTALL.md#linux) gives it; by hand: `sudo setcap cap_net_raw+ep szm`, again after each new szm). While it counts, `logman query -ets` lists the session "SnipeZilla Server Manager network".
- Live console (`-condebug`) with RCON. Source RCON uses TCP and GoldSrc RCON uses UDP.
- File manager: browse, edit configs, upload by drag-and-drop, download, rename, delete. Columns sort both ways; tick several entries to delete them together. **Copy** (or **Cut**) files and folders, then **Paste** them in another folder: of the same server, another server, an addon package or the shared copies (Ctrl+C, Ctrl+X, Ctrl+V work too). What is already there is replaced, kept as "name (2)", or skipped, as you choose; pasted in the same folder, a copy is named "name - copy". A `logs` folder shows the newest first, and **Select older than…** ticks its old files.
- **Old logs deleted by themselves** (Settings > Logs, off by default): the files older than a number of days in every folder named `logs` of the servers (`csgo/logs`, `addons/sourcemod/logs`, `addons/counterstrikesharp/logs`…), every 6 hours.
- Map list with a drag-to-reorder map cycle editor and one-click map change.
- Per-server settings with a live command-line preview.
- Create-server wizard. Pick an empty folder and the game is installed there; pick the folder of a server you already have and it is added as it is (its program is found, its `server.cfg` RCON password kept, and it is taken over if it runs).
- Each server's **Program** (Settings tab): the one the manager found (`srcds.exe`, `srcds_win64.exe`…), or **Pick…** another one from its folder.
- Activity log.
- Users with rights and a server list per user.
- Works on a phone.

## Backups and off-site copies

On the **Backups** page (every server), a server's **Backups** tab, and Settings > Backups. A backup is a zip of each server's settings and plugins (in its game folder: `cfg`, `addons`, `*.txt`…, without logs, demos, maps or the .NET runtime of CounterStrikeSharp), by hand (the default), daily, weekly or monthly at a time, and before an update, an addons push and a restore. The newest 14 of each server are kept, in `backups/` next to szm.exe or a folder of your choice. **Restore** puts a backup's files back (the server is stopped and started again; what it has then is backed up first; files the backup does not have stay). **Files** shows what one holds; **Download** gives the zip. The Backups page lists every server's (and those of servers removed since): **Download all**, all of one server, or the ones ticked come as one zip, a folder per server; **Back up all now** backs every server up.

**Off-site copies** (Settings > Off-site copies): each new backup is also copied elsewhere, and the same number is kept there, a folder per server. To **Google Drive, OneDrive** (or Dropbox, SFTP, S3… anything [rclone](https://rclone.org) reaches): **Download rclone** (into `rclone/` of the data folder), then once, on the server (Remote Desktop), run the command shown, `rclone config --config "<data>\rclone\rclone.conf"`: `n`, a name (`gdrive`, `onedrive`), the storage (`drive`, `onedrive`), Enter for the defaults, `y` to sign in with the browser, `q`. Back in the settings, click the remote, **Test**, save. Or to **a folder**: one the Google Drive or OneDrive app syncs, another disk, a network share. Copies that fail are tried again every hour; the Backups page shows how each destination goes (**Copy now** copies what is missing).

## Addons (Metamod, CounterStrikeSharp, SourceMod, AMX Mod X)

Plugins live in **packages**: one folder per brand and version, in `addons\` next to `szm.exe` (Settings → Addons to move it):

```
addons\metamod2\addons\metamod\...                 Metamod:Source 2 (CS2)
addons\counterstrikesharp\addons\counterstrikesharp\...
addons\sourcemod\addons\sourcemod\...  cfg\sourcemod\...
addons\amxmodx\addons\amxmodx\...
```

Each package mirrors the game folder of a server (`game\csgo` for CS2, `cstrike` for CS:S and CS 1.6).

On the **Addons** page:

- **New package**, three ways to start it:
  - from the plugin's release archive;
  - from a server's `addons\<folder>`;
  - empty.
- **Upload archive** brings a new version: `.zip` or `.tar.gz`, as downloaded.
  - An archive that wraps everything in a folder is unwrapped, so `addons\` lands at the top.
  - **Replace** swaps the whole package; **Add** merges into it.
- **Files** browses a package: upload, edit or delete single files.
- **Servers that get it**: tick the servers. A server's Settings tab has the same choice.
- **Push** copies what changed into those servers.
  - A running server stops for the copy and starts again. With players on it, you choose to warn them first or go now.
  - A server's **Update addons** button pushes all its packages.
- **What the server already has is kept**: `addons/counterstrikesharp/configs`, `addons/sourcemod/configs`, `addons/sourcemod/plugins`, `cfg/sourcemod`, `*.db`… A listed folder the server has is left exactly as it is, nothing overwritten or added, so a plugin you removed or moved to `disabled` does not come back. A listed folder the server does not have yet (a first push) is copied. To have pushes bring new plugins, take `addons/sourcemod/plugins` off the list. The list is in Settings → Addons, and wildcards work.
- Nothing is ever deleted from a server.
- **CS2 Metamod line:** after every game update, every addons push and before every start, szm puts `Game csgo/addons/metamod` back into `gameinfo.gi`, right after `Game_LowViolence`, if it is missing. A server's **Metamod** setting chooses when:
  - **When addons/metamod is there** (the default),
  - **Always**,
  - **Never**.

## CS2 Workshop maps

On the **Maps** tab of a CS2 server: paste a map's ID or Workshop address, or a collection's (all its maps are added), and see their names, pictures and sizes (from Steam). **Play** changes to one now (`host_workshop_map`), **Start on it** makes it the map the server starts on (`+host_workshop_map`, in place of `+map`), and a **collection** can be played (`+host_workshop_collection`, the server downloads it and plays its maps). CS2 downloads the maps itself.

## Discord and XenForo

**Discord:** crashes, updates and errors are posted to channels through webhooks, with the game's icon, the server's name and hostname, in a color per kind of event. Add as many channels as wanted (Settings > Discord > Add a Discord channel), each with what it gets and for which servers: a public channel the updates and starts (the **Public** preset), a staff channel everything (**Admins**). This replaces the email alerts.

**XenForo:** the same posts on your forums through their REST API: as replies of one thread (a "Server status" thread), or each one a new thread of a forum. As many as wanted, each with what it gets and for which servers (Settings > XenForo > Add a forum); Check tests one without posting. The game icon comes with each post, as on Discord: attached (the API key needs the scope attachment:write), or shown from a folder of your site when the forum's "Game icons address" is set. The single webhook and forum of earlier versions are moved into these lists at the first start.

## Machines: one page for several machines

With servers on several machines (a Windows server in the US, an Ubuntu one in Europe…), one manager can show them all: the **hub**. The managers of the other machines, its **nodes**, link to it.

- **Each manager stays in charge of its own servers.** It watches, restarts, updates and backs them up, as before. If the link drops, or the hub is down, nothing changes for the servers.
- **The hub's page shows every server.** Each card says which machine it is on. A strip above the servers shows each machine (connected or not, servers running, RAM); click one to see only its servers.
- **Everything works the same on a node's server:** console and RCON, files (upload, download, copy), maps, settings, backups, crash reports. The hub passes the clicks to the node.
- **Copy and paste between machines:** copy (or cut) files on a server of one machine, paste them on a server of another. The hub carries them across, in the background: a bar at the bottom of the page shows how far it is (MB copied of the total). A cut removes the originals only once the copy arrived whole.
- **Settings, Addons, Games, Backups, Shared copies, New server** belong to a machine: a **Machine** bar at the top of these pages chooses which.
- **Activity** shows the events of every machine, each with its machine's name. Those of a machine whose link was down arrive when it is back.
- **The hub posts for every machine:** its Discord channels and XenForo forums get the events of all the servers, each channel with the servers it chooses (of any machine). Set them on the hub only: the other machines need none (theirs would post a second time). Events that arrive more than 30 minutes late (a link that was down) go to Activity, not to the posts.
- **One sign-in:** the hub's users, with their rights. A user's server list can include servers of the nodes. On the node, what they do shows with the hub's name: "admin (US)".
- **A machine that goes offline:** its servers show "EU offline" with what they said last. After 90 seconds the hub logs an error (so Discord or XenForo hears of it if they get errors), and "connected again" when it is back.

### Linking a machine

1. On the hub: **Settings → Machines → Add a machine**. Give it a name (EU), check the hub's address (how the other machine reaches it), click **Add a machine**. A code shows: `SZM-…`. It works once, for 30 minutes.
2. On the other machine's manager: **Settings → Machines → Or link this manager to a hub**, paste the code, click **Link**.

That is all: its servers show on the hub's page within seconds. A code of another hub moves a machine to that hub at once (it leaves the one it had).

### A hub with no servers

The hub can run no game at all: on the machine of your website, for example, with every game machine linked to it as a node. Such a hub is left out of the machine strip and the Machine bars, except on **Settings** (its users, Discord, forums, machines: it opens on it) and **New server** (to give it a server after all).

### How they talk

- **The node connects to the hub, never the other way.** Nothing to open on the node's firewall, and the node's manager does not need to be on the internet at all (it can stay on `127.0.0.1:8888`).
- **Over the hub's own web address.** On the internet that must be **HTTPS** (your domain, behind IIS: see [INSTALL.md](INSTALL.md#your-domain-with-https)): the link carries RCON passwords and files. Plain `http://` is accepted only for an address on a private network (192.168.x.x, 10.x.x.x).
- **The code is used once.** The node gets its own secret in exchange, and the hub keeps only a hash of it (in `links.json`). **Remove** on the hub, or **Unlink** on the node, ends the link at once. **New code** makes a fresh one, and the old secret stops working.
- **It reconnects by itself** after a drop or a restart of either side.


Users are added on the **Users** page (top bar), each with rights and, without the right *e*, a list of the servers they see. The letters are the same as in 1.x:

| Letter | Right |
| --- | --- |
| a | Admin: everything |
| b | Manager settings |
| c | Add, install and delete servers |
| d | Users |
| e | Sees every server (without it, a user sees only the servers on their list) |
| f | Server settings and command line |
| g | Console and RCON |
| h | Update servers |
| i | Start, stop and restart |
| j | Files |

## Commands

```
szm                    run in this window (or as the service, when Windows starts it)
szm install            install and start the Windows service (as administrator)
szm uninstall          remove the service (the game servers keep running)
szm start | stop       start or stop the service
szm import <file>      import the old manager's config.xml
szm password <user>    set a user's password (if you are locked out)
szm listen <address>   save the web address: 127.0.0.1:8888 (this machine only), :8888 (every address)
szm version
```

`szm password` works at once, even while the manager runs. `szm listen` is Settings → General → **Web interface address**, for when you can not reach the page. The manager uses it at its next start: on Windows `szm stop` then `szm start` (as administrator); on Linux `sudo /opt/szm/szm listen 127.0.0.1:8888`, then `sudo systemctl restart szm`.

Options:

- `-data <folder>` keeps the data somewhere other than next to `szm.exe`.
- `-listen 127.0.0.1:8888` uses another web address for this run only, without saving it.

## Files next to szm.exe

| File | What it is |
| --- | --- |
| `szm.json` | Settings and servers. It is written by the web interface; you can edit it while the manager is stopped. |
| `users.json` | Users. Passwords are stored as PBKDF2-SHA256 hashes. |
| `state.json` | Which servers should run and the processes the manager started. |
| `logs\YYYY-MM-DD.log` | Activity, one file per day, kept 14 days by default. |
| `games.json` | The games added or changed: on the **Games** page (top bar), or by hand (the format of `internal/games/games.json`; an entry with the key of a built-in game replaces it). Read again within seconds when it changes, no restart; a mistake in it is shown with its line and column on the Games page and in Activity, and the games stay as they were until it is fixed. The built-in list (`internal/games/games.json`) is part of szm.exe: build again after changing it. |
| `steamcmd\` | steamcmd, downloaded when first needed (unless you point to yours in Settings). |
| `cache\` | Shared copies of the games, for servers in shared-copy mode. |
| `szm.log` | What the service prints. |
| `setup-code.txt` | Until the admin account is created: the code to create it from another machine. |
| `backups\` | The backups, a folder per server (unless Settings > Backups says another folder). |
| `crashes\` | The crash reports, and `crashes\dumps` the crash dumps Windows saves. |
| `addons\` | The addon packages (unless Settings > Addons says another folder). |
| `icons\` | Game icons uploaded on the Games page. |
| `rclone\` | rclone and its `rclone.conf`, for off-site copies to Google Drive, OneDrive… |
| `links.json` | The machines linked to this manager (on a hub), or the hub it is linked to (on a node), with their secrets. |

`szm.json`, `users.json`, `links.json`, `rclone\rclone.conf` and the logs hold RCON passwords, tokens and access to your cloud drive: keep them private, and out of any backup you share.

## Performance

There is no limit on the number of servers. What the manager does for each running server:

- **Every second:** a quick look at its state, and at its console log (read only when the server wrote something).
- **Every 5 seconds:** its CPU and RAM, read from the system.
- **Every 30 seconds:** one Steam query (A2S), in the background: a server that does not answer never holds up the others.
- **About 50 KB of memory:** mostly its last 600 console lines, for the Console tab.

Update checks are per game, not per server: 50 CS2 servers ask Steam once.

## Security

- Sign-in uses a session cookie (`HttpOnly`, `SameSite=Strict`, and `Secure` over HTTPS).
- Every change must carry a header that a page from another site cannot send.
- Failed sign-ins are slowed down.
- File access is limited to each server's own folder.
- Forwarded headers are trusted only from a proxy on the same machine.
- On its own, szm speaks plain HTTP. On the internet, put it behind IIS with HTTPS (see [INSTALL.md](INSTALL.md#your-domain-with-https)), or keep it on `127.0.0.1:8888` and reach it through RDP or an SSH tunnel.
