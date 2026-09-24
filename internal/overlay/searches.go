package overlay

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"poe2filter/internal/prices"
	"poe2filter/internal/trade"
)

const MaxSavedSearches = 30

type SavedSearch struct {
	ID        string                `json:"id"`
	Name      string                `json:"name"`
	Query     trade.EvaluateRequest `json:"query"`
	CreatedAt time.Time             `json:"createdAt"`
}

type SearchStore struct {
	mu   sync.Mutex
	path string
}

func NewSearchStore(path string) *SearchStore {
	return &SearchStore{path: path}
}

func (s *SearchStore) List() ([]SavedSearch, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.load()
}

func (s *SearchStore) Save(name string, query trade.EvaluateRequest) ([]SavedSearch, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, errors.New("saved search name is empty")
	}
	if strings.TrimSpace(query.BaseType) == "" {
		return nil, errors.New("saved search base type is empty")
	}
	items, err := s.load()
	if err != nil {
		return nil, err
	}
	if len(items) >= MaxSavedSearches {
		return nil, fmt.Errorf("at most %d searches can be saved", MaxSavedSearches)
	}
	items = append(items, SavedSearch{
		ID:        fmt.Sprintf("%x", time.Now().UnixNano()),
		Name:      name,
		Query:     query,
		CreatedAt: time.Now().UTC(),
	})
	return items, s.write(items)
}

func (s *SearchStore) Delete(id string) ([]SavedSearch, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	items, err := s.load()
	if err != nil {
		return nil, err
	}
	kept := make([]SavedSearch, 0, len(items))
	for _, item := range items {
		if item.ID != id {
			kept = append(kept, item)
		}
	}
	if len(kept) == len(items) {
		return items, nil
	}
	return kept, s.write(kept)
}

func (s *SearchStore) load() ([]SavedSearch, error) {
	raw, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return []SavedSearch{}, nil
	}
	if err != nil {
		return nil, err
	}
	var items []SavedSearch
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil, err
	}
	if items == nil {
		items = []SavedSearch{}
	}
	return items, nil
}

func (s *SearchStore) write(items []SavedSearch) error {
	raw, err := json.MarshalIndent(items, "", "  ")
	if err != nil {
		return err
	}
	return prices.WriteFileAtomic(s.path, raw)
}
