package overlay

import (
	"bytes"
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

const (
	MaxSavedSearches = 100
	MaxSearchFolders = 20
)

type SavedSearch struct {
	ID        string                `json:"id"`
	Name      string                `json:"name"`
	Folder    string                `json:"folder,omitempty"` // folder id, "" at the top level
	Query     trade.EvaluateRequest `json:"query"`
	CreatedAt time.Time             `json:"createdAt"`
}

type SearchFolder struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// SearchLibrary is everything the market's saved list shows: the folders in
// the user's order and the searches, each naming its folder.
type SearchLibrary struct {
	Folders  []SearchFolder `json:"folders"`
	Searches []SavedSearch  `json:"searches"`
}

// libraryFile is the file layout since folders arrived. The first version
// was a bare array of searches; load still reads it.
type libraryFile struct {
	Version  int            `json:"version"`
	Folders  []SearchFolder `json:"folders"`
	Searches []SavedSearch  `json:"searches"`
}

type SearchStore struct {
	mu   sync.Mutex
	path string
}

func NewSearchStore(path string) *SearchStore {
	return &SearchStore{path: path}
}

func (s *SearchStore) List() (SearchLibrary, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.load()
}

// Save adds a search, into a folder when folder names one that exists.
func (s *SearchStore) Save(name, folder string, query trade.EvaluateRequest) (SearchLibrary, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	name = strings.TrimSpace(name)
	if name == "" {
		return SearchLibrary{}, errors.New("saved search name is empty")
	}
	if strings.TrimSpace(query.BaseType) == "" {
		return SearchLibrary{}, errors.New("saved search base type is empty")
	}
	lib, err := s.load()
	if err != nil {
		return SearchLibrary{}, err
	}
	if len(lib.Searches) >= MaxSavedSearches {
		return lib, fmt.Errorf("at most %d searches can be saved", MaxSavedSearches)
	}
	if !lib.hasFolder(folder) {
		folder = ""
	}
	lib.Searches = append(lib.Searches, SavedSearch{
		ID:        newID(),
		Name:      name,
		Folder:    folder,
		Query:     query,
		CreatedAt: time.Now().UTC(),
	})
	return lib, s.write(lib)
}

func (s *SearchStore) Delete(id string) (SearchLibrary, error) {
	return s.change(func(lib *SearchLibrary) error {
		kept := lib.Searches[:0]
		for _, item := range lib.Searches {
			if item.ID != id {
				kept = append(kept, item)
			}
		}
		lib.Searches = kept
		return nil
	})
}

// Move puts a search into a folder, or back to the top level with "".
func (s *SearchStore) Move(id, folder string) (SearchLibrary, error) {
	return s.change(func(lib *SearchLibrary) error {
		if !lib.hasFolder(folder) {
			return errors.New("no such folder")
		}
		for i := range lib.Searches {
			if lib.Searches[i].ID == id {
				lib.Searches[i].Folder = folder
			}
		}
		return nil
	})
}

func (s *SearchStore) CreateFolder(name string) (SearchLibrary, error) {
	return s.change(func(lib *SearchLibrary) error {
		name = strings.TrimSpace(name)
		if name == "" {
			return errors.New("folder name is empty")
		}
		if len(lib.Folders) >= MaxSearchFolders {
			return fmt.Errorf("at most %d folders", MaxSearchFolders)
		}
		lib.Folders = append(lib.Folders, SearchFolder{ID: newID(), Name: name})
		return nil
	})
}

func (s *SearchStore) RenameFolder(id, name string) (SearchLibrary, error) {
	return s.change(func(lib *SearchLibrary) error {
		name = strings.TrimSpace(name)
		if name == "" {
			return errors.New("folder name is empty")
		}
		for i := range lib.Folders {
			if lib.Folders[i].ID == id {
				lib.Folders[i].Name = name
			}
		}
		return nil
	})
}

// DeleteFolder removes a folder; its searches move to the top level rather
// than going with it.
func (s *SearchStore) DeleteFolder(id string) (SearchLibrary, error) {
	return s.change(func(lib *SearchLibrary) error {
		kept := lib.Folders[:0]
		for _, f := range lib.Folders {
			if f.ID != id {
				kept = append(kept, f)
			}
		}
		lib.Folders = kept
		for i := range lib.Searches {
			if lib.Searches[i].Folder == id {
				lib.Searches[i].Folder = ""
			}
		}
		return nil
	})
}

func (s *SearchStore) change(apply func(*SearchLibrary) error) (SearchLibrary, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	lib, err := s.load()
	if err != nil {
		return SearchLibrary{}, err
	}
	if err := apply(&lib); err != nil {
		return lib, err
	}
	return lib, s.write(lib)
}

func (lib SearchLibrary) hasFolder(id string) bool {
	if id == "" {
		return true
	}
	for _, f := range lib.Folders {
		if f.ID == id {
			return true
		}
	}
	return false
}

func (s *SearchStore) load() (SearchLibrary, error) {
	empty := SearchLibrary{Folders: []SearchFolder{}, Searches: []SavedSearch{}}
	raw, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return empty, nil
	}
	if err != nil {
		return SearchLibrary{}, err
	}
	lib := empty
	if trimmed := bytes.TrimSpace(raw); len(trimmed) > 0 && trimmed[0] == '[' {
		// The first layout: a bare array of searches, no folders.
		if err := json.Unmarshal(raw, &lib.Searches); err != nil {
			return SearchLibrary{}, err
		}
	} else {
		var file libraryFile
		if err := json.Unmarshal(raw, &file); err != nil {
			return SearchLibrary{}, err
		}
		lib.Folders, lib.Searches = file.Folders, file.Searches
	}
	if lib.Folders == nil {
		lib.Folders = []SearchFolder{}
	}
	if lib.Searches == nil {
		lib.Searches = []SavedSearch{}
	}
	// A search whose folder is gone shows at the top level.
	for i := range lib.Searches {
		if !lib.hasFolder(lib.Searches[i].Folder) {
			lib.Searches[i].Folder = ""
		}
	}
	return lib, nil
}

func (s *SearchStore) write(lib SearchLibrary) error {
	raw, err := json.MarshalIndent(libraryFile{Version: 2, Folders: lib.Folders, Searches: lib.Searches}, "", "  ")
	if err != nil {
		return err
	}
	return prices.WriteFileAtomic(s.path, raw)
}

func newID() string { return fmt.Sprintf("%x", time.Now().UnixNano()) }
