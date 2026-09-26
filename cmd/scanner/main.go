// Command scanner is the shared exceptional-price scan server. It runs on a
// few machines, each searching its own share of the keys (base × exceptional
// kind × item level range) with its own IP's trade quota, anonymously, and
// uploads what it knows to a GitHub release every few hours. The app
// downloads those files; players never see the machines behind them.
//
// Nothing listens on the network: the process only calls out to GGG,
// poe.ninja/poe2scout (exchange rates), GitHub (NeverSink's filter, uploads).
package main

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"poe2filter/internal/collector"
	"poe2filter/internal/filter"
	"poe2filter/internal/neversink"
	"poe2filter/internal/prices"
	"poe2filter/internal/publish"
	"poe2filter/internal/trade"
)

// exitLeagueChanged asks systemd to restart the scanner for a new league.
const exitLeagueChanged = 3

type options struct {
	shard, weights, league, dataDir, repo, tag, tokenFile string
	budget                                               float64
	publishEvery                                         time.Duration
	maxScans                                             int
	dryRun                                               bool
}

func main() {
	var o options
	flag.StringVar(&o.shard, "shard", "", "this machine's share name (e.g. office)")
	flag.StringVar(&o.weights, "weights", "office=60,home=40", "every machine's share, name=weight,…")
	flag.Float64Var(&o.budget, "budget", 0.5, "share of this IP's trade search quota to use (0..1)")
	flag.StringVar(&o.league, "league", "auto", `league name, or "auto" for the current challenge league`)
	flag.StringVar(&o.dataDir, "data", envOr("STATE_DIRECTORY", "poe2scan-data"), "state directory")
	flag.StringVar(&o.repo, "repo", "kadircelebi/poe2-filtre-data", "GitHub repository receiving the files")
	flag.StringVar(&o.tag, "tag", "data", "release tag holding the files")
	flag.StringVar(&o.tokenFile, "token-file", credential("github-token"), "file holding the GitHub token")
	flag.DurationVar(&o.publishEvery, "publish-every", 6*time.Hour, "how often to upload results")
	flag.IntVar(&o.maxScans, "max-scans", 0, "stop after this many searches (testing)")
	flag.BoolVar(&o.dryRun, "dry-run", false, "write the upload to the data directory instead of GitHub")
	flag.Parse()
	log.SetFlags(0) // journald adds the time

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	code, err := run(ctx, o)
	if err != nil {
		log.Printf("fatal: %v", err)
		os.Exit(1)
	}
	os.Exit(code)
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		// systemd may list several directories; the first is ours.
		return strings.Split(v, ":")[0]
	}
	return def
}

// credential is where systemd's LoadCredential= puts a secret for us.
func credential(name string) string {
	if dir := os.Getenv("CREDENTIALS_DIRECTORY"); dir != "" {
		return filepath.Join(dir, name)
	}
	return ""
}

func run(ctx context.Context, o options) (int, error) {
	weights, err := trade.ParseShardWeights(o.weights)
	if err != nil {
		return 0, err
	}
	accept, err := trade.ShardFilter(o.shard, weights)
	if err != nil {
		return 0, err
	}
	var uploader *publish.GitHub
	if !o.dryRun {
		token, err := os.ReadFile(o.tokenFile)
		if err != nil || len(bytes.TrimSpace(token)) == 0 {
			return 0, fmt.Errorf("GitHub token not readable (%s); use -dry-run to test without one", o.tokenFile)
		}
		uploader = &publish.GitHub{Repo: o.repo, Tag: o.tag, Token: string(bytes.TrimSpace(token))}
	}
	if err := os.MkdirAll(o.dataDir, 0o700); err != nil {
		return 0, err
	}

	league, err := pickLeague(ctx, o.league)
	if err != nil {
		return 0, err
	}
	log.Printf("poe2scan: shard %s (%s), league %q, budget %.0f%%, data %s", o.shard, o.weights, league, o.budget*100, o.dataDir)

	client := trade.NewClient(league, o.budget)
	scanner := trade.NewScanner(client, filepath.Join(o.dataDir, "exceptional_scan.json"), func(s string) { log.Print(s) })
	scanner.SetIlvlBuckets([]trade.IlvlBucket{{Min: 79, Max: 81}, {Min: 82}})
	scanner.SetShard(accept)
	scanner.SetRefresh(trade.RefreshPolicy{Error: time.Hour, Empty: 72 * time.Hour, Hot: 12 * time.Hour, Normal: 24 * time.Hour})

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	leagueChanged := make(chan string, 1)

	// Exchange rates (to turn listing prices into Exalted) and the base list
	// are refreshed in the background; the scanner waits for the first ones.
	go every(ctx, 6*time.Hour, func() { refreshMarket(ctx, scanner, league, o.dataDir) })
	go every(ctx, 24*time.Hour, func() { refreshCandidates(ctx, scanner, o.dataDir) })
	if o.league == "auto" {
		go every(ctx, 6*time.Hour, func() {
			if now, err := pickLeague(ctx, "auto"); err == nil && now != league {
				leagueChanged <- now
			}
		})
	}

	scanned := 0
	scanner.SetOnChange(func() {
		st := scanner.Status()
		if st.Current == "" && st.Last != "" {
			scanned++
			log.Printf("%s  [%d/%d]", st.Last, st.Scanned, st.Keys)
			if o.maxScans > 0 && scanned >= o.maxScans {
				cancel()
			}
		}
	})

	publishNow := func() {
		if err := publishResults(context.WithoutCancel(ctx), scanner, uploader, o); err != nil {
			log.Printf("publish: %v", err)
		}
	}
	go func() {
		// The first upload comes after half an hour, so a restart shows up
		// on GitHub soon; then every publishEvery.
		wait := min(30*time.Minute, o.publishEvery)
		for {
			select {
			case <-ctx.Done():
				return
			case <-time.After(wait):
				publishNow()
				wait = o.publishEvery
			}
		}
	}()

	done := make(chan struct{})
	go func() { scanner.Run(ctx); close(done) }()
	select {
	case <-done:
	case now := <-leagueChanged:
		log.Printf("league changed to %q; restarting", now)
		cancel()
		<-done
		publishNow()
		return exitLeagueChanged, nil
	}
	publishNow() // what was found since the last upload is not lost
	return 0, nil
}

// every runs f now and then at each interval until ctx ends.
func every(ctx context.Context, d time.Duration, f func()) {
	f()
	ticker := time.NewTicker(d)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			f()
		}
	}
}

// pickLeague resolves "auto" to the current challenge league: the first
// trade league that is not Standard, hardcore, SSF or Ruthless.
func pickLeague(ctx context.Context, want string) (string, error) {
	if want != "auto" {
		return want, nil
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	leagues, err := collector.FetchLeagues(ctx, nil)
	if err != nil {
		return "", fmt.Errorf("league list: %w", err)
	}
	for _, l := range leagues {
		low := strings.ToLower(l)
		if low == "standard" || strings.HasPrefix(low, "hc ") || strings.Contains(low, "hardcore") ||
			strings.Contains(low, "ssf") || strings.Contains(low, "ruthless") {
			continue
		}
		return l, nil
	}
	return filter.DefaultLeagues[0], nil
}

var lastMarket *prices.Snapshot

func refreshMarket(ctx context.Context, s *trade.Scanner, league, dir string) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	snap, err := collector.Collect(ctx, collector.Options{League: league, Log: func(m string) { log.Print(strings.TrimSpace(m)) }}, lastMarket)
	if err != nil {
		log.Printf("exchange rates: %v", err)
		return
	}
	lastMarket = snap
	s.SetMarket(snap)
	log.Printf("exchange rates: 1 divine = %.0f ex", snap.Rates.DivineEx)
}

func refreshCandidates(ctx context.Context, s *trade.Scanner, dir string) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	bases, err := trade.EquipmentBaseTypes(ctx, filepath.Join(dir, "trade_items.json"))
	if err != nil {
		log.Printf("base list: %v", err)
		return
	}
	// NeverSink's exceptional list only orders the work; without it every
	// base is still scanned.
	preferred := map[string]bool{}
	if path, err := neversink.Ensure(ctx, 6, filepath.Join(dir, "neversink"), 24*time.Hour, "poe2scan"); err == nil {
		if raw, err := os.ReadFile(path); err == nil {
			preferred = neversink.ExceptionalBases(string(raw))
		}
	} else {
		log.Printf("NeverSink: %v", err)
	}
	s.SetCandidates(trade.BuildCandidates(bases, nil, preferred))
	log.Printf("candidates: %d bases (%d preferred)", len(bases), len(preferred))
}

// publishResults uploads this machine's results as exceptional-<shard>.json.gz
// (the app's scan share format, newest scan of a key wins when merged).
func publishResults(ctx context.Context, s *trade.Scanner, up *publish.GitHub, o options) error {
	raw, err := s.Export()
	if err != nil {
		return err
	}
	var share trade.Share
	if err := json.Unmarshal(raw, &share); err != nil {
		return err
	}
	if len(share.Keys) == 0 {
		return errors.New("nothing scanned yet")
	}
	var buf bytes.Buffer
	zw, _ := gzip.NewWriterLevel(&buf, gzip.BestCompression)
	if _, err := zw.Write(raw); err != nil {
		return err
	}
	if err := zw.Close(); err != nil {
		return err
	}
	name := "exceptional-" + o.shard + ".json.gz"
	if up == nil {
		path := filepath.Join(o.dataDir, name)
		if err := os.WriteFile(path, buf.Bytes(), 0o600); err != nil {
			return err
		}
		log.Printf("dry run: wrote %s (%d keys, %d KB)", path, len(share.Keys), buf.Len()/1024)
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	if err := up.Put(ctx, name, buf.Bytes(), "application/gzip"); err != nil {
		return err
	}
	log.Printf("published %s (%d keys, %d KB)", name, len(share.Keys), buf.Len()/1024)
	return nil
}
