// Package prices defines the price snapshot shared by the desktop app and the
// (future) collector server. Everything that consumes prices reads a Snapshot;
// nothing downstream knows whether it was built locally or downloaded.
package prices

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// SchemaVersion must be bumped on any incompatible change to Snapshot.
const SchemaVersion = 1

// Snapshot is the complete, self-contained price data for one league.
// All values are in Exalted Orbs.
type Snapshot struct {
	SchemaVersion int                     `json:"schema_version"`
	League        string                  `json:"league"`
	GeneratedAt   time.Time               `json:"generated_at"`
	Rates         Rates                   `json:"rates"`
	Currency      []CurrencyPrice         `json:"currency"`
	UniqueBases   map[string]UniqueBase   `json:"unique_bases"`
	Exceptional   []ExceptionalPrice      `json:"exceptional"`
	Sources       map[string]SourceStatus `json:"sources"`
}

// Rates holds exchange rates relative to the Exalted Orb.
type Rates struct {
	DivineEx float64 `json:"divine_ex"`
	ChaosEx  float64 `json:"chaos_ex"`
}

// CurrencyPrice is a stackable/bulk item (currency, runes, omens, essences...).
type CurrencyPrice struct {
	Name     string  `json:"name"`
	APIID    string  `json:"api_id,omitempty"`
	Category string  `json:"category"`
	ValueEx  float64 `json:"value_ex"`
}

// UniqueBase groups every unique that can drop on one ground base type.
// A loot filter only sees the base of an unidentified unique, so the base is
// worth as much as the most valuable unique it can turn into.
type UniqueBase struct {
	TopName string   `json:"top_name"`
	MaxEx   float64  `json:"max_ex"`
	Uniques []Unique `json:"uniques"`
}

// Unique is a single unique item price.
type Unique struct {
	Name     string  `json:"name"`
	Category string  `json:"category"`
	ValueEx  float64 `json:"value_ex"`
	Listings int     `json:"listings"`
	// Icon is the unique's art (poe.ninja), shown when an unidentified unique
	// could be one of several.
	Icon string `json:"icon,omitempty"`
}

// ExceptionalKind is the property that makes a base "exceptional".
type ExceptionalKind string

const (
	KindSockets ExceptionalKind = "sockets" // more sockets than the class normally rolls
	KindQuality ExceptionalKind = "quality" // quality above the normal 20% cap
)

// ExceptionalPrice is the observed price of an exceptional normal-rarity base.
type ExceptionalPrice struct {
	Base  string          `json:"base"`
	Class string          `json:"class,omitempty"`
	Kind  ExceptionalKind `json:"kind"`
	Min   int             `json:"min"` // Sockets >= Min or Quality >= Min
	// MinIlvl/MaxIlvl bound the item level the price was searched for; 0 is
	// unbounded. The shared scan servers price 79-81 and 82+ apart, since the
	// affix tiers a crafter can roll depend on it.
	MinIlvl   int       `json:"min_ilvl,omitempty"`
	MaxIlvl   int       `json:"max_ilvl,omitempty"`
	ValueEx   float64   `json:"value_ex"` // trimmed mean of the cheapest listings
	Listings  int       `json:"listings"` // total matching listings on trade
	Samples   int       `json:"samples"`  // listings used for ValueEx
	ScannedAt time.Time `json:"scanned_at"`
}

// SourceStatus records whether an upstream source was fresh in this snapshot.
type SourceStatus struct {
	OK        bool      `json:"ok"`
	FetchedAt time.Time `json:"fetched_at,omitempty"`
	Error     string    `json:"error,omitempty"`
}

// Validate rejects snapshots that are unusable for filter generation.
func (s *Snapshot) Validate() error {
	if s == nil {
		return errors.New("empty snapshot")
	}
	if s.SchemaVersion != SchemaVersion {
		return fmt.Errorf("unsupported schema version %d (expected %d)", s.SchemaVersion, SchemaVersion)
	}
	if s.Rates.DivineEx <= 0 {
		return errors.New("divine rate missing")
	}
	if len(s.Currency) == 0 && len(s.UniqueBases) == 0 {
		return errors.New("no price data")
	}
	return nil
}

// ToEx converts an amount of the given trade currency id into Exalted Orbs.
func (s *Snapshot) ToEx(amount float64, currency string) (float64, bool) {
	switch currency {
	case "exalted":
		return amount, true
	case "divine":
		return amount * s.Rates.DivineEx, s.Rates.DivineEx > 0
	case "chaos":
		return amount * s.Rates.ChaosEx, s.Rates.ChaosEx > 0
	}
	for _, c := range s.Currency {
		if c.APIID == currency && c.ValueEx > 0 {
			return amount * c.ValueEx, true
		}
	}
	return 0, false
}

// Load reads a snapshot from disk and validates it.
func Load(path string) (*Snapshot, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var s Snapshot
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("could not read the snapshot: %w", err)
	}
	if err := s.Validate(); err != nil {
		return nil, err
	}
	return &s, nil
}

// Save writes the snapshot atomically so a crash never leaves a half file.
func Save(path string, s *Snapshot) error {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return WriteFileAtomic(path, data)
}

// WriteFileAtomic writes to a temp file in the same directory and renames it.
func WriteFileAtomic(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".tmp*")
	if err != nil {
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	return os.Rename(tmp.Name(), path)
}
