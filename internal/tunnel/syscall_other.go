//go:build !linux

package tunnel

import "os"

func syscallSIGTERM() os.Signal { return os.Interrupt }

func errFileNotExist(err error) bool { return os.IsNotExist(err) }
