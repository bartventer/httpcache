// Package expapi provides HTTP handlers for managing and interacting with cache backends.
//
// WARNING: This package is intended for debugging, development, or administrative use only.
// It is NOT recommended to expose these endpoints in production environments, as they
// allow direct access to cache contents and deletion.
//
// Endpoints:
//
//	GET    /debug/httpcache           -- List cache keys (if supported)
//	GET    /debug/httpcache/{key}     -- Retrieve a cache entry
//	DELETE /debug/httpcache/{key}     -- Delete a cache entry
//
// Backends that implement the [KeyLister] interface will support key listing.
// All handlers expect a "dsn" query parameter to select the cache backend.
package expapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/bartventer/httpcache/store/driver"
	"github.com/bartventer/httpcache/store/internal/registry"
)

type connOpener interface {
	OpenConn(dsn string) (driver.Conn, error)
}

type kvMux struct {
	co connOpener
}

// KeyLister is an optional interface implemented by cache backends that
// support listing keys. It provides a method to retrieve all keys in the
// cache that match a given prefix.
type KeyLister interface {
	Keys(prefix string) []string
}

func dsnFromRequest(r *http.Request) string { return r.URL.Query().Get("dsn") }
func keyFromRequest(r *http.Request) string { return r.PathValue("key") }

func (m *kvMux) list(w http.ResponseWriter, r *http.Request) {
	conn, err := m.co.OpenConn(dsnFromRequest(r))
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to open cache: %v", err), http.StatusInternalServerError)
		return
	}
	kl, ok := conn.(KeyLister)
	if !ok {
		http.Error(w, "cache does not support listing keys", http.StatusNotImplemented)
		return
	}
	prefix := r.URL.Query().Get("prefix")
	keys := kl.Keys(prefix)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string][]string{"keys": keys})
}

func (m *kvMux) retrieve(w http.ResponseWriter, r *http.Request) {
	conn, err := m.co.OpenConn(dsnFromRequest(r))
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to open cache: %v", err), http.StatusInternalServerError)
		return
	}
	key := keyFromRequest(r)
	value, err := conn.Get(key)
	if err != nil {
		if errors.Is(err, driver.ErrNotExist) {
			http.Error(w, fmt.Sprintf("key %q not found", key), http.StatusNotFound)
		} else {
			http.Error(w, fmt.Sprintf("failed to get value for key %q: %v", key, err), http.StatusInternalServerError)
		}
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(value); err != nil {
		http.Error(
			w,
			fmt.Sprintf("failed to write response: %v", err),
			http.StatusInternalServerError,
		)
	}
}

func (m *kvMux) destroy(w http.ResponseWriter, r *http.Request) {
	conn, err := m.co.OpenConn(dsnFromRequest(r))
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to open cache: %v", err), http.StatusInternalServerError)
		return
	}
	key := keyFromRequest(r)
	if err := conn.Delete(key); err != nil {
		if errors.Is(err, driver.ErrNotExist) {
			http.Error(w, fmt.Sprintf("key %q not found", key), http.StatusNotFound)
		} else {
			http.Error(w, fmt.Sprintf("failed to delete value for key %q: %v", key, err), http.StatusInternalServerError)
		}
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type handlerConfig struct {
	Mux *http.ServeMux
}

type HandlerOption interface {
	apply(*handlerConfig)
}

type handlerOptionFunc func(*handlerConfig)

func (f handlerOptionFunc) apply(cfg *handlerConfig) {
	f(cfg)
}

// WithServeMux allows specifying a custom http.ServeMux for the HTTP cache API
// handlers; default: [http.DefaultServeMux].
func WithServeMux(mux *http.ServeMux) HandlerOption {
	return handlerOptionFunc(func(cfg *handlerConfig) {
		cfg.Mux = mux
	})
}

func (m *kvMux) Register(opts ...HandlerOption) {
	cfg := &handlerConfig{
		Mux: http.DefaultServeMux,
	}
	for _, opt := range opts {
		opt.apply(cfg)
	}
	mux := cfg.Mux
	mux.HandleFunc("GET /debug/httpcache", m.list)
	mux.HandleFunc("GET /debug/httpcache/{key}", m.retrieve)
	mux.HandleFunc("DELETE /debug/httpcache/{key}", m.destroy)
}

var defaultKVMux = &kvMux{co: registry.Default()}

// Register registers the HTTP cache API handlers with the provided options.
func Register(opts ...HandlerOption) { defaultKVMux.Register(opts...) }

// ListHandler returns the list handler for the HTTP cache API.
//
// This is only needed to install the handler in a non-standard location.
func ListHandler() http.Handler { return http.HandlerFunc(defaultKVMux.list) }

// RetrieveHandler returns the retrieve handler for the HTTP cache API.
//
// This is only needed to install the handler in a non-standard location.
func RetrieveHandler() http.Handler { return http.HandlerFunc(defaultKVMux.retrieve) }

// DestroyHandler returns the destroy handler for the HTTP cache API.
//
// This is only needed to install the handler in a non-standard location.
func DestroyHandler() http.Handler { return http.HandlerFunc(defaultKVMux.destroy) }
