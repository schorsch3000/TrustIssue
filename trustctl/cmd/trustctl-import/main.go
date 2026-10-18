// trustctl-import stores the accounts of a Google Authenticator export on a Trust Issue device.
//
// It opens a page on a local web server (127.0.0.1, random port and token) in the browser: the
// browser shows the webcam and finds the QR codes of the export, then lists the accounts with a
// checkbox and an editable name each; "Import selected" runs `trustctl add` for every ticked
// one. The binary is static and needs nothing but a browser and trustctl.
//
//	trustctl-import [--trustctl PATH] [--no-browser] [-- OPTIONS FOR TRUSTCTL]
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"time"
)

const idleLimit = 30 * time.Minute

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	set := flag.NewFlagSet("trustctl-import", flag.ContinueOnError)
	binary := set.String("trustctl", "", "the trustctl binary (default: $TRUSTCTL, next to this program, or the PATH)")
	noBrowser := set.Bool("no-browser", false, "do not open the browser, only print the address")
	set.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: trustctl-import [--trustctl PATH] [--no-browser] [-- OPTIONS FOR TRUSTCTL]")
		fmt.Fprintln(os.Stderr, "The options after -- go in front of \"add\", e.g. -- --device /dev/ttyUSB0")
	}
	if err := set.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 64
	}
	path, err := findTrustctl(*binary)
	if err != nil {
		fmt.Fprintln(os.Stderr, "trustctl-import:", err)
		return 1
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		fmt.Fprintln(os.Stderr, "trustctl-import:", err)
		return 1
	}
	token := randomToken()
	srv := newServer(token, listener.Addr().String(), append([]string{path}, set.Args()...))
	httpServer := &http.Server{Handler: srv.handler(), ReadHeaderTimeout: 10 * time.Second}
	go httpServer.Serve(listener)

	address := fmt.Sprintf("http://%s/?t=%s", listener.Addr(), token)
	fmt.Println("Open", address)
	if !*noBrowser {
		if err := openBrowser(address); err != nil {
			fmt.Fprintln(os.Stderr, "could not start a browser, open the address above yourself:", err)
		}
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	idle := time.NewTicker(time.Minute)
	defer idle.Stop()
	for {
		select {
		case <-ctx.Done():
		case <-srv.quit:
		case <-idle.C:
			srv.mu.Lock()
			expired := !srv.running && time.Since(srv.lastUse) > idleLimit
			srv.mu.Unlock()
			if !expired {
				continue
			}
			fmt.Fprintln(os.Stderr, "trustctl-import: nobody used the page for", idleLimit, "- stopping")
		}
		break
	}
	shutdown, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	httpServer.Shutdown(shutdown)
	return 0
}

func randomToken() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}

// findTrustctl looks at --trustctl, $TRUSTCTL, the directory of this program and the PATH.
func findTrustctl(given string) (string, error) {
	if given == "" {
		given = os.Getenv("TRUSTCTL")
	}
	if given != "" {
		if p, err := exec.LookPath(given); err == nil {
			return p, nil
		}
		return "", fmt.Errorf("trustctl not found: %s", given)
	}
	name := "trustctl"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	if self, err := os.Executable(); err == nil {
		sibling := filepath.Join(filepath.Dir(self), name)
		if info, err := os.Stat(sibling); err == nil && !info.IsDir() {
			return sibling, nil
		}
	}
	if p, err := exec.LookPath(name); err == nil {
		return p, nil
	}
	return "", errors.New("trustctl not found: give its path with --trustctl or put it in the PATH")
}

func openBrowser(address string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", address)
	case "darwin":
		cmd = exec.Command("open", address)
	default:
		cmd = exec.Command("xdg-open", address)
	}
	return cmd.Start()
}
