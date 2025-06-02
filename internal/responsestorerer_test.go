package internal

import (
	"net/http"
	"testing"
	"time"

	"github.com/bartventer/httpcache/internal/testutil"
)

func Test_responseStorer_StoreResponse(t *testing.T) {
	base := time.Unix(0, 0).UTC()
	type fields struct {
		cache ResponseCache
	}
	type args struct {
		resp              *http.Response
		key               string
		reqTime, respTime time.Time
	}
	tests := []struct {
		name      string
		fields    fields
		args      args
		assertion func(*testing.T, *http.Response, error)
	}{
		{
			name: "Successful store",
			fields: fields{
				cache: &MockResponseCache{
					SetFunc: func(key string, entry *Entry) error {
						testutil.AssertEqual(t, "test-key", key)
						testutil.AssertNotNil(t, entry)
						return nil
					},
				},
			},
			args: args{
				resp: &http.Response{
					Header: http.Header{
						"Connection": {"keep-alive"}, // Expect this to be removed
					},
				},
				key:      "test-key",
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
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &responseStorer{
				cache: tt.fields.cache,
			}
			err := r.StoreResponse(tt.args.resp, tt.args.key, tt.args.reqTime, tt.args.respTime)
			if tt.assertion != nil {
				tt.assertion(t, tt.args.resp, err)
			} else {
				testutil.RequireNoError(t, err)
			}
		})
	}
}
