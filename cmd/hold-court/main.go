// Command hold-court serves the Hold Court web UI: a mutt-style three-pane
// view over a feed directory of held PRs, with vim keys and rulings out.
// See DESIGN.md for the full spec.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/BurntSushi/toml"

	"github.com/quad341/hold-court/internal/server"
	"github.com/quad341/hold-court/internal/store"
)

// configFileName is holdcourt.toml, read from the current directory. It is
// optional: CLI flags and their defaults are enough to run without one.
const configFileName = "holdcourt.toml"

// fileConfig is holdcourt.toml's shape. OnRuling is a TOML array rather than
// a CLI flag so a hook's argv can hold arguments with spaces without any
// shell-quoting hack.
type fileConfig struct {
	Feed     string   `toml:"feed"`
	Rulings  string   `toml:"rulings"`
	OnRuling []string `toml:"on_ruling"`
}

func main() {
	if len(os.Args) < 2 || os.Args[1] != "serve" {
		fmt.Fprintln(os.Stderr, "usage: hold-court serve [flags]")
		os.Exit(2)
	}

	if err := serve(os.Args[2:]); err != nil {
		fmt.Fprintln(os.Stderr, "hold-court:", err)
		os.Exit(1)
	}
}

func serve(args []string) error {
	fset := flag.NewFlagSet("serve", flag.ExitOnError)
	feedDir := fset.String("feed", "feed", "directory of hold JSON documents to scan")
	rulingsDir := fset.String("rulings", "rulings", "directory rulings and their hook results are written/read")
	dbPath := fset.String("db", "hold-court.db", "path to the SQLite state file")
	addr := fset.String("addr", "127.0.0.1:0", "listen address (127.0.0.1:0 picks a free port)")
	user := fset.String("user", "operator", "local maintainer identity for read state and ruled_by")
	if err := fset.Parse(args); err != nil {
		return err
	}

	// CLI flags override holdcourt.toml only when explicitly passed, so an
	// unset flag's default doesn't silently clobber a configured value.
	explicit := map[string]bool{}
	fset.Visit(func(f *flag.Flag) { explicit[f.Name] = true })

	fc, err := loadFileConfig(configFileName)
	if err != nil {
		return fmt.Errorf("load %s: %w", configFileName, err)
	}
	if !explicit["feed"] && fc.Feed != "" {
		*feedDir = fc.Feed
	}
	if !explicit["rulings"] && fc.Rulings != "" {
		*rulingsDir = fc.Rulings
	}

	st, err := store.Open(*dbPath)
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	defer func() {
		if cerr := st.Close(); cerr != nil {
			fmt.Fprintln(os.Stderr, "hold-court: close store:", cerr)
		}
	}()

	handler, err := server.New(server.Config{
		FeedDir:    *feedDir,
		RulingsDir: *rulingsDir,
		Store:      st,
		OnRuling:   fc.OnRuling,
		User:       *user,
	})
	if err != nil {
		return fmt.Errorf("build server: %w", err)
	}

	ln, err := net.Listen("tcp", *addr)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", *addr, err)
	}

	fmt.Printf("Hold Court is in session: http://%s\n", ln.Addr())

	httpSrv := &http.Server{Handler: handler, ReadHeaderTimeout: 10 * time.Second}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() { errCh <- httpSrv.Serve(ln) }()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return httpSrv.Shutdown(shutdownCtx)
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}

// loadFileConfig reads holdcourt.toml if present. A missing config file is
// not an error: CLI flags and their defaults are enough to run without one.
func loadFileConfig(path string) (fileConfig, error) {
	var fc fileConfig
	_, err := toml.DecodeFile(path, &fc)
	if err != nil && errors.Is(err, os.ErrNotExist) {
		return fileConfig{}, nil
	}
	return fc, err
}
