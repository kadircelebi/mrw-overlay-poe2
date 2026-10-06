package profilesrv

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"poe2filter/internal/filter"
	"poe2filter/internal/publicprofile"
)

// Limits of the API.
const (
	// Many players share an address (CGNAT), so this only slows down
	// someone minting installs to inflate follower counts.
	InstallsPerIPPerDay = 10
	MaxVersionIDs       = 100
	MaxNoteLen          = 300
	maxBody             = publicprofile.MaxBytes + 8<<10
)

// ReportReasons are the reasons a report may give.
var ReportReasons = []string{"spam", "offensive", "broken", "other"}

var (
	idRE     = regexp.MustCompile(`^[a-z2-7]{8,16}$`)
	keyRE    = regexp.MustCompile(`^[A-Za-z0-9_-]{43}$`)
	authorRE = regexp.MustCompile(`^[^\s#]{1,40}#[0-9]{4}$`)
)

// Server is the HTTP API.
type Server struct {
	Store *Store
	// IPSalt makes the stored address hashes useless outside this server.
	IPSalt string
	Log    *log.Logger

	reads  *limiter
	writes *limiter
}

// Handler returns the API's routes.
func (s *Server) Handler() http.Handler {
	if s.Log == nil {
		s.Log = log.New(io.Discard, "", 0)
	}
	// Per address: listing and update checks; per install: changes.
	s.reads = newLimiter(240, time.Minute)
	s.writes = newLimiter(60, time.Hour)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/health", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, 200, map[string]string{"status": "ok"}) })
	mux.HandleFunc("POST /v1/installs", s.newInstall)
	mux.HandleFunc("GET /v1/profiles", s.list)
	mux.HandleFunc("GET /v1/profiles/{id}", s.get)
	mux.HandleFunc("POST /v1/profiles", s.auth(s.create))
	mux.HandleFunc("PUT /v1/profiles/{id}", s.auth(s.update))
	mux.HandleFunc("DELETE /v1/profiles/{id}", s.auth(s.remove))
	mux.HandleFunc("GET /v1/mine", s.auth(s.mine))
	mux.HandleFunc("POST /v1/versions", s.versions)
	mux.HandleFunc("PUT /v1/profiles/{id}/follow", s.auth(s.follow(true)))
	mux.HandleFunc("DELETE /v1/profiles/{id}/follow", s.auth(s.follow(false)))
	mux.HandleFunc("POST /v1/profiles/{id}/report", s.auth(s.report))
	return s.guard(mux)
}

// guard applies the per-address limit, the body limit and common headers.
func (s *Server) guard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Cache-Control", "no-store")
		if !s.reads.allow(ClientIP(r)) {
			writeErr(w, http.StatusTooManyRequests, "rate_limited")
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, maxBody)
		next.ServeHTTP(w, r)
	})
}

type ctxKey struct{}

// auth resolves the install key and applies the per-install write limit.
func (s *Server) auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		key, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		if !ok || !keyRE.MatchString(key) {
			writeErr(w, http.StatusUnauthorized, "no_install_key")
			return
		}
		install, err := s.Store.Install(r.Context(), key)
		if errors.Is(err, ErrNotFound) {
			writeErr(w, http.StatusUnauthorized, "unknown_install_key")
			return
		}
		if err != nil {
			s.fail(w, err)
			return
		}
		if r.Method != http.MethodGet && !s.writes.allow(install) {
			writeErr(w, http.StatusTooManyRequests, "rate_limited")
			return
		}
		next(w, r.WithContext(context.WithValue(r.Context(), ctxKey{}, install)))
	}
}

func installOf(r *http.Request) string { return r.Context().Value(ctxKey{}).(string) }

func (s *Server) ipHash(r *http.Request) string {
	sum := sha256.Sum256([]byte(s.IPSalt + "|" + ClientIP(r)))
	return hex.EncodeToString(sum[:16])
}

func (s *Server) newInstall(w http.ResponseWriter, r *http.Request) {
	key, err := s.Store.NewInstall(r.Context(), s.ipHash(r), InstallsPerIPPerDay)
	if errors.Is(err, ErrLimit) {
		writeErr(w, http.StatusTooManyRequests, "install_limit")
		return
	}
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"key": key})
}

func (s *Server) list(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	query, tag := q.Get("q"), q.Get("tag")
	if utf8.RuneCountInString(query) > publicprofile.MaxNameLen || !utf8.ValidString(query) {
		writeErr(w, http.StatusBadRequest, "bad_query")
		return
	}
	if tag != "" && !isTag(tag) {
		writeErr(w, http.StatusBadRequest, "bad_tag")
		return
	}
	page, _ := strconv.Atoi(q.Get("page"))
	if page < 0 || page > 1000 {
		writeErr(w, http.StatusBadRequest, "bad_page")
		return
	}
	list, err := s.Store.List(r.Context(), query, tag, page)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"profiles": list, "page_size": PageSize})
}

func (s *Server) get(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !idRE.MatchString(id) {
		writeErr(w, http.StatusNotFound, "not_found")
		return
	}
	p, err := s.Store.Get(r.Context(), id, false)
	if err != nil {
		s.fail(w, err)
		return
	}
	doc, _, err := s.Store.Document(r.Context(), id)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"profile": p, "document": json.RawMessage(doc)})
}

// publishBody is what an author sends.
type publishBody struct {
	Author   string          `json:"author"`
	Meta     json.RawMessage `json:"meta"`
	Document json.RawMessage `json:"document"`
}

// readPublish checks everything an author sends; the document kept is the
// canonical re-encoding, never the bytes received.
func readPublish(r *http.Request) (Publish, string) {
	var body publishBody
	if code := decodeBody(r, &body); code != "" {
		return Publish{}, code
	}
	if !authorRE.MatchString(body.Author) || filter.CleanText(body.Author) != body.Author {
		return Publish{}, "bad_author"
	}
	meta, err := publicprofile.DecodeMeta(body.Meta)
	if err != nil {
		return Publish{}, "bad_meta"
	}
	doc, err := publicprofile.Decode(body.Document)
	if err != nil {
		return Publish{}, "bad_document"
	}
	canonical, err := publicprofile.Encode(doc)
	if err != nil {
		return Publish{}, "bad_document"
	}
	return Publish{Author: body.Author, Name: meta.Name, Description: meta.Description, Tags: meta.Tags, Document: canonical}, ""
}

func (s *Server) create(w http.ResponseWriter, r *http.Request) {
	p, code := readPublish(r)
	if code != "" {
		writeErr(w, http.StatusBadRequest, code)
		return
	}
	out, err := s.Store.Create(r.Context(), installOf(r), p)
	if errors.Is(err, ErrLimit) {
		writeErr(w, http.StatusConflict, "profile_limit")
		return
	}
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"profile": out})
}

func (s *Server) update(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !idRE.MatchString(id) {
		writeErr(w, http.StatusNotFound, "not_found")
		return
	}
	p, code := readPublish(r)
	if code != "" {
		writeErr(w, http.StatusBadRequest, code)
		return
	}
	out, err := s.Store.Update(r.Context(), installOf(r), id, p)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"profile": out})
}

func (s *Server) remove(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !idRE.MatchString(id) {
		writeErr(w, http.StatusNotFound, "not_found")
		return
	}
	if err := s.Store.Delete(r.Context(), installOf(r), id); err != nil {
		s.fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) mine(w http.ResponseWriter, r *http.Request) {
	list, err := s.Store.Owned(r.Context(), installOf(r))
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"profiles": list})
}

func (s *Server) versions(w http.ResponseWriter, r *http.Request) {
	var body struct {
		IDs []string `json:"ids"`
	}
	if code := decodeBody(r, &body); code != "" {
		writeErr(w, http.StatusBadRequest, code)
		return
	}
	if len(body.IDs) > MaxVersionIDs {
		writeErr(w, http.StatusBadRequest, "too_many_ids")
		return
	}
	var ids []string
	for _, id := range body.IDs {
		if idRE.MatchString(id) {
			ids = append(ids, id)
		}
	}
	v, err := s.Store.Versions(r.Context(), ids)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"versions": v})
}

func (s *Server) follow(on bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if !idRE.MatchString(id) {
			writeErr(w, http.StatusNotFound, "not_found")
			return
		}
		n, err := s.Store.Follow(r.Context(), installOf(r), id, on)
		if err != nil {
			s.fail(w, err)
			return
		}
		writeJSON(w, 200, map[string]int{"followers": n})
	}
}

func (s *Server) report(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !idRE.MatchString(id) {
		writeErr(w, http.StatusNotFound, "not_found")
		return
	}
	var body struct {
		Reason string `json:"reason"`
		Note   string `json:"note"`
	}
	if code := decodeBody(r, &body); code != "" {
		writeErr(w, http.StatusBadRequest, code)
		return
	}
	note := strings.TrimSpace(body.Note)
	if !oneOf(body.Reason, ReportReasons) || utf8.RuneCountInString(note) > MaxNoteLen || filter.CleanText(note) != note {
		writeErr(w, http.StatusBadRequest, "bad_report")
		return
	}
	if err := s.Store.Report(r.Context(), installOf(r), id, body.Reason, note); err != nil {
		s.fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// decodeBody reads a JSON body strictly; it returns an error code or "".
func decodeBody(r *http.Request, v any) string {
	data, err := io.ReadAll(r.Body)
	if err != nil {
		return "body_too_large"
	}
	if !utf8.Valid(data) {
		return "bad_json"
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return "bad_json"
	}
	if _, err := dec.Token(); err != io.EOF {
		return "bad_json"
	}
	return ""
}

func (s *Server) fail(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrNotFound):
		writeErr(w, http.StatusNotFound, "not_found")
	case errors.Is(err, ErrNotOwner):
		writeErr(w, http.StatusForbidden, "not_owner")
	default:
		s.Log.Printf("error: %v", err)
		writeErr(w, http.StatusInternalServerError, "server_error")
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, code string) {
	writeJSON(w, status, map[string]string{"error": code})
}

func isTag(t string) bool { return oneOf(t, publicprofile.Tags) }

func oneOf(v string, list []string) bool {
	for _, x := range list {
		if v == x {
			return true
		}
	}
	return false
}
