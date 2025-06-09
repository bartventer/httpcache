package internal

import (
	"iter"
	"maps"
	"net/http"
	"testing"
	"time"

	"github.com/bartventer/httpcache/internal/testutil"
)

type mockVaryKeyer struct{}

func (m mockVaryKeyer) VaryKey(urlKey string, varyHeaders map[string]string) string {
	return urlKey + "#mock"
}

func Test_responseStorer_StoreResponse(t *testing.T) {
	base := time.Unix(0, 0).UTC()
	type fields struct {
		cache ResponseCache
		vhn   VaryHeaderNormalizer
		vk    VaryKeyer
	}
	type args struct {
		resp              *http.Response
		key               string
		headers           VaryHeaderEntries
		reqTime, respTime time.Time
	}
	tests := []struct {
		name      string
		fields    fields
		args      args
		assertion func(*testing.T, *http.Response, error)
	}{
		{
			name: "Successful store with nil headers",
			fields: fields{
				cache: &MockResponseCache{
					SetFunc: func(key string, entry *ResponseEntry) error {
						testutil.AssertEqual(t, "test-key#mock", key)
						testutil.AssertNotNil(t, entry)
						return nil
					},
					SetHeadersFunc: func(key string, headers VaryHeaderEntries) error {
						testutil.AssertEqual(t, "test-key", key)
						testutil.AssertTrue(t, len(headers) == 1)
						return nil
					},
				},
				vhn: VaryHeaderNormalizerFunc(
					func(vary string, reqHeader http.Header) iter.Seq2[string, string] {
						return maps.All(map[string]string{
							"Accept": reqHeader.Get("Accept"),
						})
					},
				),
				vk: mockVaryKeyer{},
			},
			args: args{
				resp: &http.Response{
					Header: http.Header{
						"Connection": {"keep-alive"}, // Should be removed
						"Vary":       {"Accept"},
						"Date":       {base.Add(1 * time.Hour).Format(http.TimeFormat)},
					},
					Request: &http.Request{
						Header: http.Header{"Accept": []string{"text/html"}},
					},
				},
				key:      "test-key",
				headers:  nil,
				reqTime:  base.Add(10 * time.Second),
				respTime: base.Add(20 * time.Second),
			},
			assertion: func(t *testing.T, r *http.Response, err error) {
				testutil.RequireNoError(t, err)
				testutil.AssertNotNil(t, r)
				_, ok := r.Header["Connection"]
				testutil.AssertTrue(t, !ok)
			},
		},
		{
			name: "Successful store with existing headers",
			fields: fields{
				cache: &MockResponseCache{
					SetFunc: func(key string, entry *ResponseEntry) error {
						return nil
					},
					SetHeadersFunc: func(key string, headers VaryHeaderEntries) error {
						testutil.AssertTrue(t, len(headers) == 2)
						return nil
					},
				},
				vhn: VaryHeaderNormalizerFunc(
					func(vary string, reqHeader http.Header) iter.Seq2[string, string] {
						return maps.All(map[string]string{
							"Accept": reqHeader.Get("Accept"),
						})
					},
				),
				vk: mockVaryKeyer{},
			},
			args: args{
				resp: &http.Response{
					Header: http.Header{
						"Vary": {"Accept"},
					},
					Request: &http.Request{
						Header: http.Header{"Accept": []string{"text/html"}},
					},
				},
				key: "test-key",
				headers: VaryHeaderEntries{
					&VaryHeaderEntry{
						Vary:         "Accept",
						VaryResolved: map[string]string{"Accept": "application/json"},
						ResponseID:   "test-key#mock",
						Timestamp:    base,
					},
				},
				reqTime:  base.Add(10 * time.Second),
				respTime: base.Add(20 * time.Second),
			},
			assertion: func(t *testing.T, r *http.Response, err error) {
				testutil.RequireNoError(t, err)
			},
		},
		{
			name: "SetHeaders returns error",
			fields: fields{
				cache: &MockResponseCache{
					SetHeadersFunc: func(key string, headers VaryHeaderEntries) error {
						return testutil.ErrSample
					},
				},
				vhn: VaryHeaderNormalizerFunc(
					func(vary string, reqHeader http.Header) iter.Seq2[string, string] {
						return maps.All(map[string]string{
							"Accept": reqHeader.Get("Accept"),
						})
					},
				),
				vk: mockVaryKeyer{},
			},
			args: args{
				resp: &http.Response{
					Header: http.Header{
						"Vary": {"Accept"},
					},
					Request: &http.Request{
						Header: http.Header{"Accept": []string{"text/html"}},
					},
				},
				key:      "test-key",
				headers:  nil,
				reqTime:  base.Add(10 * time.Second),
				respTime: base.Add(20 * time.Second),
			},
			assertion: func(t *testing.T, r *http.Response, err error) {
				testutil.RequireErrorIs(t, err, testutil.ErrSample)
			},
		},
		{
			name: "Set returns error",
			fields: fields{
				cache: &MockResponseCache{
					SetHeadersFunc: func(key string, headers VaryHeaderEntries) error {
						return nil
					},
					SetFunc: func(key string, entry *ResponseEntry) error {
						return testutil.ErrSample
					},
				},
				vhn: VaryHeaderNormalizerFunc(
					func(vary string, reqHeader http.Header) iter.Seq2[string, string] {
						return maps.All(map[string]string{
							"Accept": reqHeader.Get("Accept"),
						})
					},
				),
				vk: mockVaryKeyer{},
			},
			args: args{
				resp: &http.Response{
					Header: http.Header{
						"Vary": {"Accept"},
					},
					Request: &http.Request{
						Header: http.Header{"Accept": []string{"text/html"}},
					},
				},
				key:      "test-key",
				headers:  nil,
				reqTime:  base.Add(10 * time.Second),
				respTime: base.Add(20 * time.Second),
			},
			assertion: func(t *testing.T, r *http.Response, err error) {
				testutil.RequireErrorIs(t, err, testutil.ErrSample)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &responseStorer{
				cache: tt.fields.cache,
				vhn:   tt.fields.vhn,
				vk:    tt.fields.vk,
			}
			err := r.StoreResponse(
				tt.args.resp,
				tt.args.key,
				tt.args.headers,
				tt.args.reqTime,
				tt.args.respTime,
			)
			if tt.assertion != nil {
				tt.assertion(t, tt.args.resp, err)
			} else {
				testutil.RequireNoError(t, err)
			}
		})
	}
}
