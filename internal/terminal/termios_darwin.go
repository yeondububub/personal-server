//go:build darwin

package terminal

import (
	"os"

	"golang.org/x/sys/unix"
)

func disableFlowControl(ptmx *os.File) {
	fd := int(ptmx.Fd())
	t, err := unix.IoctlGetTermios(fd, unix.TIOCGETA)
	if err != nil {
		return
	}
	t.Iflag &^= unix.IXON | unix.IXOFF
	_ = unix.IoctlSetTermios(fd, unix.TIOCSETA, t)
}
