// Command profilesrv is the public profile server (internal/profilesrv).
//
//	profilesrv -domain profiles.mrwproject.com        serve HTTPS on :443 (+ :80 for certificates)
//	profilesrv -dev 127.0.0.1:8787                     serve plain HTTP for local testing
//	profilesrv admin stats|reports                     moderation, on the same data directory
//	profilesrv admin hide|unhide|delete|sources <id>
//	profilesrv admin drop-follows <id> <address-hash>
//	profilesrv admin backup <file>                     consistent copy of the live database
//	profilesrv verify <file>                           integrity check of a copy (read-only)
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"golang.org/x/crypto/acme/autocert"

	"poe2filter/internal/profilesrv"
)

func main() {
	dataDir := flag.String("data", envOr("STATE_DIRECTORY", "profilesrv-data"), "state directory (database, certificates, backups)")
	domain := flag.String("domain", "", "serve HTTPS for this name with a Let's Encrypt certificate")
	dev := flag.String("dev", "", "serve plain HTTP on this address instead (local testing)")
	keepBackups := flag.Int("backups", 7, "daily database copies to keep")
	flag.Parse()

	logger := log.New(os.Stdout, "", log.LstdFlags)
	// verify reads only the file it is given; it never touches -data.
	if flag.Arg(0) == "verify" {
		if flag.NArg() != 2 {
			fmt.Fprintln(os.Stderr, "verify <file>")
			os.Exit(2)
		}
		stats, err := profilesrv.Verify(context.Background(), flag.Arg(1))
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Println("ok:", stats)
		return
	}
	if err := os.MkdirAll(*dataDir, 0o700); err != nil {
		logger.Fatal(err)
	}
	store, err := profilesrv.Open(filepath.Join(*dataDir, "profiles.db"))
	if err != nil {
		logger.Fatal(err)
	}
	defer store.Close()

	if flag.Arg(0) == "admin" {
		if err := admin(store, flag.Args()[1:]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	if (*domain == "") == (*dev == "") {
		logger.Fatal("give exactly one of -domain or -dev")
	}
	salt, err := ipSalt(filepath.Join(*dataDir, "ip-salt"))
	if err != nil {
		logger.Fatal(err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go backups(ctx, store, filepath.Join(*dataDir, "backups"), *keepBackups, logger)

	api := (&profilesrv.Server{Store: store, IPSalt: salt, Log: logger}).Handler()
	var servers []*http.Server
	if *dev != "" {
		servers = append(servers, newServer(*dev, api))
		go serve(servers[0], logger, func(s *http.Server) error { return s.ListenAndServe() })
	} else {
		m := &autocert.Manager{
			Prompt:     autocert.AcceptTOS,
			HostPolicy: autocert.HostWhitelist(*domain),
			Cache:      autocert.DirCache(filepath.Join(*dataDir, "certs")),
		}
		tlsSrv := newServer(":443", api)
		tlsSrv.TLSConfig = m.TLSConfig()
		// Port 80 answers certificate challenges and sends the rest to HTTPS.
		plain := newServer(":80", m.HTTPHandler(nil))
		servers = append(servers, tlsSrv, plain)
		go serve(tlsSrv, logger, func(s *http.Server) error { return s.ListenAndServeTLS("", "") })
		go serve(plain, logger, func(s *http.Server) error { return s.ListenAndServe() })
	}
	logger.Printf("profilesrv started (data %s)", *dataDir)

	<-ctx.Done()
	shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	for _, s := range servers {
		_ = s.Shutdown(shutdown)
	}
	logger.Print("profilesrv stopped")
}

func newServer(addr string, h http.Handler) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           h,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    16 << 10,
	}
}

func serve(s *http.Server, logger *log.Logger, run func(*http.Server) error) {
	if err := run(s); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Fatalf("%s: %v", s.Addr, err)
	}
}

// ipSalt reads the secret mixed into stored address hashes, creating it once.
func ipSalt(path string) (string, error) {
	if data, err := os.ReadFile(path); err == nil && len(data) >= 32 {
		return strings.TrimSpace(string(data)), nil
	}
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	salt := hex.EncodeToString(buf)
	return salt, os.WriteFile(path, []byte(salt), 0o600)
}

// backups copies the database once a day and keeps the newest keep copies.
func backups(ctx context.Context, store *profilesrv.Store, dir string, keep int, logger *log.Logger) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		logger.Printf("backup: %v", err)
		return
	}
	for {
		name := filepath.Join(dir, "profiles-"+time.Now().UTC().Format("20060102")+".db")
		if _, err := os.Stat(name); errors.Is(err, os.ErrNotExist) {
			if err := store.Backup(ctx, name); err != nil {
				logger.Printf("backup: %v", err)
			} else {
				logger.Printf("backup: %s", filepath.Base(name))
			}
			old, _ := filepath.Glob(filepath.Join(dir, "profiles-*.db"))
			sort.Strings(old)
			for len(old) > keep {
				_ = os.Remove(old[0])
				old = old[1:]
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Hour):
		}
	}
}

func admin(store *profilesrv.Store, args []string) error {
	ctx := context.Background()
	if len(args) == 0 {
		return errors.New("admin: stats | reports | hide <id> | unhide <id> | delete <id> | sources <id> | drop-follows <id> <address-hash> | backup <file>")
	}
	need := func(n int) error {
		if len(args) != n+1 {
			return fmt.Errorf("admin %s: %d argument(s) needed", args[0], n)
		}
		return nil
	}
	switch args[0] {
	case "stats":
		s, err := store.Stats(ctx)
		if err == nil {
			fmt.Println(s)
		}
		return err
	case "reports":
		rows, err := store.Reports(ctx)
		for _, r := range rows {
			fmt.Printf("%s  %dx  hidden=%v  %q by %s  last %s\n    reasons: %s\n    notes: %s\n",
				r.Profile, r.Count, r.Hidden, r.Name, r.Author, r.Last.Format(time.DateTime), r.Reason, r.Note)
		}
		return err
	case "hide", "unhide":
		if err := need(1); err != nil {
			return err
		}
		return store.SetHidden(ctx, args[1], args[0] == "hide")
	case "delete":
		if err := need(1); err != nil {
			return err
		}
		return store.AdminDelete(ctx, args[1])
	case "sources":
		if err := need(1); err != nil {
			return err
		}
		src, err := store.FollowSource(ctx, args[1])
		for ip, n := range src {
			fmt.Printf("%s  %d\n", ip, n)
		}
		return err
	case "backup":
		if err := need(1); err != nil {
			return err
		}
		// VACUUM INTO refuses an existing file; replace it whole.
		tmp := args[1] + ".tmp"
		_ = os.Remove(tmp)
		if err := store.Backup(ctx, tmp); err != nil {
			return err
		}
		if _, err := profilesrv.Verify(ctx, tmp); err != nil {
			_ = os.Remove(tmp)
			return err
		}
		return os.Rename(tmp, args[1])
	case "drop-follows":
		if err := need(2); err != nil {
			return err
		}
		n, err := store.DropFollowsFrom(ctx, args[1], args[2])
		if err == nil {
			fmt.Printf("%d follows removed\n", n)
		}
		return err
	}
	return fmt.Errorf("admin: unknown command %q", args[0])
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return strings.Split(v, ":")[0]
	}
	return def
}
