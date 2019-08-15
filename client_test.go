package configmanager

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mixpanel/configmanager/testutil"

	"github.com/mixpanel/configmanager/model"

	"github.com/mixpanel/obs"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type testStruct struct {
	X int
	Y float64
}

func cfg(t *testing.T, key string, val interface{}) *model.Config {
	data, err := json.Marshal(val)
	assert.NoError(t, err)
	return &model.Config{
		Key:      key,
		RawValue: data,
	}
}

func getMarshalledState(t *testing.T, persist *model.State) ([]byte, error) {
	return json.Marshal(persist.Configs)
}

func writePersistToFile(t *testing.T, persist *model.State, dirPath string, ns string) {
	assert.NoError(t, os.Mkdir(path.Join(dirPath, ns), 0777))
	data, err := getMarshalledState(t, persist)
	assert.NoError(t, err)
	filePath := path.Join(dirPath, ns, "configs.json")
	assert.NoError(t, ioutil.WriteFile(filePath, data, 0777))
}

func getNs() string {
	return fmt.Sprintf("test-ns-%d", time.Now().UnixNano())
}
func TestUnmarshal(t *testing.T) {
	persist := &model.State{
		Configs: []*model.Config{
			cfg(t, "foo", map[string]string{
				"x": "y",
			}),
			cfg(t, "bar", testStruct{
				X: 1,
				Y: 3.0,
			}),
		},
	}
	dir, done := testutil.MkTempDir(t)
	defer done()

	ns := getNs()
	writePersistToFile(t, persist, dir, ns)
	client, err := NewClient(dir, ns, obs.NullFR)
	assert.NoError(t, err)
	actual := &testStruct{}
	assert.NoError(t, client.Unmarshal("bar", actual))
	assert.EqualValues(t, *actual, testStruct{1, 3.0})
}

type countUnmarshal struct {
	c int32
}

func (cu *countUnmarshal) unmarshal(raw []byte, val interface{}) error {
	atomic.AddInt32(&cu.c, 1)
	return json.Unmarshal(raw, val)
}

func (cu *countUnmarshal) count() int {
	v := atomic.LoadInt32(&cu.c)
	return int(v)
}

func (cu *countUnmarshal) setCount(v int) {
	atomic.StoreInt32(&cu.c, int32(v))
}

type fixture struct {
	dir string
	c   Client
	cc  *client
	cu  *countUnmarshal
}

func withFixture(t *testing.T, persist *model.State, fn func(f *fixture)) {
	dir, done := testutil.MkTempDir(t)
	defer done()

	ns := getNs()
	writePersistToFile(t, persist, dir, ns)

	c, err := NewClient(dir, ns, obs.NullFR)
	require.NoError(t, err)
	defer c.Close()

	cc, ok := c.(*client)
	assert.True(t, ok)

	cu := &countUnmarshal{}
	cc.unmarshalFn = cu.unmarshal

	f := &fixture{
		dir: dir,
		c:   c,
		cc:  cc,
		cu:  cu,
	}
	fn(f)
}

func TestBool(t *testing.T) {
	persist := &model.State{
		Configs: []*model.Config{
			cfg(t, "foo", true),
			cfg(t, "bar", 3),
		},
	}
	withFixture(t, persist, func(f *fixture) {
		for i := 0; i < 5; i++ {
			val := f.c.GetBoolean("foo", false)
			assert.True(t, val)
		}
		assert.Equal(t, f.cu.count(), 1)
		f.cu.setCount(0)
		val := f.c.GetBoolean("bar", true)
		assert.True(t, val)
	})
}

func TestInt64(t *testing.T) {
	persist := &model.State{
		Configs: []*model.Config{
			cfg(t, "foo", 1.0),
			cfg(t, "bar", true),
		},
	}
	withFixture(t, persist, func(f *fixture) {
		c := f.c
		for i := 0; i < 5; i++ {
			val := c.GetInt64("foo", 2)
			assert.EqualValues(t, val, 1)
		}
		assert.EqualValues(t, f.cu.count(), 1)
		val := c.GetInt64("bar", 2)
		assert.EqualValues(t, val, 2)
	})
}

func TestFloat64(t *testing.T) {
	persist := &model.State{
		Configs: []*model.Config{
			cfg(t, "foo", 1.0),
			cfg(t, "bar", true),
		},
	}
	withFixture(t, persist, func(f *fixture) {
		c := f.c
		for i := 0; i < 5; i++ {
			val := c.GetFloat64("foo", 2)
			assert.EqualValues(t, val, 1.0)
		}
		assert.EqualValues(t, f.cu.count(), 1)
		val := c.GetFloat64("bar", 2.0)
		assert.EqualValues(t, val, 2.0)
	})
}

func TestString(t *testing.T) {
	persist := &model.State{
		Configs: []*model.Config{
			cfg(t, "foo", "hello"),
			cfg(t, "bar", 1),
		},
	}
	withFixture(t, persist, func(f *fixture) {
		c := f.c
		for i := 0; i < 5; i++ {
			val := c.GetString("foo", "what")
			assert.EqualValues(t, val, "hello")
		}
		assert.EqualValues(t, f.cu.count(), 1)
		val := c.GetString("bar", "what")
		assert.EqualValues(t, val, "what")
	})
}

func TestByte(t *testing.T) {
	persist := &model.State{
		Configs: []*model.Config{
			cfg(t, "foo", 1),
			cfg(t, "bar", 255),
			cfg(t, "baz", 256),
		},
	}
	withFixture(t, persist, func(f *fixture) {
		c := f.c
		for i := 0; i < 5; i++ {
			val := c.GetByte("foo", 0)
			assert.EqualValues(t, val, 1)
		}
		assert.EqualValues(t, f.cu.count(), 1)
		val := c.GetByte("bar", 0)
		assert.EqualValues(t, val, 255)

		val = c.GetByte("baz", 0)
		assert.EqualValues(t, val, 0)
	})
}

type testrnd struct {
}

func (tr *testrnd) Float64() float64 {
	return 0.8
}

func TestFeatureEnabled(t *testing.T) {
	persist := &model.State{
		Configs: []*model.Config{
			cfg(t, "foo", 0.9),
			cfg(t, "bar", 0.1),
		},
	}
	withFixture(t, persist, func(f *fixture) {
		cc := f.cc
		c := f.c
		cc.rng = &testrnd{}

		flag := c.IsFeatureEnabled("foo", false)
		assert.True(t, flag)

		flag = c.IsFeatureEnabled("bar", true)
		assert.False(t, flag)

		flag = c.IsFeatureEnabled("foobar", true)
		assert.True(t, flag)

		flag = c.IsFeatureEnabled("foobar", false)
		assert.False(t, flag)

		assert.EqualValues(t, f.cu.count(), 2)
	})
}

func TestProjectWhitelisted(t *testing.T) {
	persist := &model.State{
		Configs: []*model.Config{
			cfg(t, "foo", map[int]struct{}{
				3: {},
			}),
			cfg(t, "foobar", map[string]struct{}{
				"3": {},
			}),
			cfg(t, "bar", map[string]interface{}{
				"idontparseasint": struct{}{},
			}),
		},
	}
	withFixture(t, persist, func(f *fixture) {
		cc := f.cc
		for i := 0; i < 5; i++ {
			assert.True(t, cc.IsProjectWhitelisted("foo", 3, false))
			assert.False(t, cc.IsProjectWhitelisted("foo", 4, true))
		}
		assert.EqualValues(t, f.cu.count(), 1)
		assert.True(t, cc.IsProjectWhitelisted("bar", 3, true))
		assert.True(t, cc.IsProjectWhitelisted("foobar", 3, false))
	})
}

func TestMultiThreadedGet(t *testing.T) {
	persist := &model.State{
		Configs: []*model.Config{
			cfg(t, "foo", 0.9),
			cfg(t, "bar", 0.1),
		},
	}
	withFixture(t, persist, func(f *fixture) {
		cc := f.cc
		c := f.c
		cc.rng = &testrnd{}
		var wg sync.WaitGroup
		for i := 0; i < 100; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				flag := c.IsFeatureEnabled("foo", false)
				assert.True(t, flag)

				flag = c.IsFeatureEnabled("bar", true)
				assert.False(t, flag)
			}()
		}
		wg.Wait()
	})
}

func TestNullClient(t *testing.T) {
	c := NewNullClient()
	defer c.Close()
	assert.EqualValues(t, c.GetBoolean("foo", true), true)
	assert.EqualValues(t, c.GetInt64("foo", 5), 5)
	assert.EqualValues(t, c.GetFloat64("foo", 5.0), 5)
	assert.EqualValues(t, c.GetString("foo", "test"), "test")
	assert.EqualValues(t, c.IsFeatureEnabled("foo", true), true)
}

func TestClientWithDummy(t *testing.T) {
	client := NewTestClient().
		SetFloat64("foo", 0.9).
		SetFloat64("bar", 0.1)
	assert.EqualValues(t, client.GetFloat64("foo", 0.0), 0.9)
	assert.EqualValues(t, client.GetFloat64("bar", 0.2), 0.1)
	assert.EqualValues(t, client.GetFloat64("foobar", 0.5), 0.5)

	client.
		SetFloat64("foobar", 0.6).
		SetProjectsWhitelist("blah", 1, 2)

	assert.EqualValues(t, client.GetFloat64("foobar", 0.5), 0.6)
	assert.True(t, client.IsProjectWhitelisted("blah", 1, false))
	assert.True(t, client.IsProjectWhitelisted("blah", 2, false))
}
