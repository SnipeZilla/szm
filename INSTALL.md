# Installing SnipeZilla Server Manager 2

- [Windows](#windows)
- [Linux](#linux)
- [Coming from SnipeZilla Srcds Manager 1.x](#coming-from-snipezilla-srcds-manager-1x)
- [Your domain with HTTPS](#your-domain-with-https)
- [Several machines on one page](#several-machines-on-one-page)

Download `szm.exe` (Windows) or `szm` (Linux) from [snipezilla.com](https://snipezilla.com). It is the whole program.

---

## Windows

64-bit Windows 10, 11, or Windows Server 2016 or newer.

### Install

1. Make a folder for the manager, for example `C:\SnipeZilla`, and put `szm.exe` in it. Its settings, users, logs, backups and steamcmd will live next to it.
2. **Unblock** it: right-click `szm.exe`, then **Properties**. If there is an **Unblock** box at the bottom, tick it and click **OK**. (Windows marks files downloaded from the internet; if you skip this, Windows may say "Windows protected your PC": click **More info**, then **Run anyway**.)
3. Open a command prompt **as administrator**: Start menu, type `cmd`, right-click **Command Prompt**, **Run as administrator**. Then:

   ```
   cd /d C:\SnipeZilla
   szm install
   ```

   This installs and starts the Windows service **SnipeZilla Server Manager**. It starts with Windows and comes back by itself if it ever stops.
4. Open **http://localhost:8888** on the server and create the admin account.

**From another computer:** open `http://<server-ip>:8888`. Creating the admin from there asks for the **setup code**, which is in `setup-code.txt` next to `szm.exe`. Windows' firewall must let port 8888 in. In the same administrator command prompt:

```
netsh advfirewall firewall add rule name="SnipeZilla Server Manager" dir=in action=allow protocol=TCP localport=8888
```

The game servers need their own ports open too (27015 and up, UDP and TCP), as with any game server. For example, for the ports 27015 to 27030:

```
netsh advfirewall firewall add rule name="Game servers UDP" dir=in action=allow protocol=UDP localport=27015-27030
netsh advfirewall firewall add rule name="Game servers TCP" dir=in action=allow protocol=TCP localport=27015-27030
```

> **On the internet**, the manager speaks plain HTTP. Use it behind your domain with HTTPS ([below](#your-domain-with-https)), or keep it to the server and reach it through Remote Desktop.

### Update the manager

The game servers keep running while you do this.

```
cd /d C:\SnipeZilla
szm stop
```

Replace `szm.exe` with the new one (unblock it as above), then:

```
szm start
```

The manager finds its running servers again and keeps watching them.

### Uninstall

```
szm uninstall
```

This removes the service. The game servers keep running; stop them first on the web page if you want them stopped. Then delete the folder.

### Locked out?

In a command prompt **as administrator**, in the manager's folder (`cd /d C:\SnipeZilla`):

```
szm password admin            a new password for admin, or your user's name (it asks for it)
szm listen 127.0.0.1:8888     the web address: 127.0.0.1:8888 this machine only, :8888 every address
```

A new password works at once, even while the manager runs. A user that does not exist is created, as an admin. A new web address is used at the next start of the manager: `szm stop`, then `szm start`.

### Useful commands

In a command prompt as administrator, in the manager's folder:

```
szm                        run in this window (or as the service, when Windows starts it)
szm install                install and start the Windows service
szm uninstall              remove the service (the game servers keep running)
szm start | stop           start or stop the service
szm import <file>          import the old manager's config.xml
szm password <user>        set a user's password
szm listen <address>       save the web address (used at the next start)
szm version
```

### Without the service

Double-click `szm.exe`: the manager runs in that window until you close it. It is handy to try it out. Crash dumps and the network of each server need it to run as administrator or as the service.

---

## Linux

64-bit Linux with systemd: Ubuntu 20.04 or newer, Debian 11 or newer. Every command below is typed in a terminal on the server (over SSH), as a user that can use `sudo`.

### Install

1. **A user for the manager.** The game servers will run as this user, in `/opt/szm`:

   ```
   sudo useradd -r -m -d /opt/szm -s /usr/sbin/nologin szm
   ```

2. **The 32-bit libraries steamcmd needs:**

   ```
   sudo dpkg --add-architecture i386
   sudo apt update
   sudo apt install -y lib32gcc-s1 lib32stdc++6
   ```

3. **Put `szm` on the server.** From your Windows computer, send the downloaded file with [WinSCP](https://winscp.net) (drag it into `/tmp`), or from a Windows command prompt:

   ```
   scp szm youruser@your-server-ip:/tmp/
   ```

   Then, on the server:

   ```
   sudo mv /tmp/szm /opt/szm/szm
   sudo chmod +x /opt/szm/szm
   sudo chown -R szm:szm /opt/szm
   ```

4. **Run it as a service**, so it starts with the server and comes back if it stops. Create the service file:

   ```
   sudo nano /etc/systemd/system/szm.service
   ```

   Paste this, then save with **Ctrl+O**, **Enter**, and quit with **Ctrl+X**:

   ```ini
   [Unit]
   Description=SnipeZilla Server Manager
   After=network-online.target
   Wants=network-online.target

   [Service]
   User=szm
   Group=szm
   WorkingDirectory=/opt/szm
   ExecStart=/opt/szm/szm -data /opt/szm
   Restart=always
   RestartSec=5
   KillMode=process
   LimitNOFILE=65536
   AmbientCapabilities=CAP_NET_RAW

   [Install]
   WantedBy=multi-user.target
   ```

   `KillMode=process` keeps the game servers running when the manager stops or restarts. `CAP_NET_RAW` lets it count each server's network.

5. **Start it:**

   ```
   sudo systemctl daemon-reload
   sudo systemctl enable --now szm
   ```

6. **The setup code**, to create the admin account from your computer:

   ```
   sudo cat /opt/szm/setup-code.txt
   ```

7. **Open the port** if the firewall is on (`sudo ufw status` says "active"):

   ```
   sudo ufw allow 8888/tcp
   ```

   and the game servers' ports too, for example `sudo ufw allow 27015:27030/udp` and `sudo ufw allow 27015:27030/tcp`.

8. Open **http://your-server-ip:8888**, enter the setup code and create the admin account.

> **On the internet**, the manager speaks plain HTTP. Use it behind your domain with HTTPS ([below](#your-domain-with-https)), or keep it to the server and reach it through an SSH tunnel (`ssh -L 8888:127.0.0.1:8888 you@server`, then `http://localhost:8888` on your computer).

When you add a server, its folder is a Linux path such as `/opt/szm/servers/Counter-Strike 2 - Server 1`.

### Locked out?

```
sudo /opt/szm/szm password admin            a new password for admin, or your user's name (it asks for it)
sudo /opt/szm/szm listen 127.0.0.1:8888     the web address: 127.0.0.1:8888 this machine only, :8888 every address
```

A new password works at once, even while the manager runs. A user that does not exist is created, as an admin. A new web address is used at the next start of the manager: `sudo systemctl restart szm` (the game servers keep running).

### Useful commands

```
sudo systemctl status szm        is it running?
sudo journalctl -u szm -f        what it prints, live (Ctrl+C to stop watching)
sudo systemctl restart szm       restart the manager (the game servers keep running)
sudo systemctl stop szm          stop it (start: the same with start)
sudo /opt/szm/szm import <file>  import the old manager's config.xml (the manager stopped)
/opt/szm/szm version
```

### Update the manager

Send the new `szm` to `/tmp` as in step 3, then:

```
sudo systemctl stop szm
sudo mv /tmp/szm /opt/szm/szm
sudo chmod +x /opt/szm/szm
sudo chown szm:szm /opt/szm/szm
sudo systemctl start szm
```

### Uninstall

```
sudo systemctl disable --now szm
sudo rm /etc/systemd/system/szm.service
sudo systemctl daemon-reload
```

The game servers keep running: stop them on the web page first if you want them stopped. `sudo userdel -r szm` then removes the user and everything in `/opt/szm`, servers included.

### What differs on Linux

- `srcds_run` and `hlds_run` get `-norestart`: the manager restarts a crashed server itself.
- **Steam's library** (`steamclient.so`): before a server starts, the manager links steamcmd's into `~/.steam/sdk32` and `~/.steam/sdk64` of its user (`/opt/szm/.steam`), where Source servers look for it. Without it a server says "Unable to load Steam support library" and plays on the LAN only. The `steam` packages of Ubuntu (the Steam client) are not needed.
- No crash dumps; a crash report has the kernel's line about it instead (segfault, out of memory).
- No check of which program holds a server's RCON port (a Windows problem), and no IIS: use nginx or Caddy (below).

---

## Coming from SnipeZilla Srcds Manager 1.x

1. **Stop the old manager first.** If both run, they fight over the same servers.
2. In **Settings → Import**, click **Browse** and pick the old `config.xml`. Or, with the new manager stopped: `szm import "C:\path\config.xml"`.

What comes over:

- **Every server:** folder, IP, port, map, max players, RCON password, say command and priority.
- **Each server's options**, from `sz.cmd.cfg` when there is one. The Steam token goes into its own field.
- **Update mode:** `xcopy` becomes the shared-copy mode, with the old `apps\` folder as the shared copies.
- **Server state:** servers the old manager had stopped (`sz.stop.txt`) stay stopped. Running ones are taken over without a restart.
- **Settings:** steamcmd path, priority, restart hour and log days.

---

## Your domain with HTTPS

The manager has its own small web server. A web server you already run can sit in front of it, so the manager is at your domain with your certificate, for example `https://manager.snipezilla.com` or `https://snipezilla.com/szm/`.

First, **keep the manager off the internet**: in Settings, set **Web interface address** to `127.0.0.1:8888`, then restart the manager. Only the web server on the same machine can reach it then.

### IIS (Windows)

1. Install two free IIS modules from Microsoft: **URL Rewrite** and **Application Request Routing (ARR)**.
2. In IIS Manager, select the server, open **Application Request Routing Cache**, then **Server Proxy Settings**:
   - tick **Enable proxy**;
   - set **Response buffer threshold (KB)** to **0**, so the live updates are not held back;
   - set **Time-out (seconds)** to **3600**: its 120 s cut uploads of big files (maps) short.

   Or all three at once, in a Command Prompt or PowerShell as administrator (these settings are the server's: a `web.config` can not hold them). It answers `Applied configuration changes to section "system.webServer/proxy"`:

   ```
   C:\Windows\System32\inetsrv\appcmd.exe set config -section:system.webServer/proxy /enabled:"True" /responseBufferLimit:"0" /timeout:"01:00:00" /commit:apphost
   ```

   Click **Apply**.
3. Create the site (a subdomain such as `manager.snipezilla.com`), or use a folder of an existing site (`szm`). Bind your certificate for HTTPS.
4. In that site or folder, create a file named `web.config` with this:

   ```xml
   <?xml version="1.0" encoding="UTF-8"?>
   <configuration>
     <system.webServer>
       <rewrite>
         <rules>
           <!-- example.com/szm -> example.com/szm/ -->
           <rule name="SZM trailing slash" stopProcessing="true">
             <match url="^$" />
             <conditions>
               <add input="{URL}" pattern="[^/]$" />
             </conditions>
             <action type="Redirect" url="{URL}/" redirectType="Permanent" />
           </rule>
           <!-- Optional: always HTTPS. Remove the comment marks once the site has its certificate.
           <rule name="SZM https" stopProcessing="true">
             <match url="(.*)" />
             <conditions>
               <add input="{HTTPS}" pattern="off" />
             </conditions>
             <action type="Redirect" url="https://{HTTP_HOST}{REQUEST_URI}" redirectType="Permanent" />
           </rule>
           -->
           <rule name="SZM proxy" stopProcessing="true">
             <match url="(.*)" />
             <action type="Rewrite" url="http://127.0.0.1:8888/{R:1}" />
           </rule>
         </rules>
       </rewrite>
       <!-- Compression would hold back the live updates -->
       <urlCompression doStaticCompression="false" doDynamicCompression="false" />
       <security>
         <requestFiltering>
           <!-- Uploads up to 4 GB (maps, plugins) instead of 30 MB -->
           <requestLimits maxAllowedContentLength="4294967295" />
         </requestFiltering>
       </security>
     </system.webServer>
   </configuration>
   ```

   Change `8888` if the manager listens on another port.

5. **Only if saving fails with "405 Method Not Allowed"**: the WebDAV feature of IIS is installed and takes the saves for itself. Check with `C:\Windows\System32\inetsrv\appcmd.exe list modules | findstr /i webdav`. The simplest: remove it (Server Manager → Remove Roles and Features → Web Server → Common HTTP Features → untick **WebDAV Publishing**). Or keep it and, in a Command Prompt or PowerShell as administrator, unlock two sections:

   ```
   C:\Windows\System32\inetsrv\appcmd.exe unlock config -section:system.webServer/handlers
   C:\Windows\System32\inetsrv\appcmd.exe unlock config -section:system.webServer/modules
   ```

   then add to the `web.config`, inside `<system.webServer>`:

   ```xml
   <modules>
     <remove name="WebDAVModule" />
   </modules>
   <handlers>
     <remove name="WebDAV" />
   </handlers>
   ```

   (Without the unlock, IIS refuses the file: "This configuration section cannot be used at this path", error 0x80070021.)

### nginx (Linux)

In the `server { ... }` block of your site, with its certificate:

```nginx
location / {
    proxy_pass http://127.0.0.1:8888/;
    proxy_http_version 1.1;
    proxy_set_header Host $host;
    proxy_set_header X-Forwarded-For $remote_addr;
    proxy_set_header X-Forwarded-Proto $scheme;
    proxy_buffering off;              # the live updates
    proxy_read_timeout 1h;
    client_max_body_size 4g;          # uploads
}
```

For a folder instead (`/szm/`), use `location /szm/ { ... }` with the same lines.

### Caddy (Linux)

Caddy gets the certificate by itself:

```
manager.example.com {
    reverse_proxy 127.0.0.1:8888 {
        flush_interval -1
    }
}
```

### What the manager does behind a proxy

- The **setup code** is always asked to create the first admin, though every request comes from the proxy on 127.0.0.1.
- **Failed sign-ins** are counted per real visitor, not for everyone at once.
- The **session cookie** is marked HTTPS-only when the browser uses HTTPS.
- If the proxy holds back the live updates anyway, the page asks every few seconds instead.

---

## Several machines on one page

One manager, the **hub**, can show the servers of your other machines: their managers link to it. For example, a Windows server in the US as the hub, and an Ubuntu server in Europe linked to it. What it does is in [GUIDE.md](GUIDE.md#machines-one-page-for-several-machines).

1. **Install the manager on each machine** as above (Windows or Linux): each one runs its own servers.
2. **The hub must be reachable with HTTPS** from the other machines: its domain behind IIS, nginx or Caddy ([above](#your-domain-with-https)), for example `https://manager.snipezilla.com/`. Nothing to open on the other machines: they connect to the hub.
3. **On the hub:** Settings → Machines. Give this machine a name (US), then **Add a machine**: its name (EU), and the hub's address as the other machine reaches it, `https://…`. The page fills in the one you are using, unless it is `localhost` (the hub's own page, opened on the hub): the other machine could not reach that. Click **Add a machine** and copy the code (`SZM-…`, good once for 30 minutes).
4. **On the other machine's manager:** Settings → Machines → **Or link this manager to a hub**, paste the code, **Link**. It says "Connected", and its servers show on the hub's page.

Then the other machine's manager can stay off the internet: set its **Web interface address** to `127.0.0.1:8888` (Settings → General, or `szm listen 127.0.0.1:8888`) and restart it, then use the hub's page. On Linux, reach it when needed with an SSH tunnel: `ssh -L 8888:127.0.0.1:8888 you@server`, then open `http://localhost:8888`.

**Behind IIS:** nothing more than the steps above (the link uses the same address and the same live updates as the page). If the other machine says "the connection to the hub was lost" over and over, check the **Response buffer threshold (KB)** of step 2 of IIS is **0**.

