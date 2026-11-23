package config

import (
	"encoding/json"
	"maps"
	"os"
	"path/filepath"
	"slices"
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
		Servers            map[string]ConfigServer   `json:"servers"`              // server name (alias) -> server config
		LiveStatusMessages []ConfigLiveStatusMessage `json:"live_status_messages"` // bound self-updating status messages
	}

	// ConfigServer holds the configuration for a single DayZ server with StateZ daemon installed.
	ConfigServer struct {
		BaseUrl   string `json:"base_url"`
		AuthToken string `json:"auth_token"`
	}

	// ConfigLiveStatusMessage binds a self-updating server-status message to a chat (optionally a forum thread).
	ConfigLiveStatusMessage struct {
		ChatID     int64  `json:"chat_id"`
		ThreadID   int    `json:"thread_id,omitempty"` // 0 when not a forum thread
		MessageID  int    `json:"message_id"`
		ServerName string `json:"server_name"`
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

	cfg := f.cfg // copy the struct to avoid callers mutating shared state

	cfg.Servers = make(map[string]ConfigServer, len(f.cfg.Servers))
	maps.Copy(cfg.Servers, f.cfg.Servers)

	// struct elems have no reference fields, so a plain clone is a deep copy
	cfg.LiveStatusMessages = slices.Clone(f.cfg.LiveStatusMessages)

	return cfg
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
