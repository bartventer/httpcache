// Copyright (c) 2026 Bart Venter <72999113+bartventer@users.noreply.github.com>
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

// Package testutil provides utility functions for testing in Go.
package testutil

import (
	"cmp"
	"errors"
	"reflect"
)

var ErrSample = errors.New("an error")

type T interface {
	Helper()
	Errorf(format string, args ...interface{})
	Fatalf(format string, args ...interface{})
}

type testFunc func(format string, args ...interface{})

func (tf testFunc) do(fallback string, msgAndArgs ...interface{}) {
	if len(msgAndArgs) > 0 {
		if format, ok := msgAndArgs[0].(string); ok && len(msgAndArgs) > 1 {
			tf(format, msgAndArgs[1:]...)
		} else {
			tf("%v", msgAndArgs...)
		}
	} else {
		tf(fallback)
	}
}

func assert(t T, condition bool, msgAndArgs ...interface{}) bool {
	t.Helper()
	if !condition {
		testFunc(t.Errorf).do("assert failed", msgAndArgs...)
		return false
	}
	return true
}

func require(t T, condition bool, msgAndArgs ...interface{}) {
	t.Helper()
	if !condition {
		testFunc(t.Fatalf).do("require failed", msgAndArgs...)
	}
}

func AssertEqual[V cmp.Ordered](t T, expected, actual V, msgAndArgs ...interface{}) bool {
	t.Helper()
	got := cmp.Compare(expected, actual)
	return assert(
		t,
		got == 0,
		"assertEqual failed: expected %q, got %q, %s",
		expected,
		actual,
		msgAndArgs,
	)
}

func AssertTrue(t T, condition bool, msgAndArgs ...interface{}) bool {
	t.Helper()
	return assert(t, condition, "assertTrue failed: condition is false, %s", msgAndArgs)
}

func hasError(t T, err error) bool {
	t.Helper()
	return err != nil
}

func RequireError(t T, err error, msgAndArgs ...interface{}) {
	t.Helper()
	got := hasError(t, err)
	require(t, got, "requireError failed: expected error, got nil, %s", msgAndArgs)
}

func RequireNoError(t T, err error, msgAndArgs ...interface{}) {
	t.Helper()
	got := hasError(t, err)
	require(t, !got, "requireNoError failed: expected no error, got %v, %s", err, msgAndArgs)
}

func RequireErrorIs(t T, err error, target error, msgAndArgs ...interface{}) {
	t.Helper()
	got := errors.Is(err, target)
	require(t, got, "requireErrorIs failed: expected error %v, got %v, %s", target, err, msgAndArgs)
}

func RequireErrorAs(t T, err error, target interface{}, msgAndArgs ...interface{}) {
	t.Helper()
	got := errors.As(err, target)
	require(
		t,
		got,
		"requireErrorAs failed: expected error to be of type %T, got %v, %s",
		target,
		err,
		msgAndArgs,
	)
}

func RequireTrue(t T, condition bool, msgAndArgs ...interface{}) {
	t.Helper()
	require(
		t,
		condition,
		"requireTrue failed: expected condition to be true, got false, %s",
		msgAndArgs,
	)
}

// From github.com/stretchr/testify/assert
// Copyright (c) 2012-2020 Mat Ryer, Tyler Bunnell and contributors.
// Licensed under the MIT License (MIT).
func isNil(object interface{}) bool {
	if object == nil {
		return true
	}

	value := reflect.ValueOf(object)
	switch value.Kind() { //nolint:exhaustive // exhaustive is not needed here, as we handle all common cases
	case
		reflect.Chan, reflect.Func,
		reflect.Interface, reflect.Map,
		reflect.Ptr, reflect.Slice, reflect.UnsafePointer:

		return value.IsNil()
	}

	return false
}

func AssertNil(t T, object interface{}, msgAndArgs ...interface{}) bool {
	t.Helper()
	got := isNil(object)
	return assert(
		t,
		got,
		"assertNil failed: expected nil, got %v, %s",
		object,
		msgAndArgs,
	)
}

func AssertNotNil(t T, object interface{}, msgAndArgs ...interface{}) bool {
	t.Helper()
	got := !isNil(object)
	return assert(
		t,
		got,
		"assertNotNil failed: expected not nil, got nil, %s",
		msgAndArgs,
	)
}

func RequireNotNil(t T, object interface{}, msgAndArgs ...interface{}) {
	t.Helper()
	got := isNil(object)
	require(
		t,
		!got,
		"requireNotNil failed: expected not nil, got nil, %s",
		msgAndArgs,
	)
}

func RequirePanics(t T, f func(), msgAndArgs ...interface{}) bool {
	t.Helper()
	defer func() {
		got := recover()
		require(
			t,
			got != nil,
			"requirePanics failed: expected panic, got none, %s",
			msgAndArgs,
		)
	}()
	f()
	return true
}
