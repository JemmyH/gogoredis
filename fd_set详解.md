本次调试机器：

```
$ uname -a
Linux localhost 4.15.0-52-generic #56-Ubuntu SMP Tue Jun 4 22:49:08 UTC 2019 x86_64 x86_64 x86_64 GNU/Linux
```

`fd_set` 的源码中有非常多的预编译指令：

```c
// /usr/include/x86_64-linux-gnu/sys/select.h
/* The fd_set member is required to be an array of longs.  */
typedef long int __fd_mask;

/* Some versions of <linux/posix_types.h> define this macros.  */
#undef __NFDBITS
/* It's easier to assume 8-bit bytes than to get CHAR_BIT.  */
#define __NFDBITS (8 * (int) sizeof (__fd_mask))
#define __FD_MASK(d) ((__fd_mask) (1UL << ((d) % __NFDBITS)))

/* fd_set for select and pselect.  */
typedef struct
  {
    /* XPG4.2 requires this member name.  Otherwise avoid the name
       from the global namespace.  */
#ifdef __USE_XOPEN
    __fd_mask fds_bits[__FD_SETSIZE / __NFDBITS];
# define __FDS_BITS(set) ((set)->fds_bits)
#else
    __fd_mask __fds_bits[__FD_SETSIZE / __NFDBITS];
# define __FDS_BITS(set) ((set)->__fds_bits)
#endif
  } fd_set;
```

我简单整理一下，去掉和本系统无关的，结果如下：

```c
typedef long int __fd_mask;  // 8
#define __NFDBITS (8 * (int) sizeof (__fd_mask)  // 64
#define __FD_SETSIZE  1024

typedef struct
{
    __fd_mask __fds_bits[__FD_SETSIZE / __NFDBITS];  // 长度为 1024/64=16，类型为 long 
} fd_set;

```

接着，我们看一个 demo：

```c
#include <stdio.h>
#include <sys/select.h>

void print_set1(fd_set *fdset);
void print_set2(fd_set *fdset);

int main()
{
    fd_set fdset;
    FD_ZERO(&fdset);

    printf("sizeof long int: %ld\n", sizeof(long int)); // 8
    printf("sizeof int: %ld\n", sizeof(int));           // 4
    printf("sizeof short: %ld\n", sizeof(short));       // 2

    FD_SET(1, &fdset);
    FD_SET(2, &fdset);
    FD_SET(3, &fdset);
    FD_SET(7, &fdset);
    print_set1(&fdset); // 10001110 -> 第 1 2 3 7 位分别设置成了 1
    FD_SET(15, &fdset);
    FD_SET(16, &fdset);
    FD_SET(31, &fdset); // 10000000000000011000000010001110 -> 长度为 32
    FD_SET(32, &fdset);
    FD_SET(33, &fdset);
    FD_SET(62, &fdset); // 100000000000000000000000000001110000000000000011000000010001110 0->长度为 63
    print_set2(&fdset);
    FD_SET(63, &fdset); // 1100000000000000000000000000001110000000000000011000000010001110 0 -> 长度为 64
    print_set2(&fdset);
    FD_SET(64, &fdset); // 1100000000000000000000000000001110000000000000011000000010001110 1 -> 长度还是 64，但产生了进位
    print_set2(&fdset);
    FD_SET(128, &fdset); // 1100000000000000000000000000001110000000000000011000000010001110 1 1-> 长度还是 63，但是在 64 和 128 的时候产生了进位
    FD_SET(129, &fdset); // 1100000000000000000000000000001110000000000000011000000010001110 1 3-> 长度还是 63，但是在 64 和 128 的时候产生了进位

    FD_SET(1023, &fdset); // 13835058070314647694 1 3 0 0 0 0 0 0 0 0 0 0 0 0 9223372036854775808
    FD_SET(1024, &fdset); // 13835058070314647694 1 3 0 0 0 0 0 0 0 0 0 0 0 0 9223372036854775808
    FD_SET(1025, &fdset); // 13835058070314647694 1 3 0 0 0 0 0 0 0 0 0 0 0 0 9223372036854775808
    print_set2(&fdset);

    int isset = FD_ISSET(7, &fdset); 
    printf("isset = %d\n", isset);// 输出 1，代表第 7 位被设置
    FD_CLR(7, &fdset);
    print_set2(&fdset);           // 1100000000000000000000000000001110000000000000011000000000001110 1 3 0 0 0 0 0 0 0 0 0 0 0 0 9223372036854775808 -> 第 7 位的 1 变成了 0
    isset = FD_ISSET(7, &fdset); 
    printf("isset = %d\n", isset);// 输出 0，代表第 7 位没有被设置
    return 0;
}

void print_set2(fd_set *fdset)
{
    int i;
    for (i = 0; i < 16; i++)
    {
        printf("%llu ", (unsigned long long)fdset->__fds_bits[i]);
    }
    printf("\n");
}

void print_set1(fd_set *fdset)
{
    int i;
    for (i = 0; i < 16; i++)
    {
        printf("%ld ",fdset->__fds_bits[i]);
    }
    printf("\n");
}
```

`print_set()` 函数是我自己添加的，用于打印出 `fd_set` 中的 `__fds_bits` 数组中的内容。需要注意两点：

1. 数组长度是 16，是如何确定的？答：在处理过预编译指令之后，`__FD_SETSIZE` 的值是 `1024`，`__NFDBITS` 的值是 `64`，计算得到数组长度为 16；

2. 类型的长度如何确定？答：在最开始使用了 `typedef long int __fd_mask`，`long int` 其实就是 `long`，即 `long signed integer type`。熟悉 C语言的同学都知道，当我们描述 `short`、`int` 和 `long` 的长度时，只有对 `short` 的长度是肯定的，而对后两者都使用了 **可能** 的说法：可能长度为 `8`。这是因为 C语言 没有对其做出严格的规定，只做了宽泛的限制：`short` 占 `2字节`；`int` 建议为机器字长，64 位机器下占`4字节`；`2 ≤ short ≤ int ≤ long`，如上述代码中打印结果所示，在我测试的这台机器上，`long` 占 `8字节` 即 `64位`。

接下来我们看 `main()` 中的代码：

在调用 `FD_SET()` 设置 1 2 3 7 后，我们调用`print_set1()`打印结果，输出：`142 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0`，数组第一位竟然是 `142`？！哪来的？但我们将 `142` 转成二进制就一下子了然了：`10001110`， 总共占 8 位，从右往左从 0 开始数，只有第 1 2 3 7 位被设置成了 1， 这个二进制对应的数就是 142，因为 142 完全在一个 `long` 的范围(64位)内，所以正常表示了。那如果我们对一个超过 `long` 范围的数调用 `FD_SET()`，会是什么效果？

代码继续走到 `FD_SET(62, &fdset)`，我们调用 `print_set1()`，输出 `4611686033459871886 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0`，`4611686033459871886` 转成二进制为 `100000000000000000000000000001110000000000000011000000010001110`，字符串长度为 63，可以看到，依旧在 `long` 的范围之内；执行到 `FD_SET(63,&fdset)` 呢，调用 `print_set1()`，输出 `-4611686003394903922 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0`，出现了负数。我们想一下原因。只执行`FD_SET(62,&fdset)` 会将第 63 位处设置为 1，对应的二进制为 `100000000000000000000000000000000000000000000000000000000000000`(长度为 63)；根据 `FD_SET()` 的功能，我们可以猜测，`FD_SET(63,&fdset)` 会将第 64 位设置为 1，其对应的二进制应该是 `1000000000000000000000000000000000000000000000000000000000000000`(长度为 64)。

> 在这里我们补充一个知识：参考这里 [Wiki-无符号数](https://zh.wikipedia.org/wiki/%E6%97%A0%E7%AC%A6%E5%8F%B7%E6%95%B0)，在计算机底层，有符号数可以表示特定范围内的整数(包括负数)，无符号数只能表示非负数(0 以及正数)，有符号数能够表示负数的原因是，最高位被用来当做符号位，1 表示负数，0 表示正数，代价就是所表示的数的范围少了一半，举个例子，8 位可以表示无符号数 [0,255](最小`00000000`；最大`11111111`，对应的十进制就是 255)，对应的有符号数范围就是 [-127,127]。

再回头看 `1000000000000000000000000000000000000000000000000000000000000000`，`__fd_mask` 的类型是 `long`，是一个有符号数，最高位的 1 表示负数，后面的才是真正的值，于是这个二进制转成有符号十进制的结果就是 `0`，而且还是个 `-0`。为了验证我们的想法，我们将 `print_set1()` 换成 `print_set2()`，这两个函数唯一的不同是，将数组中的每一位的类型强转成了无符号数，这下结果就成了 `9223372036854775808`，符合我们的预期。 所以后面的调试，我们都使用 `print_set2()` 这个函数。

刚才的 `FD_SET(63,&fdset)` 已经到达一个 `long` 的最大可 表示范围了，如果再多一位，会发生什么？我们看下 `FD_SET(64, &fdset)`，输出 `13835058070314647694 1 0 0 0 0 0 0 0 0 0 0 0 0 0 0`，`13835058070314647694` 转换成二进制后为 `1100000000000000000000000000001110000000000000011000000010001110`(长度为 64)，和 设置 63 时的一样，但数组的第二位变成了 1。按照前面的推测，单独执行 `FD_SET(64, &fdset)`，应该将第 65 位设置成 1，长度为 65，但是 65 显然超过了 `long` 的可表示的长度(64位)，于是，产生了“进位”，这个进位的基本单位就是 64位，即 `__NFDBITS` 的值。于是，可以用如下 python 代码表示(python 对大数处理非常好，一般不会出现溢出)：

```python
#!/usr/local/env python3
# coding:utf-8

# get_fd_set_bin 返回 fd_set 表示的真实二进制(从右往左方向)
# every_fd_bits 表示数组中每个元素代表多少位
# set_array 表示 fd_set 的 long 数组
def get_fd_set_bin(every_fd_bits, set_array):
    int_value = 0
    for idx in range(len(set_array)):
        int_value = int_value | (set_array[idx] << every_fd_bits * idx)
    return bin(int_value)[2:]  # 输出 "0bxxxxxx"，为了方便展示，去掉前缀


# print_bin 将二进制按照 step 为一组打印
def print_bin(output, step=64):
    le = len(output)
    m = le % step
    padding = step - m if m != 0 else 0
    output = output.zfill(le + padding)
    print(' '.join(''.join(output[idx * step:(idx + 1) * step]) for idx in range((le + padding) // step)))

```

在我们当前的例子中，`every_fd_bits = 64`, `set_array` 的长度为 16。测试一下我们的代码：

```bash
# 输入(相当于设置了 1 2 3 7)
get_fd_set_bin(64, [142,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0])
# 输出 
0000000000000000000000000000000000000000000000000000000010001110 

# 输入(相当于设置了 1 2 3 7 64)
get_fd_set_bin(64, [142,1,0,0,0,0,0,0,0,0,0,0,0,0,0,0])
# 输出
0000000000000000000000000000000000000000000000000000000000000001 0000000000000000000000000000000000000000000000000000000010001110
```
