// Copyright 2015 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.
//包abi实现了以太坊ABI（应用程序二进制文件）
//接口）。
//
//以太网ABI是强类型的，在编译时已知
//和静态这个ABI将处理基本型铸造;无符号
//签名，反之亦然。它不处理这样的切片铸造
//作为带符号切片的无符号切片。比特尺寸类型铸造也是
//处理位大小为32的整数将正确转换为int256，
//等
// Package abi implements the Ethereum ABI (Application Binary
// Interface).
//
// The Ethereum ABI is strongly typed, known at compile time
// and static. This ABI will handle basic type casting; unsigned
// to signed and visa versa. It does not handle slice casting such
// as unsigned slice to signed slice. Bit size type casting is also
// handled. ints with a bit size of 32 will be properly cast to int256,
// etc.
package abi
