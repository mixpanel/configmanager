package configmap

import (
	"io"
	"io/ioutil"
	"os"
	"path"
	"sync/atomic"
	"testing"

	"github.com/mixpanel/configmanager/testutil"

	"github.com/mixpanel/obs"
	"github.com/mixpanel/obs/obserr"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	yaml "gopkg.in/yaml.v1"
)

var cfgPath = "/tmp/configs/arb-binary-test-config"
var cfgFile = cfgPath + "/config.yaml"

var nullOnFileEvent OnFileEvent = func(_ string) error { return nil }

func initTest() {
	os.RemoveAll(cfgPath)
	os.MkdirAll(cfgPath, os.ModePerm)
}

// yaml file does not exist => fail to create config object
func TestLoadConfigNoYaml(t *testing.T) {
	t.Parallel()
	testutil.WithTempDir(t, func(root string) {
		cfgFile := path.Join(root, "config.yaml")
		w, err := NewCmWatcher(cfgFile, nullOnFileEvent, obs.NullFR)
		require.NoError(t, err, "expected NewCmWatcher to not error out when the file does not exist")
		require.Error(t, w.Start(), "expected Start() to return an error when the file does not exist")
	})
}

// start with empty ConfgMap file, add entries to ConfigMap file, make sure entries are
// aded to in-memory config object
func TestConfigDynamicAdd(t *testing.T) {
	t.Parallel()

	testutil.WithTempDir(t, func(root string) {
		cfgFile := path.Join(root, "config.yaml")
		require.NoError(t, ioutil.WriteFile(cfgFile, []byte{}, 0700))

		var (
			v atomic.Value
			c = testutil.NewCallCounter()
		)
		onNotify := func(p string) error {
			bs, err := ioutil.ReadFile(p)
			require.NoError(t, err)

			var fileContents map[string]interface{}
			if err := yaml.Unmarshal(bs, &fileContents); err != nil {
				return obserr.Annotate(err, "yaml.Unmarshal failed")
			}

			v.Store(fileContents)
			c.Incr()
			return nil
		}

		w, err := NewCmWatcher(cfgFile, onNotify, obs.NullFR)
		require.NoError(t, err)

		require.NoError(t, w.Start())
		defer w.Stop()

		c.Wait(1)
		expectedFileContents := map[string]interface{}(nil)
		assert.Equal(t, expectedFileContents, v.Load().(map[string]interface{}))

		safeWriteFile(t, cfgFile, "foo: bar")

		c.Wait(2)
		expectedFileContents = map[string]interface{}{
			"foo": "bar",
		}
		assert.Equal(t, expectedFileContents, v.Load().(map[string]interface{}))
	})
}

// start with ConfigMap file containing entries, delete one entry, make sure
// in-memory config has this entry deleted as well
func TestConfigDynamicDelete(t *testing.T) {
	t.Parallel()

	testutil.WithTempDir(t, func(root string) {
		cfgFile := path.Join(root, "config.yaml")
		require.NoError(t, ioutil.WriteFile(cfgFile, []byte("foo: bar\nfizz: buzz"), 0700))

		var (
			v atomic.Value
			c = testutil.NewCallCounter()
		)
		onNotify := func(p string) error {
			bs, err := ioutil.ReadFile(p)
			require.NoError(t, err)

			var fileContents map[string]interface{}
			if err := yaml.Unmarshal(bs, &fileContents); err != nil {
				return obserr.Annotate(err, "yaml.Unmarshal failed")
			}

			v.Store(fileContents)
			c.Incr()
			return nil
		}

		w, err := NewCmWatcher(cfgFile, onNotify, obs.NullFR)
		require.NoError(t, err)

		require.NoError(t, w.Start())
		defer w.Stop()

		c.Wait(1)
		expectedFileContents := map[string]interface{}{
			"foo":  "bar",
			"fizz": "buzz",
		}
		assert.Equal(t, expectedFileContents, v.Load().(map[string]interface{}))

		safeWriteFile(t, cfgFile, "foo: bar")

		c.Wait(2)
		expectedFileContents = map[string]interface{}{
			"foo": "bar",
		}
		assert.Equal(t, expectedFileContents, v.Load().(map[string]interface{}))
	})
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
