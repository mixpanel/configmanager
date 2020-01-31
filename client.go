package configmanager

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"sync"
	"time"

	"github.com/mixpanel/obs/obserr"

	"github.com/mixpanel/configmanager/logger"
	"github.com/mixpanel/configmanager/model"
)

// Client is the interface for reading configs stored in configmap.
// It has helper methods for different data types so the user
// does not have to care about the structure of configs.
type Client interface {
	Unmarshal(key string, val interface{}) error
	GetBoolean(key string, defaultVal bool) bool
	GetInt64(key string, defaultVal int64) int64
	GetByte(key string, defaultVal uint8) uint8

	GetFloat64(key string, defaultVal float64) float64
	GetString(key string, defaultVal string) string
	GetRaw(key string) ([]byte, error)

	IsFeatureEnabled(key string, enabledByDefault bool) bool
	// we use project whitelisting quite a lot. This expects
	// map [int64]struct{}
	IsProjectWhitelisted(key string, projectID int64, defaultVal bool) bool
	IsTokenWhitelisted(key string, token string, defaultVal bool) bool
	Close()
}

type client struct {
	logger      logger.Logger
	sm          model.StateManager
	unmarshalFn func([]byte, interface{}) error
	rng         rnd
	mu          sync.Mutex // Lock for rng since the one we use is not concurrent-safe
}

type rnd interface {
	Float64() float64
}

// NewNullClient returns a client that will just
// echo back the default value you set in your Gets
func NewNullClient() Client {
	return newClientFromStateManager(&model.NullStateManager{}, logger.NullLogger{})
}

// TestClient is to be used only for tests
// It is threadsafe but it can NOT be used for production
// Values can be set on the test client using the set
// methods and then it echoes back those values in the Get
// methods
type TestClient struct {
	*client
	dm *model.DummyStateManager
}

// NewTestClient returns a TestClient
func NewTestClient() *TestClient {
	dm := model.NewDummyStateManager()
	return &TestClient{
		client: newClientFromStateManager(dm, logger.DefaultLogger),
		dm:     dm,
	}
}

func (t *TestClient) setValue(key string, val interface{}) *TestClient {
	data, err := json.Marshal(val)
	if err != nil {
		panic(fmt.Errorf("Error marshalling the value to json %v %v", val, err))
	}
	t.dm.SetConfig(&model.Config{Key: key, RawValue: data})
	return t
}

func (t *TestClient) SetProjectsWhitelist(key string, projects ...int) *TestClient {
	val := make(map[int]struct{})
	for _, p := range projects {
		val[p] = struct{}{}
	}
	return t.setValue(key, val)
}

func (t *TestClient) SetBoolean(key string, val bool) *TestClient {
	return t.setValue(key, val)
}

func (t *TestClient) SetInt64(key string, val int64) *TestClient {
	return t.setValue(key, val)
}

func (t *TestClient) SetFloat64(key string, val float64) *TestClient {
	return t.setValue(key, val)
}

func (t *TestClient) SetString(key string, val string) *TestClient {
	return t.setValue(key, val)
}

func (t *TestClient) SetRaw(key string, raw []byte) *TestClient {
	t.dm.SetConfig(&model.Config{Key: key, RawValue: raw})
	return t
}

func (t *TestClient) SetByte(key string, val uint8) *TestClient {
	return t.setValue(key, val)
}

// NewClient returns a config manager client for a scope specified.
// If you created the configs loggerom the jsonnet config helper then your configs
// will be placed like /etc/configs/storage-server/configs.
// This client assumes that there will a file called configs.json for a scope
// and inside the file there will be configs specified in the model
// If there is an error initing the client, it will become a null client, return an error
// and just return default values on the Gets
// For your services instead of creating one type of config in a configmap, group all
// of your configs into logical scope and create the configmap using the jsonnet helper.
// With adoption of this client, you will at least every single service having
// one scope with bunch of configs that are relevant to that service.
func NewClient(dirPath string, scope string, logger logger.Logger) (Client, error) {
	sm, err := model.NewStateManager(dirPath, scope, nil, logger)
	if err != nil {
		return nil, obserr.Annotate(err, "Error creating config manager client").Set(
			"scope", scope,
			"dir_path", dirPath,
		)
	}
	return newClientFromStateManager(sm, logger), err
}

func newClientFromStateManager(sm model.StateManager, logger logger.Logger) *client {
	return &client{
		logger:      logger,
		sm:          sm,
		unmarshalFn: json.Unmarshal,
		rng:         defaultRng(time.Now().UnixNano()),
	}
}

func (c *client) Unmarshal(key string, val interface{}) error {
	config, err := c.sm.GetKey(key)
	if err != nil {
		return obserr.Annotate(err, "Unmarshal: error getting the key").Set("key", key)
	}
	if err := c.unmarshalFn(config.RawValue, val); err != nil {
		return obserr.Annotate(err, "Unmarshal: error unmarshalling the key").Set("key", key)
	}
	// we could set the parsed value but because we
	// dont have templates we will not be able to verify if the parsed
	// value matches the val so json unmarshal every time
	return nil
}

func (c *client) logErrGet(err error, key string, defaultVal interface{}, logger logger.Logger) {
	if obserr.Original(err) == model.ErrNotFound {
		// no log
		return
	}
	logger.Warn(
		"error while doing get",
		"key", key,
		"default_value", defaultVal,
		"err", err,
	)
}

func (c *client) getByte(key string, defaultVal uint8) (uint8, error) {
	config, err := c.sm.GetKey(key)
	if err != nil {
		return defaultVal, obserr.Annotate(err, "getByte: Error getting key loggerom config")
	}
	pv := c.sm.GetParsedValue(config)
	if pv != nil {
		val, ok := pv.(uint8)
		if ok {
			return val, nil
		}
	}
	var val uint8
	if err := c.Unmarshal(key, &val); err != nil {
		return defaultVal, obserr.Annotate(err, "getByte: error unmarshalling")
	}
	c.sm.SetParsedValue(config, val)
	return val, nil

}

func (c *client) GetByte(key string, defaultVal uint8) uint8 {
	logger := c.logger.ScopeName("get_byte")
	val, err := c.getByte(key, defaultVal)
	if err != nil {
		c.logErrGet(err, key, defaultVal, logger)
		return defaultVal
	}
	return val
}

func (c *client) GetBoolean(key string, defaultVal bool) bool {
	logger := c.logger.ScopeName("get_boolean")
	val, err := c.getBoolean(key, defaultVal)
	if err != nil {
		c.logErrGet(err, key, defaultVal, logger)
		return defaultVal
	}
	return val
}

func (c *client) getBoolean(key string, defaultVal bool) (bool, error) {
	config, err := c.sm.GetKey(key)
	if err != nil {
		return defaultVal, obserr.Annotate(err, "getBoolean: Error getting key loggerom config")
	}
	pv := c.sm.GetParsedValue(config)
	if pv != nil {
		val, ok := pv.(bool)
		if ok {
			return val, nil
		}
	}
	var val bool
	if err := c.Unmarshal(key, &val); err != nil {
		return defaultVal, obserr.Annotate(err, "getBoolean: error unmarshalling")
	}
	c.sm.SetParsedValue(config, val)
	return val, nil
}

func (c *client) GetInt64(key string, defaultVal int64) int64 {
	logger := c.logger.ScopeName("get_int64")
	val, err := c.getInt64(key, defaultVal)
	if err != nil {
		c.logErrGet(err, key, defaultVal, logger)
		return defaultVal
	}
	return val
}

func (c *client) getInt64(key string, defaultVal int64) (int64, error) {
	config, err := c.sm.GetKey(key)
	if err != nil {
		return defaultVal, obserr.Annotate(err, "getInt64: error getting key loggerom config")
	}
	pv := c.sm.GetParsedValue(config)
	if pv != nil {
		switch val := pv.(type) {
		case int64:
			return val, nil
		case int32:
			return int64(val), nil
		case int:
			return int64(val), nil
		}
	}
	var val int64
	if err := c.Unmarshal(key, &val); err != nil {
		return defaultVal, obserr.Annotate(err, "getInt64: error unmarshalling")
	}
	c.sm.SetParsedValue(config, val)
	return val, nil
}

func (c *client) GetFloat64(key string, defaultVal float64) float64 {
	logger := c.logger.ScopeName("get_float64")
	val, err := c.getFloat64(key, defaultVal)
	if err != nil {
		c.logErrGet(err, key, defaultVal, logger)
		return defaultVal
	}
	return val
}

func (c *client) getFloat64(key string, defaultVal float64) (float64, error) {
	config, err := c.sm.GetKey(key)
	if err != nil {
		return defaultVal, obserr.Annotate(err, "getFloat64: error getting key")
	}
	pv := c.sm.GetParsedValue(config)
	if pv != nil {
		switch val := pv.(type) {
		case float64:
			return val, nil
		case float32:
			return float64(val), nil
		}
	}
	var val float64
	if err := c.Unmarshal(key, &val); err != nil {
		return defaultVal, obserr.Annotate(err, "getFloat64: error unmarshalling")
	}
	c.sm.SetParsedValue(config, val)
	return val, nil

}

func (c *client) GetString(key string, defaultVal string) string {
	logger := c.logger.ScopeName("get_string")
	val, err := c.getString(key, defaultVal)
	if err != nil {
		c.logErrGet(err, key, defaultVal, logger)
		return defaultVal
	}
	return val
}

func (c *client) getString(key string, defaultVal string) (string, error) {
	config, err := c.sm.GetKey(key)
	if err != nil {
		return defaultVal, obserr.Annotate(err, "getString: error getting key")
	}
	pv := c.sm.GetParsedValue(config)
	if pv != nil {
		if val, ok := pv.(string); ok {
			return val, nil
		}
	}
	var val string
	if err := c.Unmarshal(key, &val); err != nil {
		return defaultVal, obserr.Annotate(err, "getString: error unmarshalling")
	}
	c.sm.SetParsedValue(config, val)
	return val, nil

}

func (c *client) GetRaw(key string) ([]byte, error) {
	config, err := c.sm.GetKey(key)
	if err != nil {
		return nil, err
	}
	return config.RawValue, nil
}

func defaultRng(seed int64) rnd {
	return rand.New(rand.NewSource(seed))
}

func (c *client) IsFeatureEnabled(key string, enabledByDefault bool) bool {
	return c.rollDie(key, enabledByDefault)
}

func (c *client) rollDie(name string, enabledByDefault bool) bool {
	defaultValue := float64(0)
	if enabledByDefault {
		defaultValue = 1.0
	}

	// This can return error but will return default value
	val := c.GetFloat64(name, defaultValue)
	c.mu.Lock()
	randomFloat := c.rng.Float64()
	c.mu.Unlock()
	return randomFloat < val
}

func (c *client) IsProjectWhitelisted(key string, projectID int64, defaultVal bool) bool {
	logger := c.logger.ScopeName("is_project_whitelisted")
	val, err := c.isProjectWhitelisted(key, projectID, defaultVal)
	if err != nil {
		c.logErrGet(err, key, defaultVal, logger)
		return defaultVal
	}
	return val
}

func (c *client) IsTokenWhitelisted(key string, token string, defaultVal bool) bool {
	logger := c.logger.ScopeName("is_token_whitelisted")
	val, err := c.isTokenWhitelisted(key, token, defaultVal)
	if err != nil {
		c.logErrGet(err, key, defaultVal, logger)
		return defaultVal
	}
	return val
}

func (c *client) isTokenWhitelisted(key string, token string, defaultVal bool) (bool, error) {
	config, err := c.sm.GetKey(key)
	if err != nil {
		return defaultVal, obserr.Annotate(err, "isTokenWhitelisted: error getting key loggerom sm")
	}
	pv := c.sm.GetParsedValue(config)
	if pv != nil {
		switch val := pv.(type) {
		case map[string]struct{}:
			_, ok := val[token]
			return ok, nil
		default:
		}
	}
	val := make(map[string]struct{})
	if err := c.unmarshalFn(config.RawValue, &val); err != nil {
		return defaultVal, obserr.Annotate(err, "isTokenWhitelisted: error unmarshaling value")
	}
	c.sm.SetParsedValue(config, val)
	_, ok := val[token]
	return ok, nil
}

func (c *client) isProjectWhitelisted(key string, projectID int64, defaultVal bool) (bool, error) {
	config, err := c.sm.GetKey(key)
	if err != nil {
		return defaultVal, obserr.Annotate(err, "isProjectWhitelisted: error getting key loggerom sm")
	}
	pv := c.sm.GetParsedValue(config)
	if pv != nil {
		switch val := pv.(type) {
		case map[int64]struct{}:
			_, ok := val[projectID]
			return ok, nil
		default:
		}
	}
	val := make(map[int64]struct{})
	if err := c.unmarshalFn(config.RawValue, &val); err != nil {
		return defaultVal, obserr.Annotate(err, "isProjectWhitelisted: error unmarshaling value")
	}
	c.sm.SetParsedValue(config, val)
	_, ok := val[projectID]
	return ok, nil
}

func (c *client) Close() {
	c.sm.Close()
}
