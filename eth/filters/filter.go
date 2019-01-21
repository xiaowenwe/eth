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

package filters

import (
	"context"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/bloombits"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethdb"
	"github.com/ethereum/go-ethereum/event"
	"github.com/ethereum/go-ethereum/rpc"
)

//fiter.go里面定义了一个Filter对象。这个对象主要用来根据 区块的BloomIndexer和布隆过滤器等来执行日志的过滤功能。
//// 后端， 这个后端其实是在core里面实现的。 布隆过滤器的主要算法在core里面实现了。
/*
提供了 发布订阅模式的filter RPC。用来给rpc客户端提供实时的交易，区块，日志等的过滤
提供了 基于bloomIndexer的日志过滤模式，这种模式下，可以快速的对大量区块执行布隆过滤操作。 还提供了历史的日志的过滤操作。
*/

type Backend interface {
	ChainDb() ethdb.Database
	EventMux() *event.TypeMux
	HeaderByNumber(ctx context.Context, blockNr rpc.BlockNumber) (*types.Header, error)
	GetReceipts(ctx context.Context, blockHash common.Hash) (types.Receipts, error)
	GetLogs(ctx context.Context, blockHash common.Hash) ([][]*types.Log, error)

	SubscribeTxPreEvent(chan<- core.TxPreEvent) event.Subscription
	SubscribeChainEvent(ch chan<- core.ChainEvent) event.Subscription
	SubscribeRemovedLogsEvent(ch chan<- core.RemovedLogsEvent) event.Subscription
	SubscribeLogsEvent(ch chan<- []*types.Log) event.Subscription

	BloomStatus() (uint64, uint64)
	ServiceFilter(ctx context.Context, session *bloombits.MatcherSession)
}

// Filter can be used to retrieve and filter logs.
type Filter struct {
	backend Backend // 后端

	db         ethdb.Database   // 数据库
	begin, end int64            // 开始结束区块
	addresses  []common.Address // 筛选地址
	topics     [][]common.Hash  // 筛选主题

	matcher *bloombits.Matcher // 布隆过滤器的匹配器
}

//构造函数把address和topic都加入到filters容器。
// 然后构建了一个bloombits.NewMatcher(size, filters)。这个函数在core里面实现， 暂时不会讲解。
// New creates a new filter which uses a bloom filter on blocks to figure out whether
// a particular block is interesting or not.
// 这个是filter.go里面的代码，对于理解filters这个参数比较有用。 filter.go是Matcher的调用者。

// 可以看到无论有多少个addresses，在filters里面也只占一个位置。 filters[0]=addresses
// filters[1] = topics[0] = 多个topic
// filters[2] = topics[1] = 多个topic
// filters[n] = topics[n] = 多个topic

// filter 的参数addresses 和 topics 的过滤算法是， (含有addresses中任意一个address) 并且 (含有topics[0]里面的任意一个topic) 并且 (含有topics[1]里面任意一个topic) 并且 (含有topics[n]里面的任意一个topic)

// 可以看到 对于filter 实行的是  对第一维的数据 执行 与操作， 对于第二维度的数据， 执行或操作。

// 而在NewMatcher方法中，把第三维的具体数据转换成 布隆过滤器的指定三个位置。 所以在filter.go里面的var filters [][][]byte 在Matcher里面的filters变成了 [][][3]

func New(backend Backend, begin, end int64, addresses []common.Address, topics [][]common.Hash) *Filter {
	// Flatten the address and topic filter clauses into a single bloombits filter
	// system. Since the bloombits are not positional, nil topics are permitted,
	// which get flattened into a nil byte slice.
	var filters [][][]byte
	if len(addresses) > 0 {
		filter := make([][]byte, len(addresses))
		for i, address := range addresses {
			filter[i] = address.Bytes()
		}
		filters = append(filters, filter)
	}
	for _, topicList := range topics {
		filter := make([][]byte, len(topicList))
		for i, topic := range topicList {
			filter[i] = topic.Bytes()
		}
		filters = append(filters, filter)
	} //组装并返回过滤器
	// Assemble and return the filter
	size, _ := backend.BloomStatus()

	return &Filter{
		backend:   backend,
		begin:     begin,
		end:       end,
		addresses: addresses,
		topics:    topics,
		db:        backend.ChainDb(),
		matcher:   bloombits.NewMatcher(size, filters),
	}
}

//Logs 执行过滤
// Logs searches the blockchain for matching log entries, returning all from the
// first block that contains matches, updating the start of the filter accordingly.
func (f *Filter) Logs(ctx context.Context) ([]*types.Log, error) {
	// Figure out the limits of the filter range
	header, _ := f.backend.HeaderByNumber(ctx, rpc.LatestBlockNumber)
	if header == nil {
		return nil, nil
	}
	head := header.Number.Uint64()

	if f.begin == -1 {
		f.begin = int64(head)
	}
	end := uint64(f.end)
	if f.end == -1 {
		end = head
	}
	// Gather all indexed logs, and finish with non indexed ones
	var (
		logs []*types.Log
		err  error
	)
	size, sections := f.backend.BloomStatus()
	// indexed 是指创建了索引的区块的最大值。 如果过滤的范围落在了创建了索引的部分。
	// 那么执行索引搜索。
	if indexed := sections * size; indexed > uint64(f.begin) {
		if indexed > end {
			logs, err = f.indexedLogs(ctx, end)
		} else {
			logs, err = f.indexedLogs(ctx, indexed-1)
		}
		if err != nil {
			return logs, err
		}
	}
	// 对于剩下的部分执行非索引的搜索。
	rest, err := f.unindexedLogs(ctx, end)
	logs = append(logs, rest...)
	return logs, err
}

//索引搜索
// indexedLogs returns the logs matching the filter criteria based on the bloom
// bits indexed available locally or via the network.
func (f *Filter) indexedLogs(ctx context.Context, end uint64) ([]*types.Log, error) {
	// Create a matcher session and request servicing from the backend
	matches := make(chan uint64, 64)
	// 启动matcher
	session, err := f.matcher.Start(ctx, uint64(f.begin), end, matches)
	if err != nil {
		return nil, err
	}
	defer session.Close()
	// 进行过滤服务。 这些都在core里面。后续分析core的代码会进行分析。
	f.backend.ServiceFilter(ctx, session)
	//迭代匹配，直到用尽或关闭上下文
	// Iterate over the matches until exhausted or context closed
	var logs []*types.Log

	for {
		select {
		case number, ok := <-matches:
			// Abort if all matches have been fulfilled
			if !ok { // 没有接收到值并且channel已经被关闭
				err := session.Error()
				if err == nil {
					f.begin = int64(end) + 1 //更新begin。以便于下面的非索引搜索
				}
				return logs, err
			}
			f.begin = int64(number) + 1

			// Retrieve the suggested block and pull any truly matching logs
			header, err := f.backend.HeaderByNumber(ctx, rpc.BlockNumber(number))
			if header == nil || err != nil {
				return logs, err
			}
			found, err := f.checkMatches(ctx, header) //查找匹配的值
			if err != nil {
				return logs, err
			}
			logs = append(logs, found...)

		case <-ctx.Done():
			return logs, ctx.Err()
		}
	}
}

//unindexedLogs,非索引查询，循环遍历所有的区块。
// 首先用区块里面的header.Bloom来看是否有可能存在，如果有可能存在， 再使用checkMatches来检索所有的匹配。
// indexedLogs returns the logs matching the filter criteria based on raw block
// iteration and bloom matching.
func (f *Filter) unindexedLogs(ctx context.Context, end uint64) ([]*types.Log, error) {
	var logs []*types.Log

	for ; f.begin <= int64(end); f.begin++ {
		header, err := f.backend.HeaderByNumber(ctx, rpc.BlockNumber(f.begin))
		if header == nil || err != nil {
			return logs, err
		}
		if bloomFilter(header.Bloom, f.addresses, f.topics) {
			found, err := f.checkMatches(ctx, header)
			if err != nil {
				return logs, err
			}
			logs = append(logs, found...)
		}
	}
	return logs, nil
}

//checkMatches,拿到所有的收据，并从收据中拿到所有的日志。 执行filterLogs方法。
// checkMatches checks if the receipts belonging to the given header contain any log events that
// match the filter criteria. This function is called when the bloom filter signals a potential match.
func (f *Filter) checkMatches(ctx context.Context, header *types.Header) (logs []*types.Log, err error) {
	// Get the logs of the block
	logsList, err := f.backend.GetLogs(ctx, header.Hash())
	if err != nil {
		return nil, err
	}
	var unfiltered []*types.Log
	for _, logs := range logsList {
		unfiltered = append(unfiltered, logs...)
	}
	logs = filterLogs(unfiltered, nil, nil, f.addresses, f.topics)
	if len(logs) > 0 {
		// We have matching logs, check if we need to resolve full logs via the light client
		if logs[0].TxHash == (common.Hash{}) {
			receipts, err := f.backend.GetReceipts(ctx, header.Hash())
			if err != nil {
				return nil, err
			}
			unfiltered = unfiltered[:0]
			for _, receipt := range receipts {
				unfiltered = append(unfiltered, receipt.Logs...)
			}
			logs = filterLogs(unfiltered, nil, nil, f.addresses, f.topics)
		}
		return logs, nil
	}
	return nil, nil
}

func includes(addresses []common.Address, a common.Address) bool {
	for _, addr := range addresses {
		if addr == a {
			return true
		}
	}

	return false
}

//filterLogs,这个方法从给定的logs里面找到能够匹配上的。并返回。
// filterLogs creates a slice of logs matching the given criteria.
func filterLogs(logs []*types.Log, fromBlock, toBlock *big.Int, addresses []common.Address, topics [][]common.Hash) []*types.Log {
	var ret []*types.Log
Logs:
	for _, log := range logs {
		if fromBlock != nil && fromBlock.Int64() >= 0 && fromBlock.Uint64() > log.BlockNumber {
			continue
		}
		if toBlock != nil && toBlock.Int64() >= 0 && toBlock.Uint64() < log.BlockNumber {
			continue
		}

		if len(addresses) > 0 && !includes(addresses, log.Address) {
			continue
		}
		// If the to filtered topics is greater than the amount of topics in logs, skip.
		if len(topics) > len(log.Topics) {
			continue Logs
		}
		for i, topics := range topics {
			match := len(topics) == 0 // empty rule set == wildcard
			for _, topic := range topics {
				if log.Topics[i] == topic {
					match = true
					break
				}
			}
			if !match {
				continue Logs
			}
		}
		ret = append(ret, log)
	}
	return ret
}

func bloomFilter(bloom types.Bloom, addresses []common.Address, topics [][]common.Hash) bool {
	if len(addresses) > 0 {
		var included bool
		for _, addr := range addresses {
			if types.BloomLookup(bloom, addr) {
				included = true
				break
			}
		}
		if !included {
			return false
		}
	}

	for _, sub := range topics {
		included := len(sub) == 0 // empty rule set == wildcard
		for _, topic := range sub {
			if types.BloomLookup(bloom, topic) {
				included = true
				break
			}
		}
		if !included {
			return false
		}
	}
	return true
}
