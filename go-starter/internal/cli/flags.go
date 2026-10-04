package cli

import (
	"fmt"
	"time"
)

// durationFlag accepts Go durations such as 30s or 1m30s. It records whether a
// value was provided so that unset flags never override the environment.
type durationFlag struct {
	value string
}

func (d *durationFlag) String() string {
	if d == nil {
		return ""
	}
	return d.value
}

func (d *durationFlag) Set(raw string) error {
	parsed, err := time.ParseDuration(raw)
	if err != nil {
		return fmt.Errorf("invalid duration %q: use a value such as 30s, 1m or 1m30s", raw)
	}
	if parsed <= 0 {
		return fmt.Errorf("invalid duration %q: must be greater than zero", raw)
	}
	d.value = parsed.String()
	return nil
}
