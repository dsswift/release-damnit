// Package contracts provides assertion helpers for design-by-contract programming.
// These functions are used throughout the codebase to validate preconditions,
// postconditions, and invariants. They panic with detailed context when violated.
//
// Unlike traditional assertions that might be disabled in production, these
// contracts are always active because correctness matters more than performance
// for release infrastructure.
package contracts

import (
	"fmt"
	"runtime"
	"strings"
)

// Require validates a precondition. Call at the start of functions to validate inputs.
// Panics with a ContractViolation if the condition is false.
//
// Example:
//
//	func BumpVersion(current string, bumpType BumpType) string {
//	    contracts.Require(current != "", "current version cannot be empty")
//	    contracts.Require(semver.IsValid("v"+current), "invalid semver: %s", current)
//	    // ...
//	}
func Require(condition bool, format string, args ...interface{}) {
	if !condition {
		panic(newViolation("precondition", format, args...))
	}
}

// RequireNotNil validates that a value is not nil.
// Panics with a ContractViolation if the value is nil.
func RequireNotNil(value interface{}, name string) {
	if value == nil {
		panic(newViolation("precondition", "%s cannot be nil", name))
	}
}

// RequireNotEmpty validates that a string is not empty.
// Panics with a ContractViolation if the string is empty.
func RequireNotEmpty(value string, name string) {
	if value == "" {
		panic(newViolation("precondition", "%s cannot be empty", name))
	}
}

// RequireOneOf validates that a value is one of the allowed values.
// Panics with a ContractViolation if the value is not in the allowed set.
func RequireOneOf[T comparable](value T, allowed []T, format string, args ...interface{}) {
	for _, v := range allowed {
		if value == v {
			return
		}
	}
	panic(newViolation("precondition", format, args...))
}

// Ensure validates a postcondition. Call before returning from functions to validate outputs.
// Panics with a ContractViolation if the condition is false.
//
// Example:
//
//	func BumpVersion(current string, bumpType BumpType) string {
//	    // ... implementation
//	    contracts.Ensure(result != "", "result cannot be empty")
//	    contracts.Ensure(semver.IsValid("v"+result), "invalid result semver: %s", result)
//	    return result
//	}
func Ensure(condition bool, format string, args ...interface{}) {
	if !condition {
		panic(newViolation("postcondition", format, args...))
	}
}

// EnsureNotNil validates that a return value is not nil.
// Panics with a ContractViolation if the value is nil.
func EnsureNotNil(value interface{}, name string) {
	if value == nil {
		panic(newViolation("postcondition", "%s cannot be nil", name))
	}
}

// Invariant validates an invariant condition that must always be true.
// Use for validating internal state that should never be violated.
// Panics with a ContractViolation if the condition is false.
func Invariant(condition bool, format string, args ...interface{}) {
	if !condition {
		panic(newViolation("invariant", format, args...))
	}
}

// Assert is a general-purpose assertion for any logical condition.
// Use when the assertion doesn't fit cleanly into pre/postcondition categories.
// Panics with a ContractViolation if the condition is false.
func Assert(condition bool, format string, args ...interface{}) {
	if !condition {
		panic(newViolation("assertion", format, args...))
	}
}

// Unreachable marks code paths that should never be executed.
// Always panics with a ContractViolation.
//
// Example:
//
//	switch bumpType {
//	case Patch:
//	    // ...
//	case Minor:
//	    // ...
//	case Major:
//	    // ...
//	default:
//	    contracts.Unreachable("unknown bump type: %v", bumpType)
//	}
func Unreachable(format string, args ...interface{}) {
	panic(newViolation("unreachable", format, args...))
}

// ContractViolation represents a violated contract.
type ContractViolation struct {
	Type     string // "precondition", "postcondition", "invariant", "assertion", "unreachable"
	Message  string
	Location string // file:line
}

func (v ContractViolation) Error() string {
	return fmt.Sprintf("contract violation (%s) at %s: %s", v.Type, v.Location, v.Message)
}

// newViolation creates a new ContractViolation with caller information.
func newViolation(violationType string, format string, args ...interface{}) ContractViolation {
	message := fmt.Sprintf(format, args...)
	location := getCallerLocation(3) // Skip newViolation, the contract function, and the caller
	return ContractViolation{
		Type:     violationType,
		Message:  message,
		Location: location,
	}
}

// getCallerLocation returns the file:line of the caller at the given stack depth.
func getCallerLocation(skip int) string {
	_, file, line, ok := runtime.Caller(skip)
	if !ok {
		return "unknown"
	}
	// Simplify the file path to just the last two components
	parts := strings.Split(file, "/")
	if len(parts) > 2 {
		file = strings.Join(parts[len(parts)-2:], "/")
	}
	return fmt.Sprintf("%s:%d", file, line)
}
