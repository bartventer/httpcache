// Package testutil provides utility functions for testing in Go.
package testutil

import (
	"cmp"
	"errors"
	"reflect"
	"testing"
)

var ErrSample = errors.New("an error")

func AssertEqual[T cmp.Ordered](t *testing.T, expected, actual T, msgAndArgs ...interface{}) bool {
	t.Helper()
	if cmp.Compare(expected, actual) != 0 {
		t.Errorf("assertEqual failed: expected %v, got %v, %s", expected, actual, msgAndArgs)
		return false
	}
	return true
}

func AssertTrue(t *testing.T, condition bool, msgAndArgs ...interface{}) bool {
	t.Helper()
	if !condition {
		t.Errorf("assertTrue failed: condition is false, %s", msgAndArgs)
		return false
	}
	return true
}

func hasError(t *testing.T, err error) bool {
	t.Helper()
	return err != nil
}

func RequireError(t *testing.T, err error, msgAndArgs ...interface{}) {
	t.Helper()
	if !hasError(t, err) {
		t.Fatalf("expected error, got none, %s", msgAndArgs)
	}
}

func RequireNoError(t *testing.T, err error, msgAndArgs ...interface{}) {
	t.Helper()
	if hasError(t, err) {
		t.Fatalf("expected no error, got: %v, %s", err, msgAndArgs)
	}
}

func RequireErrorIs(t *testing.T, err error, target error, msgAndArgs ...interface{}) {
	t.Helper()
	if !errors.Is(err, target) {
		t.Fatalf("expected error %v, got %v, %s", target, err, msgAndArgs)
	}
}

func RequireTrue(t *testing.T, condition bool, msgAndArgs ...interface{}) {
	t.Helper()
	if !condition {
		t.Fatalf("expected condition to be true, got false, %s", msgAndArgs)
	}
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

func AssertNil(t *testing.T, object interface{}, msgAndArgs ...interface{}) bool {
	t.Helper()
	if !isNil(object) {
		t.Errorf("assertNil failed: expected nil, got %v, %s", object, msgAndArgs)
		return false
	}
	return true
}

func AssertNotNil(t *testing.T, object interface{}, msgAndArgs ...interface{}) bool {
	t.Helper()
	if isNil(object) {
		t.Errorf("assertNotNil failed: expected not nil, got nil, %s", msgAndArgs)
		return false
	}
	return true
}

func RequireNotNil(t *testing.T, object interface{}, msgAndArgs ...interface{}) {
	t.Helper()
	if isNil(object) {
		t.Fatalf("expected not nil, got nil, %s", msgAndArgs)
	}
}
