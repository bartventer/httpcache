package testutil

import (
	"reflect"
	"testing"
)

type MockT struct {
	HelperCalled bool
	ErrorfCalls  []struct {
		Format string
		Args   []interface{}
	}
	FatalfCalled bool
}

func (m *MockT) Helper() {
	m.HelperCalled = true
}

func (m *MockT) Errorf(format string, args ...interface{}) {
	m.ErrorfCalls = append(m.ErrorfCalls, struct {
		Format string
		Args   []interface{}
	}{format, args})
}

func (m *MockT) Fatalf(format string, args ...interface{}) {
	m.FatalfCalled = true
}

var _ T = (*MockT)(nil)

func Test_assert(t *testing.T) {
	t.Run("returns true when condition is true", func(t *testing.T) {
		mt := &MockT{}
		got := assert(mt, true)
		if !got {
			t.Error("expected assert to return true when condition is true")
		}
		if !mt.HelperCalled {
			t.Error("expected Helper to be called")
		}
		if len(mt.ErrorfCalls) != 0 {
			t.Error("expected Errorf not to be called")
		}
	})

	t.Run(
		"returns false and prints 'assert failed' when condition is false and no msgAndArgs",
		func(t *testing.T) {
			mt := &MockT{}
			got := assert(mt, false)
			if got {
				t.Error("expected assert to return false when condition is false")
			}
			if len(mt.ErrorfCalls) != 1 || mt.ErrorfCalls[0].Format != "assert failed" {
				t.Errorf(
					"expected Errorf to be called with 'assert failed', got %+v",
					mt.ErrorfCalls,
				)
			}
		},
	)

	t.Run(
		"returns false and prints formatted message when condition is false and format string + arg",
		func(t *testing.T) {
			mt := &MockT{}
			got := assert(mt, false, "fail: %s", "reason")
			if got {
				t.Error("expected assert to return false with format string")
			}
			if len(mt.ErrorfCalls) != 1 || mt.ErrorfCalls[0].Format != "fail: %s" ||
				!reflect.DeepEqual(mt.ErrorfCalls[0].Args, []interface{}{"reason"}) {
				t.Errorf(
					"expected Errorf to be called with format and arg, got %+v",
					mt.ErrorfCalls,
				)
			}
		},
	)

	t.Run(
		"returns false and prints message when condition is false and only one arg",
		func(t *testing.T) {
			mt := &MockT{}
			got := assert(mt, false, "fail message")
			if got {
				t.Error("expected assert to return false with single message")
			}
			if len(mt.ErrorfCalls) != 1 || mt.ErrorfCalls[0].Format != "%v" ||
				!reflect.DeepEqual(mt.ErrorfCalls[0].Args, []interface{}{"fail message"}) {
				t.Errorf("expected Errorf to be called with %%v and arg, got %+v", mt.ErrorfCalls)
			}
		},
	)
}
