package overlay

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"poe2filter/internal/prices"
)

// Modifier tiers come from RePoE's export of the game data
// (https://repoe-fork.github.io/poe2/): which families each base rolls, each
// tier's item level and its value range. The three source files are ~15 MB
// (~1 MB gzipped); the app downloads them on first use, keeps only the rows
// the trade site can search (a few hundred KB) and checks once a day whether
// the export changed, so a new league's tiers and bases arrive without a
// release of ours.
const repoeBaseURL = "https://repoe-fork.github.io/poe2/"

// tierFormat is bumped when the built file changes shape or the build rules
// change, so an old cache is rebuilt instead of read.
const tierFormat = 2

// Tier is one tier of a modifier family: T1 is the best, the one needing the
// highest item level. Min and Max are what the trade site compares ("Adds X to
// Y" by the average of the two).
type Tier struct {
	Tier  int     `json:"tier"`
	Name  string  `json:"name"`
	Level int     `json:"level"`
	Min   float64 `json:"min"`
	Max   float64 `json:"max"`
}

// TierTable is the tiers of one stat line of one modifier family on a base.
// A hybrid family rolls another stat too (With names it); its tiers are
// smaller than the plain family's, so the two are offered apart.
type TierTable struct {
	Stat   string `json:"stat"`
	Affix  string `json:"affix"`
	Hybrid bool   `json:"hybrid"`
	With   string `json:"with,omitempty"`
	Tiers  []Tier `json:"tiers"`
}

// TierData is the built file: tables once, and per base and per item class
// the indexes of the tables that apply.
type TierData struct {
	Format  int              `json:"format"`
	Source  string           `json:"source"`
	Tables  []TierTable      `json:"tables"`
	Bases   map[string][]int `json:"bases"`
	Classes map[string][]int `json:"classes"`
}

// For returns the tables of a base, or of an item class when the base is not
// known (a search by category). The class is tried as the game prints it and
// in RePoE's singular ("Tablets" / "Tablet").
func (d *TierData) For(base, class string) []TierTable {
	idx, ok := d.Bases[base]
	if !ok || base == "" {
		idx = nil
		for _, name := range []string{class, strings.TrimSuffix(class, "s"), class + "s"} {
			if got, found := d.Classes[name]; found && name != "" {
				idx = got
				break
			}
		}
	}
	out := make([]TierTable, 0, len(idx))
	for _, i := range idx {
		if i >= 0 && i < len(d.Tables) {
			out = append(out, d.Tables[i])
		}
	}
	return out
}

// ---- Building ---------------------------------------------------------------

type repoeGroup struct {
	Bases []string                             `json:"bases"`
	Mods  map[string]map[string]map[string]int `json:"mods"`
}

type repoeMod struct {
	Name  string `json:"name"`
	Text  string `json:"text"`
	Stats []struct {
		ID string `json:"id"`
	} `json:"stats"`
}

type repoeBase struct {
	Name         string `json:"name"`
	ReleaseState string `json:"release_state"`
}

var (
	// "[Physical|Physical]" and "[Strength]" are game link markup.
	repoeLinkRE = regexp.MustCompile(`\[([^\]|]*)\|([^\]]*)\]|\[([^\]]*)\]`)
	// A rolled range "(5-8)", "(-10--5)", "(0.5-0.7)", or a fixed number.
	repoeRangeRE = regexp.MustCompile(`\((-?\d+(?:\.\d+)?)-(-?\d+(?:\.\d+)?)\)|(-?\d+(?:\.\d+)?)`)
)

func unlinkRepoe(s string) string {
	return repoeLinkRE.ReplaceAllStringFunc(s, func(m string) string {
		sub := repoeLinkRE.FindStringSubmatch(m)
		if sub[2] != "" || strings.Contains(m, "|") {
			return sub[2]
		}
		return sub[3]
	})
}

// lineRange reads the value range of one mod line: several ranges ("Adds
// (1-2) to (4-5)") are averaged the way the trade site compares them.
func lineRange(line string) (lo, hi float64, ok bool) {
	var los, his []float64
	for _, m := range repoeRangeRE.FindAllStringSubmatch(line, -1) {
		if m[1] != "" {
			a, _ := strconv.ParseFloat(m[1], 64)
			b, _ := strconv.ParseFloat(m[2], 64)
			los, his = append(los, math.Min(a, b)), append(his, math.Max(a, b))
		} else if m[3] != "" {
			v, _ := strconv.ParseFloat(m[3], 64)
			los, his = append(los, v), append(his, v)
		}
	}
	if len(los) == 0 {
		return 0, 0, false
	}
	for i := range los {
		lo += los[i]
		hi += his[i]
	}
	n := float64(len(los))
	return round2(lo / n), round2(hi / n), true
}

func round2(v float64) float64 { return math.Round(v*100) / 100 }

// repoeKey is a mod line in the form normalizeStat compares: its ranges become
// one number first, since normalizeStat drops anything in parentheses.
func repoeKey(line string) string {
	return normalizeStat(repoeRangeRE.ReplaceAllString(line, "1"))
}

// statIndex finds trade stats by wording, among the stats that modifiers of
// items roll (explicit) and relics' (sanctum).
type statIndex map[string][]StatEntry

func newStatIndex(catalog Catalog) statIndex {
	idx := statIndex{}
	for _, group := range catalog.Stats {
		for _, e := range group.Entries {
			if e.Type != "explicit" && e.Type != "sanctum" {
				continue
			}
			key := normalizeStat(strings.SplitN(e.Text, "\n", 2)[0])
			idx[key] = append(idx[key], e)
		}
	}
	return idx
}

// find returns the trade stat of a mod line and whether its values must be
// negated: a "reduced" roll is searched as a negative "increased" stat when
// the site lists only the latter (and the other way round).
func (idx statIndex) find(line string, local bool) (string, bool) {
	key := repoeKey(line)
	negate := false
	entries := idx[key]
	if len(entries) == 0 {
		for _, swap := range [][2]string{{"reduced", "increased"}, {"increased", "reduced"}, {"less", "more"}, {"more", "less"}} {
			if strings.Contains(key, swap[0]) {
				if got := idx[strings.Replace(key, swap[0], swap[1], 1)]; len(got) > 0 {
					entries, negate = got, true
					break
				}
			}
		}
	}
	if len(entries) == 0 {
		return "", false
	}
	for _, e := range entries {
		if strings.Contains(e.Text, "(Local)") == local {
			return e.ID, negate
		}
	}
	return entries[0].ID, negate
}

// BuildTiers turns RePoE's mods_by_base, mods and base_items files into the
// tables the market offers, matched to the trade catalog's stats.
func BuildTiers(modsByBase, mods, baseItems []byte, catalog Catalog, source string) (*TierData, error) {
	var byClass map[string]map[string]repoeGroup
	if err := json.Unmarshal(modsByBase, &byClass); err != nil {
		return nil, fmt.Errorf("decode mods_by_base: %w", err)
	}
	var modList map[string]repoeMod
	if err := json.Unmarshal(mods, &modList); err != nil {
		return nil, fmt.Errorf("decode mods: %w", err)
	}
	var baseList map[string]repoeBase
	if err := json.Unmarshal(baseItems, &baseList); err != nil {
		return nil, fmt.Errorf("decode base_items: %w", err)
	}
	stats := newStatIndex(catalog)
	data := &TierData{Format: tierFormat, Source: source, Bases: map[string][]int{}, Classes: map[string][]int{}}
	tableIndex := map[string]int{}

	for class, groups := range byClass {
		classSet := map[int]bool{}
		for _, group := range groups {
			var names []string
			for _, path := range group.Bases {
				if b, ok := baseList[path]; ok && b.Name != "" && b.ReleaseState == "released" {
					names = append(names, b.Name)
				}
			}
			if len(names) == 0 {
				continue
			}
			var tables []int
			for _, affix := range []string{"prefix", "suffix"} {
				for _, family := range group.Mods[affix] {
					for _, table := range familyTables(family, modList, affix, stats) {
						raw, _ := json.Marshal(table)
						i, ok := tableIndex[string(raw)]
						if !ok {
							i = len(data.Tables)
							tableIndex[string(raw)] = i
							data.Tables = append(data.Tables, table)
						}
						tables = append(tables, i)
					}
				}
			}
			if len(tables) == 0 {
				continue
			}
			for _, name := range names {
				data.Bases[name] = mergeIndexes(data.Bases[name], tables)
			}
			for _, i := range tables {
				classSet[i] = true
			}
		}
		if len(classSet) > 0 {
			for i := range classSet {
				data.Classes[class] = append(data.Classes[class], i)
			}
			slices.Sort(data.Classes[class])
		}
	}
	if len(data.Tables) == 0 {
		return nil, fmt.Errorf("no modifier tiers matched the trade catalog")
	}
	return data, nil
}

func mergeIndexes(a, b []int) []int {
	out := append(slices.Clone(a), b...)
	slices.Sort(out)
	return slices.Compact(out)
}

// familyTables builds one table per searchable line of a modifier family.
func familyTables(family map[string]int, modList map[string]repoeMod, affix string, stats statIndex) []TierTable {
	type entry struct {
		id    string
		level int
		mod   repoeMod
	}
	var entries []entry
	for id, level := range family {
		if m, ok := modList[id]; ok && m.Text != "" {
			entries = append(entries, entry{id, level, m})
		}
	}
	if len(entries) == 0 {
		return nil
	}
	// T1 is the tier needing the highest item level.
	slices.SortFunc(entries, func(a, b entry) int {
		if c := cmp.Compare(b.level, a.level); c != 0 {
			return c
		}
		return cmp.Compare(b.id, a.id)
	})
	best := strings.Split(unlinkRepoe(entries[0].mod.Text), "\n")
	local := slices.ContainsFunc(entries[0].mod.Stats, func(s struct {
		ID string `json:"id"`
	}) bool {
		return strings.HasPrefix(s.ID, "local_")
	})
	var out []TierTable
	for li, line := range best {
		stat, negate := stats.find(line, local)
		if stat == "" {
			continue
		}
		table := TierTable{Stat: stat, Affix: affix, Hybrid: len(best) > 1}
		if table.Hybrid {
			var others []string
			for oi, other := range best {
				if oi != li {
					others = append(others, repoeRangeRE.ReplaceAllString(other, "#"))
				}
			}
			table.With = strings.Join(others, " / ")
		}
		for n, e := range entries {
			lines := strings.Split(unlinkRepoe(e.mod.Text), "\n")
			if li >= len(lines) {
				continue
			}
			lo, hi, ok := lineRange(lines[li])
			if !ok {
				continue
			}
			if negate {
				lo, hi = -hi, -lo
			}
			table.Tiers = append(table.Tiers, Tier{Tier: n + 1, Name: e.mod.Name, Level: e.level, Min: lo, Max: hi})
		}
		if len(table.Tiers) > 0 {
			out = append(out, table)
		}
	}
	return out
}

// ---- Store ------------------------------------------------------------------

// TierStore keeps the built tiers on disk (data\stat_tiers.json) and in
// memory. The export is checked at most once a day; a failed download keeps
// the tiers already built, and with none the market simply offers no tiers.
type TierStore struct {
	dir     string
	baseURL string
	client  *http.Client
	catalog func(context.Context) (Catalog, error)

	mu      sync.Mutex
	value   *TierData
	checked time.Time
}

func NewTierStore(dataDir string, catalog func(context.Context) (Catalog, error)) *TierStore {
	return &TierStore{
		dir:     filepath.Join(dataDir, "data"),
		baseURL: repoeBaseURL,
		client:  &http.Client{Timeout: 90 * time.Second},
		catalog: catalog,
	}
}

func (s *TierStore) path() string { return filepath.Join(s.dir, "stat_tiers.json") }

// Load returns the tiers, building them on first use. Concurrent callers wait
// for the one build.
func (s *TierStore) Load(ctx context.Context) (*TierData, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.value == nil {
		if raw, err := os.ReadFile(s.path()); err == nil {
			var cached TierData
			if json.Unmarshal(raw, &cached) == nil && cached.Format == tierFormat && len(cached.Tables) > 0 {
				s.value = &cached
				if fi, err := os.Stat(s.path()); err == nil {
					s.checked = fi.ModTime()
				}
			}
		}
	}
	if s.value != nil && time.Since(s.checked) < 24*time.Hour {
		return s.value, nil
	}
	built, err := s.refresh(ctx)
	if err != nil {
		if s.value != nil {
			// Try again in an hour rather than on every call.
			s.checked = time.Now().Add(-23 * time.Hour)
			return s.value, nil
		}
		return nil, err
	}
	s.value, s.checked = built, time.Now()
	return built, nil
}

// refresh rebuilds the tiers when the export changed since the last build
// (by its Last-Modified date); otherwise it only marks the cache as checked.
func (s *TierStore) refresh(ctx context.Context) (*TierData, error) {
	source, err := s.lastModified(ctx)
	if err != nil {
		return nil, err
	}
	if s.value != nil && source != "" && s.value.Source == source {
		now := time.Now()
		_ = os.Chtimes(s.path(), now, now)
		return s.value, nil
	}
	files := map[string][]byte{}
	for _, name := range []string{"mods_by_base.min.json", "mods.min.json", "base_items.min.json"} {
		raw, err := s.get(ctx, name)
		if err != nil {
			return nil, err
		}
		files[name] = raw
	}
	catalog, err := s.catalog(ctx)
	if err != nil {
		return nil, err
	}
	data, err := BuildTiers(files["mods_by_base.min.json"], files["mods.min.json"], files["base_items.min.json"], catalog, source)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(s.dir, 0o755); err == nil {
		if raw, err := json.Marshal(data); err == nil {
			_ = prices.WriteFileAtomic(s.path(), raw)
		}
	}
	return data, nil
}

func (s *TierStore) request(ctx context.Context, method, name string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, s.baseURL+name, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "poe2-filter/overlay")
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("repoe %s: HTTP %d", name, resp.StatusCode)
	}
	return resp, nil
}

func (s *TierStore) lastModified(ctx context.Context) (string, error) {
	resp, err := s.request(ctx, http.MethodHead, "mods_by_base.min.json")
	if err != nil {
		return "", err
	}
	resp.Body.Close()
	return resp.Header.Get("Last-Modified"), nil
}

func (s *TierStore) get(ctx context.Context, name string) ([]byte, error) {
	resp, err := s.request(ctx, http.MethodGet, name)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
	if err != nil {
		return nil, err
	}
	if !json.Valid(raw) {
		return nil, fmt.Errorf("repoe %s: not JSON", name)
	}
	return raw, nil
}
