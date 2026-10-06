// Package profilesrv is the public profile server: installs publish filter
// profiles (internal/publicprofile documents), others find and follow them.
//
// There are no accounts. Each app install gets a random key on first use and
// owns what it publishes; the PoE account name shown as the author is only
// what the app claims. Only a hash of each key is stored.
package profilesrv

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base32"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	_ "modernc.org/sqlite"

	"poe2filter/internal/publicprofile"
)

// Limits kept by the store.
const (
	MaxProfilesPerInstall = 20
	PageSize              = 50
)

var (
	ErrNotFound = errors.New("not found")
	ErrNotOwner = errors.New("not the owner")
	ErrLimit    = errors.New("limit reached")
)

// Store is the SQLite database.
type Store struct {
	db  *sql.DB
	now func() time.Time
}

const schema = `
CREATE TABLE IF NOT EXISTS installs (
	key_hash   TEXT PRIMARY KEY,
	ip_hash    TEXT NOT NULL,
	created_at INTEGER NOT NULL,
	seen_at    INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS installs_ip ON installs(ip_hash, created_at);
CREATE TABLE IF NOT EXISTS profiles (
	id          TEXT PRIMARY KEY,
	owner       TEXT NOT NULL REFERENCES installs(key_hash),
	author      TEXT NOT NULL,
	name        TEXT NOT NULL,
	description TEXT NOT NULL,
	tags        TEXT NOT NULL,
	document    BLOB NOT NULL,
	version     INTEGER NOT NULL,
	followers   INTEGER NOT NULL DEFAULT 0,
	hidden      INTEGER NOT NULL DEFAULT 0,
	created_at  INTEGER NOT NULL,
	updated_at  INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS profiles_owner ON profiles(owner);
CREATE INDEX IF NOT EXISTS profiles_rank ON profiles(hidden, followers DESC, updated_at DESC);
CREATE TABLE IF NOT EXISTS profile_tags (
	profile TEXT NOT NULL REFERENCES profiles(id) ON DELETE CASCADE,
	tag     TEXT NOT NULL,
	PRIMARY KEY (profile, tag)
);
CREATE INDEX IF NOT EXISTS profile_tags_tag ON profile_tags(tag);
CREATE TABLE IF NOT EXISTS follows (
	install    TEXT NOT NULL REFERENCES installs(key_hash),
	profile    TEXT NOT NULL REFERENCES profiles(id) ON DELETE CASCADE,
	created_at INTEGER NOT NULL,
	PRIMARY KEY (install, profile)
);
CREATE INDEX IF NOT EXISTS follows_profile ON follows(profile);
CREATE TABLE IF NOT EXISTS reports (
	profile    TEXT NOT NULL REFERENCES profiles(id) ON DELETE CASCADE,
	install    TEXT NOT NULL,
	reason     TEXT NOT NULL,
	note       TEXT NOT NULL,
	created_at INTEGER NOT NULL,
	PRIMARY KEY (profile, install)
);
`

// Open opens (and creates) the database at path.
func Open(path string) (*Store, error) {
	dsn := "file:" + path + "?_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)&_pragma=synchronous(NORMAL)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	// One writer at a time is all SQLite does anyway; a single connection
	// keeps the pragmas and avoids "database is locked" between our own calls.
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, err
	}
	return &Store{db: db, now: time.Now}, nil
}

func (s *Store) Close() error { return s.db.Close() }

// HashKey is how an install key is stored and looked up.
func HashKey(key string) string {
	sum := sha256.Sum256([]byte("mrw-profile-install:" + key))
	return hex.EncodeToString(sum[:])
}

// NewInstall creates an install and returns its key. ipHash limits how many
// installs one address creates per day (0 = no limit).
func (s *Store) NewInstall(ctx context.Context, ipHash string, perDay int) (string, error) {
	now := s.now().Unix()
	if perDay > 0 {
		var n int
		err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM installs WHERE ip_hash = ? AND created_at > ?`,
			ipHash, now-24*3600).Scan(&n)
		if err != nil {
			return "", err
		}
		if n >= perDay {
			return "", ErrLimit
		}
	}
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	key := base64.RawURLEncoding.EncodeToString(buf)
	_, err := s.db.ExecContext(ctx, `INSERT INTO installs(key_hash, ip_hash, created_at, seen_at) VALUES (?, ?, ?, ?)`,
		HashKey(key), ipHash, now, now)
	return key, err
}

// Install looks a key up, returning its hash. Seen time is refreshed at most
// once an hour.
func (s *Store) Install(ctx context.Context, key string) (string, error) {
	h := HashKey(key)
	var seen int64
	err := s.db.QueryRowContext(ctx, `SELECT seen_at FROM installs WHERE key_hash = ?`, h).Scan(&seen)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", err
	}
	if now := s.now().Unix(); now-seen > 3600 {
		_, _ = s.db.ExecContext(ctx, `UPDATE installs SET seen_at = ? WHERE key_hash = ?`, now, h)
	}
	return h, nil
}

// Profile is a published profile as listed.
type Profile = publicprofile.Listing

// Publish holds what an author sends; Document is already canonical.
type Publish struct {
	Author      string
	Name        string
	Description string
	Tags        []string
	Document    []byte
}

func newID() (string, error) {
	buf := make([]byte, 7)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return strings.ToLower(base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(buf)), nil
}

// Create publishes a new profile for owner.
func (s *Store) Create(ctx context.Context, owner string, p Publish) (Profile, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Profile{}, err
	}
	defer tx.Rollback()
	var n int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM profiles WHERE owner = ?`, owner).Scan(&n); err != nil {
		return Profile{}, err
	}
	if n >= MaxProfilesPerInstall {
		return Profile{}, ErrLimit
	}
	id, err := newID()
	if err != nil {
		return Profile{}, err
	}
	now := s.now().Unix()
	_, err = tx.ExecContext(ctx, `INSERT INTO profiles(id, owner, author, name, description, tags, document, version, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, 1, ?, ?)`, id, owner, p.Author, p.Name, p.Description, strings.Join(p.Tags, ","), p.Document, now, now)
	if err != nil {
		return Profile{}, err
	}
	if err := setTags(ctx, tx, id, p.Tags); err != nil {
		return Profile{}, err
	}
	if err := tx.Commit(); err != nil {
		return Profile{}, err
	}
	return Profile{ID: id, Author: p.Author, Name: p.Name, Description: p.Description, Tags: p.Tags, Version: 1, UpdatedAt: now}, nil
}

// Update replaces an owner's profile; the version goes up only when the
// document changed, so followers download only real changes.
func (s *Store) Update(ctx context.Context, owner, id string, p Publish) (Profile, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Profile{}, err
	}
	defer tx.Rollback()
	var have string
	var doc []byte
	var version, followers int
	err = tx.QueryRowContext(ctx, `SELECT owner, document, version, followers FROM profiles WHERE id = ?`, id).Scan(&have, &doc, &version, &followers)
	if errors.Is(err, sql.ErrNoRows) {
		return Profile{}, ErrNotFound
	}
	if err != nil {
		return Profile{}, err
	}
	if have != owner {
		return Profile{}, ErrNotOwner
	}
	if string(doc) != string(p.Document) {
		version++
	}
	now := s.now().Unix()
	_, err = tx.ExecContext(ctx, `UPDATE profiles SET author = ?, name = ?, description = ?, tags = ?, document = ?, version = ?, updated_at = ? WHERE id = ?`,
		p.Author, p.Name, p.Description, strings.Join(p.Tags, ","), p.Document, version, now, id)
	if err != nil {
		return Profile{}, err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM profile_tags WHERE profile = ?`, id); err != nil {
		return Profile{}, err
	}
	if err := setTags(ctx, tx, id, p.Tags); err != nil {
		return Profile{}, err
	}
	if err := tx.Commit(); err != nil {
		return Profile{}, err
	}
	return Profile{ID: id, Author: p.Author, Name: p.Name, Description: p.Description, Tags: p.Tags, Version: version, Followers: followers, UpdatedAt: now}, nil
}

func setTags(ctx context.Context, tx *sql.Tx, id string, tags []string) error {
	for _, t := range tags {
		if _, err := tx.ExecContext(ctx, `INSERT INTO profile_tags(profile, tag) VALUES (?, ?)`, id, t); err != nil {
			return err
		}
	}
	return nil
}

// Delete removes an owner's profile with its follows and reports.
func (s *Store) Delete(ctx context.Context, owner, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM profiles WHERE id = ? AND owner = ?`, id, owner)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		if _, err := s.Get(ctx, id, true); err == nil {
			return ErrNotOwner
		}
		return ErrNotFound
	}
	return nil
}

const profileCols = `id, author, name, description, tags, version, followers, updated_at, hidden`

func scanProfile(row interface{ Scan(...any) error }) (Profile, error) {
	var p Profile
	var tags string
	var hidden int
	if err := row.Scan(&p.ID, &p.Author, &p.Name, &p.Description, &tags, &p.Version, &p.Followers, &p.UpdatedAt, &hidden); err != nil {
		return Profile{}, err
	}
	p.Tags = []string{}
	if tags != "" {
		p.Tags = strings.Split(tags, ",")
	}
	p.Hidden = hidden != 0
	return p, nil
}

// Get returns one profile. Hidden ones only when withHidden.
func (s *Store) Get(ctx context.Context, id string, withHidden bool) (Profile, error) {
	p, err := scanProfile(s.db.QueryRowContext(ctx, `SELECT `+profileCols+` FROM profiles WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) || (err == nil && p.Hidden && !withHidden) {
		return Profile{}, ErrNotFound
	}
	return p, err
}

// Document returns a visible profile's document and version.
func (s *Store) Document(ctx context.Context, id string) ([]byte, int, error) {
	var doc []byte
	var version int
	err := s.db.QueryRowContext(ctx, `SELECT document, version FROM profiles WHERE id = ? AND hidden = 0`, id).Scan(&doc, &version)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, 0, ErrNotFound
	}
	return doc, version, err
}

// List returns visible profiles, most followed first. query matches name or
// author (case-insensitive substring); tag narrows to one tag.
func (s *Store) List(ctx context.Context, query, tag string, page int) ([]Profile, error) {
	where := []string{"hidden = 0"}
	var args []any
	if q := strings.ToLower(strings.TrimSpace(query)); q != "" {
		where = append(where, "(instr(lower(name), ?) > 0 OR instr(lower(author), ?) > 0)")
		args = append(args, q, q)
	}
	if tag != "" {
		where = append(where, "EXISTS (SELECT 1 FROM profile_tags t WHERE t.profile = profiles.id AND t.tag = ?)")
		args = append(args, tag)
	}
	args = append(args, PageSize, max(page, 0)*PageSize)
	rows, err := s.db.QueryContext(ctx, `SELECT `+profileCols+` FROM profiles WHERE `+strings.Join(where, " AND ")+
		` ORDER BY followers DESC, updated_at DESC, id LIMIT ? OFFSET ?`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Profile{}
	for rows.Next() {
		p, err := scanProfile(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// Owned lists an install's own profiles, hidden ones included.
func (s *Store) Owned(ctx context.Context, owner string) ([]Profile, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+profileCols+` FROM profiles WHERE owner = ? ORDER BY created_at`, owner)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Profile{}
	for rows.Next() {
		p, err := scanProfile(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// Versions returns the current version of each visible profile among ids;
// missing ones were deleted or hidden.
func (s *Store) Versions(ctx context.Context, ids []string) (map[string]int, error) {
	out := map[string]int{}
	for _, id := range ids {
		var v int
		err := s.db.QueryRowContext(ctx, `SELECT version FROM profiles WHERE id = ? AND hidden = 0`, id).Scan(&v)
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return nil, err
		}
		out[id] = v
	}
	return out, nil
}

// Follow marks install as following id (or not) and keeps the count.
func (s *Store) Follow(ctx context.Context, install, id string, on bool) (int, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	var hidden int
	err = tx.QueryRowContext(ctx, `SELECT hidden FROM profiles WHERE id = ?`, id).Scan(&hidden)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && hidden != 0 && on) {
		return 0, ErrNotFound
	}
	if err != nil {
		return 0, err
	}
	var res sql.Result
	if on {
		res, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO follows(install, profile, created_at) VALUES (?, ?, ?)`, install, id, s.now().Unix())
	} else {
		res, err = tx.ExecContext(ctx, `DELETE FROM follows WHERE install = ? AND profile = ?`, install, id)
	}
	if err != nil {
		return 0, err
	}
	if n, _ := res.RowsAffected(); n > 0 {
		if _, err := tx.ExecContext(ctx, `UPDATE profiles SET followers = (SELECT COUNT(*) FROM follows WHERE profile = ?) WHERE id = ?`, id, id); err != nil {
			return 0, err
		}
	}
	var count int
	if err := tx.QueryRowContext(ctx, `SELECT followers FROM profiles WHERE id = ?`, id).Scan(&count); err != nil {
		return 0, err
	}
	return count, tx.Commit()
}

// Report records one report per install and profile (a repeat replaces it).
func (s *Store) Report(ctx context.Context, install, id, reason, note string) error {
	if _, err := s.Get(ctx, id, false); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, `INSERT OR REPLACE INTO reports(profile, install, reason, note, created_at) VALUES (?, ?, ?, ?, ?)`,
		id, install, reason, note, s.now().Unix())
	return err
}

// ReportRow is a report as the admin command lists it.
type ReportRow struct {
	Profile, Name, Author, Reason, Note string
	Count                               int
	Last                                time.Time
	Hidden                              bool
}

// Reports lists reported profiles, most reported first.
func (s *Store) Reports(ctx context.Context) ([]ReportRow, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT r.profile, p.name, p.author, p.hidden, COUNT(*), MAX(r.created_at),
		group_concat(r.reason, ' | '), group_concat(r.note, ' | ')
		FROM reports r JOIN profiles p ON p.id = r.profile GROUP BY r.profile ORDER BY COUNT(*) DESC, MAX(r.created_at) DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ReportRow
	for rows.Next() {
		var r ReportRow
		var hidden int
		var last int64
		if err := rows.Scan(&r.Profile, &r.Name, &r.Author, &hidden, &r.Count, &last, &r.Reason, &r.Note); err != nil {
			return nil, err
		}
		r.Hidden, r.Last = hidden != 0, time.Unix(last, 0)
		out = append(out, r)
	}
	return out, rows.Err()
}

// SetHidden hides a profile from listing and download (moderation).
func (s *Store) SetHidden(ctx context.Context, id string, hidden bool) error {
	v := 0
	if hidden {
		v = 1
	}
	res, err := s.db.ExecContext(ctx, `UPDATE profiles SET hidden = ? WHERE id = ?`, v, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// AdminDelete removes a profile whoever owns it.
func (s *Store) AdminDelete(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM profiles WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// FollowSource summarises where a profile's follows come from, to spot
// inflated counts: follows per install-creating address hash.
func (s *Store) FollowSource(ctx context.Context, id string) (map[string]int, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT i.ip_hash, COUNT(*) FROM follows f JOIN installs i ON i.key_hash = f.install
		WHERE f.profile = ? GROUP BY i.ip_hash ORDER BY COUNT(*) DESC`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var ip string
		var n int
		if err := rows.Scan(&ip, &n); err != nil {
			return nil, err
		}
		out[ip] = n
	}
	return out, rows.Err()
}

// DropFollowsFrom removes a profile's follows by installs created from one
// address hash, and recounts.
func (s *Store) DropFollowsFrom(ctx context.Context, id, ipHash string) (int64, error) {
	res, err := s.db.ExecContext(ctx, `DELETE FROM follows WHERE profile = ? AND install IN (SELECT key_hash FROM installs WHERE ip_hash = ?)`, id, ipHash)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	_, err = s.db.ExecContext(ctx, `UPDATE profiles SET followers = (SELECT COUNT(*) FROM follows WHERE profile = ?) WHERE id = ?`, id, id)
	return n, err
}

// Stats is a short summary for the admin command.
func (s *Store) Stats(ctx context.Context) (string, error) {
	var installs, profiles, follows, reports int
	err := s.db.QueryRowContext(ctx, `SELECT (SELECT COUNT(*) FROM installs), (SELECT COUNT(*) FROM profiles),
		(SELECT COUNT(*) FROM follows), (SELECT COUNT(*) FROM reports)`).Scan(&installs, &profiles, &follows, &reports)
	return fmt.Sprintf("installs %d, profiles %d, follows %d, reports %d", installs, profiles, follows, reports), err
}

// Backup writes a consistent copy of the database to path.
func (s *Store) Backup(ctx context.Context, path string) error {
	_, err := s.db.ExecContext(ctx, `VACUUM INTO ?`, path)
	return err
}
