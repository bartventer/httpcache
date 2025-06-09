package internal

import (
	"bytes"
	"encoding/json"
	"net/http"
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
		want      *ResponseEntry
		assertion func(tt *testing.T, err error, i ...interface{}) bool
	}{
		{
			name: "cache hit",
			fields: fields{
				cache: &MockCache{
					GetFunc: func(key string) ([]byte, error) {
						reqTime := time.Date(2023, 10, 1, 0, 0, 0, 0, time.UTC)
						respTime := time.Date(2023, 10, 1, 0, 0, 1, 0, time.UTC)
						reqTimeBytes, _ := reqTime.MarshalBinary()
						respTimeBytes, _ := respTime.MarshalBinary()
						respBytes := []byte("HTTP/1.1 200 OK\r\nContent-Length: 0\r\n\r\n")
						var buf bytes.Buffer
						buf.Write(reqTimeBytes)
						buf.WriteByte('\n')
						buf.Write(respTimeBytes)
						buf.WriteByte('\n')
						buf.Write(respBytes)
						return buf.Bytes(), nil
					},
				},
			},
			args: args{
				key: "test-key",
				req: &http.Request{},
			},
			want: &ResponseEntry{
				Response: &http.Response{
					StatusCode: http.StatusOK,
					Header:     http.Header{"Content-Length": []string{"0"}},
					Body:       http.NoBody,
				},
				ReqTime:  time.Date(2023, 10, 1, 0, 0, 0, 0, time.UTC),
				RespTime: time.Date(2023, 10, 1, 0, 0, 1, 0, time.UTC),
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
				testutil.AssertEqual(t, tt.want.Response.StatusCode, got.Response.StatusCode)
				testutil.AssertTrue(t, tt.want.ReqTime.Equal(got.ReqTime), "ReqTime mismatch")
				testutil.AssertTrue(t, tt.want.RespTime.Equal(got.RespTime), "RespTime mismatch")
				testutil.AssertEqual(
					t,
					tt.want.Response.Header.Get("Content-Length"),
					got.Response.Header.Get("Content-Length"),
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
		entry *ResponseEntry
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
				entry: &ResponseEntry{
					Response: &http.Response{
						StatusCode: http.StatusOK,
						Header:     http.Header{"Content-Length": []string{"0"}},
						Body:       http.NoBody,
					},
					ReqTime:  time.Date(2023, 10, 1, 0, 0, 0, 0, time.UTC),
					RespTime: time.Date(2023, 10, 1, 0, 0, 1, 0, time.UTC),
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
		assertion func(tt *testing.T, got VaryHeaderEntries, err error, i ...interface{}) bool
	}{
		{
			name: "successful get headers",
			fields: fields{
				cache: &MockCache{
					GetFunc: func(key string) ([]byte, error) {
						data := `[{"vary":"Accept","vary_resolved":{"Accept":"application/json"},"response_id":"https://example.com/test#1234567890","timestamp":"2023-10-01T00:00:00Z"}]`
						return []byte(data), nil
					},
				},
			},
			args: args{
				key: "test-key",
			},
			assertion: func(tt *testing.T, got VaryHeaderEntries, err error, i ...interface{}) bool {
				testutil.RequireNoError(tt, err)
				testutil.AssertNotNil(tt, got)
				testutil.AssertTrue(tt, len(got) > 0, "Expected non-empty headers")
				testutil.AssertEqual(tt, got[0].Vary, "Accept")
				testutil.AssertEqual(tt, got[0].VaryResolved["Accept"], "application/json")
				testutil.AssertEqual(tt, got[0].ResponseID, "https://example.com/test#1234567890")
				testutil.AssertTrue(
					tt,
					got[0].Timestamp.Equal(time.Date(2023, 10, 1, 0, 0, 0, 0, time.UTC)),
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
			assertion: func(tt *testing.T, got VaryHeaderEntries, err error, i ...interface{}) bool {
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
			assertion: func(tt *testing.T, got VaryHeaderEntries, err error, i ...interface{}) bool {
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
			got, err := r.GetHeaders(tt.args.key)
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
		headers VaryHeaderEntries
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
				headers: VaryHeaderEntries{
					{
						Vary:         "Accept",
						VaryResolved: map[string]string{"Accept": "application/json"},
						ResponseID:   "https://example.com/test#1234567890",
						Timestamp:    time.Date(2023, 10, 1, 0, 0, 0, 0, time.UTC),
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
			err := r.SetHeaders(tt.args.key, tt.args.headers)
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
