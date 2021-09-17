> 本文档基于 redis 5.0.0 版本

## 草稿

### redis 为什么“快”？  

- 使用大量设计精妙的数据结构，如压缩列表、跳表等，这些数据结构在查询和存储方面做了极致优化；
- redis 是纯内存操作，只有非常少的非关键场景涉及磁盘 IO；
- 单线程模型，单线程无法利用多核，但是从另一个层面来说则避免了多线程频繁上下文切换，以及同步机制如锁带来的开销；
- I/O 多路复用，基于 epoll/select/kqueue 等 I/O 多路复用技术，实现高吞吐的网络 I/O；
- C 语言实现(但语言并不是主要原因)

redis 底层大部分都是使用的单线程来处理客户端的请求，只有少部分的耗时任务(比如持久化)才会 fork 出一个线程来帮助处理。

## 通过源码看单线程如何工作

我们的大致需要了解 4 个组成部分：
![](https://upload-images.jianshu.io/upload_images/5110077-50e77ea9037da34e.png?imageMogr2/auto-orient/strip|imageView2/2/w/772/format/webp)

I/O多路复用程序负责监听多个套接字，并向文件事件分派器传送那些产生了事件的套接字。 尽管多个文件事件可能会并发地出现，但I/O多路复用程序总是会将所有产生事 件的套接字都放到一个队列里面，然后通过这个队列，文件事件分派器有序的交给各个处理器处理处。

### 如何选择 I/O 多路复用技术？

在 `src/ae.c` 中，Redis 的事件驱动实现方式采用了 “实现与接口分离”。有点类似 `TCP/IP` 中的分层实现方式，即下层提供接口，上层调用底层提供的接口完成自己的功能，而不用关心底层的接口是如何实现的。

针对不同的操作系统，Redis 会自动选择对应的多路复用技术，主要用到了 `evport`、`epoll`、`kqueue` 和 `select`，优先级由高到低，性能也是由高到低：

```c
/* Include the best multiplexing layer supported by this system.
 * The following should be ordered by performances, descending. */
#ifdef HAVE_EVPORT
#include "ae_evport.c"  // Solaris 系统
#else
    #ifdef HAVE_EPOLL
    #include "ae_epoll.c" // Linux 系统 
    #else
        #ifdef HAVE_KQUEUE
        #include "ae_kqueue.c" // FreeBSD 和 MacOS
        #else
        #include "ae_select.c"  // 其他Unix平台, 兜底的
        #endif
    #endif
#endif
```

> 因为 select 函数是作为 POSIX 标准中的系统调用，在不同版本的操作系统上都会实现，所以将其作为保底方案：  
> Redis 会优先选择时间复杂度为 的 I/O 多路复用函数作为底层实现，包括 Solaries 10 中的 evport、Linux 中的 epoll 和 macOS/FreeBSD 中的 kqueue，上述的这些函数都使用了内核内部的结构，并且能够服务几十万的文件描述符。
>但是如果当前编译环境没有上述函数，就会选择 select 作为备选方案，由于其在使用时会扫描全部监听的描述符，所以其时间复杂度较差 ，并且只能同时服务 1024 个文件描述符，所以一般并不会以 select 作为第一方案使用。

`ae` 是 Redis 中 **事件驱动器** 的名字，目前还没有找到为什么叫 `ae`...

### I/O 多路复用的分层设计与实现

事件驱动有一个核心的数据结构类型：`aeEventLoop`，它就像一个事件管理器，对上层提供所有的 `ae` 的接口：

```c
// src/ae.h
// 这些就是ae上层代码提供的所有接口
// 上层代码只管调用，不用关心底层用了哪种IO多路复用方案
aeEventLoop *aeCreateEventLoop(int setsize);
void aeDeleteEventLoop(aeEventLoop *eventLoop);
void aeStop(aeEventLoop *eventLoop);
int aeProcessEvents(aeEventLoop *eventLoop, int flags);
int aeWait(int fd, int mask, long long milliseconds);
void aeMain(aeEventLoop *eventLoop);
char *aeGetApiName(void);
void aeSetBeforeSleepProc(aeEventLoop *eventLoop, aeBeforeSleepProc *beforesleep);
void aeSetAfterSleepProc(aeEventLoop *eventLoop, aeBeforeSleepProc *aftersleep);
int aeGetSetSize(aeEventLoop *eventLoop);
int aeResizeSetSize(aeEventLoop *eventLoop, int setsize)

// 文件事件相关
int aeCreateFileEvent(aeEventLoop *eventLoop, int fd, int mask,
        aeFileProc *proc, void *clientData);
void aeDeleteFileEvent(aeEventLoop *eventLoop, int fd, int mask);
int aeGetFileEvents(aeEventLoop *eventLoop, int fd);

// 时间事件相关
long long aeCreateTimeEvent(aeEventLoop *eventLoop, long long milliseconds, aeTimeProc *proc, void *clientData, aeEventFinalizerProc *finalizerProc);
int aeDeleteTimeEvent(aeEventLoop *eventLoop, long long id);
```

事件驱动器在 Redis 中全局唯一，创建后放在 `redisServer.el` 中；`redisServer` 也是全局唯一，名字就叫 `server`：

```c
// src/server.h
struct redisServer {
  // ...
  aeEventLoop *el
  // ...
}

// src/server.c
void initServer(void) {
  // ...
  // 传参是最大允许多少个client建立连接
  server.el = aeCreateEventLoop(server.maxclients+CONFIG_FDSET_INCR)
  // ...
}
```

不同的 `I/O复用`方案经过封装，对 `ae` 提供一套一致的接口。对应的实现分别是 `src/ae_`,``,`` 。以 `ae_epoll.c` 举例：

```c
#include <sys/epoll.h>

typedef struct aeApiState {
    int epfd;
    struct epoll_event *events;
} aeApiState;

// 类似构造函数
static int aeApiCreate(aeEventLoop *eventLoop) {}
// 调整大小
static int aeApiResize(aeEventLoop *eventLoop, int setsize) {}
// 类似析构函数，释放资源
static void aeApiFree(aeEventLoop *eventLoop) {}
// 针对某个文件描述符添加关注的事件
static int aeApiAddEvent(aeEventLoop *eventLoop, int fd, int mask) {}
// 删除关心的文件描述符 或 删除文件描述符上某个事件
static void aeApiDelEvent(aeEventLoop *eventLoop, int fd, int delmask) {}
// 带timeout形式，阻塞等待获取可以读/写的文件描述符
static int aeApiPoll(aeEventLoop *eventLoop, struct timeval *tvp) {}
// 获取IO复用函数名称
static char *aeApiName(void) {}
```

> `aeApiState` 是每一个 I/O 多路复用实现所需要的参数组成的结构体。每一个 I/O 实现不同，所需要的参数也不同，为了统一以及方便，每个实现中都定义了同名的 `aeApiState` 结构体。

所以，最终我们看到的是这样一个分层的结构：

1. (封装好的) I/O 多路复用函数对上层的 `ae` 提供服务；
2. (封装好的) `ae` 对 `redisServer` 提供服务;
3. 上层调用下层提供的api，完成自己的功能，并不关心下层如何实现。

### 事件循环器 aeEventLoop 干了什么

这个结构体很重要，前面说到的 I/O复用函数接口 和 ae接口，参数中都有这个结构体：

```c
/* State of an event based program */
// 保存待处理事件(文件事件和时间事件)的结构体，里面保存了大量的时间执行的上下文信息
typedef struct aeEventLoop {
    int maxfd;   /* highest file descriptor currently registered */  // 当前注册的最大文件描述符
    int setsize; /* max number of file descriptors tracked */ // 关注的文件描述符上限
    long long timeEventNextId;  // 保存事件事件链表的下一个 id，递增，几乎将单链表当成一个指针来用，这也是无序单链表不影响性能的原因
    time_t lastTime;     /* Used to detect system clock skew */  // 用于帮助检测系统事件是否发生了改变
    aeFileEvent *events; /* Registered events */  // 监听的文件事件列表
    aeFiredEvent *fired; /* Fired events */  // 待处理的 事件就绪有读写IO的 **文件事件** 列表
    aeTimeEvent *timeEventHead; // 监听的时间事件列表
    int stop;  // 是否停止事件循环
    void *apidata; /* This is used for polling API specific data */  // 不同的IO复用函数，poll方法需要参数类型不一样。apidata专门放置这些传参类型
    aeBeforeSleepProc *beforesleep;  // 事件循环器 新一轮循环前的钩子函数
    aeBeforeSleepProc *aftersleep;  // 事件循环器 一轮循环后的钩子函数
} aeEventLoop;
```

它实际上就是一个事件循环器。Redis 中支持两种事件类型：时间事件 和 文件事件。
> 详情请参考：[redis 中的事件(时间事件和文件事件)到底是什么？](https://github.com/JemmyH/gogoredis/issues/2)

#### 1. 启动

在 `src/server.c` 中经过一系列的初始化之后，通过下面的代码启动时间调度：

```c
int main(int argc, char **argv) {
    // ...
    aeSetBeforeSleepProc(server.el, beforeSleep);
    aeSetAfterSleepProc(server.el, afterSleep);
    aeMain(server.el);  // 启动单线程网络模型，死循环，直到服务停止；退出循环意味着服务停止
    aeDeleteEventLoop(server.el);
    return 0;
}
```

再来看 `aeMain`，它在 `src/ae.c` 中：

```c
// redis服务器启动之后，会调用此方法
void aeMain(aeEventLoop *eventLoop) {
    eventLoop->stop = 0;
    // 这是一个死循环，一直到 redis-server 停止
    while (!eventLoop->stop) {
        if (eventLoop->beforesleep != NULL)
            eventLoop->beforesleep(eventLoop);
        aeProcessEvents(eventLoop, AE_ALL_EVENTS,AE_CALL_AFTER_SLEEP); 
    }
}
```

接下来就是 `int aeProcessEvents(aeEventLoop *eventLoop, int flags)` 函数。
这个函数比较长，但是我也添加了详细的注释：

```c
int aeProcessEvents(aeEventLoop *eventLoop, int flags)
{
    int processed = 0, numevents;

    /* Nothing to do? return ASAP */
    // 时间事件 和 文件事件 都不在 flag 中，不处理
    if (!(flags & AE_TIME_EVENTS) && !(flags & AE_FILE_EVENTS)) return 0;

    /* Note that we want call select() even if there are no
     * file events to process as long as we want to process time
     * events, in order to sleep until the next time event is ready
     * to fire. */
    // 注意，只要我们想处理 时间事件，即使没有对应的 文件事件 需要处理，
    // 我们也要调用 select()，以便在下一个 时间事件 准备好之前休眠等待
    if (eventLoop->maxfd != -1 ||
        ((flags & AE_TIME_EVENTS) && !(flags & AE_DONT_WAIT))) {
        int j;
        aeTimeEvent *shortest = NULL;
        // 计算 I/O 多路复用的等待时间 tvp
        struct timeval tv, *tvp;

        // 获取最近的 时间事件，主要是为了得到应该阻塞多久
        if (flags & AE_TIME_EVENTS && !(flags & AE_DONT_WAIT))
            shortest = aeSearchNearestTimer(eventLoop);

        if (shortest) {
            // 如果有需要处理的时间事件
            // 那么根据最近可执行时间事件和现在时间的时间差来决定文件事件的阻塞时间
            // 并将这个结果保存在 tcp 这个结构中
            long now_sec, now_ms;
            aeGetTime(&now_sec, &now_ms);
            tvp = &tv;

            /* How many milliseconds we need to wait for the next
             * time event to fire? */
            long long ms =
                (shortest->when_sec - now_sec)*1000 +
                shortest->when_ms - now_ms;

            if (ms > 0) {
                // 对应的时间事件还没到达，则阻塞对应的到达时间
                tvp->tv_sec = ms/1000;
                tvp->tv_usec = (ms % 1000)*1000;
            } else {
                // 时间差小于 0，说明时间已经可以执行了，那么可以无阻塞地调用文件事件的等待函数
                tvp->tv_sec = 0;
                tvp->tv_usec = 0;
            }
        } else {
            /* If we have to check for events but need to return
             * ASAP because of AE_DONT_WAIT we need to set the timeout
             * to zero */
            // 没有时间事件
            // 根据 AE_DONT_WAIT flag 来判断文件事件是否阻塞
            if (flags & AE_DONT_WAIT) {
                // 设置了 dnot_wait flag，则文件事件不阻塞
                tv.tv_sec = tv.tv_usec = 0;
                tvp = &tv;
            } else {
                /* Otherwise we can block */
                // 否则文件事件可以阻塞到一直有事件到达为止
                tvp = NULL; /* wait forever */
            }
        }

        /* Call the multiplexing API, will return only on timeout or when
         * some event fires. */
        // 计算完 最近的时间事件发生所需要等待的事件 tvp 之后，
        // 调用 aeApiPoll 在这段事件中等待事件的发生，在这段时间中如果发生了文件事件，优先处理文件事件，否则就会一直等待，直到最近的时间事件发生。
        // 获取已经就绪的事件数组
        numevents = aeApiPoll(eventLoop, tvp);

        /* After sleep callback. */
        if (eventLoop->aftersleep != NULL && flags & AE_CALL_AFTER_SLEEP)
            eventLoop->aftersleep(eventLoop);

        for (j = 0; j < numevents; j++) {
            // 从已就绪数组中获取事件
            aeFileEvent *fe = &eventLoop->events[eventLoop->fired[j].fd];
            int mask = eventLoop->fired[j].mask;
            int fd = eventLoop->fired[j].fd;
            int fired = 0; /* Number of events fired for current fd. */

            /* Normally we execute the readable event first, and the writable
             * event laster. This is useful as sometimes we may be able
             * to serve the reply of a query immediately after processing the
             * query.
             *
             * However if AE_BARRIER is set in the mask, our application is
             * asking us to do the reverse: never fire the writable event
             * after the readable. In such a case, we invert the calls.
             * This is useful when, for instance, we want to do things
             * in the beforeSleep() hook, like fsynching a file to disk,
             * before replying to a client. */
            // 正常情况下，一个文件句柄上同时有读事件和写事件时，应该先处理读事件，再处理写事件
            // 但如果设置了 AE_BARRIER 标志，我们应该反过来：千万不要在处理读事件之后才处理写事件
            int invert = fe->mask & AE_BARRIER;

            /* Note the "fe->mask & mask & ..." code: maybe an already
             * processed event removed an element that fired and we still
             * didn't processed, so we check if the event is still valid.
             *
             * Fire the readable event if the call sequence is not
             * inverted. */
            // 触发读事件
            // 注意这个 invert 判断条件，invert > 0表示应该先处理写事件，那么此处的 !invert 就不会命中，不会先执行读事件了
            if (!invert && fe->mask & mask & AE_READABLE) {
                fe->rfileProc(eventLoop,fd,fe->clientData,mask);
                fired++;
            }

            /* Fire the writable event. */
            // 触发写事件
            if (fe->mask & mask & AE_WRITABLE) {
                if (!fired || fe->wfileProc != fe->rfileProc) {
                    fe->wfileProc(eventLoop,fd,fe->clientData,mask);
                    fired++;
                }
            }

            /* If we have to invert the call, fire the readable event now
             * after the writable one. */
            // 再次处理因为 invert 而没处理的读事件
            if (invert && fe->mask & mask & AE_READABLE) {
                if (!fired || fe->wfileProc != fe->rfileProc) {
                    fe->rfileProc(eventLoop,fd,fe->clientData,mask);
                    fired++;
                }
            }

            processed++;
        }
    }
    /* Check time events */
    // 执行事件事件
    if (flags & AE_TIME_EVENTS)
        processed += processTimeEvents(eventLoop);

    // 最后返回成功处理的事件个数
    return processed; /* return the number of processed file/time events */
}
```

主要干了这么几件事：

1. 先检查有没有等待执行的时间事件(定时任务)，离现在最近的一个时间事件还要多久才执行。
2. 如果有这样的时间事件，记录下还要多久执行它。取为timeout。
3. 带着这个timeout，通过aeApiPoll()阻塞等待可读写的IO事件。如果第一步找不到时间事件，这里就没有timeout了，一直阻塞直至有可读性的IO事件。
4. 从aeApiPoll()返回后，如果有IO事件的话。就挨个处理。对该IO事件是读还是写，都有flag标志。并且读写的回调处理函数，也通过aeCreateFileEvent()注册进来了。
5. 处理完所有IO事件后，就可以执行时间事件了。因为时间事件是定时任务，所有执行完毕后，还需要设置好下一次执行的时间点。
6. 结束了。可以开始新一轮循环。
