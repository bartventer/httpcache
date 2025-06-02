package internal

import (
	"bufio"
	"bytes"
	"encoding"
	"errors"
	"fmt"
	"net/http"
	"net/http/httputil"
	"time"
)

// Entry represents a cached HTTP response entry.
type Entry struct {
	Response *http.Response // The HTTP response to cache
	ReqTime  time.Time      // Timestamp of the request
	RespTime time.Time      // Timestamp of the response
}

var _ encoding.BinaryMarshaler = (*Entry)(nil)

func (e *Entry) MarshalBinary() ([]byte, error) {
	reqTimeBytes, err := e.ReqTime.MarshalBinary()
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request time: %w", err)
	}
	respTimeBytes, err := e.RespTime.MarshalBinary()
	if err != nil {
		return nil, fmt.Errorf("failed to marshal response time: %w", err)
	}
	respBytes, err := httputil.DumpResponse(e.Response, true)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal response: %w", err)
	}
	var buf bytes.Buffer
	buf.Write(reqTimeBytes)
	buf.WriteByte('\n')
	buf.Write(respTimeBytes)
	buf.WriteByte('\n')
	buf.Write(respBytes)
	return buf.Bytes(), nil
}

var (
	errReadBytes           = errors.New("failed to read bytes")
	errInvalidRequestTime  = errors.New("invalid request time")
	errInvalidResponseTime = errors.New("invalid response time")
	errInvalidResponse     = errors.New("invalid response")
)

func (e *Entry) UnmarshalBinaryWithRequest(data []byte, req *http.Request) error {
	reader := bufio.NewReader(bytes.NewReader(data))

	reqTimeLine, err := reader.ReadBytes('\n')
	if err != nil {
		return errors.Join(errReadBytes, fmt.Errorf("failed to read request time: %w", err))
	}
	var reqTime time.Time
	if unmarshalErr := reqTime.UnmarshalBinary(bytes.TrimSpace(reqTimeLine)); unmarshalErr != nil {
		return errors.Join(errInvalidRequestTime, unmarshalErr)
	}

	respTimeLine, err := reader.ReadBytes('\n')
	if err != nil {
		return errors.Join(errReadBytes, fmt.Errorf("failed to read response time: %w", err))
	}
	var respTime time.Time
	if unmarshalErr := respTime.UnmarshalBinary(bytes.TrimSpace(respTimeLine)); unmarshalErr != nil {
		return errors.Join(errInvalidResponseTime, unmarshalErr)
	}

	//nolint:bodyclose // The response body is not closed here, as it may be reused later.
	resp, err := http.ReadResponse(reader, req)
	if err != nil {
		return errors.Join(errInvalidResponse, err)
	}

	e.ReqTime = reqTime
	e.RespTime = respTime
	e.Response = resp
	return nil
}
