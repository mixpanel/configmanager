package model

import (
	"encoding/json"
	"expvar"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path"
	"testing"

	"github.com/mixpanel/configmanager/configmap"

	"github.com/mixpanel/obs"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func mkTempDir(t *testing.T) (string, func()) {
	name, err := ioutil.TempDir("", "test-golang")
	assert.NoError(t, err)
	return name, func() { os.RemoveAll(name) }
}

func fillRawValues(t *testing.T, persist *State) {
	for _, cfg := range persist.Configs {
		data, err := json.Marshal(cfg.parsedValue)
		assert.NoError(t, err)
		cfg.RawValue = json.RawMessage(data)
		cfg.parsedValue = nil
	}
}

func getMarshalledState(t *testing.T, s *State) ([]byte, error) {
	persist := &State{Configs: make([]*Config, len(s.Configs))}
	for i, c := range s.Configs {
		tmp := *c
		persist.Configs[i] = &tmp
	}
	fillRawValues(t, persist)
	return json.Marshal(persist.Configs)
}

func TestConfigLoadAndUpdate(t *testing.T) {
	persist := &State{
		Configs: []*Config{
			{
				Key:         "foo",
				parsedValue: 1,
			},
			{
				Key:         "bar",
				parsedValue: 3,
			},
			{
				Key:         "baz",
				parsedValue: 4,
			},
		},
	}
	dir, done := mkTempDir(t)
	defer done()
	ns := "test"
	assert.NoError(t, os.Mkdir(path.Join(dir, ns), 0777))
	rootDir := dir
	dir = path.Join(dir, ns)

	data, err := getMarshalledState(t, persist)
	assert.NoError(t, err)
	filePath := path.Join(dir, "configs.json")
	assert.NoError(t, ioutil.WriteFile(filePath, data, 0777))

	ch := make(chan struct{})
	sm := newStateManagerForTest(t, rootDir, ns, ch)
	defer sm.Close()

	sm.watcher.NotifyCounter.Wait(1)

	assertConfigNoError := func(key string, val string) {
		config, err := sm.GetKey(key)
		assert.NoError(t, err)
		assert.EqualValues(t, string(config.RawValue), val)
	}

	assertConfigNoError("foo", "1")
	assertConfigNoError("bar", "3")
	assertConfigNoError("baz", "4")

	persist.Configs[0].parsedValue = 2
	persist.Configs = persist.Configs[:len(persist.Configs)-1]
	data, err = getMarshalledState(t, persist)
	require.NoError(t, err)
	safeWriteFile(t, filePath, string(data))

	sm.watcher.NotifyCounter.Wait(2)
	assertConfigNoError("foo", "2")
	assertConfigNoError("bar", "3")
	_, err = sm.GetKey("baz")
	assert.Equal(t, err, ErrNotFound)
}

func newStateManagerForTest(t *testing.T, root, scope string, ch chan struct{}) *stateManager {
	sm := &stateManager{
		filePath: path.Join(root, scope, "configs.json"),
		emap:     expvar.NewMap(fmt.Sprintf("configmanager.%s.%s", root, scope)),
	}

	w, err := configmap.NewCmWatcherForTest(sm.filePath, sm.loadConfig, obs.NullFR)
	require.NoError(t, err)
	sm.watcher = w

	require.NoError(t, sm.init(obs.NullFR))
	return sm
}

func safeWriteFile(t *testing.T, destPath, contents string) {
	err := os.MkdirAll(path.Dir(destPath), 0700)
	require.NoError(t, err)

	tf, err := ioutil.TempFile(path.Dir(destPath), "tmp-file.")
	require.NoError(t, err)

	_, err = io.WriteString(tf, contents)
	require.NoError(t, err)
	require.NoError(t, tf.Sync())
	require.NoError(t, tf.Close())
	require.NoError(t, os.Rename(tf.Name(), destPath))
}
