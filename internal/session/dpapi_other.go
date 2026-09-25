//go:build !windows

package session

import "errors"

var errUnsupported = errors.New("storing the pathofexile.com session needs Windows")

func protect([]byte) ([]byte, error)   { return nil, errUnsupported }
func unprotect([]byte) ([]byte, error) { return nil, errUnsupported }
