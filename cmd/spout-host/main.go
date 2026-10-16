package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/justjoseorg/SpoutRemotePlayHost/internal/apps"
	"github.com/justjoseorg/SpoutRemotePlayHost/internal/artwork"
	"github.com/justjoseorg/SpoutRemotePlayHost/internal/config"
	"github.com/justjoseorg/SpoutRemotePlayHost/internal/display"
	"github.com/justjoseorg/SpoutRemotePlayHost/internal/hotkey"
	"github.com/justjoseorg/SpoutRemotePlayHost/internal/notify"
	"github.com/justjoseorg/SpoutRemotePlayHost/internal/pairing"
	"github.com/justjoseorg/SpoutRemotePlayHost/internal/server"
	"github.com/justjoseorg/SpoutRemotePlayHost/internal/signin"
	"github.com/justjoseorg/SpoutRemotePlayHost/internal/steamlib"
	"github.com/justjoseorg/SpoutRemotePlayHost/internal/steamlog"
	"github.com/justjoseorg/SpoutRemotePlayHost/internal/tray"
)

// version is set at build time via -ldflags.
var version = "dev"

func main() {
	// Loopback by default; non-loopback clients must present the API token.
	addr := flag.String("listen", "127.0.0.1:47995", "address for the web UI/API (use 0.0.0.0:47995 so the Decky plugin can reach it; non-local requests need the token)")
	cfgPath := flag.String("config", "", "config file path (default: user config dir)")
	noTray := flag.Bool("no-tray", false, "do not show the system tray icon")
	background := flag.Bool("background", false, "do not open the web UI at startup (Windows opens it when launched by hand)")
	signinService := flag.Bool("signin-service", false, "run the Windows sign-in service (lets paired devices type the sign-in PIN; needs -config)")
	signinListen := flag.String("signin-listen", signin.DefaultListen, "address for the sign-in service")
	startSteam := flag.Bool("start-steam", true, "Windows: with -background, start Steam if it isn't running 5 seconds after sign-in")
	signinType := flag.Bool("signin-type", false, "internal: type the PIN read from stdin into the sign-in screen")
	persistent := flag.String("persistent-display", "", "Linux: keep one virtual monitor connected while the host runs, e.g. 2560x1600@144 (experimental; lets Steam's PipeWire capture be granted that screen once)")
	flag.Parse()

	if *signinType {
		os.Exit(signin.TypeFromStdin())
	}
	if *signinService {
		if *cfgPath == "" {
			fatal(fmt.Errorf("-signin-service needs -config pointing at the signed-in user's config.json"))
		}
		setupLog(filepath.Join(os.Getenv("ProgramData"), "SpoutRemotePlayHost"))
		log.Printf("spout-host %s sign-in service starting (listen %s)", version, *signinListen)
		if err := signin.RunService(*signinListen, filepath.Join(filepath.Dir(*cfgPath), "paired.json")); err != nil {
			fatal(err)
		}
		return
	}

	if *cfgPath == "" {
		p, err := config.DefaultPath()
		if err != nil {
			fatal(err)
		}
		*cfgPath = p
	}
	setupLog(filepath.Dir(*cfgPath))
	log.Printf("spout-host %s starting (listen %s)", version, *addr)
	_, port, _ := net.SplitHostPort(*addr)
	uiURL := "http://127.0.0.1:" + port + "/"

	// Checked before binding too: Windows lets 0.0.0.0 and 127.0.0.1 listeners share a port.
	alreadyUp := alreadyRunning(uiURL)
	var ln net.Listener
	var err error
	if !alreadyUp {
		ln, err = net.Listen("tcp", *addr)
	}
	if alreadyUp || err != nil {
		// Launching it again while it already runs (e.g. from the logon task) just shows the UI.
		if alreadyUp || alreadyRunning(uiURL) {
			if *background {
				return
			}
			log.Println("already running; opening", uiURL)
			tray.OpenURL(uiURL)
			return
		}
		fatal(fmt.Errorf("listen on %s: %w", *addr, err))
	}

	store, err := config.Open(*cfgPath)
	if err != nil {
		fatal(err)
	}

	token, err := config.LoadOrCreateToken(*cfgPath)
	if err != nil {
		fatal(err)
	}
	fmt.Println("API token for the Decky plugin / remote access:", token)

	if *background && *startSteam {
		go ensureSteam(5 * time.Second)
	}

	if *persistent != "" {
		var m display.Mode
		if _, err := fmt.Sscanf(*persistent, "%dx%d@%d", &m.Width, &m.Height, &m.RefreshHz); err != nil {
			fatal(fmt.Errorf("-persistent-display wants WIDTHxHEIGHT@HZ: %w", err))
		}
		display.Persistent = &m
	}
	disp := display.New()
	display.KeepOutputs = store.KeepDisplays

	pair, err := pairing.New(filepath.Join(filepath.Dir(*cfgPath), "paired.json"), notify.Send)
	if err != nil {
		fatal(err)
	}
	pair.URL = uiURL

	stop := make(chan struct{})
	defer close(stop)
	go func() {
		err := hotkey.Listen(stop, func() {
			log.Println("Ctrl+Alt+Shift+Q: closing virtual monitor")
			if err := disp.Destroy(); err != nil {
				log.Println("destroy:", err)
			}
		})
		if err != nil {
			log.Println("hotkey disabled:", err)
		}
	}()

	catalog, err := apps.Open(filepath.Join(filepath.Dir(*cfgPath), "apps.json"))
	if err != nil {
		log.Fatal(err)
	}
	srv := server.NewServer(store, disp, version, token, pair).WithApps(catalog, steamlib.NewCEF()).
		WithArtwork(artwork.Open(filepath.Join(filepath.Dir(*cfgPath), "steamgriddb.key")))
	handler := srv.Handler()
	go steamlog.Watch(steamlog.Candidates(), time.Second, stop, func(ev steamlog.Event) {
		if !ev.Start {
			srv.Sessions().Stop("")
			return
		}
		id := srv.ResolveClient(ev.Client)
		log.Printf("Remote Play session started (client %q, device %q)", ev.Client, id)
		if _, err := srv.Sessions().Start(id); err != nil {
			log.Println("virtual monitor:", err)
		}
	})
	fmt.Printf("spout-host %s: UI at http://%s\n", version, *addr)
	if runtime.GOOS == "windows" && !*background {
		tray.OpenURL(pair.URL)
	}
	if *noTray {
		serve(ln, handler)
		return
	}
	go serve(ln, handler)
	tray.Run(pair.URL, version, func() { os.Exit(0) })
}

func serve(ln net.Listener, h http.Handler) {
	if err := http.Serve(ln, h); err != nil {
		fatal(err)
	}
}

// alreadyRunning reports whether another spout-host answers at url.
func alreadyRunning(url string) bool {
	c := http.Client{Timeout: 2 * time.Second}
	resp, err := c.Get(url + "api/status")
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	var st struct {
		Version *string `json:"version"`
	}
	return resp.StatusCode == http.StatusOK && json.NewDecoder(resp.Body).Decode(&st) == nil && st.Version != nil
}
