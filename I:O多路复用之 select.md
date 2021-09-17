# select

## 概述

Linux 系统下，输入 `man 2 select`可以看到 `select` 函数原型如下：

```c
/* According to POSIX.1-2001, POSIX.1-2008 */
#include <sys/select.h>

int select(int nfds, fd_set *readfds, fd_set *writefds,
            fd_set *exceptfds, struct timeval *timeout);

/* A time value that is accurate to the nearest
   microsecond but also has a range of years.  */
struct timeval
{
  long tv_sec;  /* Seconds. 秒  */
  long tv_usec; /* Microseconds. 毫秒  */
}
```

`select()` 函数允许程序监视多个文件描述符，等待所监视的一个或多个文件描述符状态变成 `ready`——该文件描述符不再是阻塞状态，出现了 **读** 、**写** 、**异常** 中的某个 IO 操作。

再来看 `poll`，输入 `man 2 poll`：

```c
#include <poll.h>

/* Type used for the number of file descriptors.  */
typedef unsigned long int nfds_t

int poll(struct pollfd *fds, nfds_t nfds, int timeout);
// poll() performs a similar task to select(2): it waits for one of a set of file descriptors to become ready to perform I/O.

struct pollfd {
    int   fd;         /* file descriptor */
    short events;     /* requested events */
    short revents;    /* returned events */
};
```

简单来说是通过底层驱动对应设备文件的 `poll()` 来查询是否有可用资源(可读 或者 可写)，如果没有则睡眠。

额外地，我们需要先认识 `fd_set` 这个结构，详细了解请移步这里：[理解 fd_set 及其用法](https://github.com/JemmyH/gogoredis/issues/4)。

```c
void FD_CLR(int fd, fd_set *set);
int  FD_ISSET(int fd, fd_set *set);
void FD_SET(int fd, fd_set *set);
void FD_ZERO(fd_set *set);
```

简单来说，我们可以把这个 `fd_set` 当成一个长度为 1024 的字节数组，`FD_SET()` 会将对应位设置为 `1`，`FD_CLR()` 会将对应位设置为 `0`，`FD_ZERO()` 将全部位置为 `0`，`FD_ISSET()` 返回该位是否被设置成了 `1`。

## 认识 select 函数

看一下 `select()` 函数的参数：

- `nfds`：是一个整数值，代表要监控的最大文件描述符 `fd` + 1；
- 
