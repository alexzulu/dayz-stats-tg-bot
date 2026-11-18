package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

type (
	// File is a JSON-backed config with concurrency-safe load, update, and atomic save.
	File struct {
		cfg  Config
		path string
		mu   sync.Mutex
	}

	// Config is the configuration structure loaded from and saved to a JSON file.
	//
	// NOTE: when adding map or slice fields, deep-copy them in the Config() method to avoid data races.
	Config struct {
		ConfigField1 string `json:"config_field_1"`
		ConfigField2 int    `json:"config_field_2"`
	}
)

// NewFile returns a File backed by the given path. Nothing is read until Load is called.
func NewFile(path string) *File { return &File{path: path} }

// Load reads the config from disk. A missing file is not an error and leaves the config at its zero value.
func (f *File) Load() error {
	f.mu.Lock()
	defer f.mu.Unlock()

	sys, fErr := os.Open(f.path)
	if fErr != nil {
		if os.IsNotExist(fErr) {
			return nil // missing file is not an error -leaves the zero-value config
		}

		return fErr
	}
	defer func() { _ = sys.Close() }()

	var raw Config
	if err := json.NewDecoder(sys).Decode(&raw); err != nil {
		return err
	}

	f.cfg = raw

	return sys.Close()
}

// Update applies fn to the configuration under the lock, so concurrent access is safe. Changes stay in memory until
// Save is called.
//
// IMPORTANT: fn must NOT call other non-lock-free methods on the File (e.g. Load, Save, Config, Update) while it
// runs, or it will deadlock on the non-reentrant mutex.
func (f *File) Update(fn func(*Config)) { f.mu.Lock(); fn(&f.cfg); f.mu.Unlock() }

// Config returns a copy of the current configuration.
func (f *File) Config() Config {
	f.mu.Lock()
	defer f.mu.Unlock()

	return f.cfg // returned by value, so callers can't mutate shared state
}

// Save writes the config to disk atomically.
func (f *File) Save() (outErr error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	// ensure the parent directory exists
	if err := os.MkdirAll(filepath.Dir(f.path), 0o755); err != nil { //nolint:mnd
		return err
	}

	tmp := f.path + ".tmp"

	sys, fErr := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644) //nolint:mnd
	if fErr != nil {
		return fErr
	}
	defer func() {
		_ = sys.Close() // second close is a no-op if the explicit one below succeeded

		if outErr != nil {
			_ = os.Remove(tmp) // drop the temp file if Save failed
		}
	}()

	if err := json.NewEncoder(sys).Encode(&f.cfg); err != nil {
		return err
	}

	if err := sys.Close(); err != nil {
		return err
	}

	if err := os.Rename(tmp, f.path); err != nil { // atomically replace the real file
		return err
	}

	return nil
}
