// Copyright (c) 2012, Suryandaru Triandana <syndtr@gmail.com>
// All rights reserved.
//
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package iterator provides interface and implementation to traverse over
// contents of a database.
package iterator

import (
	"errors"

	"github.com/syndtr/goleveldb/leveldb/util"
)

var (
	ErrIterReleased = errors.New("leveldb/iterator: iterator released")
)

// IteratorSeeker is the interface that wraps the 'seeks method'.
type IteratorSeeker interface {
	// First moves the iterator to the first key/value pair. If the iterator
	// only contains one key/value pair then First and Last would moves
	// to the same key/value pair.
	// It returns whether such pair exist.
	First() bool

	// Last moves the iterator to the last key/value pair. If the iterator
	// only contains one key/value pair then First and Last would moves
	// to the same key/value pair.
	// It returns whether such pair exist.
	Last() bool

	// Seek moves the iterator to the first key/value pair whose key is greater
	// than or equal to the given key.
	// It returns whether such pair exist.
	//
	// It is safe to modify the contents of the argument after Seek returns.
	Seek(key []byte) bool

	// Next moves the iterator to the next key/value pair.
	// It returns whether the iterator is exhausted.
	Next() bool

	// Prev moves the iterator to the previous key/value pair.
	// It returns whether the iterator is exhausted.
	Prev() bool
}

// CommonIterator is the interface that wraps common iterator methods.
type CommonIterator interface {
	IteratorSeeker

	// util.Releaser is the interface that wraps basic Release method.
	// When called Release will releases any resources associated with the
	// iterator.Util.释放器是包裹基本释放方法的接口。	当被调用的释放将释放与迭代器相关联的任何资源时。
	util.Releaser

	// util.ReleaseSetter is the interface that wraps the basic SetReleaser
	// method.util.ReleaseSetter是包装基本SetReleaser方法的接口。
	util.ReleaseSetter

	// TODO: Remove this when ready.
	Valid() bool

	// Error returns any accumulated error. Exhausting all the key/value pairs
	// is not considered to be an error.
	Error() error
}

// Iterator iterates over a DB's key/value pairs in key order.
//当遇到错误时，任何“查找方法”都会返回false，并且不会产生键/值对。可以通过调用Error方法来查询错误。调用发布仍然是必要的。
// When encounter an error any 'seeks method' will return false and will
// yield no key/value pairs. The error can be queried by calling the Error
// method. Calling Release is still necessary.
//迭代器必须在使用后释放，但在耗尽之前不需要读取迭代器。
// An iterator must be released after use, but it is not necessary to read
// an iterator until exhaustion.此外，迭代器对于并发使用不一定是安全的，但是它是安全的。安全地同时使用多个迭代器，每个迭代器位于一个专用的goroutine中
// Also, an iterator is not necessarily safe for concurrent use, but it is
// safe to use multiple iterators concurrently, with each in a dedicated
// goroutine.
type Iterator interface {
	CommonIterator

	// Key returns the key of the current key/value pair, or nil if done.Key返回当前键/值对的键，如果完成，则返回零。
	// The caller should not modify the contents of the returned slice, and
	// its contents may change on the next call to any 'seeks method'.调用方不应该修改返回的片的内容，并且在下一次调用“查找方法”时，它的内容可能会改变。
	Key() []byte

	// Value returns the value of the current key/value pair, or nil if done.值返回当前键/值对的值，如果完成，则返回零。
	// The caller should not modify the contents of the returned slice, and调用方不应该修改返回的片的内容，并且在下一次调用“查找方法”时，它的内容可能会改变
	// its contents may change on the next call to any 'seeks method'.
	Value() []byte
}

// ErrorCallbackSetter is the interface that wraps basic SetErrorCallback
// method.
//
// ErrorCallbackSetter implemented by indexed and merged iterator.
type ErrorCallbackSetter interface {
	// SetErrorCallback allows set an error callback of the corresponding
	// iterator. Use nil to clear the callback.
	SetErrorCallback(f func(err error))
}

type emptyIterator struct {
	util.BasicReleaser
	err error
}

func (i *emptyIterator) rErr() {
	if i.err == nil && i.Released() {
		i.err = ErrIterReleased
	}
}

func (*emptyIterator) Valid() bool            { return false }
func (i *emptyIterator) First() bool          { i.rErr(); return false }
func (i *emptyIterator) Last() bool           { i.rErr(); return false }
func (i *emptyIterator) Seek(key []byte) bool { i.rErr(); return false }
func (i *emptyIterator) Next() bool           { i.rErr(); return false }
func (i *emptyIterator) Prev() bool           { i.rErr(); return false }
func (*emptyIterator) Key() []byte            { return nil }
func (*emptyIterator) Value() []byte          { return nil }
func (i *emptyIterator) Error() error         { return i.err }

// NewEmptyIterator creates an empty iterator. The err parameter can be
// nil, but if not nil the given err will be returned by Error method.
func NewEmptyIterator(err error) Iterator {
	return &emptyIterator{err: err}
}
