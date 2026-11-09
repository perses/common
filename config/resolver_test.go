package config

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type foo struct {
	FieldToSet string
}

func (f *foo) Verify() error {
	if len(f.FieldToSet) == 0 {
		f.FieldToSet = "set"
	}
	return nil
}

type myConfig struct {
	Foo foo
}

func TestValidatorImpl_VerifyShouldSetDefaultValue(t *testing.T) {
	mc := &myConfig{}
	v := &validatorImpl{
		config: mc,
	}
	_ = v.Verify()
	assert.Equal(t, "set", mc.Foo.FieldToSet)
}

type requiredName struct {
	Name string `yaml:"name"`
	Port int    `yaml:"port"`
}

func (r *requiredName) Verify() error {
	if len(r.Name) == 0 {
		return errors.New("name cannot be empty")
	}
	if r.Port == 0 {
		r.Port = 8080
	}
	return nil
}

func TestValidatorImpl_VerifyCollections(t *testing.T) {
	type Config struct {
		Values   map[string]requiredName    `yaml:"values"`
		Pointers map[string]*requiredName   `yaml:"pointers"`
		Slices   map[string][]requiredName  `yaml:"slices"`
		Nested   map[string]map[string]*foo `yaml:"nested"`
		Array    [2]requiredName            `yaml:"array"`
		Any      map[string]any             `yaml:"any"`
	}
	validArray := [2]requiredName{{Name: "a"}, {Name: "b"}}

	t.Run("the values are verified", func(t *testing.T) {
		for name, c := range map[string]*Config{
			"map of structs":       {Array: validArray, Values: map[string]requiredName{"a": {}}},
			"map of pointers":      {Array: validArray, Pointers: map[string]*requiredName{"a": {}}},
			"map of slices":        {Array: validArray, Slices: map[string][]requiredName{"a": {{}}}},
			"element of the array": {Array: [2]requiredName{{Name: "a"}, {}}},
		} {
			assert.EqualError(t, (&validatorImpl{config: c}).Verify(), "name cannot be empty", name)
		}
	})

	t.Run("the default values are saved", func(t *testing.T) {
		c := &Config{
			Values:   map[string]requiredName{"a": {Name: "a"}},
			Pointers: map[string]*requiredName{"a": {Name: "a"}, "nil": nil},
			Slices:   map[string][]requiredName{"a": {{Name: "a"}}},
			Nested:   map[string]map[string]*foo{"a": {"b": {}}},
			Array:    validArray,
			Any:      map[string]any{"a": 1, "nil": nil},
		}
		require.NoError(t, (&validatorImpl{config: c}).Verify())
		assert.Equal(t, requiredName{Name: "a", Port: 8080}, c.Values["a"])
		assert.Equal(t, &requiredName{Name: "a", Port: 8080}, c.Pointers["a"])
		assert.Contains(t, c.Pointers, "nil")
		assert.Nil(t, c.Pointers["nil"])
		assert.Equal(t, []requiredName{{Name: "a", Port: 8080}}, c.Slices["a"])
		assert.Equal(t, "set", c.Nested["a"]["b"].FieldToSet)
		assert.Equal(t, [2]requiredName{{Name: "a", Port: 8080}, {Name: "b", Port: 8080}}, c.Array)
		assert.Equal(t, map[string]any{"a": 1, "nil": nil}, c.Any)
	})
}

func TestResolveImpl_VerifyMapValues(t *testing.T) {
	type Config struct {
		Providers map[string]requiredName `yaml:"providers"`
	}
	c := Config{}
	require.NoError(t, NewResolver[Config]().SetConfigData([]byte("providers:\n  a:\n    name: foo\n")).Resolve(&c).Verify())
	assert.Equal(t, map[string]requiredName{"a": {Name: "foo", Port: 8080}}, c.Providers)

	c = Config{}
	assert.EqualError(t, NewResolver[Config]().SetConfigData([]byte("providers:\n  a:\n    port: 80\n")).Resolve(&c).Verify(), "name cannot be empty")
}

func TestResolveImpl_EmptyEnvPrefix(t *testing.T) {
	type Config struct {
		FieldByEnv1 string `yaml:"fieldbyenv1"`
	}
	var config Config

	_ = os.Setenv("FIELDBYENV1", "value")

	err := NewResolver[Config]().
		SetConfigData([]byte("")).
		Resolve(&config).
		Verify()

	assert.NoError(t, err)
	assert.Equal(t, "value", config.FieldByEnv1)
}

// callbackRecorder records the configs received by a change callback.
// The callback is called by the goroutine watching the config file, so the access to the configs is synchronized.
type callbackRecorder[T any] struct {
	mutex   sync.Mutex
	configs []T
}

func (r *callbackRecorder[T]) record(config *T) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	r.configs = append(r.configs, *config)
}

func (r *callbackRecorder[T]) received() []T {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	return slices.Clone(r.configs)
}

const (
	waitFor = 5 * time.Second
	tick    = 10 * time.Millisecond
	// noCallbackDelay is the time to wait to check that the callbacks are not called.
	noCallbackDelay = 500 * time.Millisecond
)

func TestResolveImpl_WatchConfigShouldNotifyOnlyWhenValuesChange(t *testing.T) {
	type Config struct {
		Field1 string `yaml:"field1"`
	}

	configFile := filepath.Join(t.TempDir(), "ut_resolve_1.yaml")
	const initialContent = "field1: toto"
	const changedContent = "field1: yoyo"

	require.NoError(t, os.WriteFile(configFile, []byte(initialContent), 0600))

	var config Config
	recorder := &callbackRecorder[Config]{}
	err := NewResolver[Config]().
		SetConfigFile(configFile).
		AddChangeCallback(recorder.record).
		Resolve(&config).
		Verify()
	require.NoError(t, err)

	// No change, callbacks shouldn't be called
	require.NoError(t, os.WriteFile(configFile, []byte(initialContent), 0600))
	assert.Never(t, func() bool {
		return len(recorder.received()) > 0
	}, noCallbackDelay, tick)

	// Changes done, callbacks must be called
	require.NoError(t, os.WriteFile(configFile, []byte(changedContent), 0600))
	assert.EventuallyWithT(t, func(c *assert.CollectT) {
		assert.Equal(c, []Config{{Field1: "yoyo"}}, recorder.received())
	}, waitFor, tick)
	assert.Equal(t, "toto", config.Field1)
}

func TestResolveImpl_WatchSliceConfigShouldApplyChanges(t *testing.T) {
	type Config []int

	configFile := filepath.Join(t.TempDir(), "ut_resolve_2.yaml")
	const initialContent = "[0,1]"
	const changedContent = "[3,4,5]"

	require.NoError(t, os.WriteFile(configFile, []byte(initialContent), 0600))

	var config Config
	recorder := &callbackRecorder[Config]{}
	err := NewResolver[Config]().
		SetConfigFile(configFile).
		AddChangeCallback(recorder.record).
		Resolve(&config).
		Verify()
	require.NoError(t, err)

	// Only the elements coming from the file are checked:
	// when no environment prefix is set, lamenv appends an empty element to a slice.
	require.GreaterOrEqual(t, len(config), 2)
	assert.Equal(t, Config{0, 1}, config[:2])

	// Changes done, callbacks must be called
	require.NoError(t, os.WriteFile(configFile, []byte(changedContent), 0600))
	assert.EventuallyWithT(t, func(c *assert.CollectT) {
		received := recorder.received()
		require.Len(c, received, 1)
		require.GreaterOrEqual(c, len(received[0]), 3)
		assert.Equal(c, Config{3, 4, 5}, received[0][:3])
	}, waitFor, tick)
}
