/*
Copyright 2022 Flant JSC

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

package generate_password

import (
	"testing"
)

func Test_extractPassword(t *testing.T) {
	tests := []struct {
		name   string
		in     string
		expect string
		err    bool
	}{
		{
			"password",
			"admin:{PLAIN}pass",
			"pass",
			false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := NewBasicAuthPlainHook("testMod", "default", "auth")
			pass, err := h.extractPasswordFromBasicAuth(tt.in)
			if tt.err && err == nil {
				t.Fatalf("expect error for input '%s'", tt.in)
			}
			if !tt.err && pass != tt.expect {
				t.Fatalf("expect password in '%s', got '%s'", tt.expect, pass)
			}
		})
	}
}
