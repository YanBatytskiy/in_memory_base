package compute

import "errors"

// Sentinel errors returned by [Compute.ParseAndValidate].
var (
	// ErrInvalidCommand is returned when the command name contains
	// characters other than ASCII letters.
	ErrInvalidCommand = errors.New("invalid syntax of command")

	// ErrInvalidArgument is returned when one of the argument tokens
	// contains characters outside the allowed set.
	ErrInvalidArgument = errors.New("invalid syntax of argument")

	// ErrEmptyCommand is returned when the input has no tokens.
	ErrEmptyCommand = errors.New("empty command")
)
