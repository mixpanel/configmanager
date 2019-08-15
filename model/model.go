package model

import (
	"configmap"
	"encoding/json"
	"errors"
	"expvar"
	"fmt"
	"io/ioutil"
	"path"
	"sync"

	"github.com/mixpanel/obs"
	"github.com/mixpanel/obs/obserr"
)

var (
	ErrNotFound = errors.New("Config not found")
)

type Config struct {
	Key         string          `json:"key"`
	RawValue    json.RawMessage `json:"value"`
	parsedValue interface{}
}

func (c *Config) String() string {
	return string(c.RawValue)
}

type State struct {
	Configs []*Config
	cache   map[string]*Config
}

func (s *State) buildCache() {
	if s.cache == nil {
		s.cache = make(map[string]*Config)
	}
	for _, cfg := range s.Configs {
		s.cache[cfg.Key] = cfg
	}
}

func (s *State) get(key string) (*Config, error) {
	cfg, ok := s.cache[key]
	if !ok {
		return nil, ErrNotFound
	}
	return cfg, nil
}

type stateManager struct {
	filePath string

	mu    sync.RWMutex
	cond  *sync.Cond
	state *State

	updateChan chan struct{}

	watcher *configmap.CmWatcher

	emap *expvar.Map
}

type StateManager interface {
	GetKey(string) (*Config, error)
	GetParsedValue(*Config) interface{}
	SetParsedValue(*Config, interface{})
	Close()
}

type NullStateManager struct {
}

func (n *NullStateManager) GetKey(string) (*Config, error) {
	return nil, ErrNotFound
}

func (n *NullStateManager) GetParsedValue(*Config) interface{} {
	return nil
}

func (n *NullStateManager) SetParsedValue(*Config, interface{}) {
}

func (n *NullStateManager) Close() {
}

func NewStateManager(dirPath string, scope string, updateChan chan struct{}, fr obs.FlightRecorder) (StateManager, error) {
	fr = fr.ScopeName("state_manager")

	sm := &stateManager{
		filePath: path.Join(dirPath, scope, "configs.json"),
		emap:     expvar.NewMap(fmt.Sprintf("configmanager.%s", scope)),
	}

	cmWatcher, err := configmap.NewCmWatcher(sm.filePath, sm.loadConfig, fr)
	if err != nil {
		return nil, obserr.Annotate(err, "Error making cm watcher for the config manager").Set("path", sm.filePath)
	}
	sm.watcher = cmWatcher

	if err := sm.init(fr); err != nil {
		return nil, obserr.Annotate(err, "init failed")
	}

	return sm, nil
}

func (sm *stateManager) init(fr obs.FlightRecorder) error {
	if sm.updateChan == nil {
		// just make a dummy chan
		sm.updateChan = make(chan struct{})
	}
	sm.cond = sync.NewCond(&sm.mu)

	if err := sm.watcher.Start(); err != nil {
		return obserr.Annotate(err, "error starting cm watcher")
	}

	// wait for the initial loadConfig
	sm.cond.L.Lock()
	for sm.state == nil {
		sm.cond.Wait()
	}
	sm.cond.L.Unlock()
	return nil
}

func (sm *stateManager) GetParsedValue(cfg *Config) interface{} {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	return cfg.parsedValue
}

func (sm *stateManager) SetParsedValue(cfg *Config, val interface{}) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	cfg.parsedValue = val
}

func (sm *stateManager) loadConfig(filePath string) error {
	defer sm.cond.Broadcast()

	data, err := ioutil.ReadFile(filePath)
	if err != nil {
		return obserr.Annotate(err, "Error reading the config file").Set("path", filePath)
	}
	state := &State{
		cache: make(map[string]*Config),
	}
	if err := json.Unmarshal(data, &(state.Configs)); err != nil {
		return obserr.Annotate(err, "error json unmarshal the state").Set("path", filePath)
	}
	return sm.loadState(state)
}

func (sm *stateManager) loadState(state *State) error {
	state.buildCache()
	sm.mu.Lock()
	sm.state = state
	sm.mu.Unlock()
	sm.notify()
	for _, cfg := range state.Configs {
		sm.emap.Set(cfg.Key, cfg)
	}
	return nil
}

func (sm *stateManager) notify() {
	select {
	case sm.updateChan <- struct{}{}:
	default:
	}
}

func (sm *stateManager) GetKey(key string) (*Config, error) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.state.get(key)
}

func (sm *stateManager) Close() {
	if sm.watcher != nil {
		sm.watcher.Stop()
	}
}
