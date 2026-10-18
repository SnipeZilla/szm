# SnipeZilla Server Manager 2

SnipeZilla Srcds Manager has been restarting crashed game servers on Windows, quietly and at all hours, for over ten years. This is its upgrade: the same proven manager, no gimmicks, rebuilt for today, and now on **Linux** too.

It installs, updates, runs and watches Source, Source 2 and GoldSrc servers (CS2, CS:S, CS 1.6, TF2, L4D2, GMod, DoD:S, HL2DM and more), from a web page. It is **one file**, `szm.exe` on Windows or `szm` on Linux: no PHP, no web server, no database to install.

**Download:** [snipezilla.com](https://snipezilla.com/resources/snipezilla-server-manager.52/). the ready-to-run program.

**Source code:** [github.com](https://github.com/SnipeZilla/szm). This repository holds the source code.

## The same job, still done right

What made 1.x worth running is all here, working the way it did:

- **Installs** servers with steamcmd (downloaded by itself when first needed).
- **Watches** every server, and restarts it when it crashes or stops answering.
- **Updates** when Steam has a new version, after warning the players; an empty server is updated at once.
- **Restarts** each server daily at a set time, or after 24 h when nobody plays.
- **Imports** your 1.x `config.xml`: servers, passwords and options.

## What is new

- **Crash reports that recognize the pattern.** Each crash says what most likely caused it (the plugin or module, the map, what changed just before), so debugging stops being guesswork.
- **Hub and nodes:** one page for all your machines, Windows and Linux, anywhere ([below](#hub-and-nodes)).
- **Addons manager:** Metamod, SourceMod, CounterStrikeSharp and AMX Mod X kept as packages, and pushed into the servers without touching their configs or the plugins you removed.
- **Posts to Discord and XenForo:** crashes, updates and restarts, each channel with what it wants. No more emails.
- **Files across servers and machines:** copy and paste between servers, even from one machine to another, in a file manager with upload and editing; plus a map cycle editor and CS2 Workshop maps.
- **Backups** of each server's settings and plugins, with a copy on **Google Drive, OneDrive** or another disk.
- **A modern web page:** live status, console and RCON, users with rights, and it works on a phone.

## Hub and nodes

Optional, and worth it with more than one machine. One manager is the **hub**, for example on the machine that already runs your website. The managers of your game machines link to it as **nodes**:

- **Safer game machines.** A node connects out to the hub, so its manager needs no open port (it listens on `127.0.0.1` only). The firewall opens the game ports and nothing else.
- **Bare-bone game machines.** No Apache, no IIS, no PHP on them: the games, plus a manager of about 20 MB of memory.
- **One secure website** (HTTPS, behind your own web server) to watch every server, add new ones, push addons, and make the Discord and forum posts for all of them.
- **You hear when a machine goes down**, and each node keeps running its servers by itself even when the hub is away.

See [Machines](GUIDE.md#machines-one-page-for-several-machines).

## Quick start

**Windows**

1. Put `szm.exe` in its own folder, for example `C:\SnipeZilla`.
2. Open a command prompt **as administrator** in that folder and run `szm install`.
3. Open **http://localhost:8888** and create the admin account.

**Linux**

Follow the step-by-step guide in [INSTALL.md](INSTALL.md#linux). It takes about 10 commands.

## Documentation

| | |
| --- | --- |
| [INSTALL.md](INSTALL.md) | Installing on Windows and Linux, updating, uninstalling, locked out, your domain with HTTPS (IIS, nginx), moving from 1.x, several machines. |
| [GUIDE.md](GUIDE.md) | Everything it does, in detail: crash reports, updates, backups, addons, machines, rights, commands, files. |
| [BUILD.md](BUILD.md) | Building from the source, the code, making a release. |

## Support

On the [SnipeZilla forum](https://snipezilla.com).
