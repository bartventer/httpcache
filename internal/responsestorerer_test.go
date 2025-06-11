package internal

import (
	"iter"
	"maps"
	"net/http"
	"testing"
	"time"

	"github.com/bartventer/httpcache/internal/testutil"
)

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
		refs              ResponseRefs
		reqTime, respTime time.Time
		refIndex          int
	}
	tests := []struct {
		name      string
		fields    fields
		args      args
		assertion func(*testing.T, *http.Response, error)
	}{
		{
			name: "nil refs creates new slice and appends",
			fields: fields{
				cache: &MockResponseCache{
					SetFunc: func(key string, entry *Response) error {
						testutil.AssertEqual(t, "test-key#mock", key)
						return nil
					},
					SetRefsFunc: func(key string, refs ResponseRefs) error {
						testutil.AssertEqual(t, "test-key", key)
						testutil.AssertTrue(t, len(refs) == 1)
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
				vk: VaryKeyerFunc(func(urlKey string, varyHeaders map[string]string) string {
					return urlKey + "#mock"
				}),
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
				refs:     nil,
				reqTime:  base.Add(10 * time.Second),
				respTime: base.Add(20 * time.Second),
				refIndex: 0,
			},
			assertion: func(t *testing.T, r *http.Response, err error) {
				testutil.RequireNoError(t, err)
			},
		},
		{
			name: "refIndex in range updates existing ref",
			fields: fields{
				cache: &MockResponseCache{
					SetFunc: func(key string, entry *Response) error { return nil },
					SetRefsFunc: func(key string, refs ResponseRefs) error {
						testutil.AssertTrue(t, len(refs) == 1)
						testutil.AssertEqual(t, refs[0].VaryResolved["Accept"], "text/html")
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
				vk: VaryKeyerFunc(func(urlKey string, varyHeaders map[string]string) string {
					return urlKey + "#mock"
				}),
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
				refs: ResponseRefs{
					&ResponseRef{
						Vary:         "Accept",
						VaryResolved: map[string]string{"Accept": "application/json"},
						ReceivedAt:   base,
						ResponseID:   "test-key#mock",
					},
				},
				reqTime:  base.Add(10 * time.Second),
				respTime: base.Add(20 * time.Second),
				refIndex: 0,
			},
			assertion: func(t *testing.T, r *http.Response, err error) {
				testutil.RequireNoError(t, err)
			},
		},
		{
			name: "refIndex out of range appends new ref",
			fields: fields{
				cache: &MockResponseCache{
					SetFunc: func(key string, entry *Response) error { return nil },
					SetRefsFunc: func(key string, refs ResponseRefs) error {
						testutil.AssertTrue(t, len(refs) == 2)
						testutil.AssertEqual(t, refs[1].VaryResolved["Accept"], "text/html")
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
				vk: VaryKeyerFunc(func(urlKey string, varyHeaders map[string]string) string {
					return urlKey + "#mock"
				}),
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
				refs: ResponseRefs{
					&ResponseRef{
						Vary:         "Accept",
						VaryResolved: map[string]string{"Accept": "application/json"},
						ReceivedAt:   base,
						ResponseID:   "test-key#mock",
					},
				},
				reqTime:  base.Add(10 * time.Second),
				respTime: base.Add(20 * time.Second),
				refIndex: 1, // out of range (len(refs) == 1)
			},
			assertion: func(t *testing.T, r *http.Response, err error) {
				testutil.RequireNoError(t, err)
			},
		},
		{
			name: "SetRefs returns error",
			fields: fields{
				cache: &MockResponseCache{
					SetFunc: func(key string, entry *Response) error { return nil },
					SetRefsFunc: func(key string, headers ResponseRefs) error {
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
				vk: VaryKeyerFunc(func(urlKey string, varyHeaders map[string]string) string {
					return urlKey + "#mock"
				}),
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
				refs:     nil,
				reqTime:  base.Add(10 * time.Second),
				respTime: base.Add(20 * time.Second),
				refIndex: 0,
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
			FixDateHeader(tt.args.resp.Header, tt.args.respTime)
			err := r.StoreResponse(
				tt.args.resp,
				tt.args.key,
				tt.args.refs,
				tt.args.reqTime,
				tt.args.respTime,
				tt.args.refIndex,
			)
			if tt.assertion != nil {
				tt.assertion(t, tt.args.resp, err)
			} else {
				testutil.RequireNoError(t, err)
			}
		})
	}
}
