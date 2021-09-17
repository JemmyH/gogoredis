package eventloop

import (
	"github.com/JemmyH/gogoredis/utils/spinlock"
	"go.uber.org/atomic"
)

type Socket interface {
}
type EventLoop struct {
	// 注册的事件数
	ConnCount atomic.Int64

	// 每个文件句柄与对应的回调函数的映射
	sockets map[int]Socket

	mu spinlock.SpinLock
	// 事件等待队列，新的事件加入 EventLoop 时，不会先执行，会先 append 到这个数组中
	taskQueueWait []func()

	// 就绪队列。当 epoll 被唤醒之后，会执行 doPendingFunc，这个函数中会将 taskQueueW 全部转移到 taskQueueR，taskQueueR 逐个执行完后会清空
	taskQueueReady []func()
}
