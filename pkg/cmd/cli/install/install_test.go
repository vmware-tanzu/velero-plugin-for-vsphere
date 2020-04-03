/*
Copyright 2020 the Velero contributors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package install

import (
	"gotest.tools/assert"
	"testing"
)

type testPattern struct {
	currentVersion, minVersion string
	expected int
}

const verEquals = 0
const currentGreater = 1
const currentLess = -1
func TestCompareVersion(t *testing.T) {
	t.Log("TestCompareVersion called")

	var patterns = []testPattern {
		testPattern {
			currentVersion:"v1.0.0", minVersion:"v1.0.0", expected: verEquals,
		},
		testPattern {
			currentVersion:"v0.1.1", minVersion:"v0.1", expected: currentGreater,
		},
		testPattern {
			currentVersion:"", minVersion:"v0.3.3", expected: currentLess,
		},
		testPattern {
			currentVersion:"v0.2.0", minVersion:"v1.2.0", expected: currentLess,
		},
		testPattern {
			currentVersion: "v0.1.9", minVersion:"v0.1.0", expected: currentGreater,
		},
		testPattern {
			currentVersion: "v1.0", minVersion: "v1.1.1", expected: currentLess,
		},
		testPattern {
			currentVersion: "v1.1", minVersion: "v1.1.1", expected: currentLess,
		},
	}

	for patternNum, curPattern := range patterns {
		assert.Equal(t, CompareVersion(curPattern.currentVersion, curPattern.minVersion), curPattern.expected,
			"Pattern # %d failed, currentVersion = %s and inVersion = %s did not return %d",
			patternNum, curPattern.currentVersion, curPattern.minVersion, curPattern.expected)
	}

}