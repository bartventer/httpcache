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
	"testing"

	"github.com/bartventer/httpcache/internal/testutil"
)

func TestParseQuotedStringE(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		want      string
		wantError error
	}{
		{
			name:      "Valid quoted string with no escapes",
			input:     `"hello"`,
			want:      "hello",
			wantError: nil,
		},
		{
			name:      "Valid quoted string with escapes",
			input:     `"he\\llo"`,
			want:      `he\llo`,
			wantError: nil,
		},
		{
			name:      "Valid quoted string with special characters",
			input:     "\"he\tlo\"",
			want:      "he\tlo",
			wantError: nil,
		},
		{
			name:      "Empty quoted string",
			input:     `""`,
			want:      "",
			wantError: nil,
		},
		{
			name:      "Invalid: no surrounding quotes",
			input:     `hello`,
			want:      "",
			wantError: errInvalidQuotedString,
		},
		{
			name:      "Invalid: unescaped backslash at end",
			input:     `"hello\"`,
			want:      "",
			wantError: errUnfinishedEscape,
		},
		{
			name:      "Invalid: invalid character",
			input:     "\"hello\x01\"",
			want:      "",
			wantError: errInvalidCharacter,
		},
		{
			name:      "Valid: obs-text characters",
			input:     "\"hello\x80world\"",
			want:      "hello\x80world",
			wantError: nil,
		},
		{
			name:      "Invalid: single quote",
			input:     `"`,
			want:      "",
			wantError: errInvalidQuotedString,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseQuotedStringE(tt.input)
			if tt.wantError != nil {
				testutil.RequireErrorIs(t, err, tt.wantError)
				return
			}
			testutil.AssertEqual(t, tt.want, got)
		})
	}
}

func Test_validQDTextByte(t *testing.T) {
	type testCase struct {
		name   string
		input  byte
		expect bool
	}

	cases := []testCase{
		{"HTAB allowed", '\t', true},
		{"SP allowed", ' ', true},
		{"! (0x21) allowed", 0x21, true},
		{"# (0x23) allowed", 0x23, true},
		{"[ (0x5B) allowed", 0x5B, true},
		{"] (0x5D) allowed", 0x5D, true},
		{"~ (0x7E) allowed", 0x7E, true},
		{"obs-text lower bound (0x80) allowed", 0x80, true},
		{"obs-text upper bound (0xFF) allowed", 0xFF, true},

		{"DQUOTE (0x22) not allowed", 0x22, false},
		{"Backslash (0x5C) not allowed", 0x5C, false},
		{"DEL (0x7F) not allowed", 0x7F, false},
		{"Below range (0x00) not allowed", 0x00, false},
		{"Below range (0x08) not allowed", 0x08, false},
		{"Below range (0x0A) not allowed", 0x0A, false},
		{"Below range (0x1F) not allowed", 0x1F, false},
	}

	// Add more control chars (0x00-0x08 except \t, and 0x0A-0x1F)
	for b := byte(0x00); b < 0x10; b++ {
		if b == '\t' {
			continue // already tested
		}
		cases = append(cases, testCase{
			name: "Control char not allowed: 0x" + string(
				"0123456789ABCDEF"[b>>4],
			) + string(
				"0123456789ABCDEF"[b&0xF],
			),
			input:  b,
			expect: false,
		})
	}
	for b := byte(0x0A); b < 0x20; b++ {
		cases = append(cases, testCase{
			name: "Control char not allowed: 0x" + string(
				"0123456789ABCDEF"[b>>4],
			) + string(
				"0123456789ABCDEF"[b&0xF],
			),
			input:  b,
			expect: false,
		})
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			testutil.AssertTrue(t, tc.expect == validQDTextByte(tc.input))
		})
	}
}
