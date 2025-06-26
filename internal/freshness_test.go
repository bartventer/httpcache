// Copyright (c) 2025 Bart Venter <bartventer@proton.me>
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//   http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package internal

import (
	"maps"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/bartventer/httpcache/internal/testutil"
)

func Test_heuristicFreshness(t *testing.T) {
	now := time.Now()
	lastMod := now.Add(-100 * time.Second)
	headerWithLastMod := http.Header{}
	headerWithLastMod.Set("Last-Modified", lastMod.UTC().Format(time.RFC850))

	headerNoLastMod := http.Header{}
	headerZeroLastMod := http.Header{}
	headerZeroLastMod.Set("Last-Modified", time.Time{}.Format(time.RFC850))

	headerFutureLastMod := http.Header{}
	headerFutureLastMod.Set("Last-Modified", now.Add(10*time.Second).Format(time.RFC850))

	tests := []struct {
		name string
		args struct {
			h    http.Header
			date time.Time
		}
		want time.Duration
	}{
		{
			name: "valid Last-Modified",
			args: struct {
				h    http.Header
				date time.Time
			}{headerWithLastMod, now},
			want: 10 * time.Second,
		},
		{
			name: "no Last-Modified",
			args: struct {
				h    http.Header
				date time.Time
			}{headerNoLastMod, now},
			want: 0,
		},
		{
			name: "Last-Modified after date",
			args: struct {
				h    http.Header
				date time.Time
			}{headerFutureLastMod, now},
			want: 0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testutil.AssertTrue(
				t,
				tt.want == heuristicFreshness(tt.args.h, tt.args.date).Round(time.Second),
			)
		})
	}
}

func Test_calculateCurrentAge(t *testing.T) {
	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	tests := []struct {
		name         string
		ageHeader    string
		date         time.Time
		requestTime  time.Time
		responseTime time.Time
		clockNow     time.Time
		wantValue    time.Duration
	}{
		{
			name:         "Valid Age header",
			ageHeader:    "10",
			date:         base,
			requestTime:  base.Add(10 * time.Second),
			responseTime: base.Add(15 * time.Second),
			clockNow:     base.Add(25 * time.Second),
			wantValue:    15*time.Second + 10*time.Second, // correctedInitialAge + residentTime = 15s + 10s = 25s
		},
		{
			name:         "No Age header",
			ageHeader:    "",
			date:         base,
			requestTime:  base.Add(10 * time.Second),
			responseTime: base.Add(15 * time.Second),
			clockNow:     base.Add(30 * time.Second),
			wantValue:    15*time.Second + 15*time.Second, // apparentAge=15s, responseDelay=5s, correctedInitialAge=15s, residentTime=15s
		},
		{
			name:         "Negative apparent age",
			ageHeader:    "",
			date:         base.Add(20 * time.Second),
			requestTime:  base.Add(10 * time.Second),
			responseTime: base.Add(15 * time.Second),
			clockNow:     base.Add(20 * time.Second),
			wantValue:    10 * time.Second, // apparentAge=0s, responseDelay=5s, correctedInitialAge=5s, residentTime=5s
		},
		{
			name:         "Resident time added",
			ageHeader:    "5",
			date:         base,
			requestTime:  base.Add(10 * time.Second),
			responseTime: base.Add(15 * time.Second),
			clockNow:     base.Add(40 * time.Second),
			wantValue:    15*time.Second + 25*time.Second, // correctedInitialAge=15s, residentTime=25s
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := http.Header{}
			if tt.ageHeader != "" {
				h.Set("Age", tt.ageHeader)
			}
			mockClock := MockClock{
				NowResult:   tt.clockNow,
				SinceResult: tt.clockNow.Sub(tt.responseTime),
			}
			got := calculateCurrentAge(
				&mockClock,
				h,
				tt.date,
				tt.requestTime,
				tt.responseTime,
			)
			if got.Value != tt.wantValue {
				t.Errorf("got.Value = %v, want %v", got.Value, tt.wantValue)
			}
			if !got.Timestamp.Equal(tt.clockNow) {
				t.Errorf("got.Timestamp = %v, want %v", got.Timestamp, tt.clockNow)
			}
		})
	}
}

func Test_freshnessCalculator_CalculateFreshness(t *testing.T) {
	var fakeResponse = func(date time.Time, header http.Header) *http.Response {
		r := httptest.NewRecorder().Result()
		r.Header.Set("Date", date.UTC().Format(http.TimeFormat))
		maps.Copy(r.Header, header)
		return r
	}

	base := time.Unix(0, 0).UTC()
	tests := []struct {
		name  string
		clock Clock
		entry *Response
		reqCC map[string]string
		resCC map[string]string
		want  *Freshness
	}{
		{
			name:  "Request with Max-Age=0",
			clock: &MockClock{NowResult: base.Add(30 * time.Second)},
			entry: &Response{
				Data:        fakeResponse(base.Add(10*time.Second), http.Header{}),
				ReceivedAt:  base.Add(10 * time.Second),
				RequestedAt: base.Add(5 * time.Second),
			},
			reqCC: map[string]string{"max-age": "0"},
			resCC: map[string]string{},
			want: &Freshness{
				IsStale:    true,
				Age:        &Age{Value: 0, Timestamp: base.Add(30 * time.Second)},
				UsefulLife: 0,
			},
		},
		{
			name: "Fresh response with Max-Age",
			clock: &MockClock{
				NowResult:   base.Add(30 * time.Second),
				SinceResult: time.Second * 20,
			},
			entry: &Response{
				Data:        fakeResponse(base.Add(10*time.Second), http.Header{}),
				ReceivedAt:  base.Add(10 * time.Second),
				RequestedAt: base.Add(10 * time.Second),
			},
			reqCC: map[string]string{},
			resCC: map[string]string{"max-age": "25"},
			want: &Freshness{
				IsStale:    false,
				Age:        &Age{Value: 20 * time.Second, Timestamp: base.Add(30 * time.Second)},
				UsefulLife: 25 * time.Second,
			},
		},
		{
			name: "Fresh response with Expires header",
			clock: &MockClock{
				NowResult:   base.Add(30 * time.Second),
				SinceResult: time.Second * 20,
			},
			entry: &Response{
				Data: fakeResponse(base.Add(10*time.Second), http.Header{
					"Expires": {base.Add(60 * time.Second).UTC().Format(http.TimeFormat)},
					"Date":    {""}, // Simulate missing Date header
				}),
				ReceivedAt:  base.Add(10 * time.Second),
				RequestedAt: base.Add(10 * time.Second),
			},
			reqCC: map[string]string{},
			resCC: map[string]string{},
			want: &Freshness{
				IsStale:    false,
				Age:        &Age{Value: 20 * time.Second, Timestamp: base.Add(30 * time.Second)},
				UsefulLife: 50 * time.Second, // Expires - Date = 60s - 10s
			},
		},
		{
			name: "Stale response with Request Max-Age",
			clock: &MockClock{
				NowResult:   base.Add(30 * time.Second),
				SinceResult: time.Second * 20,
			},
			entry: &Response{
				Data:        fakeResponse(base.Add(10*time.Second), http.Header{}),
				ReceivedAt:  base.Add(10 * time.Second),
				RequestedAt: base.Add(10 * time.Second),
			},
			reqCC: map[string]string{"max-age": "15"},
			resCC: map[string]string{"max-age": "25"},
			want: &Freshness{
				IsStale:    true, // Request prefers a response no older than 15 seconds
				Age:        &Age{Value: 20 * time.Second, Timestamp: base.Add(30 * time.Second)},
				UsefulLife: 15 * time.Second, // Request Max-Age overrides response Max-Age
			},
		},
		{
			name: "Stale response with expired Max-Age",
			clock: &MockClock{
				NowResult:   base.Add(60 * time.Second),
				SinceResult: time.Second * 50,
			},
			entry: &Response{
				Data:        fakeResponse(base.Add(10*time.Second), http.Header{}),
				ReceivedAt:  base.Add(10 * time.Second),
				RequestedAt: base.Add(10 * time.Second),
			},
			reqCC: map[string]string{},
			resCC: map[string]string{"max-age": "25"},
			want: &Freshness{
				IsStale:    true,
				Age:        &Age{Value: 50 * time.Second, Timestamp: base.Add(60 * time.Second)},
				UsefulLife: 25 * time.Second,
			},
		},
		{
			name: "Heuristic freshness applied",
			clock: &MockClock{
				NowResult:   base.Add(15 * time.Second),
				SinceResult: time.Second * 5,
			},
			entry: &Response{
				Data: fakeResponse(base.Add(10*time.Second), http.Header{
					"Last-Modified": {base.Add(-50 * time.Second).UTC().Format(time.RFC850)},
				}),
				ReceivedAt:  base.Add(10 * time.Second),
				RequestedAt: base.Add(10 * time.Second),
			},
			reqCC: map[string]string{},
			resCC: map[string]string{"public": ""},
			want: &Freshness{
				IsStale: false,
				Age: &Age{
					Value:     5 * time.Second, // 50s x 0.1 = 5s
					Timestamp: base.Add(15 * time.Second),
				},
				UsefulLife: 6 * time.Second,
			},
		},
		{
			name: "Stale with Min-Fresh directive",
			clock: &MockClock{
				NowResult:   base.Add(30 * time.Second),
				SinceResult: time.Second * 20,
			},
			entry: &Response{
				Data:        fakeResponse(base.Add(10*time.Second), http.Header{}),
				ReceivedAt:  base.Add(10 * time.Second),
				RequestedAt: base.Add(10 * time.Second),
			},
			reqCC: map[string]string{"min-fresh": "10"},
			resCC: map[string]string{"max-age": "25"},
			want: &Freshness{
				IsStale:    true, // Only been fresh for 10 seconds
				Age:        &Age{Value: 20 * time.Second, Timestamp: base.Add(30 * time.Second)},
				UsefulLife: 25 * time.Second,
			},
		},
		{
			name: "Fresh with Max-Stale directive (defined)",
			clock: &MockClock{
				NowResult:   base.Add(40 * time.Second),
				SinceResult: time.Second * 30,
			},
			entry: &Response{
				Data:        fakeResponse(base.Add(10*time.Second), http.Header{}),
				ReceivedAt:  base.Add(10 * time.Second),
				RequestedAt: base.Add(10 * time.Second),
			},
			reqCC: map[string]string{"max-stale": "20"},
			resCC: map[string]string{"max-age": "15"},
			want: &Freshness{
				IsStale:    false, // Stale but within max-stale allowance
				Age:        &Age{Value: 30 * time.Second, Timestamp: base.Add(40 * time.Second)},
				UsefulLife: 15 * time.Second,
			},
		},
		{
			name: "Fresh with Max-Stale directive (empty/max duration)",
			clock: &MockClock{
				NowResult:   base.Add(50 * time.Second),
				SinceResult: time.Second * 40,
			},
			entry: &Response{
				Data:        fakeResponse(base.Add(10*time.Second), http.Header{}),
				ReceivedAt:  base.Add(10 * time.Second),
				RequestedAt: base.Add(10 * time.Second),
			},
			reqCC: map[string]string{"max-stale": ""},
			resCC: map[string]string{"max-age": "15"},
			want: &Freshness{
				IsStale:    false, // Technically stale but allowed by max-stale
				Age:        &Age{Value: 40 * time.Second, Timestamp: base.Add(50 * time.Second)},
				UsefulLife: 15 * time.Second,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &freshnessCalculator{clock: tt.clock}
			FixDateHeader(tt.entry.Data.Header, tt.entry.ReceivedAt)
			got := f.CalculateFreshness(
				tt.entry,
				CCRequestDirectives(tt.reqCC),
				CCResponseDirectives(tt.resCC),
			)
			testutil.AssertTrue(t, tt.want.IsStale == got.IsStale, "IsStale mismatch")
			testutil.AssertEqual(t, tt.want.Age.Value, got.Age.Value, "Age.Value mismatch")
			testutil.AssertEqual(t, tt.want.UsefulLife, got.UsefulLife, "Lifetime mismatch")
			testutil.AssertTrue(
				t,
				tt.want.Age.Timestamp.Equal(got.Age.Timestamp),
				"Age.Timestamp mismatch",
			)
		})
	}
}
