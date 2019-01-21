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

// Contains the block download scheduler to collect download tasks and schedule
// them in an ordered, and throttled way.
//queue给downloader提供了调度功能和限流的功能。
// 通过调用Schedule/ScheduleSkeleton来申请对任务进行调度，然后调用ReserveXXX方法来领取调度完成的任务，
// 并在downloader里面的线程来执行，调用DeliverXXX方法把下载完的数据给queue。
// 最后通过WaitResults来获取已经完成的任务。
// 中间还有一些对任务的额外控制，ExpireXXX用来控制任务是否超时， CancelXXX用来取消任务。
package downloader

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/metrics"
	"gopkg.in/karalabe/cookiejar.v2/collections/prque"
)

//Schedule调用申请对一些区块头进行下载调度。可以看到做了一些合法性检查之后，把任务插入了blockTaskPool，receiptTaskPool，receiptTaskQueue，receiptTaskPool。 TaskPool是Map，用来记录header的hash是否存在。
//TaskQueue是优先级队列，优先级是区块的高度的负数， 这样区块高度越小的优先级越高，就实现了首先调度小的任务的功能。
var (
	blockCacheItems      = 8192             // Maximum number of blocks to cache before throttling the download
	blockCacheMemory     = 64 * 1024 * 1024 // Maximum amount of memory to use for block caching
	blockCacheSizeWeight = 0.1              // Multiplier to approximate the average block size based on past ones
)

var (
	errNoFetchesPending = errors.New("no fetches pending")
	errStaleDelivery    = errors.New("stale delivery")
)

// fetchRequest是当前正在运行的数据检索操作。
// fetchRequest is a currently running data retrieval operation.
type fetchRequest struct {
	Peer    *peerConnection // Peer to which the request was sent请求发送到的对等方
	From    uint64          // [eth/62] Requested chain element index (used for skeleton fills only)请求的链元素索引（仅用于骨架填充）
	Headers []*types.Header // [eth/62] Requested headers, sorted by request order请求的标头，按请求顺序排序
	Time    time.Time       // Time when the request was made请求的时间
}

// fetchResult是一个从数据获取器收集部分结果的结构，直到完成所有未完成的部分并且可以处理整个结果。
// fetchResult is a struct collecting partial results from data fetchers until
// all outstanding pieces complete and the result as a whole can be processed.
type fetchResult struct {
	Pending int         // Number of data fetches still pending仍在等待的数据提取数
	Hash    common.Hash // Hash of the header to prevent recalculating标题的哈希以防止重新计算

	Header       *types.Header
	Uncles       []*types.Header
	Transactions types.Transactions
	Receipts     types.Receipts
}

// queue表示需要获取或正在获取的哈希值
// queue represents hashes that are either need fetching or are being fetched
type queue struct {
	mode SyncMode // Synchronisation mode to decide on the block parts to schedule for fetching同步模式决定要调度获取的块部分
	//标题是“特殊的”，它们分批下载，由骨架链支持
	// Headers are "special", they download in batches, supported by a skeleton chain
	headerHead      common.Hash                    // [eth/62] Hash of the last queued header to verify order用于验证订单的最后一个排队标头的哈希值
	headerTaskPool  map[uint64]*types.Header       // [eth/62] Pending header retrieval tasks, mapping starting indexes to skeleton headers待标头检索任务，将起始索引映射到骨架标头
	headerTaskQueue *prque.Prque                   // [eth/62] Priority queue of the skeleton indexes to fetch the filling headers for骨架索引的优先级队列，用于获取填充标头
	headerPeerMiss  map[string]map[uint64]struct{} // [eth/62] Set of per-peer header batches known to be unavailable已知不可用的每个对等标头批次集
	headerPendPool  map[string]*fetchRequest       // [eth/62] Currently pending header retrieval operations目前正在等待标头检索操作
	headerResults   []*types.Header                // [eth/62] Result cache accumulating the completed headers结果缓存累积已完成的标头
	headerProced    int                            // [eth/62] Number of headers already processed from the results已从结果中处理的标头数
	headerOffset    uint64                         // [eth/62] Number of the first header in the result cache结果缓存中第一个标头的编号
	headerContCh    chan bool                      // [eth/62] Channel to notify when header download finishes标题下载完成时通知的通道
	//以下所有数据检索均基于已组装的标题链
	// All data retrievals below are based on an already assembles header chain
	blockTaskPool  map[common.Hash]*types.Header // [eth/62] Pending block (body) retrieval tasks, mapping hashes to headers挂起块（正文）检索任务，将哈希映射到标题
	blockTaskQueue *prque.Prque                  // [eth/62] Priority queue of the headers to fetch the blocks (bodies) for用于获取块（主体）的标头的优先级队列
	blockPendPool  map[string]*fetchRequest      // [eth/62] Currently pending block (body) retrieval operations目前正在进行块（正文）检索操作
	blockDonePool  map[common.Hash]struct{}      // [eth/62] Set of the completed block (body) fetches

	receiptTaskPool  map[common.Hash]*types.Header // [eth/63] Pending receipt retrieval tasks, mapping hashes to headers待收货检索任务，将哈希映射到标题
	receiptTaskQueue *prque.Prque                  // [eth/63] Priority queue of the headers to fetch the receipts for要获取收据的标头的优先级队列
	receiptPendPool  map[string]*fetchRequest      // [eth/63] Currently pending receipt retrieval operations目前待定收据检索操作
	receiptDonePool  map[common.Hash]struct{}      // [eth/63] Set of the completed receipt fetches已完成的收据集提取

	resultCache  []*fetchResult     // Downloaded but not yet delivered fetch results已下载但尚未提供获取结果
	resultOffset uint64             // Offset of the first cached fetch result in the block chain块链中第一个缓存的获取结果的偏移量
	resultSize   common.StorageSize // Approximate size of a block (exponential moving average)块的近似大小（指数移动平均线）

	lock   *sync.Mutex
	active *sync.Cond
	closed bool
}

// newQueue为调度块检索创建一个新的下载队列。
// newQueue creates a new download queue for scheduling block retrieval.
func newQueue() *queue {
	lock := new(sync.Mutex)
	return &queue{
		headerPendPool:   make(map[string]*fetchRequest),
		headerContCh:     make(chan bool),
		blockTaskPool:    make(map[common.Hash]*types.Header),
		blockTaskQueue:   prque.New(),
		blockPendPool:    make(map[string]*fetchRequest),
		blockDonePool:    make(map[common.Hash]struct{}),
		receiptTaskPool:  make(map[common.Hash]*types.Header),
		receiptTaskQueue: prque.New(),
		receiptPendPool:  make(map[string]*fetchRequest),
		receiptDonePool:  make(map[common.Hash]struct{}),
		resultCache:      make([]*fetchResult, blockCacheItems),
		active:           sync.NewCond(lock),
		lock:             lock,
	}
}

//重置清除队列内容。
// Reset clears out the queue contents.
func (q *queue) Reset() {
	q.lock.Lock()
	defer q.lock.Unlock()

	q.closed = false
	q.mode = FullSync

	q.headerHead = common.Hash{}
	q.headerPendPool = make(map[string]*fetchRequest)

	q.blockTaskPool = make(map[common.Hash]*types.Header)
	q.blockTaskQueue.Reset()
	q.blockPendPool = make(map[string]*fetchRequest)
	q.blockDonePool = make(map[common.Hash]struct{})

	q.receiptTaskPool = make(map[common.Hash]*types.Header)
	q.receiptTaskQueue.Reset()
	q.receiptPendPool = make(map[string]*fetchRequest)
	q.receiptDonePool = make(map[common.Hash]struct{})

	q.resultCache = make([]*fetchResult, blockCacheItems)
	q.resultOffset = 0
}

// Close表示同步结束，解锁WaitResults。
//即使队列已经关闭，也可以调用它。
// Close marks the end of the sync, unblocking WaitResults.
// It may be called even if the queue is already closed.
func (q *queue) Close() {
	q.lock.Lock()
	q.closed = true
	q.lock.Unlock()
	q.active.Broadcast()
}

// PendingHeaders检索待检索的头请求数。
// PendingHeaders retrieves the number of header requests pending for retrieval.
func (q *queue) PendingHeaders() int {
	q.lock.Lock()
	defer q.lock.Unlock()

	return q.headerTaskQueue.Size()
}

// PendingBlocks检索待检索的块（正文）请求数。
// PendingBlocks retrieves the number of block (body) requests pending for retrieval.
func (q *queue) PendingBlocks() int {
	q.lock.Lock()
	defer q.lock.Unlock()

	return q.blockTaskQueue.Size()
}

// PendingReceipts检索待检索的块收据数。
// PendingReceipts retrieves the number of block receipts pending for retrieval.
func (q *queue) PendingReceipts() int {
	q.lock.Lock()
	defer q.lock.Unlock()

	return q.receiptTaskQueue.Size()
}

// InFlightHeaders检索当前是否存在正在进行的标头提取请求。
// InFlightHeaders retrieves whether there are header fetch requests currently
// in flight.
func (q *queue) InFlightHeaders() bool {
	q.lock.Lock()
	defer q.lock.Unlock()

	return len(q.headerPendPool) > 0
}

// InFlightBlocks检索当前是否存在阻止获取请求。
// InFlightBlocks retrieves whether there are block fetch requests currently in
// flight.
func (q *queue) InFlightBlocks() bool {
	q.lock.Lock()
	defer q.lock.Unlock()

	return len(q.blockPendPool) > 0
}

// InFlightReceipts检索当前是否有飞行中的收据提取请求
// InFlightReceipts retrieves whether there are receipt fetch requests currently
// in flight.
func (q *queue) InFlightReceipts() bool {
	q.lock.Lock()
	defer q.lock.Unlock()

	return len(q.receiptPendPool) > 0
}

//如果队列完全空闲或者内部仍有一些数据，则空闲返回。
// Idle returns if the queue is fully idle or has some data still inside.
func (q *queue) Idle() bool {
	q.lock.Lock()
	defer q.lock.Unlock()

	queued := q.blockTaskQueue.Size() + q.receiptTaskQueue.Size()
	pending := len(q.blockPendPool) + len(q.receiptPendPool)
	cached := len(q.blockDonePool) + len(q.receiptDonePool)

	return (queued + pending + cached) == 0
}

// ShouldThrottleBlocks检查是否应该限制下载（活动块（正文）提取超过块缓存）。
// ShouldThrottleBlocks checks if the download should be throttled (active block (body)
// fetches exceed block cache).
func (q *queue) ShouldThrottleBlocks() bool {
	q.lock.Lock()
	defer q.lock.Unlock()

	return q.resultSlots(q.blockPendPool, q.blockDonePool) <= 0
}

// ShouldThrottleReceipts检查是否应该限制下载（活动收据提取超过块缓存）。
// ShouldThrottleReceipts checks if the download should be throttled (active receipt
// fetches exceed block cache).
func (q *queue) ShouldThrottleReceipts() bool {
	q.lock.Lock()
	defer q.lock.Unlock()

	return q.resultSlots(q.receiptPendPool, q.receiptDonePool) <= 0
}

// resultSlots计算可用于请求的结果槽数，同时同时遵守结果缓存的项目和内存限制。
// resultSlots calculates the number of results slots available for requests
// whilst adhering to both the item and the memory limit too of the results
// cache.
func (q *queue) resultSlots(pendPool map[string]*fetchRequest, donePool map[common.Hash]struct{}) int {
	// Calculate the maximum length capped by the memory limit//计算内存限制所限制的最大长度
	limit := len(q.resultCache)
	if common.StorageSize(len(q.resultCache))*q.resultSize > common.StorageSize(blockCacheMemory) {
		limit = int((common.StorageSize(blockCacheMemory) + q.resultSize - 1) / q.resultSize)
	}
	// Calculate the number of slots already finished//计算已完成的插槽数
	finished := 0
	for _, result := range q.resultCache[:limit] {
		if result == nil {
			break
		}
		if _, ok := donePool[result.Hash]; ok {
			finished++
		}
	}
	// Calculate the number of slots currently downloading//计算当前正在下载的插槽数
	pending := 0
	for _, request := range pendPool {
		for _, header := range request.Headers {
			if header.Number.Uint64() < q.resultOffset+uint64(limit) {
				pending++
			}
		}
	}
	// Return the free slots to distribute
	return limit - finished - pending
}

//Schedule方法传入的是已经fetch好的header。Schedule(headers []*types.Header, from uint64)。
// 而ScheduleSkeleton函数的参数是一个骨架， 然后请求对骨架进行填充。所谓的骨架是指我首先每隔192个区块请求一个区块头，然后把返回的header传入ScheduleSkeleton。
// 在Schedule函数中只需要queue调度区块体和回执的下载，而在ScheduleSkeleton函数中，还需要调度那些缺失的区块头的下载。
// ScheduleSkeleton adds a batch of header retrieval tasks to the queue to fill
// up an already retrieved header skeleton.
func (q *queue) ScheduleSkeleton(from uint64, skeleton []*types.Header) {
	q.lock.Lock()
	defer q.lock.Unlock()

	// No skeleton retrieval can be in progress, fail hard if so (huge implementation bug)
	if q.headerResults != nil {
		panic("skeleton assembly already in progress")
	}
	// 因为这个方法在skeleton为false的时候不会调用。 所以一些初始化工作放在这里执行。
	// Shedule all the header retrieval tasks for the skeleton assembly
	q.headerTaskPool = make(map[uint64]*types.Header)
	q.headerTaskQueue = prque.New()
	q.headerPeerMiss = make(map[string]map[uint64]struct{}) // Reset availability to correct invalid chains
	q.headerResults = make([]*types.Header, len(skeleton)*MaxHeaderFetch)
	q.headerProced = 0
	q.headerOffset = from
	q.headerContCh = make(chan bool, 1)

	for i, header := range skeleton {
		index := from + uint64(i*MaxHeaderFetch)
		// 每隔MaxHeaderFetch这么远有一个header
		q.headerTaskPool[index] = header
		q.headerTaskQueue.Push(index, -float32(index))
	}
}

//RetrieveHeaders，ScheduleSkeleton函数在上次调度还没有做完的情况下是不会调用的。
// 所以上次调用完成之后，会使用这个方法来获取结果，重置状态。
// RetrieveHeaders retrieves the header chain assemble based on the scheduled
// skeleton.
func (q *queue) RetrieveHeaders() ([]*types.Header, int) {
	q.lock.Lock()
	defer q.lock.Unlock()

	headers, proced := q.headerResults, q.headerProced
	q.headerResults, q.headerProced = nil, 0

	return headers, proced
}

// from表示headers里面第一个元素的区块高度。 返回值返回了所有被接收的header
// Schedule adds a set of headers for the download queue for scheduling, returning
// the new headers encountered.
func (q *queue) Schedule(headers []*types.Header, from uint64) []*types.Header {
	q.lock.Lock()
	defer q.lock.Unlock()

	// Insert all the headers prioritised by the contained block number
	inserts := make([]*types.Header, 0, len(headers))
	for _, header := range headers {
		// Make sure chain order is honoured and preserved throughout
		hash := header.Hash()
		if header.Number == nil || header.Number.Uint64() != from {
			log.Warn("Header broke chain ordering", "number", header.Number, "hash", hash, "expected", from)
			break
		}
		//headerHead存储了最后一个插入的区块头， 检查当前区块是否正确的链接。
		if q.headerHead != (common.Hash{}) && q.headerHead != header.ParentHash {
			log.Warn("Header broke chain ancestry", "number", header.Number, "hash", hash)
			break
		}
		// 检查重复，这里直接continue了，那不是from对不上了。
		// Make sure no duplicate requests are executed
		if _, ok := q.blockTaskPool[hash]; ok {
			log.Warn("Header  already scheduled for block fetch", "number", header.Number, "hash", hash)
			continue
		}
		if _, ok := q.receiptTaskPool[hash]; ok {
			log.Warn("Header already scheduled for receipt fetch", "number", header.Number, "hash", hash)
			continue
		}
		// Queue the header for content retrieval
		q.blockTaskPool[hash] = header
		q.blockTaskQueue.Push(header, -float32(header.Number.Uint64()))

		if q.mode == FastSync {
			// 如果是快速同步模式，而且区块高度也小于pivot point. 那么还要获取receipt
			q.receiptTaskPool[hash] = header
			q.receiptTaskQueue.Push(header, -float32(header.Number.Uint64()))
		}
		inserts = append(inserts, header)
		q.headerHead = hash
		from++
	}
	return inserts
}

//结果从缓存中检索并永久删除一批获取结果。 如果队列已关闭，结果切片将为空。
// Results retrieves and permanently removes a batch of fetch results from
// the cache. the result slice will be empty if the queue has been closed.
func (q *queue) Results(block bool) []*fetchResult {
	q.lock.Lock()
	defer q.lock.Unlock()
	//计算可用于处理的项目数
	// Count the number of items available for processing
	nproc := q.countProcessableItems()
	for nproc == 0 && !q.closed {
		if !block {
			return nil
		}
		q.active.Wait()
		nproc = q.countProcessableItems()
	}
	//由于我们有批量限制，所以不要将更多内容拉入“悬空”内存
	// Since we have a batch limit, don't pull more into "dangling" memory
	if nproc > maxResultsProcess {
		nproc = maxResultsProcess
	}
	results := make([]*fetchResult, nproc)
	copy(results, q.resultCache[:nproc])
	if len(results) > 0 {
		//在将结果从缓存中删除之前将结果标记为已完成。
		// Mark results as done before dropping them from the cache.
		for _, result := range results {
			hash := result.Header.Hash()
			delete(q.blockDonePool, hash)
			delete(q.receiptDonePool, hash)
		}
		// Delete the results from the cache and clear the tail.
		copy(q.resultCache, q.resultCache[nproc:])
		for i := len(q.resultCache) - nproc; i < len(q.resultCache); i++ {
			q.resultCache[i] = nil
		}
		//提前第一个缓存条目的预期块编号。
		// Advance the expected block number of the first cache entry.
		q.resultOffset += uint64(nproc)
		//重新计算结果项权重以防止内存耗尽
		// Recalculate the result item weights to prevent memory exhaustion
		for _, result := range results {
			size := result.Header.Size()
			for _, uncle := range result.Uncles {
				size += uncle.Size()
			}
			for _, receipt := range result.Receipts {
				size += receipt.Size()
			}
			for _, tx := range result.Transactions {
				size += tx.Size()
			}
			q.resultSize = common.StorageSize(blockCacheSizeWeight)*size + (1-common.StorageSize(blockCacheSizeWeight))*q.resultSize
		}
	}
	return results
}

// countProcessableItems计算可处理项。
// countProcessableItems counts the processable items.
func (q *queue) countProcessableItems() int {
	for i, result := range q.resultCache {
		if result == nil || result.Pending > 0 {
			return i
		}
	}
	return len(q.resultCache)
}

//这个方法只skeleton的模式下才会被调用。 用来给peer保留fetch 区块头的任务。
// ReserveHeaders reserves a set of headers for the given peer, skipping any
// previously failed batches.
func (q *queue) ReserveHeaders(p *peerConnection, count int) *fetchRequest {
	q.lock.Lock()
	defer q.lock.Unlock()
	//如果对等方已经下载了某些东西（完整性检查没有损坏状态），则会发生短路
	// Short circuit if the peer's already downloading something (sanity check to
	// not corrupt state)
	if _, ok := q.headerPendPool[p.id]; ok {
		return nil
	}
	// 从队列中获取一个，跳过之前失败过的节点。
	// Retrieve a batch of hashes, skipping previously failed ones
	send, skip := uint64(0), []uint64{}
	for send == 0 && !q.headerTaskQueue.Empty() {
		from, _ := q.headerTaskQueue.Pop()
		if q.headerPeerMiss[p.id] != nil {
			if _, ok := q.headerPeerMiss[p.id][from.(uint64)]; ok {
				skip = append(skip, from.(uint64))
				continue
			}
		}
		send = from.(uint64)
	} //合并所有跳过的批次
	// Merge all the skipped batches back
	for _, from := range skip {
		q.headerTaskQueue.Push(from, -float32(from))
	}
	// Assemble and return the block download request
	if send == 0 {
		return nil
	}
	request := &fetchRequest{
		Peer: p,
		From: send,
		Time: time.Now(),
	}
	q.headerPendPool[p.id] = request
	return request
}

//ReserveXXX方法用来从queue里面领取一些任务来执行。downloader里面的goroutine会调用这个方法来领取一些任务来执行。
// 这个方法直接调用了reserveHeaders方法。 所有的ReserveXXX方法都会调用reserveHeaders方法，除了传入的参数有一些区别。
// ReserveBodies reserves a set of body fetches for the given peer, skipping any
// previously failed downloads. Beside the next batch of needed fetches, it also
// returns a flag whether empty blocks were queued requiring processing.
func (q *queue) ReserveBodies(p *peerConnection, count int) (*fetchRequest, bool, error) {
	isNoop := func(header *types.Header) bool {
		return header.TxHash == types.EmptyRootHash && header.UncleHash == types.EmptyUncleHash
	}
	q.lock.Lock()
	defer q.lock.Unlock()

	return q.reserveHeaders(p, count, q.blockTaskPool, q.blockTaskQueue, q.blockPendPool, q.blockDonePool, isNoop)
}

//ReserveReceipts 可以看到和ReserveBodys差不多。不过是队列换了而已。
// ReserveReceipts reserves a set of receipt fetches for the given peer, skipping
// any previously failed downloads. Beside the next batch of needed fetches, it
// also returns a flag whether empty receipts were queued requiring importing.
func (q *queue) ReserveReceipts(p *peerConnection, count int) (*fetchRequest, bool, error) {
	isNoop := func(header *types.Header) bool {
		return header.ReceiptHash == types.EmptyRootHash
	}
	q.lock.Lock()
	defer q.lock.Unlock()

	return q.reserveHeaders(p, count, q.receiptTaskPool, q.receiptTaskQueue, q.receiptPendPool, q.receiptDonePool, isNoop)
}

// reserveHeaders reserves a set of data download operations for a given peer,
// skipping any previously failed ones. This method is a generic version used
// by the individual special reservation functions.
//reserveHeaders为指定的peer保留一些下载操作，跳过之前的任意错误。 这个方法单独被指定的保留方法调用。
// Note, this method expects the queue lock to be already held for writing. The
// reason the lock is not obtained in here is because the parameters already need
// to access the queue, so they already need a lock anywa
// //这个方法调用的时候，假设已经获取到锁，这个方法里面没有锁的原因是参数已经传入到函数里面了，所以调用的时候就需要获取锁。y.
func (q *queue) reserveHeaders(p *peerConnection, count int, taskPool map[common.Hash]*types.Header, taskQueue *prque.Prque,
	pendPool map[string]*fetchRequest, donePool map[common.Hash]struct{}, isNoop func(*types.Header) bool) (*fetchRequest, bool, error) {
	// Short circuit if the pool has been depleted, or if the peer's already
	// downloading something (sanity check not to corrupt state)
	if taskQueue.Empty() {
		return nil, false, nil
	}
	// 如果这个peer还有下载任务没有完成。
	if _, ok := pendPool[p.id]; ok {
		return nil, false, nil
	}
	// 计算我们需要获取的上限。
	// Calculate an upper limit on the items we might fetch (i.e. throttling)
	space := q.resultSlots(pendPool, donePool)
	// 还需要减去正在下载的数量。
	// Retrieve a batch of tasks, skipping previously failed ones
	send := make([]*types.Header, 0, count)
	skip := make([]*types.Header, 0)

	progress := false
	for proc := 0; proc < space && len(send) < count && !taskQueue.Empty(); proc++ {
		header := taskQueue.PopItem().(*types.Header)
		hash := header.Hash()
		// index 是结果应该存储在resultCache的哪一部分。
		// If we're the first to request this task, initialise the result container
		index := int(header.Number.Int64() - int64(q.resultOffset))
		if index >= len(q.resultCache) || index < 0 {
			common.Report("index allocation went beyond available resultCache space")
			return nil, false, errInvalidChain
		}
		if q.resultCache[index] == nil { // 第一次调度 有可能多次调度。 那这里可能就是非空的。
			components := 1
			if q.mode == FastSync { // 如果是快速同步，那么需要下载的组件还有 收据receipt
				components = 2
			}
			q.resultCache[index] = &fetchResult{
				Pending: components,
				Hash:    hash,
				Header:  header,
			}
		}
		// If this fetch task is a noop, skip this fetch operation
		if isNoop(header) { // 如果header的区块中没有包含交易，那么不需要获取区块头
			donePool[hash] = struct{}{}
			delete(taskPool, hash)

			space, proc = space-1, proc-1
			q.resultCache[index].Pending--
			progress = true
			continue
		}
		// Lacks代表节点之前明确表示过没有这个hash的数据。
		// Otherwise unless the peer is known not to have the data, add to the retrieve list
		if p.Lacks(hash) {
			skip = append(skip, header)
		} else {
			send = append(send, header)
		}
	} //合并所有跳过的批次
	// Merge all the skipped headers back
	for _, header := range skip {
		taskQueue.Push(header, -float32(header.Number.Uint64()))
	}
	if progress {
		// 通知WaitResults， resultCache有改变
		// Wake WaitResults, resultCache was modified
		q.active.Signal()
	} //汇编并返回块下载请求
	// Assemble and return the block download request
	if len(send) == 0 {
		return nil, progress, nil
	}
	request := &fetchRequest{
		Peer:    p,
		Headers: send,
		Time:    time.Now(),
	}
	pendPool[p.id] = request

	return request, progress, nil
}

// CancelHeaders中止获取请求，将所有挂起的骨架索引返回到队列。
// CancelHeaders aborts a fetch request, returning all pending skeleton indexes to the queue.
func (q *queue) CancelHeaders(request *fetchRequest) {
	q.cancel(request, q.headerTaskQueue, q.headerPendPool)
}

//Cancle函数取消已经分配的任务， 把任务重新加入到任务池。
// CancelBodies aborts a body fetch request, returning all pending headers to the
// task queue.
func (q *queue) CancelBodies(request *fetchRequest) {
	q.cancel(request, q.blockTaskQueue, q.blockPendPool)
}

// CancelReceipts中止正文提取请求，将所有待处理的标头返回给任务队列。
// CancelReceipts aborts a body fetch request, returning all pending headers to
// the task queue.
func (q *queue) CancelReceipts(request *fetchRequest) {
	q.cancel(request, q.receiptTaskQueue, q.receiptPendPool)
}

//取消中止获取请求，将所有挂起的哈希值返回给任务队列。
// Cancel aborts a fetch request, returning all pending hashes to the task queue.
func (q *queue) cancel(request *fetchRequest, taskQueue *prque.Prque, pendPool map[string]*fetchRequest) {
	q.lock.Lock()
	defer q.lock.Unlock()

	if request.From > 0 {
		taskQueue.Push(request.From, -float32(request.From))
	}
	for _, header := range request.Headers {
		taskQueue.Push(header, -float32(header.Number.Uint64()))
	}
	delete(pendPool, request.Peer.id)
}

//撤消取消属于给定对等方的所有待处理请求。 此方法旨在在对等体删除期间调用，以快速将所拥有的数据提取重新分配给剩余节点。
// Revoke cancels all pending requests belonging to a given peer. This method is
// meant to be called during a peer drop to quickly reassign owned data fetches
// to remaining nodes.
func (q *queue) Revoke(peerId string) {
	q.lock.Lock()
	defer q.lock.Unlock()

	if request, ok := q.blockPendPool[peerId]; ok {
		for _, header := range request.Headers {
			q.blockTaskQueue.Push(header, -float32(header.Number.Uint64()))
		}
		delete(q.blockPendPool, peerId)
	}
	if request, ok := q.receiptPendPool[peerId]; ok {
		for _, header := range request.Headers {
			q.receiptTaskQueue.Push(header, -float32(header.Number.Uint64()))
		}
		delete(q.receiptPendPool, peerId)
	}
}

// ExpireHeaders检查超出超时限额的飞行请求，取消它们并返回负责的同伴进行惩罚。
// ExpireHeaders checks for in flight requests that exceeded a timeout allowance,
// canceling them and returning the responsible peers for penalisation.
func (q *queue) ExpireHeaders(timeout time.Duration) map[string]int {
	q.lock.Lock()
	defer q.lock.Unlock()

	return q.expire(timeout, q.headerPendPool, q.headerTaskQueue, headerTimeoutMeter)
}

//ExpireBodies函数获取了锁，然后直接调用了expire函数。
// ExpireBodies checks for in flight block body requests that exceeded a timeout
// allowance, canceling them and returning the responsible peers for penalisation.
func (q *queue) ExpireBodies(timeout time.Duration) map[string]int {
	q.lock.Lock()
	defer q.lock.Unlock()

	return q.expire(timeout, q.blockPendPool, q.blockTaskQueue, bodyTimeoutMeter)
}

// ExpireReceipts检查超出超时限额的航班收据请求，取消它们并返回负责的同行进行处罚。
// ExpireReceipts checks for in flight receipt requests that exceeded a timeout
// allowance, canceling them and returning the responsible peers for penalisation.
func (q *queue) ExpireReceipts(timeout time.Duration) map[string]int {
	q.lock.Lock()
	defer q.lock.Unlock()

	return q.expire(timeout, q.receiptPendPool, q.receiptTaskQueue, receiptTimeoutMeter)
}

// expire is the generic check that move expired tasks from a pending pool back
// into a task pool, returning all entities caught with expired tasks.
//// expire是通用检查，将过期任务从待处理池移回任务池，返回所有捕获已到期任务的实体。
// Note, this method expects the queue lock to be already held. The
// reason the lock is not obtained in here is because the parameters already need
// to access the queue, so they already need a lock anyway.
func (q *queue) expire(timeout time.Duration, pendPool map[string]*fetchRequest, taskQueue *prque.Prque, timeoutMeter metrics.Meter) map[string]int {
	// Iterate over the expired requests and return each to the queue
	expiries := make(map[string]int)
	for id, request := range pendPool {
		if time.Since(request.Time) > timeout {
			// Update the metrics with the timeout
			timeoutMeter.Mark(1)

			// Return any non satisfied requests to the pool
			if request.From > 0 {
				taskQueue.Push(request.From, -float32(request.From))
			}
			for _, header := range request.Headers {
				taskQueue.Push(header, -float32(header.Number.Uint64()))
			}
			// Add the peer to the expiry report along the the number of failed requests
			expiries[id] = len(request.Headers)
		}
	}
	// Remove the expired requests from the pending pool
	for id := range expiries {
		delete(pendPool, id)
	}
	return expiries
}

// DeliverHeaders injects a header retrieval response into the header results
// cache. This method either accepts all headers it received, or none of them
// if they do not map correctly to the skeleton.
//// 这个方法对于所有的区块头，要么全部接收，要么全部拒绝(如果不能映射到一个skeleton上面)
// If the headers are accepted, the method makes an attempt to deliver the set
// of ready headers to the processor to keep the pipeline full. However it will
// not block to prevent stalling other pending deliveries.
//如果区块头被接收，这个方法会试图把他们投递到headerProcCh管道上面。
//不过这个方法不会阻塞式的投递。而是尝试投递，如果不能投递就返回。
func (q *queue) DeliverHeaders(id string, headers []*types.Header, headerProcCh chan []*types.Header) (int, error) {
	q.lock.Lock()
	defer q.lock.Unlock()

	// Short circuit if the data was never requested
	request := q.headerPendPool[id]
	if request == nil {
		return 0, errNoFetchesPending
	}
	headerReqTimer.UpdateSince(request.Time)
	delete(q.headerPendPool, id)

	// Ensure headers can be mapped onto the skeleton chain
	target := q.headerTaskPool[request.From].Hash()

	accepted := len(headers) == MaxHeaderFetch
	if accepted { //首先长度需要匹配， 然后检查区块号和最后一块区块的Hash值是否能够对应上。
		if headers[0].Number.Uint64() != request.From {
			log.Trace("First header broke chain ordering", "peer", id, "number", headers[0].Number, "hash", headers[0].Hash(), request.From)
			accepted = false
		} else if headers[len(headers)-1].Hash() != target {
			log.Trace("Last header broke skeleton structure ", "peer", id, "number", headers[len(headers)-1].Number, "hash", headers[len(headers)-1].Hash(), "expected", target)
			accepted = false
		}
	}
	if accepted { // 依次检查每一块区块的区块号， 以及链接是否正确。
		for i, header := range headers[1:] {
			hash := header.Hash()
			if want := request.From + 1 + uint64(i); header.Number.Uint64() != want {
				log.Warn("Header broke chain ordering", "peer", id, "number", header.Number, "hash", hash, "expected", want)
				accepted = false
				break
			}
			if headers[i].Hash() != header.ParentHash {
				log.Warn("Header broke chain ancestry", "peer", id, "number", header.Number, "hash", hash)
				accepted = false
				break
			}
		}
	}
	// If the batch of headers wasn't accepted, mark as unavailable
	if !accepted { // 如果不被接收，那么标记这个peer在这个任务上的失败。下次请求就不会投递给这个peer
		log.Trace("Skeleton filling not accepted", "peer", id, "from", request.From)

		miss := q.headerPeerMiss[id]
		if miss == nil {
			q.headerPeerMiss[id] = make(map[uint64]struct{})
			miss = q.headerPeerMiss[id]
		}
		miss[request.From] = struct{}{}

		q.headerTaskQueue.Push(request.From, -float32(request.From))
		return 0, errors.New("delivery not accepted")
	}
	// Clean up a successful fetch and try to deliver any sub-results
	copy(q.headerResults[request.From-q.headerOffset:], headers)
	delete(q.headerTaskPool, request.From)

	ready := 0
	for q.headerProced+ready < len(q.headerResults) && q.headerResults[q.headerProced+ready] != nil { //计算这次到来的header可以让headerResults有多少数据可以投递了。
		ready += MaxHeaderFetch
	}
	if ready > 0 {
		// Headers are ready for delivery, gather them and push forward (non blocking)
		process := make([]*types.Header, ready)
		copy(process, q.headerResults[q.headerProced:q.headerProced+ready])
		// 尝试投递
		select {
		case headerProcCh <- process:
			log.Trace("Pre-scheduled new headers", "peer", id, "count", len(process), "from", process[0].Number)
			q.headerProced += len(process)
		default:
		}
	}
	// Check for termination and return
	if len(q.headerTaskPool) == 0 {
		// 这个通道比较重要， 如果这个通道接收到数据，说明所有的header任务已经完成。
		q.headerContCh <- false
	}
	return len(headers), nil
}

//Deliver方法在数据下载完之后会被调用。
// DeliverBodies injects a block body retrieval response into the results queue.
// The method returns the number of blocks bodies accepted from the delivery and
// also wakes any threads waiting for data delivery.
// DeliverBodies把一个 请求区块体的返回值插入到results队列
// 这个方法返回被delivery的区块体数量，同时会唤醒等待数据的线程
func (q *queue) DeliverBodies(id string, txLists [][]*types.Transaction, uncleLists [][]*types.Header) (int, error) {
	q.lock.Lock()
	defer q.lock.Unlock()

	reconstruct := func(header *types.Header, index int, result *fetchResult) error {
		if types.DeriveSha(types.Transactions(txLists[index])) != header.TxHash || types.CalcUncleHash(uncleLists[index]) != header.UncleHash {
			return errInvalidBody
		}
		result.Transactions = txLists[index]
		result.Uncles = uncleLists[index]
		return nil
	}
	return q.deliver(id, q.blockTaskPool, q.blockTaskQueue, q.blockPendPool, q.blockDonePool, bodyReqTimer, len(txLists), reconstruct)
}

// DeliverReceipts将收据检索响应注入结果队列。
//该方法返回从交付中接受的交易收据的数量，并唤醒任何等待数据交付的线程。
// DeliverReceipts injects a receipt retrieval response into the results queue.
// The method returns the number of transaction receipts accepted from the delivery
// and also wakes any threads waiting for data delivery.
func (q *queue) DeliverReceipts(id string, receiptList [][]*types.Receipt) (int, error) {
	q.lock.Lock()
	defer q.lock.Unlock()

	reconstruct := func(header *types.Header, index int, result *fetchResult) error {
		if types.DeriveSha(types.Receipts(receiptList[index])) != header.ReceiptHash {
			return errInvalidReceipt
		}
		result.Receipts = receiptList[index]
		return nil
	}
	return q.deliver(id, q.receiptTaskPool, q.receiptTaskQueue, q.receiptPendPool, q.receiptDonePool, receiptReqTimer, len(receiptList), reconstruct)
}

// deliver将数据检索响应注入结果队列。
//注意，此方法需要保留队列锁以进行写入。 在这里没有获得锁定的原因是因为参数已经需要访问队列，所以他们无论如何都需要锁定。
// deliver injects a data retrieval response into the results queue.
//
// Note, this method expects the queue lock to be already held for writing. The
// reason the lock is not obtained in here is because the parameters already need
// to access the queue, so they already need a lock anyway.
func (q *queue) deliver(id string, taskPool map[common.Hash]*types.Header, taskQueue *prque.Prque,
	pendPool map[string]*fetchRequest, donePool map[common.Hash]struct{}, reqTimer metrics.Timer,
	results int, reconstruct func(header *types.Header, index int, result *fetchResult) error) (int, error) {
	//检查 数据是否从来没有请求过。
	// Short circuit if the data was never requested
	request := pendPool[id]
	if request == nil {
		return 0, errNoFetchesPending
	}
	reqTimer.UpdateSince(request.Time)
	delete(pendPool, id)
	//如果未检索到任何数据项，请将它们标记为原始对等项不可用
	// If no data items were retrieved, mark them as unavailable for the origin peer
	if results == 0 {
		//如果结果为空。 那么标识这个peer没有这些数据。
		for _, header := range request.Headers {
			request.Peer.MarkLacking(header.Hash())
		}
	} //使用标题和检索到的数据部分组合每个结果
	// Assemble each of the results with their headers and retrieved data parts
	var (
		accepted int
		failure  error
		useful   bool
	)
	for i, header := range request.Headers {
		//如果找不到更多的获取结果，则进行短路装配
		// Short circuit assembly if no more fetch results are found
		if i >= results {
			break
		}
		// Reconstruct the next result if contents match up
		index := int(header.Number.Int64() - int64(q.resultOffset))
		if index >= len(q.resultCache) || index < 0 || q.resultCache[index] == nil {
			failure = errInvalidChain
			break
		}
		// 调用传入的函数对数据进行构建
		if err := reconstruct(header, i, q.resultCache[index]); err != nil {
			failure = err
			break
		}
		hash := header.Hash()

		donePool[hash] = struct{}{}
		q.resultCache[index].Pending--
		useful = true
		accepted++
		// 从taskPool删除。加入donePool
		// Clean up a successful fetch
		request.Headers[i] = nil
		delete(taskPool, hash)
	}
	// 所有没有成功的请求加入taskQueue
	// Return all failed or missing fetches to the queue
	for _, header := range request.Headers {
		if header != nil {
			taskQueue.Push(header, -float32(header.Number.Uint64()))
		}
	}
	// 如果结果有变更，通知WaitResults线程启动。
	// Wake up WaitResults
	if accepted > 0 {
		q.active.Signal()
	}
	// If none of the data was good, it's a stale delivery
	switch {
	case failure == nil || failure == errInvalidChain:
		return accepted, failure
	case useful:
		return accepted, fmt.Errorf("partial failure: %v", failure)
	default:
		return accepted, errStaleDelivery
	}
}

// Prepare配置结果缓存以允许接受和缓存入站获取结果。
// Prepare configures the result cache to allow accepting and caching inbound
// fetch results.
func (q *queue) Prepare(offset uint64, mode SyncMode) {
	q.lock.Lock()
	defer q.lock.Unlock()

	// Prepare the queue for sync results
	if q.resultOffset < offset {
		q.resultOffset = offset
	}
	q.mode = mode
}
