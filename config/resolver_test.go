package config

import (
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
	time.Sleep(noCallbackDelay)
	assert.Empty(t, recorder.received())

	// Changes done, callbacks must be called
	require.NoError(t, os.WriteFile(configFile, []byte(changedContent), 0600))
	assert.Eventually(t, func() bool {
		return len(recorder.received()) == 1
	}, waitFor, tick)
	assert.Equal(t, "toto", config.Field1)
	assert.Equal(t, []Config{{Field1: "yoyo"}}, recorder.received())
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

	// Changes done, callbacks must be called
	require.NoError(t, os.WriteFile(configFile, []byte(changedContent), 0600))
	require.Eventually(t, func() bool {
		return len(recorder.received()) == 1
	}, waitFor, tick)

	assert.Equal(t, 0, config[0])
	assert.Equal(t, 1, config[1])

	updatedConfig := recorder.received()[0]
	assert.Equal(t, 3, updatedConfig[0])
	assert.Equal(t, 4, updatedConfig[1])
	assert.Equal(t, 5, updatedConfig[2])
}
