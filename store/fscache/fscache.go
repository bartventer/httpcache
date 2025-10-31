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

// Package fscache implements a file system-based cache backend.
//
// Entries are stored as files in a directory under the user's OS cache
// directory by default. The cache location and behavior are configured via DSN
// query parameters.
//
// Supported DSN query parameters:
//   - appname (required): Subdirectory name for cache isolation.
//   - timeout (optional): Operation timeout (e.g., "2s", "500ms"). Default: 5m.
//   - connect_timeout (optional): Timeout for establishing the cache
//     connection (e.g., "2s"). Default: 5m.
//   - encrypt (optional): Enable encryption for cache entries. Set to "on" or
//     "aesgcm".
//   - encrypt_key (optional): The AES key, base64-encoded (URL-safe variant
//     with/without padding as per RFC 4648 §5). Must decode to the correct
//     length as required by [crypto/aes].
//   - update_mtime (optional): Update file modification time on cache hits.
//     Set to "on" to enable. Default: "off".
//
// Example DSNs:
//
//	fscache://?appname=myapp
//	fscache:///tmp/cache?appname=myapp&timeout=2s
//	fscache:///tmp/cache?appname=myapp&connect_timeout=2s&timeout=1m
//	fscache://?appname=myapp&encrypt=on&encrypt_key=6S-Ks2YYOW0xMvTzKSv6QD30gZeOi1c6Ydr-As5csWk=
//	fscache://?appname=myapp&update_mtime=on
//
// Example environment variable:
//
//	export FSCACHE_ENCRYPT_KEY="6S-Ks2YYOW0xMvTzKSv6QD30gZeOi1c6Ydr-As5csWk="
//
// To generate a suitable encryption key (32 bytes, URL-safe, with padding):
//
//	openssl rand 32 | base64 | tr '+/' '-_' | tr -d '\n'
package fscache

import (
	"cmp"
	"context"
	"crypto/rand"
	"io/fs"
	"strings"

	"errors"
	"fmt"
	"io"

	"net/url"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/bartventer/httpcache/store"
	"github.com/bartventer/httpcache/store/driver"
	"github.com/bartventer/httpcache/store/expapi"
)

const Scheme = "fscache" // url scheme for the file system cache

//nolint:gochecknoinits // We use init to register the driver.
func init() {
	store.Register(Scheme, driver.DriverFunc(func(u *url.URL) (driver.Conn, error) {
		return fromURL(u)
	}))
}

var (
	ErrUserCacheDir   = errors.New("fscache: could not determine user cache dir")
	ErrMissingAppName = errors.New("fscache: appname query parameter is required")
	ErrCreateCacheDir = errors.New("fscache: could not create cache dir")
)

type Error struct {
	Op  string // operation being performed (e.g., "Get", "Set")
	Key string // optional key for which the operation failed
	Err error  // underlying error
}

func (e *Error) Error() string {
	if e.Key != "" {
		return fmt.Sprintf("fscache: %s for key %q: %v", e.Op, e.Key, e.Err)
	}
	return fmt.Sprintf("fscache: %s: %v", e.Op, e.Err)
}

func (e *Error) Unwrap() error {
	return e.Err
}

type fsCache struct {
	// configurable options

	root        *os.Root
	base        string        // base directory for the cache (parent of root)
	connTimeout time.Duration // optional timeout for establishing the connection
	timeout     time.Duration // optional timeout for operations
	enc         encryptor     // optional encryptor for data
	updateMTime bool          // whether to update file mtime on cache hits

	// internal dependencies

	fn  fileNamer     // generates file names from keys
	fnk fileNameKeyer // recovers keys from file names
	dw  dirWalker     // used for directory walking
}

const defaultTimeout = 5 * time.Minute

func parseTimeout(v string) time.Duration {
	if v == "" {
		return 0
	}
	timeout, err := time.ParseDuration(v)
	if err != nil {
		return 0
	}
	return max(timeout, 0)
}

var errEncryptionEnabledWithoutKey = errors.New("fscache: encryption enabled but no key provided")

type Option interface {
	apply(*fsCache) error
}

type optionFunc func(*fsCache) error

func (f optionFunc) apply(c *fsCache) error {
	return f(c)
}

// WithConnectTimeout sets the timeout for establishing the cache connection; default: 5 minutes.
func WithConnectTimeout(timeout time.Duration) Option {
	return optionFunc(func(c *fsCache) error {
		c.connTimeout = max(timeout, 0)
		return nil
	})
}

// WithTimeout sets the timeout for cache operations; default: 5 minutes.
func WithTimeout(timeout time.Duration) Option {
	return optionFunc(func(c *fsCache) error {
		c.timeout = max(timeout, 0)
		return nil
	})
}

// WithEncryption enables encryption for cache entries using AES-GCM.
func WithEncryption(key string) Option {
	return optionFunc(func(c *fsCache) (err error) {
		if key == "" {
			return errEncryptionEnabledWithoutKey
		}
		c.enc, err = newAESGCMEncryptor(rand.Reader, key)
		return err
	})
}

// WithBaseDir sets the base directory for the cache; default: user's OS cache directory.
func WithBaseDir(base string) Option {
	return optionFunc(func(c *fsCache) error {
		if base != "" {
			c.base = base
		}
		return nil
	})
}

// WithUpdateMTime enables updating file modification time on cache hits.
func WithUpdateMTime(enabled bool) Option {
	return optionFunc(func(c *fsCache) error {
		c.updateMTime = enabled
		return nil
	})
}

func fromURL(u *url.URL) (*fsCache, error) {
	appname := u.Query().Get("appname")
	if appname == "" {
		return nil, ErrMissingAppName
	}
	opts := make([]Option, 0, 5)
	if u.Path != "" && u.Path != "/" {
		opts = append(opts, WithBaseDir(u.Path))
	}
	if v := u.Query().Get("connect_timeout"); v != "" {
		opts = append(opts, WithConnectTimeout(parseTimeout(v)))
	}
	if v := u.Query().Get("timeout"); v != "" {
		opts = append(opts, WithTimeout(parseTimeout(v)))
	}
	if encrypt := u.Query().Get("encrypt"); encrypt == "on" || encrypt == "aesgcm" {
		key := cmp.Or(u.Query().Get("encrypt_key"), os.Getenv("FSCACHE_ENCRYPT_KEY"))
		opts = append(opts, WithEncryption(key))
	}
	if updateMTime := u.Query().Get("update_mtime"); updateMTime == "on" {
		opts = append(opts, WithUpdateMTime(true))
	}
	if cap(opts) > len(opts) {
		opts = slices.Clip(opts)
	}
	return Open(appname, opts...)
}

// Open creates a new file system cache with the specified application name and options.
//
// See the package documentation for supported options and examples.
func Open(appname string, opts ...Option) (*fsCache, error) {
	c := new(fsCache)
	for _, opt := range opts {
		if err := opt.apply(c); err != nil {
			return nil, err
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), cmp.Or(c.connTimeout, defaultTimeout))
	defer cancel()
	errc := make(chan error, 1)
	go func() {
		defer close(errc)
		if err := c.initialize(appname); err != nil {
			errc <- err
			return
		}
		errc <- nil
	}()
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case err := <-errc:
		if err != nil {
			return nil, err
		}
		return c, nil
	}
}

func (c *fsCache) initialize(appname string) error {
	if c.base == "" {
		userCacheDir, err := os.UserCacheDir()
		if err != nil {
			return errors.Join(ErrUserCacheDir, err)
		}
		c.base = userCacheDir
	}
	if appname == "" {
		return ErrMissingAppName
	}
	c.base = filepath.Join(c.base, appname)
	if err := os.MkdirAll(c.base, 0o755); err != nil {
		return errors.Join(ErrCreateCacheDir, err)
	}
	var err error
	c.root, err = os.OpenRoot(c.base)
	if err != nil {
		return fmt.Errorf("fscache: could not open cache directory %q: %w", c.base, err)
	}
	c.fn = fragmentingFileNamer()
	c.fnk = fragmentingFileNameKeyer()
	c.dw = dirWalkerFunc(filepath.WalkDir)
	c.timeout = cmp.Or(c.timeout, defaultTimeout)

	return nil
}

type dirWalker interface {
	WalkDir(root string, fn fs.WalkDirFunc) error
}

type dirWalkerFunc func(root string, fn fs.WalkDirFunc) error

func (f dirWalkerFunc) WalkDir(root string, fn fs.WalkDirFunc) error { return f(root, fn) }

var _ driver.Conn = (*fsCache)(nil)
var _ expapi.KeyLister = (*fsCache)(nil)

func (c *fsCache) Get(key string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()

	type result struct {
		data []byte
		err  error
	}
	errc := make(chan result, 1)
	go func() {
		defer close(errc)
		data, err := c.get(key)
		if err != nil {
			errc <- result{nil, &Error{"Get", key, err}}
			return
		}
		errc <- result{data, nil}
	}()
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case res := <-errc:
		return res.data, res.err
	}
}

// zeroTime is the zero value for [time.Time], used to leave access time unchanged
// when updating modification time.
var zeroTime time.Time

func (c *fsCache) get(key string) ([]byte, error) {
	name := c.fn.FileName(key)
	f, err := c.root.Open(name)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, errors.Join(driver.ErrNotExist, err)
		}
		return nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(f)
	if err != nil {
		return nil, err
	}
	if c.enc != nil {
		data, err = c.enc.Decrypt(data)
		if err != nil {
			return nil, err
		}
	}
	if c.updateMTime {
		mtime := time.Now()
		if err := c.root.Chtimes(name, zeroTime, mtime); err != nil {
			return nil, err
		}
	}
	return data, nil
}

func (c *fsCache) Set(key string, entry []byte) error {
	ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()

	errc := make(chan error, 1)
	go func() {
		defer close(errc)
		err := c.set(key, entry)
		if err != nil {
			errc <- &Error{"Set", key, err}
			return
		}
		errc <- nil
	}()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-errc:
		return err
	}
}

func (c *fsCache) set(key string, entry []byte) error {
	if c.enc != nil {
		var err error
		entry, err = c.enc.Encrypt(entry)
		if err != nil {
			return err
		}
	}
	name := c.fn.FileName(key)
	if err := c.root.MkdirAll(filepath.Dir(name), 0o755); err != nil {
		return err
	}
	f, err := c.root.Create(name)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(entry)
	if err != nil {
		return err
	}
	return f.Sync()
}

func (c *fsCache) Delete(key string) error {
	ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()

	errc := make(chan error, 1)
	go func() {
		defer close(errc)
		err := c.delete(key)
		if err != nil {
			errc <- &Error{"Delete", key, err}
			return
		}
		errc <- nil
	}()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-errc:
		return err
	}
}

func (c *fsCache) delete(key string) error {
	err := c.root.Remove(c.fn.FileName(key))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			err = errors.Join(driver.ErrNotExist, err)
		}
		return err
	}
	return nil
}

func (c *fsCache) Keys(prefix string) ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()

	type result struct {
		keys []string
		err  error
	}
	errc := make(chan result, 1)
	go func() {
		defer close(errc)
		keys, err := c.keys(prefix)
		if err != nil {
			errc <- result{nil, &Error{Op: "Keys", Err: fmt.Errorf("prefix %q: %w", prefix, err)}}
			return
		}
		errc <- result{keys, nil}
	}()
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case res := <-errc:
		return res.keys, res.err
	}
}

func (c *fsCache) keys(prefix string) ([]string, error) {
	dirname := c.root.Name()
	keys := make([]string, 0, 10)
	err := c.dw.WalkDir(dirname, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		key, err := c.fnk.KeyFromFileName(
			strings.TrimPrefix(path, dirname+string(os.PathSeparator)),
		)
		if err != nil {
			return err
		}
		if strings.HasPrefix(key, prefix) {
			keys = append(keys, key)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if cap(keys) > len(keys) {
		keys = slices.Clip(keys)
	}
	return keys, nil
}
