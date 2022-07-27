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

type resolveWatchingConfig struct {
	Test string `yaml:"test"`
}

func TestResolveImpl_VerifyWatching(t *testing.T) {
	const filename = "test-resolve.yml"
	const initFileContent = `test: toto`
	const updateFileContent = `test: tata`

	os.WriteFile(filename, []byte(initFileContent), 0777)

	c := &resolveWatchingConfig{}
	err := NewResolver().
		SetConfigFile(filename, true).
		Resolve(&c).
		Verify()

	if err != nil {
		t.Error(err)
		return
	}

	os.WriteFile(filename, []byte(updateFileContent), 0777)

	time.Sleep(100 * time.Millisecond)

	assert.Equal(t, "tata", c.Test)
}
