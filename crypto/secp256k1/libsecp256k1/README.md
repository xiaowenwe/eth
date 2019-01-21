libsecp256k1
============

[![Build Status](https://travis-ci.org/bitcoin-core/secp256k1.svg?branch=master)](https://travis-ci.org/bitcoin-core/secp256k1)

Optimized C library for EC operations on curve secp256k1.

This library is a work in progress and is being used to research best practices. Use at your own risk.

Features:
* secp256k1 ECDSA signing/verification and key generation.
* Adding/multiplying private/public keys.
* Serialization/parsing of private keys, public keys, signatures.
* Constant time, constant memory access signing and pubkey generation.
* Derandomized DSA (via RFC6979 or with a caller provided function.)
* Very efficient implementation.

Implementation details
----------------------

* General
  * No runtime heap allocation.
  * Extensive testing infrastructure.
  * Structured to facilitate review and analysis.
  * Intended to be portable to any system with a C89 compiler and uint64_t support.
  * Expose only higher level interfaces to minimize the API surface and improve application security. ("Be difficult to use insecurely.")
* Field operations
  * Optimized implementation of arithmetic modulo the curve's field size (2^256 - 0x1000003D1).
    * Using 5 52-bit limbs (including hand-optimized assembly for x86_64, by Diederik Huys).
    * Using 10 26-bit limbs.
  * Field inverses and square roots using a sliding window over blocks of 1s (by Peter Dettman).
* Scalar operations
  * Optimized implementation without data-dependent branches of arithmetic modulo the curve's order.
    * Using 4 64-bit limbs (relying on __int128 support in the compiler).
    * Using 8 32-bit limbs.
* Group operations
  * Point addition formula specifically simplified for the curve equation (y^2 = x^3 + 7).
  * Use addition between points in Jacobian and affine coordinates where possible.
  * Use a unified addition/doubling formula where necessary to avoid data-dependent branches.
  * Point/x comparison without a field inversion by comparison in the Jacobian coordinate space.
* Point multiplication for verification (a*P + b*G).
  * Use wNAF notation for point multiplicands.
  * Use a much larger window for multiples of G, using precomputed multiples.
  * Use Shamir's trick to do the multiplication with the public key and the generator simultaneously.
  * Optionally (off by default) use secp256k1's efficiently-computable endomorphism to split the P multiplicand into 2 half-sized ones.
* Point multiplication for signing
  * Use a precomputed table of multiples of powers of 16 multiplied with the generator, so general multiplication becomes a series of additions.
  * Access the table with branch-free conditional moves so memory access is uniform.
  * No data-dependent branches
  * The precomputed tables add and eventually subtract points for which no known scalar (private key) is known, preventing even an attacker with control over the private key used to control the data internally.

Build steps
-----------

libsecp256k1 is built using autotools:

    $ ./autogen.sh
    $ ./configure
    $ make
    $ ./tests
    $ sudo make install  # optional
ibsecp256k1
============

[！[建立状态]（https://travis-ci.org/bitcoin-core/secp256k1.svg?branch=master）]（https://travis-ci.org/bitcoin-core/secp256k1）

曲线secp256k1上EC操作的优化C库。

该库正在进行中，正在用于研究最佳实践。使用风险由您自己承担。

特征：
* secp256k1 ECDSA签名/验证和密钥生成。
*添加/增加私钥/公钥。
*私钥，公钥，签名的序列化/解析。
*恒定时间，恒定内存访问签名和pubkey生成。
*去随机化DSA（通过RFC6979或使用调用者提供的函数。）
*非常有效的实施。

实施细节
----------------------

* 一般
  *没有运行时堆分配。
  *广泛的测试基础设施。
  *结构化，便于审查和分析。
  *旨在通过C89编译器和uint64_t支持移植到任何系统。
  *仅暴露更高级别的接口以最小化API表面并提高应用程序安全性。 （“难以使用不安全。”）
*现场操作
  *优化算法的实现模拟曲线的字段大小（2 ^ 256 - 0x1000003D1）。
    *使用5个52位肢体（包括用于x86_64的手动优化组件，由Diederik Huys提供）。
    *使用10个26位肢体。
  *使用1s块的滑动窗口（由Peter Dettman制作）场反转和平方根。
*标量运算
  *优化的实现没有数据相关的算术分支模数曲线的顺序。
    *使用4个64位肢体（依赖于编译器中的__int128支持）。
    *使用8个32位肢体。
*集团业务
  *曲线方程专门简化的点加法公式（y ^ 2 = x ^ 3 + 7）。
  *尽可能在雅可比和仿射坐标之间使用点。
  *必要时使用统一的加法/加倍公式，以避免数据相关的分支。
  *在雅可比坐标空间中通过比较而没有场反演的点/ x比较。
*点乘以验证（a * P + b * G）。
  *对点被乘数使用wNAF表示法。
  *使用预先计算的倍数，对G的倍数使用更大的窗口。
  *使用Shamir的技巧同时使用公钥和生成器进行乘法运算。
  *可选（默认情况下关闭）使用secp256k1的高效可计算内同态来将P被乘数分成2个半大小的。
*用于签名的点乘法
  *使用16的倍数乘以生成器的预先计算表，因此一般乘法变为一系列加法。
  *使用无分支条件移动访问表，以便内存访问是统一的。
  *没有数据相关的分支
  *预先计算的表添加并最终减去未知已知标量（私钥）的点，甚至可以防止攻击者控制用于在内部控制数据的私钥。

构建步骤
-----------

libsecp256k1是使用autotools构建的：

    $ ./autogen.sh
    $ ./configure
    $ make
    $ ./tests
    $ sudo make install＃optional