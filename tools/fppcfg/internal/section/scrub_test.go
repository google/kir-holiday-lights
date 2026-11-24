// Copyright 2026 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package section

import (
	"errors"
	"testing"
)

func TestScrub(t *testing.T) {
	const secret = "pa\"ss\\word<>"
	for _, msg := range []string{
		`raw pa"ss\word<> here`,
		`quoted "pa\"ss\\word<>"`,
		`json {"PSK":"pa\"ss\\word<>"}`,
	} {
		got := scrub(errors.New(msg), "", secret).Error()
		if got == msg {
			t.Errorf("not scrubbed: %s", msg)
		}
	}
	plain := errors.New("nothing secret")
	if scrub(plain, secret) != plain {
		t.Error("an error without a secret was replaced")
	}
}
