# Building SnipeZilla Server Manager 2

For people who want to build the program themselves, or work on it. To just use it, download it from [snipezilla.com](https://snipezilla.com) and see [INSTALL.md](INSTALL.md).

- [What you need](#what-you-need)
- [Get the code](#get-the-code)
- [Build](#build)
- [Tests](#tests)
- [Version and icon](#version-and-icon)
- [Making a release](#making-a-release)
- [Git, step by step](#git-step-by-step)
- [The code](#the-code)

## What you need

- **Go 1.27 or newer:** [go.dev/dl](https://go.dev/dl/). Nothing else: no C compiler, and no download while building, because the one library used (`golang.org/x/sys`, for the Windows service and process APIs) is in `third_party/`.
- **Git** to get the code: [git-scm.com](https://git-scm.com), or [GitHub Desktop](https://desktop.github.com) if you prefer buttons to commands.

## Get the code

```
git clone https://github.com/SnipeZilla/szm.git
cd szm
```

Or, on GitHub, **Code → Download ZIP**, and unzip it.

## Build

**On Windows:** double-click `szm-gobuild.bat` and pick **Windows**, **Linux** or **Both**. It writes `szm.exe` and `szm` in the folder. If `szm.exe` is running (the manager, its service), Windows does not let it be replaced: the new one is then `szm-new.exe`, to rename once the manager is stopped.

**By hand**, on any system:

```
go build -trimpath -ldflags "-s -w" -o szm.exe ./cmd/szm                                     (Windows, on Windows)
GOOS=windows GOARCH=amd64 go build -trimpath -ldflags "-s -w" -o szm.exe ./cmd/szm            (Windows, from Linux or macOS)
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "-s -w" -o szm ./cmd/szm   (Linux)
```

In a Windows command prompt, set the variables first: `set GOOS=linux`, `set GOARCH=amd64`, `set CGO_ENABLED=0`, then `go build ...`.

`-ldflags "-s -w"` leaves out the debug information (a smaller file). `-trimpath` leaves out the folders of your computer.

## Tests

```
go test ./...
go vet ./...
```

To try a build without touching a real installation, give it its own data folder and port:

```
szm -data test-data -listen 127.0.0.1:8899
```

## Version and icon

The version is `var version = "2.0.0"` in `cmd/szm/main.go`. It shows in `szm version`, on the web page, and in the **Properties** of `szm.exe` (with the product name, description, copyright and icon, `cmd/szm/szm.ico`).

The Properties come from `cmd/szm/rsrc_windows_amd64.syso`. After changing the version or the icon, write it again:

```
go generate ./cmd/szm
```

It runs `tools/mkres`, plain Go, nothing to install. `szm-gobuild.bat` does it before each Windows build.

## Making a release

1. Change the version in `cmd/szm/main.go`.
2. Build both (`szm-gobuild.bat`, **Both**) and run the tests.
3. Try the new `szm.exe` on a test machine, or with `-data test-data` as above.
4. Write down each file's checksum, so people can check their download: double-click `szm-sha256.bat`. It writes `SHA256SUMS.txt` (upload it next to the files, or paste the numbers under the download links). By hand:

   ```
   certutil -hashfile szm.exe SHA256        (Windows)
   sha256sum szm                            (Linux)
   ```

   On Linux, `sha256sum -c SHA256SUMS.txt` checks the downloaded files against it.

5. Post `szm.exe` and `szm` on the forum, with the version, what changed and the checksums.
6. Commit, tag and push (below):

   ```
   git add -A
   git commit -m "Version 2.0.1"
   git tag v2.0.1
   git push
   git push --tags
   ```

The programs themselves are not in git: `.gitignore` leaves out `*.exe` and `szm`, and everything the manager writes next to itself (`szm.json`, `users.json`, logs, backups…), which holds passwords and tokens.

## Git, step by step

### The first time

1. On [github.com](https://github.com), create the repository (**New**), empty: no README, no .gitignore, no license (they are in the folder already).
2. In the folder of the code:

   ```
   git config --global user.name "Your name"
   git config --global user.email "you@example.com"
   git init
   git add -A
   git status
   ```

   `git status` lists what will be sent. Check that no `szm.exe`, `szm.json`, `users.json` or `logs` are in it. `.gitignore` keeps them out, but check once.

3. Then:

   ```
   git commit -m "SnipeZilla Server Manager 2.0.0"
   git branch -M main
   git remote add origin https://github.com/SnipeZilla/szm.git
   git push -u origin main
   ```

   The first push asks you to sign in to GitHub (a browser window opens).

### After a change

```
git status                      what changed
git diff                        the changes, line by line
git add -A                      take them all
git commit -m "What I changed"
git push
```

### Useful

```
git log --oneline               the history
git restore <file>              undo your changes to a file (since the last commit)
git pull                        get the changes made elsewhere (another computer, GitHub)
```

With **GitHub Desktop**, the same is buttons: the changed files are listed, write the summary, **Commit to main**, then **Push origin**.

## The code

| Folder | What it is |
| --- | --- |
| `cmd/szm` | The program: its commands and the Windows service. `rsrc_windows_amd64.syso` holds the icon and Properties of `szm.exe`. |
| `internal/manager` | Runs the servers: watching, crashes and crash reports, updates, restarts, shared copies, addons, backups and off-site copies, the console. |
| `internal/web` | The API, sign-in and users, the file manager. `static/` is the web page: plain JavaScript and CSS, no build step, built into the program. |
| `internal/config` | `szm.json`, a server's command line, the 1.x import. |
| `internal/games` | The game catalog. `games.json` is built into the program; the `games.json` next to szm.exe adds or replaces games. |
| `internal/steam` | steamcmd, `steam.inf` versions, Steam's up-to-date check. |
| `internal/query` | A2S queries, and RCON for Source and GoldSrc. |
| `internal/proc` | Starting, finding, measuring and ending processes, on Windows and Linux. |
| `internal/events` | The activity log. |
| `internal/netmon` | The network of each server's process (Windows ETW; Linux packet headers). |
| `internal/crashinfo` | What the system knows of a crash: Application log, kernel log, exit codes, crash dumps. |
| `internal/notify` | Discord and XenForo. |
| `internal/link` | Machines: the link between a hub and its nodes (pairing, the requests a hub passes to a node, what a node pushes). |
| `third_party` | `golang.org/x/sys`, bundled. |
| `tools/mkres` | Writes the icon and Properties of `szm.exe`. |
| `deploy/iis/web.config` | IIS in front of szm (also in [INSTALL.md](INSTALL.md#iis-windows)). |
| `deploy/linux/szm.service` | systemd on Linux (also in [INSTALL.md](INSTALL.md#linux)). |
| `szm-gobuild.bat` | The build menu (Windows). |
| `szm-sha256.bat` | Writes `SHA256SUMS.txt`, the checksums of `szm.exe` and `szm`, for a release. |
| `szm-service.bat` | A menu for the Windows service: install, start, stop, restart… Keep it next to `szm.exe`. |
