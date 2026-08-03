package interp

import "fmt"

func fmtErr(format string, args ...any) error {
	return fmt.Errorf(format, args...)
}
