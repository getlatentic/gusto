//go:build !darwin && !linux

package roomwork

import "errors"

func cloneFile(string, string) error { return errors.ErrUnsupported }
