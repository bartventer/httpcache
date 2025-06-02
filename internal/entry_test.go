package internal

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/bartventer/httpcache/internal/testutil"
)

func TestEntry_MarshalUnmarshalBinary_Success(t *testing.T) {
	reqTime := time.Now().Truncate(time.Second)
	respTime := reqTime.Add(2 * time.Second)
	resp := httptest.NewRecorder().Result()
	resp.Body = io.NopCloser(strings.NewReader("hello world"))

	entry := &Entry{
		Response: resp,
		ReqTime:  reqTime,
		RespTime: respTime,
	}

	data, err := entry.MarshalBinary()
	testutil.RequireNoError(t, err)

	// Unmarshal into a new Entry
	req := &http.Request{Method: http.MethodGet}
	var entry2 Entry
	err = entry2.UnmarshalBinaryWithRequest(data, req)

	testutil.RequireNoError(t, err)
	testutil.AssertTrue(t, entry2.ReqTime.Equal(reqTime))
	testutil.AssertTrue(t, entry2.RespTime.Equal(respTime))
	testutil.AssertEqual(t, entry2.Response.StatusCode, http.StatusOK)

	body, _ := io.ReadAll(entry2.Response.Body)
	testutil.AssertEqual(t, string(body), "hello world")
}

func TestEntry_UnmarshalBinaryWithRequest_InvalidReqTime(t *testing.T) {
	// Corrupt reqTime
	data := []byte("notatime\nsometime\nHTTP/1.1 200 OK\r\nContent-Length: 0\r\n\r\n")
	var entry Entry
	req := &http.Request{Method: http.MethodGet}
	err := entry.UnmarshalBinaryWithRequest(data, req)
	testutil.RequireErrorIs(t, err, errInvalidRequestTime)
}

func TestEntry_UnmarshalBinaryWithRequest_InvalidRespTime(t *testing.T) {
	// Valid reqTime, corrupt respTime
	now := time.Now()
	reqTimeBytes, _ := now.MarshalBinary()

	var buf bytes.Buffer
	buf.Write(reqTimeBytes)
	buf.WriteByte('\n')
	buf.WriteString("notatime\n")
	buf.WriteString("HTTP/1.1 200 OK\r\nContent-Length: 0\r\n\r\n")

	var entry Entry
	req := &http.Request{Method: http.MethodGet}
	err := entry.UnmarshalBinaryWithRequest(buf.Bytes(), req)
	testutil.RequireErrorIs(t, err, errInvalidResponseTime)
}

func TestEntry_UnmarshalBinaryWithRequest_InvalidResponse(t *testing.T) {
	// Valid times, corrupt response
	now := time.Now()
	reqTimeBytes, _ := now.MarshalBinary()
	respTimeBytes, _ := now.MarshalBinary()

	var buf bytes.Buffer
	buf.Write(reqTimeBytes)
	buf.WriteByte('\n')
	buf.Write(respTimeBytes)
	buf.WriteByte('\n')
	buf.WriteString("not a http response")

	var entry Entry
	req := &http.Request{Method: http.MethodGet}
	err := entry.UnmarshalBinaryWithRequest(buf.Bytes(), req)
	testutil.RequireErrorIs(t, err, errInvalidResponse)
}

func TestEntry_UnmarshalBinaryWithRequest_ReadBytesError(t *testing.T) {
	// Not enough data for reqTime
	data := []byte{}
	var entry Entry
	req := &http.Request{Method: http.MethodGet}
	err := entry.UnmarshalBinaryWithRequest(data, req)
	testutil.RequireErrorIs(t, err, errReadBytes)
}
