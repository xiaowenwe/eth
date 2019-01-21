// Copyright 2017 The go-ethereum Authors
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

package eth

import (
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/bitutil"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/bloombits"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethdb"
	"github.com/ethereum/go-ethereum/params"
)

const (
	// bloomServiceThreads is the number of goroutines used globally by an Ethereum
	// instance to service bloombits lookups for all running filters.
	bloomServiceThreads = 16

	// bloomFilterThreads is the number of goroutines used locally per filter to
	// multiplex requests onto the global servicing goroutines.
	bloomFilterThreads = 3

	// bloomRetrievalBatch is the maximum number of bloom bit retrievals to service
	// in a single batch.
	bloomRetrievalBatch = 16

	// bloomRetrievalWait is the maximum time to wait for enough bloom bit requests
	// to accumulate request an entire batch (avoiding hysteresis).
	bloomRetrievalWait = time.Duration(0)
)

//服务线程startBloomHandlers,这个方法是为了响应具体的查询请求， 给定指定的Section和bit来从levelDB里面查询然后返回出去。
// 单独看这里有点摸不着头脑。 这个方法的调用比较复杂。 涉及到core里面的很多逻辑。 这里先不细说了。 知道有这个方法就行了。
// startBloomHandlers starts a batch of goroutines to accept bloom bit database
// retrievals from possibly a range of filters and serving the data to satisfy.
func (eth *Ethereum) startBloomHandlers() {
	for i := 0; i < bloomServiceThreads; i++ {
		go func() {
			for {
				select {
				case <-eth.shutdownChan:
					return

				case request := <-eth.bloomRequests: // request是一个通道
					task := <-request //从通道里面获取一个task
					task.Bitsets = make([][]byte, len(task.Sections))
					for i, section := range task.Sections {
						head := core.GetCanonicalHash(eth.chainDb, (section+1)*params.BloomBitsBlocks-1)
						if compVector, err := core.GetBloomBits(eth.chainDb, task.Bit, section, head); err == nil {
							if blob, err := bitutil.DecompressBytes(compVector, int(params.BloomBitsBlocks)/8); err == nil {
								task.Bitsets[i] = blob
							} else {
								task.Error = err
							}
						} else {
							task.Error = err
						}
					}
					request <- task //通过request通道返回结果
				}
			}
		}()
	}
}

const (
	// bloomConfirms is the number of confirmation blocks before a bloom section is
	// considered probably final and its rotated bits are calculated.
	// bloomConfirms 用来表示确认区块数量， 表示经过这么多区块之后， bloom section被认为是已经不会更改了。

	bloomConfirms = 256

	// bloomThrottling is the time to wait between processing two consecutive index
	// sections. It's useful during chain upgrades to prevent disk overload.
	// bloomThrottling是处理两个连续索引段之间的等待时间。 在区块链升级过程中防止磁盘过载是很有用的。
	bloomThrottling = 100 * time.Millisecond
)

//BloomIndexer对象主要用户构建索引的过程，是core.ChainIndexer的一个接口实现，所以只实现了一些必须的接口。
// 对于创建索引的逻辑还在core.ChainIndexer里面
// BloomIndexer implements a core.ChainIndexer, building up a rotated bloom bits index
// for the Ethereum header bloom filters, permitting blazing fast filtering.
type BloomIndexer struct {
	size uint64 // section size to generate bloombits for

	db  ethdb.Database       // database instance to write index data and metadata into
	gen *bloombits.Generator // generator to rotate the bloom bits crating the bloom index

	section uint64      // Section is the section number being processed currently
	head    common.Hash // Head is the hash of the last header processed
}

// NewBloomIndexer returns a chain indexer that generates bloom bits data for the
// canonical chain for fast logs filtering.
func NewBloomIndexer(db ethdb.Database, size uint64) *core.ChainIndexer {
	backend := &BloomIndexer{
		db:   db,
		size: size,
	}
	//可以看到indexDb和chainDb实际是同一个数据库， 但是indexDb的每个key前面附加了一个BloomBitsIndexPrefix的前缀
	table := ethdb.NewTable(db, string(core.BloomBitsIndexPrefix))

	return core.NewChainIndexer(db, table, backend, size, bloomConfirms, bloomThrottling, "bloombits")
}

//Reset实现了ChainIndexerBackend的方法，启动一个新的section
// Reset implements core.ChainIndexerBackend, starting a new bloombits index
// section.
func (b *BloomIndexer) Reset(section uint64, lastSectionHead common.Hash) error {
	gen, err := bloombits.NewGenerator(uint(b.size))
	b.gen, b.section, b.head = gen, section, common.Hash{}
	return err
}

//Process实现了ChainIndexerBackend， 增加一个新的区块头到index
// Process implements core.ChainIndexerBackend, adding a new header's bloom into
// the index.
func (b *BloomIndexer) Process(header *types.Header) {
	b.gen.AddBloom(uint(header.Number.Uint64()-b.section*b.size), header.Bloom)
	b.head = header.Hash()
}

//Commit方法实现了ChainIndexerBackend，持久化并写入数据库。
// Commit implements core.ChainIndexerBackend, finalizing the bloom section and
// writing it out into the database.
func (b *BloomIndexer) Commit() error {
	batch := b.db.NewBatch()

	for i := 0; i < types.BloomBitLength; i++ {
		bits, err := b.gen.Bitset(uint(i))
		if err != nil {
			return err
		}
		core.WriteBloomBits(batch, uint(i), b.section, b.head, bitutil.CompressBytes(bits))
	}
	return batch.Write()
}
