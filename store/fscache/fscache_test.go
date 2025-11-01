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

package fscache

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"io/fs"
	"net/url"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/bartventer/httpcache/internal/testutil"
	"github.com/bartventer/httpcache/store/acceptance"
	"github.com/bartventer/httpcache/store/driver"
)

func (c *fsCache) Close() error {
	if err := c.root.Close(); err != nil {
		fmt.Printf("Warning: failed to close cache: %v", err)
	}
	return nil
}

func makeRootURL(t testing.TB) *url.URL {
	t.Helper()
	tempDir := filepath.ToSlash(t.TempDir())
	u, err := url.Parse("fscache://" + tempDir + "?appname=testapp")
	if err != nil {
		t.Fatalf("Failed to parse cache URL: %v", err)
	}
	return u
}

func TestFSCache_Acceptance(t *testing.T) {
	acceptance.Run(t, acceptance.FactoryFunc(func() (driver.Conn, func()) {
		u := makeRootURL(t)
		cache, err := fromURL(u)
		testutil.RequireNoError(t, err, "Failed to create fscache")
		cleanup := func() { cache.Close() }
		return cache, cleanup
	}))
}

func Test_fsCache_SetError(t *testing.T) {
	u := makeRootURL(t)
	cache, err := fromURL(u)
	testutil.RequireNoError(t, err, "Failed to create fscache")
	t.Cleanup(func() { cache.Close() })

	cache.fn = fileNamerFunc(func(key string) string {
		return key // no sanitization
	})
	err = cache.Set("../../invalid", []byte("value"))
	testutil.RequireError(t, err)
}

func Test_fsCache_KeysError(t *testing.T) {
	u := makeRootURL(t)
	cache, err := fromURL(u)
	testutil.RequireNoError(t, err, "Failed to create fscache")
	t.Cleanup(func() { cache.Close() })

	cache.fn = fileNamerFunc(func(key string) string {
		return key
	})
	cache.fnk = fileNameKeyerFunc(func(name string) (string, error) {
		return name, nil
	})
	cache.dw = dirWalkerFunc(func(root string, fn fs.WalkDirFunc) error {
		return testutil.ErrSample
	})
	keys, err := cache.Keys("")
	var expectedErr *Error
	testutil.RequireErrorAs(t, err, &expectedErr)
	testutil.AssertEqual(t, 0, len(keys), "Expected no keys for invalid path")
}

func TestOpen(t *testing.T) {
	type args struct {
		dsn string
	}
	tests := []struct {
		name      string
		args      args
		setup     func(*testing.T)
		assertion func(tt *testing.T, got *fsCache, err error)
	}{
		{
			name: "Valid Root Directory",
			args: args{
				dsn: "fscache://" + filepath.ToSlash(t.TempDir()) + "?appname=myapp",
			},
			assertion: func(tt *testing.T, got *fsCache, err error) {
				testutil.RequireNoError(tt, err)
				testutil.RequireNotNil(tt, got)
			},
		},
		{
			name: "Default User Cache Directory",
			args: args{
				dsn: "fscache://?appname=myapp",
			},
			assertion: func(tt *testing.T, got *fsCache, err error) {
				testutil.RequireNoError(tt, err)
				testutil.RequireNotNil(tt, got)
			},
		},
		{
			name: "Invalid Cache Directory",
			args: args{
				dsn: "fscache://?appname=myapp",
			},
			setup: func(tt *testing.T) {
				switch runtime.GOOS {
				case "windows":
					tt.Setenv("LocalAppData", "")
				default:
					tt.Setenv("XDG_CACHE_HOME", "")
					tt.Setenv("HOME", "")
				}
			},
			assertion: func(tt *testing.T, got *fsCache, err error) {
				testutil.RequireErrorIs(tt, err, ErrUserCacheDir)
				testutil.AssertNil(tt, got)
			},
		},
		{
			name: "Missing App Name",
			args: args{
				dsn: "fscache://",
			},
			assertion: func(tt *testing.T, got *fsCache, err error) {
				testutil.RequireErrorIs(tt, err, ErrMissingAppName)
				testutil.AssertNil(tt, got)
			},
		},
		{
			name: "Invalid Root Directory",
			args: args{
				dsn: "fscache://" + filepath.ToSlash(
					filepath.VolumeName(t.TempDir())+"/../invalid",
				) + "?appname=myapp",
			},
			setup: func(tt *testing.T) {
				if runtime.GOOS == "windows" {
					tt.Skip("Skipping invalid path test on Windows")
				}
			},
			assertion: func(tt *testing.T, got *fsCache, err error) {
				testutil.RequireErrorIs(tt, err, ErrCreateCacheDir)
				testutil.AssertNil(tt, got)
			},
		},
		{
			name: "Encryption Enabled with Key",
			args: args{
				dsn: "fscache://?appname=myapp&encrypt=aesgcm&encrypt_key=" + mustBase64Key(t, 16),
			},
			assertion: func(tt *testing.T, got *fsCache, err error) {
				testutil.RequireNoError(tt, err)
				testutil.RequireNotNil(tt, got)
				testutil.AssertNotNil(tt, got.enc, "Expected encryptor to be initialized")
			},
		},
		{
			name: "Encryption Enabled with Key (Environment Variable)",
			args: args{
				dsn: "fscache://?appname=myapp&encrypt=aesgcm",
			},
			setup: func(tt *testing.T) {
				tt.Setenv("FSCACHE_ENCRYPT_KEY", "6S-Ks2YYOW0xMvTzKSv6QD30gZeOi1c6Ydr-As5csWk=")
			},
			assertion: func(tt *testing.T, got *fsCache, err error) {
				testutil.RequireNoError(tt, err)
				testutil.RequireNotNil(tt, got)
				testutil.AssertNotNil(tt, got.enc, "Expected encryptor to be initialized")
			},
		},
		{
			name: "Encryption Enabled without Key",
			args: args{
				dsn: "fscache://?appname=myapp&encrypt=aesgcm",
			},
			assertion: func(tt *testing.T, got *fsCache, err error) {
				testutil.RequireErrorIs(tt, err, errEncryptionEnabledWithoutKey)
				testutil.AssertNil(tt, got)
			},
		},
		{
			name: "Connect Timeout",
			args: args{
				dsn: "fscache://?appname=myapp&connect_timeout=1ns&timeout=10s",
			},
			setup: func(tt *testing.T) {
				if runtime.GOOS == "windows" {
					tt.Skip("Skipping connect timeout test on Windows")
				}
			},
			assertion: func(tt *testing.T, got *fsCache, err error) {
				testutil.RequireErrorIs(tt, err, context.DeadlineExceeded)
				testutil.AssertNil(tt, got)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u, err := url.Parse(tt.args.dsn)
			if err != nil {
				t.Fatalf("Failed to parse URL: %v", err)
			}
			if tt.setup != nil {
				tt.setup(t)
			}
			got, err := fromURL(u)
			tt.assertion(t, got, err)
		})
	}
}

func Test_parseTimeout(t *testing.T) {
	tests := []struct {
		name string
		v    string
		want time.Duration
	}{
		{"empty", "", 0},
		{"valid", "5s", 5 * time.Second},
		{"invalid", "invalid", 0},
		{"negative", "-1s", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseTimeout(tt.v)
			testutil.AssertEqual(t, tt.want, got, "parseTimeout(%q)", tt.v)
		})
	}
}

func TestFSCache_SetGet_WithEncryption(t *testing.T) {
	u, err := url.Parse("fscache://" + filepath.ToSlash(t.TempDir()) +
		"?appname=testapp&encrypt=aesgcm&encrypt_key=6S-Ks2YYOW0xMvTzKSv6QD30gZeOi1c6Ydr-As5csWk=")
	testutil.RequireNoError(t, err)
	cache, err := fromURL(u)
	testutil.RequireNoError(t, err)
	t.Cleanup(func() { cache.Close() })

	plaintext := []byte("super secret value")
	keyName := "mykey"

	err = cache.Set(keyName, plaintext)
	testutil.RequireNoError(t, err)

	got, err := cache.Get(keyName)
	testutil.RequireNoError(t, err)
	testutil.AssertTrue(t, bytes.Equal(plaintext, got), "decrypted value mismatch")

	// Ensure that the file on disk is not plaintext
	fname := cache.fn.FileName(keyName)
	f, err := cache.root.Open(fname)
	testutil.RequireNoError(t, err)
	defer f.Close()
	ciphertext, err := io.ReadAll(f)
	testutil.RequireNoError(t, err)
	testutil.AssertTrue(
		t,
		!bytes.Contains(ciphertext, plaintext),
		"ciphertext should not contain plaintext",
	)
}

func Test_fsCache_SetGet_UpdateMTime(t *testing.T) {
	u, err := url.Parse("fscache://" + filepath.ToSlash(t.TempDir()) +
		"?appname=testapp&update_mtime=on")
	testutil.RequireNoError(t, err)
	cache, err := fromURL(u)
	testutil.RequireNoError(t, err)
	t.Cleanup(func() { cache.Close() })

	keyName := "mykey"
	value := []byte("some value")

	err = cache.Set(keyName, value)
	testutil.RequireNoError(t, err)

	// Get initial mtime
	fname := cache.fn.FileName(keyName)
	info1, err := fs.Stat(cache.root.FS(), fname)
	testutil.RequireNoError(t, err)
	mtime1 := info1.ModTime()

	// Wait a bit to ensure mtime will be different
	time.Sleep(2 * time.Second)

	// Get the key to update mtime
	_, err = cache.Get(keyName)
	testutil.RequireNoError(t, err)

	// Get updated mtime
	info2, err := fs.Stat(cache.root.FS(), fname)
	testutil.RequireNoError(t, err)
	mtime2 := info2.ModTime()

	testutil.AssertTrue(t, mtime2.After(mtime1))
}
