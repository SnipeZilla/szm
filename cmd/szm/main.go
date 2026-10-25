// SnipeZilla Server Manager: installs, updates, runs and watches Source, Source 2 and GoldSrc game servers, with a
// web interface. One program, no dependencies.
//
//	szm                    run (in this window; as a Windows service when the service starts it)
//	szm install            install and start the Windows service
//	szm uninstall          remove the Windows service (the game servers keep running)
//	szm start | stop       start or stop the Windows service
//	szm import <file>      import the servers of the old manager's config.xml
//	szm password <user>    set a user's password (when locked out)
//	szm listen <address>   save the web address (127.0.0.1:8888, :8888); the manager uses it at its next start
//	szm version
//
// Options: -data <folder> (default: the folder of szm.exe), -listen <address> (default: from szm.json, :8888).
package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"github.com/SnipeZilla/szm/internal/netcount"
	"log"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"

	"github.com/SnipeZilla/szm/internal/config"
	"github.com/SnipeZilla/szm/internal/games"
	"github.com/SnipeZilla/szm/internal/manager"
	"github.com/SnipeZilla/szm/internal/proc"
	"github.com/SnipeZilla/szm/internal/web"
)

// version of the manager. It is also the product and file version of szm.exe's Properties: after changing it (or
// szm.ico), run "go generate ./cmd/szm" to write rsrc_windows_amd64.syso again.
//
//go:generate go run ../../tools/mkres
var version = "2.0.0"

const serviceName = "SnipeZillaServerManager"

func main() {
	manager.Version = version
	netcount.CountHTTP() // the manager's own traffic: its HTTP clients' connections counted
	dataFlag := flag.String("data", "", "data folder (default: the folder of szm.exe)")
	listenFlag := flag.String("listen", "", "address of the web interface (default: from szm.json)")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "SnipeZilla Server Manager %s\n\n", version)
		fmt.Fprintln(os.Stderr, "  szm                  run (in this window)")
		fmt.Fprintln(os.Stderr, "  szm install          install and start the Windows service")
		fmt.Fprintln(os.Stderr, "  szm uninstall        remove the Windows service")
		fmt.Fprintln(os.Stderr, "  szm start | stop     start or stop the Windows service")
		fmt.Fprintln(os.Stderr, "  szm import <file>    import the old manager's config.xml")
		fmt.Fprintln(os.Stderr, "  szm password <user>  set a user's password")
		fmt.Fprintln(os.Stderr, "  szm listen <address> save the web address: 127.0.0.1:8888 (this machine only), :8888 (all)")
		fmt.Fprintln(os.Stderr, "  szm version")
		fmt.Fprintln(os.Stderr, "\nOptions:")
		flag.PrintDefaults()
	}
	flag.Parse()

	dataDir := *dataFlag
	if dataDir == "" {
		exe, err := os.Executable()
		if err != nil {
			log.Fatal(err)
		}
		exe, _ = filepath.EvalSymlinks(exe)
		dataDir = filepath.Dir(exe)
	}
	dataDir, _ = filepath.Abs(dataDir)

	args := flag.Args()
	cmd := ""
	if len(args) > 0 {
		cmd = strings.ToLower(args[0])
	}
	var err error
	switch cmd {
	case "":
		if isService() {
			err = runService(serviceName, func(ctx context.Context) error { return run(ctx, dataDir, *listenFlag, true) })
		} else {
			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			err = run(ctx, dataDir, *listenFlag, false)
		}
	case "install":
		err = installService(serviceName, dataDir)
	case "uninstall", "remove":
		err = uninstallService(serviceName)
	case "start":
		err = startService(serviceName)
	case "stop":
		err = stopService(serviceName)
	case "import":
		if len(args) < 2 {
			err = fmt.Errorf("szm import <path of config.xml>")
			break
		}
		err = importOld(dataDir, args[1])
	case "password", "passwd":
		if len(args) < 2 {
			err = fmt.Errorf("szm password <user>")
			break
		}
		err = setPassword(dataDir, args[1])
	case "listen":
		if len(args) < 2 {
			err = fmt.Errorf("szm listen <address>: 127.0.0.1:8888 (this machine only) or :8888 (every address)")
			break
		}
		err = setListen(dataDir, args[1])
	case "__console": // the helper reading a server's console window (CS2 on Windows): proc.ReadConsoleWindow
		if len(args) < 3 {
			err = fmt.Errorf("szm __console <pid> <rows>")
			break
		}
		pid, _ := strconv.Atoi(args[1])
		rows, _ := strconv.Atoi(args[2])
		err = proc.DumpConsole(pid, rows, os.Stdout)
	case "__ctrlc": // the helper sending Ctrl+C to a server's console (Windows): proc.Interrupt
		if len(args) < 2 {
			err = fmt.Errorf("szm __ctrlc <pid>")
			break
		}
		pid, _ := strconv.Atoi(args[1])
		err = proc.SendCtrlC(pid)
	case "version", "-v", "--version":
		fmt.Println("SnipeZilla Server Manager", version)
	case "help", "-h", "--help", "/?":
		flag.Usage()
	default:
		flag.Usage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}

// run runs the manager and its web interface until the context ends.
func run(ctx context.Context, dataDir, listen string, service bool) error {
	leanMemory()
	if service {
		// No window: what would go to the console goes to szm.log
		p := filepath.Join(dataDir, "szm.log")
		if st, err := os.Stat(p); err == nil && st.Size() > 5<<20 {
			_ = os.Rename(p, p+".old")
		}
		if f, err := os.OpenFile(p, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644); err == nil {
			log.SetOutput(f)
			os.Stderr = f
			os.Stdout = f
		}
	}
	cat, gamesErr := games.LoadOrBuiltin(dataDir)
	if cat == nil {
		return gamesErr
	}
	if gamesErr != nil {
		log.Printf("%v: the built-in games are used until it is fixed", gamesErr)
	}
	cfg, err := config.Load(filepath.Join(dataDir, "szm.json"))
	if err != nil {
		return err
	}
	firstRun := false
	if _, err := os.Stat(cfg.Path()); os.IsNotExist(err) {
		firstRun = true
	}
	if cfg.AddonsDir == "" {
		// The addon packages live in the manager's folder unless the admin picks another (Settings > Addons)
		cfg.AddonsDir = filepath.Join(dataDir, "addons")
		cfg.Upgraded = append(cfg.Upgraded, "the addon packages folder ("+cfg.AddonsDir+")")
	}
	_ = os.MkdirAll(cfg.AddonsDir, 0o755)
	if firstRun || len(cfg.Upgraded) > 0 {
		if err := cfg.Save(); err != nil {
			return fmt.Errorf("cannot write in %s: %w", dataDir, err)
		}
	}
	m := manager.New(cfg, cat, dataDir)
	if gamesErr != nil {
		m.SetGamesError(gamesErr)
	}
	w, err := web.New(m)
	if err != nil {
		return err
	}
	addr := cfg.Listen
	if listen != "" {
		addr = listen
	}

	log.Printf("SnipeZilla Server Manager %s — data in %s", version, dataDir)
	for _, u := range cfg.Upgraded {
		log.Printf("Settings: %s set to the new default", u)
	}
	log.Printf("Web interface: http://%s/", displayAddr(addr))
	if code := w.SetupCode(); code != "" {
		log.Printf("First run: open the web interface to create the admin. From another machine, the setup code is %s (also in setup-code.txt)", code)
	}

	mctx, cancel := context.WithCancel(ctx)
	defer cancel()
	done := make(chan struct{})
	go func() { m.Run(mctx); close(done) }()
	err = w.ListenAndServe(mctx, addr)
	cancel()
	<-done
	if err != nil {
		return fmt.Errorf("web interface on %s: %w", addr, err)
	}
	return nil
}

func displayAddr(addr string) string {
	if strings.HasPrefix(addr, ":") {
		return "localhost" + addr
	}
	return addr
}

func importOld(dataDir, path string) error {
	cat, err := games.Load(dataDir)
	if err != nil {
		return err
	}
	cfg, err := config.Load(filepath.Join(dataDir, "szm.json"))
	if err != nil {
		return err
	}
	rep, err := cfg.ImportOld(path, cat)
	if err != nil {
		return err
	}
	cfg.SortServers()
	if err := cfg.Save(); err != nil {
		return err
	}
	// The servers the old manager had stopped stay stopped
	m := manager.New(cfg, cat, dataDir)
	m.MarkStopped(rep.Stopped)
	fmt.Printf("Imported %d servers into %s\n", len(rep.Added), cfg.Path())
	for _, n := range rep.Added {
		fmt.Println("  +", n)
	}
	for _, n := range rep.Skipped {
		fmt.Println("  skipped:", n)
	}
	if len(rep.Stopped) > 0 {
		fmt.Println("  stopped (they stay stopped):", strings.Join(rep.Stopped, ", "))
	}
	fmt.Println("Stop the old manager before starting this one: both would restart the same servers.")
	return nil
}

// setListen saves the web address in szm.json (Settings, General, Web interface address), for the next start.
func setListen(dataDir, addr string) error {
	host, port, err := net.SplitHostPort(strings.TrimSpace(addr))
	if n, perr := strconv.Atoi(port); err != nil || perr != nil || n < 1 || n > 65535 || (host != "" && net.ParseIP(host) == nil && host != "localhost") {
		return fmt.Errorf("%q is not an address: 127.0.0.1:8888 (this machine only, behind your web server) or :8888 (every address)", addr)
	}
	cfg, err := config.Load(filepath.Join(dataDir, "szm.json"))
	if err != nil {
		return err
	}
	cfg.Listen = net.JoinHostPort(host, port)
	if err := cfg.Save(); err != nil {
		return err
	}
	fmt.Printf("Web address saved: %s (in %s).\n", cfg.Listen, cfg.Path())
	if runtime.GOOS == "windows" {
		fmt.Println("Restart the manager to use it: szm stop, then szm start (as administrator), or close its window and run szm again.")
	} else {
		fmt.Println("Restart the manager to use it: sudo systemctl restart szm (the game servers keep running).")
	}
	return nil
}

func setPassword(dataDir, name string) error {
	fmt.Printf("New password for %s: ", name)
	pw, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil {
		return err
	}
	return web.SetPassword(dataDir, name, strings.TrimRight(pw, "\r\n"))
}
