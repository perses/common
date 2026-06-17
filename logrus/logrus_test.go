// Copyright The Perses Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package logrus

import (
	"flag"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

func TestBuild(t *testing.T) {
	InitFlag()
	infoLevel := logrus.InfoLevel
	debugLevel := logrus.DebugLevel
	testCase := []struct {
		name            string
		flagLevel       string
		flagFormat      string
		flagMethodTrace string
		result          *Builder
	}{
		{
			name: "default",
			result: &Builder{
				level:       &infoLevel,
				format:      txtFormat,
				methodTrace: false,
			},
		},
		{
			name:            "set flag",
			flagLevel:       "debug",
			flagFormat:      "json",
			flagMethodTrace: "true",
			result: &Builder{
				level:       &debugLevel,
				format:      jsonFormat,
				methodTrace: true,
			},
		},
	}
	for _, test := range testCase {
		t.Run(test.name, func(t *testing.T) {
			flag.Parse()
			if len(test.flagLevel) > 0 {
				assert.NoError(t, flag.Set("log.level", test.flagLevel))
			}
			if len(test.flagFormat) > 0 {
				assert.NoError(t, flag.Set("log.format", test.flagFormat))
			}
			if len(test.flagMethodTrace) > 0 {
				assert.NoError(t, flag.Set("log.method-trace", test.flagMethodTrace))
			}
			b := NewBuilder()
			b.build()
			assert.Equal(t, test.result, b)
		})
	}
}
