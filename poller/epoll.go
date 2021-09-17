// +build linux

package poller

import (
	"go.uber.org/atomic"
	"golang.org/x/sys/unix"
)

var (
	wakeBytes = []byte{1, 0, 0, 0, 0, 0, 0, 0}
)

// Poller 是对 epoll 的封装
type Poller struct {
	// epoll 对应的文件句柄
	epfd int
	// eventfd 文件句柄，进程通信用途的 fd，用来唤醒 epoll，执行 epoll_wait 返回
	eventFD int
	// Poller 的运行状态
	running *atomic.Bool
}

// Create 创建一个 Poller
func Create() (*Poller, error) {
	// new epoll
	epfd, err := unix.EpollCreate1(0)
	if err != nil {
		return nil, err
	}

	// new eventfd
	eventFD0, _, err := unix.Syscall(unix.SYS_EVENTFD2, 0, 0, 0)
	if err != nil {
		_ = unix.Close(epfd)
		return nil, err
	}
	eventFD := int(eventFD0)

	// add eventfd to epoll
	err = unix.EpollCtl(epfd, unix.EPOLL_CTL_ADD, eventFD, &unix.EpollEvent{
		Events: unix.EPOLLIN,
		Fd:     int32(eventFD),
	})
	if err != nil {
		_ = unix.Close(epfd)
		_ = unix.Close(eventFD)
		return nil, err
	}

	return &Poller{
		epfd:    epfd,
		eventFD: eventFD,
		running: atomic.NewBool(false),
	}, nil
}

func (ep *Poller) Wakeup() error {
	_, err := unix.Write(ep.eventFD, wakeBytes)
	return err
}

func (ep *Poller) add(fd int, events int32) error {
	return unix.EpollCtl(ep.epfd, unix.E)
}

