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

package fake

import (
	"context"
	"strings"
	"testing"
)

func TestAPIRepliesInOrder(t *testing.T) {
	f := NewAPI(t).Reply("GET", "/api/fppd/status", 503, "").Reply("GET", "/api/fppd/status", 200, "{}")
	for _, want := range []int{503, 200, 200} {
		r, _ := f.Do(context.Background(), "GET", "/api/fppd/status", nil, "")
		if r.Status != want {
			t.Errorf("status %d, want %d", r.Status, want)
		}
	}
}

func TestAPIUnexpected(t *testing.T) {
	f := NewAPI(t).AllowUnexpected()
	r, _ := f.Do(context.Background(), "PUT", "/api/settings/x", strings.NewReader("v"), "text/plain")
	if r.Status != 404 || len(f.Unexpected()) != 1 || f.Writes()[0].Body != "v" {
		t.Errorf("got %d, unexpected %v", r.Status, f.Unexpected())
	}
}

func TestRecordedNames(t *testing.T) {
	f := NewAPI(t).Recorded("fpp10.4", "/index.php", "/api/configfile/authorized_keys", "/api/system/info")
	r, _ := f.Do(context.Background(), "GET", "/index.php", nil, "")
	if !strings.Contains(string(r.Body), "<html") {
		t.Error("index.php is not HTML")
	}
}
