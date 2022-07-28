package config

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
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

func TestResolveImpl_WatchConfigShouldCallCallbackOnlyOnConfigurationContentChange(t *testing.T) {
	type Config struct {
		Field1 string `yaml:"field1"`
	}

	const configFile = "ut_resolve_1.yaml"
	const initialContent = "field1: toto"
	const changedContent = "field1: yoyo"

	err := os.WriteFile(configFile, []byte(initialContent), 0777)
	if err != nil {
		t.Error(err)
		return
	}
	defer os.Remove(configFile)

	var config Config

	callbackCallCount := 0
	err = NewResolver[Config]().
		SetConfigFile(configFile).
		AddChangeCallback(func(newConfig *Config) {
			callbackCallCount++
		}).
		Resolve(&config).
		Verify()

	if err != nil {
		t.Error(err)
		return
	}

	// No change, callbacks shouldnt be called
	err = os.WriteFile(configFile, []byte(initialContent), 0777)
	if err != nil {
		t.Error(err)
		return
	}
	time.Sleep(50 * time.Millisecond)

	assert.Equal(t, 0, callbackCallCount)

	// Changes done, callbacks must be called
	err = os.WriteFile(configFile, []byte(changedContent), 0777)
	if err != nil {
		t.Error(err)
		return
	}
	time.Sleep(50 * time.Millisecond)

	assert.Equal(t, 1, callbackCallCount)
	assert.Equal(t, "yoyo", config.Field1)
}

func TestResolveImpl_WatchSliceConfigShouldApplyChanges(t *testing.T) {
	type Config []int

	const configFile = "ut_resolve_2.yaml"
	const initialContent = "[0,1]"
	const changedContent = "[3,4,5]"

	err := os.WriteFile(configFile, []byte(initialContent), 0777)
	if err != nil {
		t.Error(err)
		return
	}
	defer os.Remove(configFile)

	var config Config

	callbackCallCount := 0
	err = NewResolver[Config]().
		SetConfigFile(configFile).
		AddChangeCallback(func(newConfig *Config) {
			callbackCallCount++
		}).
		Resolve(&config).
		Verify()

	if err != nil {
		t.Error(err)
		return
	}

	// Changes done, callbacks must be called
	err = os.WriteFile(configFile, []byte(changedContent), 0777)
	if err != nil {
		t.Error(err)
		return
	}
	time.Sleep(50 * time.Millisecond)

	assert.Equal(t, 1, callbackCallCount)
	assert.Equal(t, 3, config[0])
	assert.Equal(t, 4, config[1])
	assert.Equal(t, 5, config[2])
}
