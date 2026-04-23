// Package compute parses and validates the wire-level command language.
//
// The grammar is intentionally tiny: a single line of whitespace-separated
// tokens, where the first token is the command name (SET / GET / DEL, case
// insensitive) and the rest are arguments. See [ValidateCommand] and
// [ValidateArgument] for the allowed symbol sets.
package compute

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
)

//go:generate go run github.com/vektra/mockery/v3@v3.6.1 --config ../../../.mockery.yaml

// Compute is a stateless parser/validator. Safe for concurrent use.
type Compute struct {
	log *slog.Logger
}

// NewCompute builds a Compute bound to the given logger. It returns an error
// if log is nil.
func NewCompute(log *slog.Logger) (*Compute, error) {
	const op = "compute.NewCompute"

	if log == nil {
		return nil, fmt.Errorf("%s: logger is invalid", op)
	}

	return &Compute{log: log}, nil
}

// ParseAndValidate splits raw into whitespace-separated tokens, upper-cases
// the command name, and returns the tokens. It returns [ErrEmptyCommand] if
// the input is empty, [ErrInvalidCommand] if the command name contains
// non-letters, and [ErrInvalidArgument] if any token contains disallowed
// characters.
func (c *Compute) ParseAndValidate(_ context.Context, raw string) ([]string, error) {
	const op = "compute.parse"

	tokens := strings.Fields(strings.TrimSpace(raw))

	if len(tokens) == 0 {
		c.log.Debug("empty command", slog.String("operation", op))
		return nil, ErrEmptyCommand
	}

	tokens[0] = strings.ToUpper(tokens[0])

	ok := ValidateCommand(tokens[0])

	if !ok {
		c.log.Debug("invalid syntax of command", slog.String("operation", op), slog.String("token", tokens[0]))
		return nil, ErrInvalidCommand
	}

	for _, token := range tokens {
		ok := ValidateArgument(token)
		if !ok {
			c.log.Debug("invalid syntax of argument", slog.String("operation", op), slog.String("token", token))
			return nil, ErrInvalidArgument
		}
	}

	return tokens, nil
}
