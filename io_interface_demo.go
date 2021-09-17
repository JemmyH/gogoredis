package main

type RedisServer struct {
	el EventLoopInterface
}

type EventLoopInterface interface {
	CreateEventLoop(setSize int) EventLoopInterface
	DeleteEventLoop(EventLoopInterface)
	Stop(EventLoopInterface) error
	Main(EventLoopInterface)
	ProcessEvents(IOMultiSelector, int)
	GetApiName() string
	//...

	// 文件事件
	CreateFileEvent(IOMultiSelector, int, int, func())
	DeleteFileEvent(IOMultiSelector)
}

type IOMultiSelector interface {
	AeApiCreate()
}

type LinuxIoMultiSelector struct{}
