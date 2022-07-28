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

	const ConfigFile = "ut_resolve_1.yaml"
	const InitialContent = "field1: toto"
	const ChangedContent = "field1: yoyo"

	os.WriteFile(ConfigFile, []byte(InitialContent), 0777)
	defer os.Remove(ConfigFile)

	// Wait to ignore file creation fs event
	time.Sleep(50 * time.Millisecond)

	var config Config

	callbackCallCount := 0
	err := NewResolver[Config]().
		SetConfigFile(ConfigFile).
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
	os.WriteFile(ConfigFile, []byte(InitialContent), 0777)
	time.Sleep(50 * time.Millisecond)

	assert.Equal(t, 0, callbackCallCount)

	// Changes done, callbacks must be called
	os.WriteFile(ConfigFile, []byte(ChangedContent), 0777)
	time.Sleep(50 * time.Millisecond)

	assert.Equal(t, 1, callbackCallCount)
}
