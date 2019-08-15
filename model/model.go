package model

import (
	"encoding/json"
	"errors"
	"expvar"
	"fmt"
	"io/ioutil"
	"path"
	"sync"

	"github.com/mixpanel/configmanager/configmap"

	"github.com/mixpanel/obs"
	"github.com/mixpanel/obs/obserr"
)

var (
	ErrNotFound = errors.New("Config not found")
)

// Config is the struct configmanager expects
// the configuration to be. When the file configs.json
// is parsed, State manager expects an array of this struct.
type Config struct {
	Key         string          `json:"key"`
	RawValue    json.RawMessage `json:"value"`
	parsedValue interface{}
}

func (c *Config) String() string {
	return string(c.RawValue)
}

// State is what is kept in memory by the statemanager
// It is an exposed struct to support the dummy State manage\r
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
	State *State

	updateChan chan struct{}

	watcher *configmap.CmWatcher

	emap *expvar.Map
}

// Statemanager is responsible for managing
// configmanager State. Configmanager client interacts
// with Statemanager to get raw configs
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

// NewStateManager returns the State manager which is used
// by the configmanager client. State manager watches the file
// for config changes and loads the State in memory.
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
	for sm.State == nil {
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
	State := &State{
		cache: make(map[string]*Config),
	}
	if err := json.Unmarshal(data, &(State.Configs)); err != nil {
		return obserr.Annotate(err, "error json unmarshal the State").Set("path", filePath)
	}
	return sm.loadState(State)
}

func (sm *stateManager) loadState(State *State) error {
	State.buildCache()
	sm.mu.Lock()
	sm.State = State
	sm.mu.Unlock()
	sm.notify()
	for _, cfg := range State.Configs {
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
	return sm.State.get(key)
}

func (sm *stateManager) Close() {
	if sm.watcher != nil {
		sm.watcher.Stop()
	}
}
