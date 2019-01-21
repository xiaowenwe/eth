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

import (
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/metrics"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/errors"
	"github.com/syndtr/goleveldb/leveldb/filter"
	"github.com/syndtr/goleveldb/leveldb/iterator"
	"github.com/syndtr/goleveldb/leveldb/opt"
)

var OpenFileLimit = 64

type LDBDatabase struct {
	fn string      // filename for reporting
	db *leveldb.DB // LevelDB instance

	compTimeMeter  metrics.Meter // Meter for measuring the total time spent in database compaction用于测量数据库压缩所花费的总时间的仪表
	compReadMeter  metrics.Meter // Meter for measuring the data read during compaction压实过程中读取数据的测量仪表
	compWriteMeter metrics.Meter // Meter for measuring the data written during compaction测量压实过程中写入的数据的仪表
	diskReadMeter  metrics.Meter // Meter for measuring the effective amount of data read测量有效读取数据量的仪表
	diskWriteMeter metrics.Meter // Meter for measuring the effective amount of data written	测量有效写入数据量的仪表

	quitLock sync.Mutex      // Mutex protecting the quit channel access互斥保护退出信道访问
	quitChan chan chan error // Quit channel to stop the metrics collection before closing the database退出通道以在关闭数据库之前停止度量集合。

	log log.Logger // Contextual logger tracking the database path
}

// NewLDBDatabase returns a LevelDB wrapped object.NewLDBDatabase返回一个LevelDB包装对象
func NewLDBDatabase(file string, cache int, handles int) (*LDBDatabase, error) {
	logger := log.New("database", file)
	//确保我们有一些最低限度的缓存和文件保证
	// Ensure we have some minimal caching and file guarantees
	if cache < 16 {
		cache = 16
	}
	if handles < 16 {
		handles = 16
	}
	logger.Info("Allocated cache and file handles", "cache", cache, "handles", handles)

	// Open the db and recover any potential corruptions打开db并恢复任何潜在的腐败行为
	db, err := leveldb.OpenFile(file, &opt.Options{
		OpenFilesCacheCapacity: handles,             //打开文件数
		BlockCacheCapacity:     cache / 2 * opt.MiB, //块存储能力定义了“排序表”块缓存的容量
		WriteBuffer:            cache / 4 * opt.MiB, // Two of these are used internally
		Filter:                 filter.NewBloomFilter(10),
	})
	if _, corrupted := err.(*errors.ErrCorrupted); corrupted { //恢复文件恢复并打开一个数据库，其中包含给定路径的丢失或损坏的清单文件。它将忽略任何有效或无效的清单文件。
		// db必须已经存在，否则它将返回一个错误。此外，Recovery将忽略错误丢失和错误存在选项。Recovery文件使用标准文件系统支持的存储实现，如平面db/Package中所述。返回的db实例对并发使用是安全的。必须在使用后通过调用Close方法关闭db。
		db, err = leveldb.RecoverFile(file, nil)
	}
	// (Re)check for errors and abort if opening of the db failed检查错误并在数据库打开失败时中止
	if err != nil {
		return nil, err
	}
	return &LDBDatabase{
		fn:  file,
		db:  db,
		log: logger,
	}, nil
}

// Path returns the path to the database directory.路径返回到数据库目录的路径
func (db *LDBDatabase) Path() string {
	return db.fn
}

// Put puts the given key / value to the queue将给定的键/值放到队列中
func (db *LDBDatabase) Put(key []byte, value []byte) error {
	// Generate the data to write to disk, update the meter and write生成要写入磁盘的数据，更新仪表并写入
	//value = rle.Compress(value)

	return db.db.Put(key, value, nil)
}

//判断key是否存在
func (db *LDBDatabase) Has(key []byte) (bool, error) {
	return db.db.Has(key, nil)
}

// Get returns the given key if it's present.如果给定的key是存在的，GET会返回它
func (db *LDBDatabase) Get(key []byte) ([]byte, error) {
	// Retrieve the key and increment the miss counter if not found如果找不到，请检索键并增加漏计数器。
	dat, err := db.db.Get(key, nil)
	if err != nil {
		return nil, err
	}
	return dat, nil
	//return rle.Decompress(dat)
}

// Delete deletes the key from the queue and database从队列和数据库中删除密钥。
func (db *LDBDatabase) Delete(key []byte) error {
	// Execute the actual operation
	return db.db.Delete(key, nil)
}

//获取迭代器
func (db *LDBDatabase) NewIterator() iterator.Iterator {
	return db.db.NewIterator(nil, nil)
}

func (db *LDBDatabase) Close() {
	// Stop the metrics collection to avoid internal database races
	db.quitLock.Lock()
	defer db.quitLock.Unlock()

	if db.quitChan != nil {
		errc := make(chan error)
		db.quitChan <- errc
		if err := <-errc; err != nil {
			db.log.Error("Metrics collection failed", "err", err)
		}
	}
	err := db.db.Close()
	if err == nil {
		db.log.Info("Database closed")
	} else {
		db.log.Error("Failed to close database", "err", err)
	}
}

//返回数据库
func (db *LDBDatabase) LDB() *leveldb.DB {
	return db.db
}

// Meter configures the database metrics collectors and 仪表配置数据库度量收集器和
func (db *LDBDatabase) Meter(prefix string) {
	// Short circuit metering if the metrics system is disabled 如果指标系统被禁用，短路计量
	if !metrics.Enabled {
		return
	}
	// Initialize all the metrics collector at the requested prefix在请求的前缀处初始化所有指标收集器
	db.compTimeMeter = metrics.NewRegisteredMeter(prefix+"compact/time", nil)
	db.compReadMeter = metrics.NewRegisteredMeter(prefix+"compact/input", nil)
	db.compWriteMeter = metrics.NewRegisteredMeter(prefix+"compact/output", nil)
	db.diskReadMeter = metrics.NewRegisteredMeter(prefix+"disk/read", nil)
	db.diskWriteMeter = metrics.NewRegisteredMeter(prefix+"disk/write", nil)

	// Create a quit channel for the periodic collector and run it为定期收集器创建退出通道并运行它
	db.quitLock.Lock()
	db.quitChan = make(chan chan error)
	db.quitLock.Unlock()

	go db.meter(3 * time.Second)
}

// meter periodically retrieves internal leveldb counters and reports them to
// the metrics subsystem.米定期检索内部级别db计数器，并将它们报告给度量子系统。
//
// This is how a stats table look like (currently):这是stats表的样子(当前)
//   Compactions
//    Level |   Tables   |    Size(MB)   |    Time(sec)  |    Read(MB)   |   Write(MB)
//   -------+------------+---------------+---------------+---------------+---------------
//      0   |          0 |       0.00000 |       1.27969 |       0.00000 |      12.31098
//      1   |         85 |     109.27913 |      28.09293 |     213.92493 |     214.26294
//      2   |        523 |    1000.37159 |       7.26059 |      66.86342 |      66.77884
//      3   |        570 |    1113.18458 |       0.00000 |       0.00000 |       0.00000
//
// This is how the iostats look like (currently):这就是iostats的样子(当前)：
// Read(MB):3895.04860 Write(MB):3654.64712
func (db *LDBDatabase) meter(refresh time.Duration) {
	// Create the counters to store current and previous compaction values//创建计数器以存储当前和以前的压缩值
	compactions := make([][]float64, 2)
	for i := 0; i < 2; i++ {
		compactions[i] = make([]float64, 3)
	}
	// Create storage for iostats.
	var iostats [2]float64
	// Iterate ad infinitum and collect the stats
	for i := 1; ; i++ {
		// Retrieve the database stats
		stats, err := db.db.GetProperty("leveldb.stats")
		if err != nil {
			db.log.Error("Failed to read database stats", "err", err)
			return
		}
		// Find the compaction table, skip the header
		lines := strings.Split(stats, "\n")
		for len(lines) > 0 && strings.TrimSpace(lines[0]) != "Compactions" {
			lines = lines[1:]
		}
		if len(lines) <= 3 {
			db.log.Error("Compaction table not found")
			return
		}
		lines = lines[3:]

		// Iterate over all the table rows, and accumulate the entries
		for j := 0; j < len(compactions[i%2]); j++ {
			compactions[i%2][j] = 0
		}
		for _, line := range lines {
			parts := strings.Split(line, "|")
			if len(parts) != 6 {
				break
			}
			for idx, counter := range parts[3:] {
				value, err := strconv.ParseFloat(strings.TrimSpace(counter), 64)
				if err != nil {
					db.log.Error("Compaction entry parsing failed", "err", err)
					return
				}
				compactions[i%2][idx] += value
			}
		} //更新所有要求的仪表
		// Update all the requested meters
		if db.compTimeMeter != nil {
			db.compTimeMeter.Mark(int64((compactions[i%2][0] - compactions[(i-1)%2][0]) * 1000 * 1000 * 1000))
		}
		if db.compReadMeter != nil {
			db.compReadMeter.Mark(int64((compactions[i%2][1] - compactions[(i-1)%2][1]) * 1024 * 1024))
		}
		if db.compWriteMeter != nil {
			db.compWriteMeter.Mark(int64((compactions[i%2][2] - compactions[(i-1)%2][2]) * 1024 * 1024))
		}
		//检索数据库iostats。
		// Retrieve the database iostats.
		ioStats, err := db.db.GetProperty("leveldb.iostats")
		if err != nil {
			db.log.Error("Failed to read database iostats", "err", err)
			return
		}
		parts := strings.Split(ioStats, " ")
		if len(parts) < 2 {
			db.log.Error("Bad syntax of ioStats", "ioStats", ioStats)
			return
		}
		r := strings.Split(parts[0], ":")
		if len(r) < 2 {
			db.log.Error("Bad syntax of read entry", "entry", parts[0])
			return
		}
		read, err := strconv.ParseFloat(r[1], 64)
		if err != nil {
			db.log.Error("Read entry parsing failed", "err", err)
			return
		}
		w := strings.Split(parts[1], ":")
		if len(w) < 2 {
			db.log.Error("Bad syntax of write entry", "entry", parts[1])
			return
		}
		write, err := strconv.ParseFloat(w[1], 64)
		if err != nil {
			db.log.Error("Write entry parsing failed", "err", err)
			return
		}
		if db.diskReadMeter != nil {
			db.diskReadMeter.Mark(int64((read - iostats[0]) * 1024 * 1024))
		}
		if db.diskWriteMeter != nil {
			db.diskWriteMeter.Mark(int64((write - iostats[1]) * 1024 * 1024))
		}
		iostats[0] = read
		iostats[1] = write
		//睡眠一下，然后重复stats集合
		// Sleep a bit, then repeat the stats collection
		select {
		case errc := <-db.quitChan:
			// Quit requesting, stop hammering the database//退出请求，停止敲击数据库
			errc <- nil
			return

		case <-time.After(refresh):
			// Timeout, gather a new set of stats//timeout，收集一组新的统计数据
		}
	}
}

func (db *LDBDatabase) NewBatch() Batch {
	return &ldbBatch{db: db.db, b: new(leveldb.Batch)}
}

type ldbBatch struct {
	db   *leveldb.DB
	b    *leveldb.Batch
	size int
}

func (b *ldbBatch) Put(key, value []byte) error {
	b.b.Put(key, value)
	b.size += len(value)
	return nil
}

func (b *ldbBatch) Write() error {
	return b.db.Write(b.b, nil)
}

func (b *ldbBatch) ValueSize() int {
	return b.size
}

func (b *ldbBatch) Reset() {
	b.b.Reset()
	b.size = 0
}

type table struct {
	db     Database
	prefix string
}

//NewTable返回一个数据库对象，该对象以给定的/字符串作为所有键的前缀。
// NewTable returns a Database object that prefixes all keys with a given
// string.
func NewTable(db Database, prefix string) Database {
	return &table{
		db:     db,
		prefix: prefix,
	}
}

func (dt *table) Put(key []byte, value []byte) error {
	return dt.db.Put(append([]byte(dt.prefix), key...), value)
}

func (dt *table) Has(key []byte) (bool, error) {
	return dt.db.Has(append([]byte(dt.prefix), key...))
}

func (dt *table) Get(key []byte) ([]byte, error) {
	return dt.db.Get(append([]byte(dt.prefix), key...))
}

func (dt *table) Delete(key []byte) error {
	return dt.db.Delete(append([]byte(dt.prefix), key...))
}

func (dt *table) Close() {
	// Do nothing; don't close the underlying DB.
	//什么也不做；不要关闭基础DB。
}

type tableBatch struct {
	batch  Batch
	prefix string
}

//NewTableBatch返回一个批处理对象，该对象以给定的字符串作为所有键的前缀。
// NewTableBatch returns a Batch object which prefixes all keys with a given string.
func NewTableBatch(db Database, prefix string) Batch {
	return &tableBatch{db.NewBatch(), prefix}
}

func (dt *table) NewBatch() Batch {
	return &tableBatch{dt.db.NewBatch(), dt.prefix}
}

func (tb *tableBatch) Put(key, value []byte) error {
	return tb.batch.Put(append([]byte(tb.prefix), key...), value)
}

func (tb *tableBatch) Write() error {
	return tb.batch.Write()
}

func (tb *tableBatch) ValueSize() int {
	return tb.batch.ValueSize()
}

func (tb *tableBatch) Reset() {
	tb.batch.Reset()
}
