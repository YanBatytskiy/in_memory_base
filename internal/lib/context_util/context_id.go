// Package contextid carries per-request transaction identifiers (LSNs)
// through [context.Context] values.
package contextid

import (
	"context"
	"strconv"
)

// ctxKey is an unexported type so external packages cannot collide on the
// same key.
type ctxKey string

// TxIDKey is the context key under which a transaction id (LSN) is stored.
const TxIDKey ctxKey = "TxID"

// GetTxIDFromContext returns the LSN previously stored under [TxIDKey]. It
// returns 0 if the key is absent, the value is not a string, or the string
// is not a valid int64.
func GetTxIDFromContext(ctx context.Context) int64 {
	value := ctx.Value(TxIDKey)
	idStr, ok := (value.(string))
	if !ok {
		return 0
	}
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		return 0
	}

	return id
}
