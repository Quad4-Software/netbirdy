//go:build !linux

package sandbox

func apply(Policy, Mode, bool) (Status, error) {
	return Status{}, nil
}
