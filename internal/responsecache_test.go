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
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/bartventer/httpcache/internal/testutil"
)

func Test_responseCache_Get(t *testing.T) {
	type fields struct {
		cache Cache
	}
	type args struct {
		key string
		req *http.Request
	}
	tests := []struct {
		name      string
		fields    fields
		args      args
		want      *Response
		assertion func(tt *testing.T, err error, i ...interface{}) bool
	}{
		{
			name: "cache hit",
			fields: fields{
				cache: &MockCache{
					GetFunc: func(key string) ([]byte, error) {
						resp := &Response{
							Data:        httptest.NewRecorder().Result(),
							RequestedAt: time.Date(2023, 10, 1, 0, 0, 0, 0, time.UTC),
							ReceivedAt:  time.Date(2023, 10, 1, 0, 0, 1, 0, time.UTC),
							ID:          "test-key",
						}
						return resp.MarshalBinary()
					},
				},
			},
			args: args{
				key: "test-key",
				req: &http.Request{},
			},
			want: &Response{
				Data: &http.Response{
					StatusCode: http.StatusOK,
					Header:     http.Header{"Content-Length": []string{"0"}},
					Body:       http.NoBody,
				},
				RequestedAt: time.Date(2023, 10, 1, 0, 0, 0, 0, time.UTC),
				ReceivedAt:  time.Date(2023, 10, 1, 0, 0, 1, 0, time.UTC),
				ID:          "test-key",
			},
			assertion: func(tt *testing.T, err error, i ...interface{}) bool {
				testutil.RequireNoError(tt, err)
				return true
			},
		},
		{
			name: "cache miss",
			fields: fields{
				cache: &MockCache{
					GetFunc: func(key string) ([]byte, error) {
						return nil, testutil.ErrSample
					},
				},
			},
			args: args{
				key: "test-key",
				req: &http.Request{},
			},
			want: nil,
			assertion: func(tt *testing.T, err error, i ...interface{}) bool {
				testutil.RequireErrorIs(tt, err, testutil.ErrSample)
				return true
			},
		},
		{
			name: "Unmarshal error",
			fields: fields{
				cache: &MockCache{
					GetFunc: func(key string) ([]byte, error) {
						return []byte("invalid data"), nil
					},
				},
			},
			args: args{
				key: "test-key",
				req: &http.Request{},
			},
			want: nil,
			assertion: func(tt *testing.T, err error, i ...interface{}) bool {
				testutil.RequireError(tt, err)
				testutil.AssertTrue(
					t,
					strings.Contains(err.Error(), "failed to unmarshal cached entry"),
				)
				return true
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &responseCache{
				cache: tt.fields.cache,
			}
			got, err := r.Get(tt.args.key, tt.args.req)
			tt.assertion(t, err)
			if tt.want != nil && got != nil {
				testutil.AssertEqual(t, tt.want.Data.StatusCode, got.Data.StatusCode)
				testutil.AssertTrue(
					t,
					tt.want.RequestedAt.Equal(got.RequestedAt),
					"ReqTime mismatch",
				)
				testutil.AssertTrue(
					t,
					tt.want.ReceivedAt.Equal(got.ReceivedAt),
					"RespTime mismatch",
				)
			} else {
				testutil.AssertNil(t, got)
			}
		})
	}
}

func Test_responseCache_Set(t *testing.T) {
	type fields struct {
		cache Cache
	}
	type args struct {
		key   string
		entry *Response
	}
	tests := []struct {
		name      string
		fields    fields
		args      args
		assertion func(tt *testing.T, err error, i ...interface{}) bool
	}{
		{
			name: "successful set",
			fields: fields{
				cache: &MockCache{
					SetFunc: func(key string, entry []byte) error {
						return nil
					},
				},
			},
			args: args{
				key: "test-key",
				entry: &Response{
					Data: &http.Response{
						StatusCode: http.StatusOK,
						Header:     http.Header{"Content-Length": []string{"0"}},
						Body:       http.NoBody,
					},
					RequestedAt: time.Date(2023, 10, 1, 0, 0, 0, 0, time.UTC),
					ReceivedAt:  time.Date(2023, 10, 1, 0, 0, 1, 0, time.UTC),
				},
			},
			assertion: func(tt *testing.T, err error, i ...interface{}) bool {
				testutil.RequireNoError(tt, err)
				return true
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &responseCache{
				cache: tt.fields.cache,
			}
			tt.assertion(t, r.Set(tt.args.key, tt.args.entry))
		})
	}
}

func Test_responseCache_Delete(t *testing.T) {
	type fields struct {
		cache Cache
	}
	type args struct {
		key string
	}
	tests := []struct {
		name      string
		fields    fields
		args      args
		assertion func(tt *testing.T, err error, i ...interface{}) bool
	}{
		{
			name: "successful delete",
			fields: fields{
				cache: &MockCache{
					DeleteFunc: func(key string) error {
						return nil
					},
				},
			},
			args: args{
				key: "test-key",
			},
			assertion: func(tt *testing.T, err error, i ...interface{}) bool {
				testutil.RequireNoError(tt, err)
				return true
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &responseCache{
				cache: tt.fields.cache,
			}
			tt.assertion(t, r.Delete(tt.args.key))
		})
	}
}

func Test_responseCache_GetHeaders(t *testing.T) {
	type fields struct {
		cache Cache
	}
	type args struct {
		key string
	}
	tests := []struct {
		name      string
		fields    fields
		args      args
		assertion func(tt *testing.T, got ResponseRefs, err error, i ...interface{}) bool
	}{
		{
			name: "successful get headers",
			fields: fields{
				cache: &MockCache{
					GetFunc: func(key string) ([]byte, error) {
						data := `[{"vary":"Accept","vary_resolved":{"Accept":"application/json"},"id":"https://example.com/test#1234567890","received_at":"2023-10-01T00:00:00Z","etag":"W/\"1234567890\"","last_modified":"2023-10-01T00:00:00Z","deleted":false}]`
						return []byte(data), nil
					},
				},
			},
			args: args{
				key: "test-key",
			},
			assertion: func(tt *testing.T, got ResponseRefs, err error, i ...interface{}) bool {
				testutil.RequireNoError(tt, err)
				testutil.AssertNotNil(tt, got)
				testutil.AssertTrue(tt, len(got) > 0, "Expected non-empty headers")
				testutil.AssertEqual(tt, got[0].Vary, "Accept")
				testutil.AssertEqual(tt, got[0].VaryResolved["Accept"], "application/json")
				testutil.AssertEqual(tt, got[0].ResponseID, "https://example.com/test#1234567890")
				testutil.AssertTrue(
					tt,
					got[0].ReceivedAt.Equal(time.Date(2023, 10, 1, 0, 0, 0, 0, time.UTC)),
					"Timestamp mismatch",
				)
				return true
			},
		},
		{
			name: "cache miss",
			fields: fields{
				cache: &MockCache{
					GetFunc: func(key string) ([]byte, error) {
						return nil, testutil.ErrSample
					},
				},
			},
			args: args{
				key: "test-key",
			},
			assertion: func(tt *testing.T, got ResponseRefs, err error, i ...interface{}) bool {
				testutil.RequireErrorIs(tt, err, testutil.ErrSample)
				testutil.AssertNil(tt, got)
				return true
			},
		},
		{
			name: "unmarshal error",
			fields: fields{
				cache: &MockCache{
					GetFunc: func(key string) ([]byte, error) {
						return []byte("invalid data"), nil
					},
				},
			},
			args: args{
				key: "test-key",
			},
			assertion: func(tt *testing.T, got ResponseRefs, err error, i ...interface{}) bool {
				var syntaxErr *json.SyntaxError
				testutil.RequireErrorAs(tt, err, &syntaxErr)
				testutil.AssertNil(tt, got)
				return true
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &responseCache{
				cache: tt.fields.cache,
			}
			got, err := r.GetRefs(tt.args.key)
			tt.assertion(t, got, err)
		})
	}
}

func Test_responseCache_SetHeaders(t *testing.T) {
	type fields struct {
		cache Cache
	}
	type args struct {
		key     string
		headers ResponseRefs
	}
	tests := []struct {
		name      string
		fields    fields
		args      args
		assertion func(tt *testing.T, err error, i ...interface{}) bool
	}{
		{
			name: "successful set headers",
			fields: fields{
				cache: &MockCache{
					SetFunc: func(key string, entry []byte) error {
						return nil
					},
				},
			},
			args: args{
				key: "test-key",
				headers: ResponseRefs{
					{
						Vary:         "Accept",
						VaryResolved: map[string]string{"Accept": "application/json"},
						ReceivedAt:   time.Date(2023, 10, 1, 0, 0, 0, 0, time.UTC),
						ResponseID:   "https://example.com/test#1234567890",
					},
				},
			},
			assertion: func(tt *testing.T, err error, i ...interface{}) bool {
				testutil.RequireNoError(tt, err)
				return true
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &responseCache{
				cache: tt.fields.cache,
			}
			err := r.SetRefs(tt.args.key, tt.args.headers)
			tt.assertion(t, err)
		})
	}
}

func TestNewResponseCache(t *testing.T) {
	type args struct {
		cache Cache
	}
	tests := []struct {
		name string
		args args
		want *responseCache
	}{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NewResponseCache(tt.args.cache); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("NewResponseCache() = %v, want %v", got, tt.want)
			}
		})
	}
}
