package model

import (
	"encoding/json"
	"fmt"
	"sync"
)

type DummyStateManager struct {
	*NullStateManager
	state *State
	mu    sync.RWMutex
}

func NewDummyStateManager(state *State) *DummyStateManager {
	state.buildCache()
	return &DummyStateManager{
		NullStateManager: &NullStateManager{},
		state:            state,
	}
}

func (d *DummyStateManager) GetKey(key string) (*Config, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.state.get(key)
}

func (d *DummyStateManager) SetConfig(cfg *Config) *DummyStateManager {
	d.mu.Lock()
	defer d.mu.Unlock()
	// state has slice of configs too but we dont care
	// here
	d.state.cache[cfg.Key] = cfg
	return d
}

func (d *DummyStateManager) ToRawVal(val interface{}) []byte {
	// No one shoudl be using this for prod code because
	// this is unsafe
	data, err := json.Marshal(val)
	if err != nil {
		panic(fmt.Errorf("Error marshalling the value to json %v %v", val, err))
	}
	return data
}
