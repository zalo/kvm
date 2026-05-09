//go:build linux

package tunnel

import "syscall"

func syscallSIGTERM() syscall.Signal { return syscall.SIGTERM }

// errFileNotExist matches the various ways exec.Start can wrap ENOENT.
func errFileNotExist(err error) bool {
	for e := err; e != nil; {
		if errno, ok := e.(syscall.Errno); ok && errno == syscall.ENOENT {
			return true
		}
		type unwrapper interface{ Unwrap() error }
		u, ok := e.(unwrapper)
		if !ok {
			return false
		}
		e = u.Unwrap()
	}
	return false
}
