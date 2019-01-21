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

package ethdb

// Code using batches should try to add this much data to the batch.使用批处理的代码应该尝试将这些数据添加到批处理中。这个数值是根据经验确定的。
// The value was determined empirically.
const IdealBatchSize = 100 * 1024

//putter包装由批处理和常规数据库支持的数据库写操作。
// Putter wraps the database write operation supported by both batches and regular databases.
type Putter interface {
	Put(key []byte, value []byte) error
}

//数据库包装所有数据库操作。 所有方法对于并发使用都是安全的
// Database wraps all database operations. All methods are safe for concurrent use.
type Database interface {
	Putter
	Get(key []byte) ([]byte, error)
	Has(key []byte) (bool, error)
	Delete(key []byte) error
	Close()
	NewBatch() Batch
}

//批处理是一个只写的数据库，它在调用写时提交对其主机数据库/的更改。不能同时使用批处理。
// Batch is a write-only database that commits changes to its host database
// when Write is called. Batch cannot be used concurrently.
type Batch interface {
	Putter
	ValueSize() int // amount of data in the batch批处理中的数据量
	Write() error
	// Reset resets the batch for reuse//RESET重置批次以重用
	Reset()
}
