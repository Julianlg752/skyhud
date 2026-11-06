//go:build !darwin && !windows

package secret

import "errors"

var errUnsupported = errors.New("keychain: not supported on this system")

type system struct{}

func (system) Get(string) (string, error) { return "", errUnsupported }
func (system) Set(string, string) error   { return errUnsupported }
func (system) Delete(string) error        { return errUnsupported }
