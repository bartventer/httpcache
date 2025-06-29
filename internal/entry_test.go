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
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/bartventer/httpcache/internal/testutil"
)

func TestResponse_MarshalBinary(t *testing.T) {
	base := time.Unix(0, 0).UTC()
	type fields struct {
		ID          string
		Data        *http.Response
		RequestedAt time.Time
		ReceivedAt  time.Time
	}
	tests := []struct {
		name      string
		fields    fields
		assertion func(tt *testing.T, got []byte, err error)
	}{
		{
			name: "valid response",
			fields: fields{
				ID:          "testid",
				Data:        httptest.NewRecorder().Result(),
				RequestedAt: base,
				ReceivedAt:  base.Add(2 * time.Second),
			},
			assertion: func(tt *testing.T, got []byte, err error) {
				testutil.RequireNoError(tt, err)

				gotResp, err := ParseResponse(got, &http.Request{Method: http.MethodGet})
				testutil.RequireNoError(tt, err)
				testutil.AssertEqual(tt, gotResp.ID, "testid")
				testutil.AssertTrue(tt, gotResp.RequestedAt.Equal(base))
				testutil.AssertTrue(tt, gotResp.ReceivedAt.Equal(base.Add(2*time.Second)))
				testutil.AssertNotNil(tt, gotResp.Data)
				testutil.AssertEqual(tt, gotResp.Data.StatusCode, http.StatusOK)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := Response{
				ID:          tt.fields.ID,
				Data:        tt.fields.Data,
				RequestedAt: tt.fields.RequestedAt,
				ReceivedAt:  tt.fields.ReceivedAt,
			}
			got, err := r.MarshalBinary()
			tt.assertion(t, got, err)
		})
	}
}

func TestParseResponse(t *testing.T) {
	base := time.Unix(0, 0).UTC()
	type args struct {
		data []byte
		req  *http.Request
	}
	tests := []struct {
		name      string
		setup     func(*testing.T) args
		assertion func(tt *testing.T, got *Response, err error)
	}{
		{
			name: "invalid meta line",
			setup: func(*testing.T) args {
				return args{
					data: []byte("invalid meta line"),
					req:  &http.Request{Method: http.MethodGet},
				}
			},
			assertion: func(tt *testing.T, got *Response, err error) {
				testutil.RequireErrorIs(tt, err, errReadBytes)
			},
		},
		{
			name: "invalid meta line format",
			setup: func(*testing.T) args {
				return args{
					data: []byte("invalid\nHTTP/1.1 200 OK\r\nContent-Length: 0\r\n\r\n"),
					req:  &http.Request{Method: http.MethodGet},
				}
			},
			assertion: func(tt *testing.T, got *Response, err error) {
				testutil.RequireErrorIs(tt, err, errInvalidMetaLine)
			},
		},
		{
			name: "invalid response",
			setup: func(*testing.T) args {
				var buf strings.Builder
				tmp := &Response{
					ID:          "testid",
					RequestedAt: base,
					ReceivedAt:  base.Add(2 * time.Second),
				}
				_, _ = tmp.WriteTo(&buf)
				buf.WriteString("not a valid http response")
				return args{
					data: []byte(buf.String()),
					req:  &http.Request{Method: http.MethodGet},
				}
			},
			assertion: func(tt *testing.T, got *Response, err error) {
				testutil.RequireErrorIs(tt, err, errInvalidResponse)
			},
		},
		{
			name: "valid response",
			setup: func(*testing.T) args {
				var buf strings.Builder
				tmp := &Response{
					ID:          "testid",
					RequestedAt: base,
					ReceivedAt:  base.Add(2 * time.Second),
					Data:        httptest.NewRecorder().Result(),
				}
				_, _ = tmp.WriteTo(&buf)
				buf.WriteString("HTTP/1.1 200 OK\r\nContent-Length: 0\r\n\r\n")
				return args{
					data: []byte(buf.String()),
					req:  &http.Request{Method: http.MethodGet},
				}
			},
			assertion: func(tt *testing.T, got *Response, err error) {
				testutil.RequireNoError(tt, err)
				testutil.AssertEqual(tt, got.ID, "testid")
				testutil.AssertTrue(tt, got.RequestedAt.Equal(base))
				testutil.AssertTrue(tt, got.ReceivedAt.Equal(base.Add(2*time.Second)))
				testutil.AssertNotNil(tt, got.Data)
				testutil.AssertEqual(tt, got.Data.StatusCode, http.StatusOK)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := tt.setup(t)
			got, err := ParseResponse(args.data, args.req)
			tt.assertion(t, got, err)
		})
	}
}

func TestParseHTTPDateCompat(t *testing.T) {
	r := Response{
		Data: &http.Response{
			Header: http.Header{
				"Expires": []string{"Mon, 02 Jan 2006 15:04:05 UTC"},
			},
		},
	}
	t.Run("Not Enabled", func(t *testing.T) {
		_, err := parseHTTPDateCompat(r.Data.Header.Get("Expires"))
		testutil.RequireNoError(t, err)

		_, _, valid := r.ExpiresHeader()
		testutil.AssertTrue(t, !valid)
	})

	t.Run("Enabled", func(t *testing.T) {
		t.Setenv("HTTPCACHE_ALLOW_UTC_DATETIMEFORMAT", "1")
		_, err := parseHTTPDateCompat(r.Data.Header.Get("Expires"))
		testutil.RequireNoError(t, err)

		_, found, valid := r.ExpiresHeader()
		testutil.AssertTrue(t, found && valid)
	})
}
