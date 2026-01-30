package contracts

import (
	"strings"
	"testing"
)

func TestRequire_PassesWhenTrue(t *testing.T) {
	// Should not panic
	Require(true, "this should not appear")
}

func TestRequire_PanicsWhenFalse(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Error("expected panic but got none")
			return
		}
		violation, ok := r.(ContractViolation)
		if !ok {
			t.Errorf("expected ContractViolation, got %T", r)
			return
		}
		if violation.Type != "precondition" {
			t.Errorf("expected precondition violation, got %s", violation.Type)
		}
		if !strings.Contains(violation.Message, "test message") {
			t.Errorf("expected message containing 'test message', got %s", violation.Message)
		}
	}()

	Require(false, "test message with %s", "formatting")
}

func TestRequireNotNil_PassesWhenNotNil(t *testing.T) {
	value := "not nil"
	RequireNotNil(&value, "value")
}

func TestRequireNotNil_PanicsWhenNil(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Error("expected panic but got none")
			return
		}
		violation, ok := r.(ContractViolation)
		if !ok {
			t.Errorf("expected ContractViolation, got %T", r)
			return
		}
		if !strings.Contains(violation.Message, "myValue cannot be nil") {
			t.Errorf("expected message about myValue, got %s", violation.Message)
		}
	}()

	RequireNotNil(nil, "myValue")
}

func TestRequireNotEmpty_PassesWhenNotEmpty(t *testing.T) {
	RequireNotEmpty("hello", "greeting")
}

func TestRequireNotEmpty_PanicsWhenEmpty(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Error("expected panic but got none")
			return
		}
		violation, ok := r.(ContractViolation)
		if !ok {
			t.Errorf("expected ContractViolation, got %T", r)
			return
		}
		if !strings.Contains(violation.Message, "greeting cannot be empty") {
			t.Errorf("expected message about greeting, got %s", violation.Message)
		}
	}()

	RequireNotEmpty("", "greeting")
}

func TestRequireOneOf_PassesWhenValueInSet(t *testing.T) {
	RequireOneOf("b", []string{"a", "b", "c"}, "value must be a, b, or c")
}

func TestRequireOneOf_PanicsWhenValueNotInSet(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Error("expected panic but got none")
			return
		}
		violation, ok := r.(ContractViolation)
		if !ok {
			t.Errorf("expected ContractViolation, got %T", r)
			return
		}
		if !strings.Contains(violation.Message, "value must be") {
			t.Errorf("expected message about valid values, got %s", violation.Message)
		}
	}()

	RequireOneOf("d", []string{"a", "b", "c"}, "value must be a, b, or c: got %s", "d")
}

func TestEnsure_PassesWhenTrue(t *testing.T) {
	Ensure(true, "this should not appear")
}

func TestEnsure_PanicsWhenFalse(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Error("expected panic but got none")
			return
		}
		violation, ok := r.(ContractViolation)
		if !ok {
			t.Errorf("expected ContractViolation, got %T", r)
			return
		}
		if violation.Type != "postcondition" {
			t.Errorf("expected postcondition violation, got %s", violation.Type)
		}
	}()

	Ensure(false, "postcondition failed")
}

func TestInvariant_PassesWhenTrue(t *testing.T) {
	Invariant(true, "this should not appear")
}

func TestInvariant_PanicsWhenFalse(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Error("expected panic but got none")
			return
		}
		violation, ok := r.(ContractViolation)
		if !ok {
			t.Errorf("expected ContractViolation, got %T", r)
			return
		}
		if violation.Type != "invariant" {
			t.Errorf("expected invariant violation, got %s", violation.Type)
		}
	}()

	Invariant(false, "invariant violated")
}

func TestAssert_PassesWhenTrue(t *testing.T) {
	Assert(true, "this should not appear")
}

func TestAssert_PanicsWhenFalse(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Error("expected panic but got none")
			return
		}
		violation, ok := r.(ContractViolation)
		if !ok {
			t.Errorf("expected ContractViolation, got %T", r)
			return
		}
		if violation.Type != "assertion" {
			t.Errorf("expected assertion violation, got %s", violation.Type)
		}
	}()

	Assert(false, "assertion failed")
}

func TestUnreachable_AlwaysPanics(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Error("expected panic but got none")
			return
		}
		violation, ok := r.(ContractViolation)
		if !ok {
			t.Errorf("expected ContractViolation, got %T", r)
			return
		}
		if violation.Type != "unreachable" {
			t.Errorf("expected unreachable violation, got %s", violation.Type)
		}
		if !strings.Contains(violation.Message, "should never happen") {
			t.Errorf("expected message about unreachable code, got %s", violation.Message)
		}
	}()

	Unreachable("should never happen: %d", 42)
}

func TestContractViolation_Error(t *testing.T) {
	v := ContractViolation{
		Type:     "precondition",
		Message:  "value cannot be empty",
		Location: "test.go:42",
	}

	err := v.Error()
	if !strings.Contains(err, "precondition") {
		t.Errorf("error should contain violation type: %s", err)
	}
	if !strings.Contains(err, "value cannot be empty") {
		t.Errorf("error should contain message: %s", err)
	}
	if !strings.Contains(err, "test.go:42") {
		t.Errorf("error should contain location: %s", err)
	}
}

func TestViolation_IncludesCallerLocation(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Error("expected panic but got none")
			return
		}
		violation, ok := r.(ContractViolation)
		if !ok {
			t.Errorf("expected ContractViolation, got %T", r)
			return
		}
		// Should contain file and line number
		if !strings.Contains(violation.Location, ".go:") {
			t.Errorf("expected location to contain .go:, got %s", violation.Location)
		}
	}()

	Require(false, "test")
}
