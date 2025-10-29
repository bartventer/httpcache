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
	"encoding/base64"
	"path/filepath"
	"strings"
)

type (
	fileNamer     interface{ FileName(key string) string }
	fileNameKeyer interface {
		KeyFromFileName(name string) (string, error)
	}
)

type (
	fileNamerFunc     func(key string) string
	fileNameKeyerFunc func(name string) (string, error)
)

func (f fileNamerFunc) FileName(key string) string                      { return f(key) }
func (f fileNameKeyerFunc) KeyFromFileName(name string) (string, error) { return f(name) }

// fragmentSize is the maximum filename length per directory level.
// 48 is chosen so that 5 fragments fit within 240 chars, well under common filesystem limits.
const fragmentSize = 48

// fragmentingFileNamer returns a fileNamer that fragments long keys into directory structures.
// This helps avoid filesystem limits on filename lengths.
func fragmentingFileNamer() fileNamer {
	return fileNamerFunc(fragmentFileName)
}

func fragmentFileName(key string) string {
	encoded := base64.RawURLEncoding.EncodeToString([]byte(key))
	if len(encoded) <= 255 { // Common filesystem filename limit
		return encoded
	}

	// Fragment the encoded string
	var parts []string
	for i := 0; i < len(encoded); i += fragmentSize {
		end := min(i+fragmentSize, len(encoded))
		parts = append(parts, encoded[i:end])
	}
	return filepath.Join(parts...)
}

func fragmentingFileNameKeyer() fileNameKeyer {
	return fileNameKeyerFunc(fragmentedFileNameToKey)
}

var filepathSeparatorReplacer = strings.NewReplacer(
	string(filepath.Separator),
	"",
)

func fragmentedFileNameToKey(name string) (string, error) {
	// Check if the name contains path separators (i.e., is fragmented)
	if strings.ContainsRune(name, filepath.Separator) {
		// Handle fragmented path
		base64Str := filepathSeparatorReplacer.Replace(name)
		decoded, err := base64.RawURLEncoding.DecodeString(base64Str)
		if err != nil {
			return "", err
		}
		return string(decoded), nil
	}

	// Handle plain base64
	decoded, err := base64.RawURLEncoding.DecodeString(name)
	if err != nil {
		return "", err
	}
	return string(decoded), nil
}
