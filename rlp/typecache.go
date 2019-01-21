// Copyright 2014 The go-ethereum Authors
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

package rlp

import (
	"fmt"
	"reflect"
	"strings"
	"sync"
)

//类型缓存， 类型缓存记录了类型->(编码器|解码器)的内容。
var (
	// //读写锁，用来在多线程的时候保护typeCache这个Map
	typeCacheMutex sync.RWMutex
	typeCache      = make(map[typekey]*typeinfo) ////核心数据结构，保存了类型->编解码器函数
)

type typeinfo struct { //存储了编码器和解码器函数
	decoder
	writer
}

// represents struct tags
type tags struct {
	// rlp:"nil" controls whether empty input results in a nil pointer.
	//rlp：“nil”控制空输入是否会产生零指针。
	nilOK bool
	// rlp:"tail" controls whether this field swallows additional list
	// elements. It can only be set for the last field, which must be
	// of slice type.
	//rlp：“尾部”控制此字段是否会吞服其他列表//要素。只能为最后一个字段设置，该字段必须是//切片类型。
	tail bool
	// rlp:"-" ignores fields.
	ignored bool
}

type typekey struct {
	reflect.Type
	// the key must include the struct tags because they
	// might generate a different decoder.
	//键必须包含结构标记，因为它们//可能生成不同的解码器。
	tags
}

type decoder func(*Stream, reflect.Value) error

type writer func(reflect.Value, *encbuf) error

//下面是用户如何获取编码器和解码器的函数
func cachedTypeInfo(typ reflect.Type, tags tags) (*typeinfo, error) {
	typeCacheMutex.RLock() //加读锁来保护，
	info := typeCache[typekey{typ, tags}]
	typeCacheMutex.RUnlock()
	if info != nil { ////如果成功获取到信息，那么就返回
		return info, nil
	}
	//否则加写锁 调用cachedTypeInfo1函数创建并返回， 这里需要注意的是在多线程环境下有可能多个线程同时调用到这个地方，所以当你进入cachedTypeInfo1方法的时候需要判断一下是否已经被别的线程先创建成功了。
	// not in the cache, need to generate info for this type.
	typeCacheMutex.Lock()
	defer typeCacheMutex.Unlock()
	return cachedTypeInfo1(typ, tags)
}

func cachedTypeInfo1(typ reflect.Type, tags tags) (*typeinfo, error) {
	key := typekey{typ, tags}
	info := typeCache[key]
	if info != nil { // 其他的线程可能已经创建成功了， 那么我们直接获取到信息然后返回
		// another goroutine got the write lock first
		return info, nil
	}
	//这个地方首先创建了一个值来填充这个类型的位置，避免遇到一些递归定义的数据类型形成死循环
	// put a dummmy value into the cache before generating.
	// if the generator tries to lookup itself, it will get
	// the dummy value and won't call itself recursively.
	typeCache[key] = new(typeinfo)
	info, err := genTypeInfo(typ, tags)
	if err != nil {
		// remove the dummy value if the generator fails
		delete(typeCache, key)
		return nil, err
	}
	*typeCache[key] = *info
	return typeCache[key], err
}

type field struct {
	index int
	info  *typeinfo
}

//这个函数定义了结构体的编码方式， 通过structFields方法得到了所有的字段的编码器， 然后返回一个方法，这个方法遍历所有的字段，每个字段调用其编码器方法。
//structFields函数遍历所有的字段，然后针对每一个字段调用cachedTypeInfo1。 可以看到这是一个递归的调用过程。
// 上面的代码中有一个需要注意的是f.PkgPath == "" 这个判断针对的是所有导出的字段， 所谓的导出的字段就是说以大写字母开头命令的字段
func structFields(typ reflect.Type) (fields []field, err error) {
	for i := 0; i < typ.NumField(); i++ {
		if f := typ.Field(i); f.PkgPath == "" { // exported
			tags, err := parseStructTag(typ, i)
			if err != nil {
				return nil, err
			}
			if tags.ignored {
				continue
			}
			info, err := cachedTypeInfo1(f.Type, tags)
			if err != nil {
				return nil, err
			}
			fields = append(fields, field{i, info})
		}
	}
	return fields, nil
}

func parseStructTag(typ reflect.Type, fi int) (tags, error) {
	f := typ.Field(fi)
	var ts tags
	for _, t := range strings.Split(f.Tag.Get("rlp"), ",") {
		switch t = strings.TrimSpace(t); t {
		case "":
		case "-":
			ts.ignored = true
		case "nil":
			ts.nilOK = true
		case "tail":
			ts.tail = true
			if fi != typ.NumField()-1 {
				return ts, fmt.Errorf(`rlp: invalid struct tag "tail" for %v.%s (must be on last field)`, typ, f.Name)
			}
			if f.Type.Kind() != reflect.Slice {
				return ts, fmt.Errorf(`rlp: invalid struct tag "tail" for %v.%s (field type is not slice)`, typ, f.Name)
			}
		default:
			return ts, fmt.Errorf("rlp: unknown struct tag %q on %v.%s", t, typ, f.Name)
		}
	}
	return ts, nil
}

//生成对应类型的编解码器函数
func genTypeInfo(typ reflect.Type, tags tags) (info *typeinfo, err error) {
	info = new(typeinfo)
	if info.decoder, err = makeDecoder(typ, tags); err != nil {
		return nil, err
	}
	if info.writer, err = makeWriter(typ, tags); err != nil {
		return nil, err
	}
	return info, nil
}

func isUint(k reflect.Kind) bool {
	return k >= reflect.Uint && k <= reflect.Uintptr
}
