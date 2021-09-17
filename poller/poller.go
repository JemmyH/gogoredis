package poller

import "golang.org/x/sys/unix"


type Event int32

const (
	EventRead  Event = unix.EPOLLIN  // 0x1
	EventWrite Event = unix.EPOLLPRI // 0x2
	EventErr   Event = unix.EPOLLERR // 0x8
	EventNone  Event = 0

)
