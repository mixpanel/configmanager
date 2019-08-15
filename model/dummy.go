package model

import (
	"sync"
)

// DummyStateManager is a statemanager
// which allows for  raw configs to be set
// and retrieved
type DummyStateManager struct {
	*NullStateManager
	state *State
	mu    sync.RWMutex
}

// NewDummyStateManager returns a new instance
// of DummyStateManager
func NewDummyStateManager() *DummyStateManager {
	state := &State{}
	state.buildCache()
	return &DummyStateManager{
		NullStateManager: &NullStateManager{},
		state:            state,
	}
}

// GetKey returns the Config struct stored in memory
func (d *DummyStateManager) GetKey(key string) (*Config, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.state.get(key)
}

// SetConfig can be used to store a config into the
// dummy state manager
func (d *DummyStateManager) SetConfig(cfg *Config) *DummyStateManager {
	d.mu.Lock()
	defer d.mu.Unlock()
	// state has slice of configs too but we dont care
	// here
	d.state.cache[cfg.Key] = cfg
	return d
}
