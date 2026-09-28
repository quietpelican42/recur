package recur

import "errors"

// ErrInvalidRule reports a syntactically or semantically invalid recurrence
// rule (bad range, COUNT and UNTIL both set, and similar). Use errors.Is to
// test for it.
var ErrInvalidRule = errors.New("recur: invalid rule")

// ErrUnsupported reports a recurrence rule that is valid RFC 5545 but not
// implemented by this package, or a combination of parts the RFC forbids
// (e.g. BYWEEKNO outside FREQ=YEARLY). Never silently produces the wrong
// dates; always returns this error instead. Use errors.Is to test for it.
var ErrUnsupported = errors.New("recur: unsupported rule")
