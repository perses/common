package config

import "testing"

func TestEtcdConfig_Verify(t *testing.T) {
	config := &EtcdConfig{
		Connections: []Connection{
			{
				Host: "test",
				Port: 0,
			},
		},
		Protocol:              "test",
		User:                  "",
		Password:              "",
		RequestTimeoutSeconds: 0,
	}
	test := validatorImpl{
		config: config,
	}
	if err := test.Verify(); err == nil {
		t.Fatal("err cannot be nil since 'test' is not a known protocol")
	}
}
