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
// Package downloader包含手动完全链同步
// Package downloader contains the manual full chain synchronisation.
package downloader

import (
	"errors"
	"fmt"
	"math/big"
	"sync"
	"sync/atomic"
	"time"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethdb"
	"github.com/ethereum/go-ethereum/event"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/metrics"
	"github.com/ethereum/go-ethereum/params"
)

var (
	MaxHashFetch    = 512 // Amount of hashes to be fetched per retrieval request每个检索请求获取的散列数量
	MaxBlockFetch   = 128 // Amount of blocks to be fetched per retrieval request每个检索请求要获取的块数量
	MaxHeaderFetch  = 192 // Amount of block headers to be fetched per retrieval request每个检索请求要获取的块标题数量
	MaxSkeletonSize = 128 // Number of header fetches to need for a skeleton assembly获取骨架程序集所需的头部提取数量
	MaxBodyFetch    = 128 // Amount of block bodies to be fetched per retrieval request每个检索请求要获取的块体数量
	MaxReceiptFetch = 256 // Amount of transaction receipts to allow fetching per request允许每个请求获取的交易收据金额
	MaxStateFetch   = 384 // Amount of node state values to allow fetching per request允许每个请求获取的节点状态值数量

	MaxForkAncestry  = 3 * params.EpochDuration // Maximum chain reorganisation最大连锁重组
	rttMinEstimate   = 2 * time.Second          // Minimum round-trip time to target for download requests下载请求的最小往返时间
	rttMaxEstimate   = 20 * time.Second         // Maximum rount-trip time to target for download requests下载请求的最大往返时间
	rttMinConfidence = 0.1                      // Worse confidence factor in our estimated RTT value我们的估计RTT值更可信的因素
	ttlScaling       = 3                        // Constant scaling factor for RTT -> TTL conversion用于RTT - > TTL转换的恒定比例因子
	ttlLimit         = time.Minute              // Maximum TTL allowance to prevent reaching crazy timeouts最大TTL限额可防止达到疯狂超时

	qosTuningPeers   = 5    // Number of peers to tune based on (best peers)根据（最佳同行）调整的同伴数量
	qosConfidenceCap = 10   // Number of peers above which not to modify RTT confidence高于此值的对等体的数量不能修改RTT置信度
	qosTuningImpact  = 0.25 // Impact that a new tuning target has on the previous value新调整目标对先前价值的影响

	maxQueuedHeaders  = 32 * 1024 // [eth/62] Maximum number of headers to queue for import (DOS protection)排队导入的最大头数（DOS保护）
	maxHeadersProcess = 2048      // Number of header download results to import at once into the chain一次导入链中的头下载结果的数量
	maxResultsProcess = 2048      // Number of content download results to import at once into the chain一次导入链中的内容下载结果的数量

	fsHeaderCheckFrequency = 100             // Verification frequency of the downloaded headers during fast sync在快速同步期间验证下载的标头的频率
	fsHeaderSafetyNet      = 2048            // Number of headers to discard in case a chain violation is detected在检测到链条违例时丢弃的标头数量
	fsHeaderForceVerify    = 24              // Number of headers to verify before and after the pivot to accept it要在接受数据透视前后验证的标题数量
	fsHeaderContCheck      = 3 * time.Second // Time interval to check for header continuations during state download	在状态下载期间检查标题延续的时间间隔
	fsMinFullBlocks        = 64              // Number of blocks to retrieve fully even in fast sync即使在快速同步中也能完全检索的块数
)

var (
	errBusy                    = errors.New("busy")
	errUnknownPeer             = errors.New("peer is unknown or unhealthy")
	errBadPeer                 = errors.New("action from bad peer ignored")
	errStallingPeer            = errors.New("peer is stalling")
	errNoPeers                 = errors.New("no peers to keep download active")
	errTimeout                 = errors.New("timeout")
	errEmptyHeaderSet          = errors.New("empty header set by peer")
	errPeersUnavailable        = errors.New("no peers available or all tried for download")
	errInvalidAncestor         = errors.New("retrieved ancestor is invalid")
	errInvalidChain            = errors.New("retrieved hash chain is invalid")
	errInvalidBlock            = errors.New("retrieved block is invalid")
	errInvalidBody             = errors.New("retrieved block body is invalid")
	errInvalidReceipt          = errors.New("retrieved receipt is invalid")
	errCancelBlockFetch        = errors.New("block download canceled (requested)")
	errCancelHeaderFetch       = errors.New("block header download canceled (requested)")
	errCancelBodyFetch         = errors.New("block body download canceled (requested)")
	errCancelReceiptFetch      = errors.New("receipt download canceled (requested)")
	errCancelStateFetch        = errors.New("state data download canceled (requested)")
	errCancelHeaderProcessing  = errors.New("header processing canceled (requested)")
	errCancelContentProcessing = errors.New("content processing canceled (requested)")
	errNoSyncActive            = errors.New("no sync active")
	errTooOld                  = errors.New("peer doesn't speak recent enough protocol version (need version >= 62)")
)

type Downloader struct {
	mode SyncMode       // Synchronisation mode defining the strategy used (per sync cycle)定义所用策略的同步模式（每个同步周期）
	mux  *event.TypeMux // Event multiplexer to announce sync operation events事件多路复用器来宣布同步操作事件
	// queue 对象用来调度 区块头，交易，和收据的下载，以及下载完之后的组装
	queue *queue // Scheduler for selecting the hashes to download调度程序用于选择要下载的散列
	// 对端的集合
	peers   *peerSet // Set of active peers from which download can proceed可从中进行下载的活动对等设备集
	stateDB ethdb.Database
	// 下载的往返时延
	rttEstimate   uint64 // Round trip time to target for download requests下载请求的往返时间
	rttConfidence uint64 // Confidence in the estimated RTT (unit: millionths to allow atomic ops)对估计RTT的信心（单位：允许原子操作的百万分之一）

	// Statistics 统计信息
	// Statistics
	syncStatsChainOrigin uint64 // Origin block number where syncing started at同步开始于的起始块号码
	syncStatsChainHeight uint64 // Highest block number known when syncing started同步开始时已知的最高块数
	syncStatsState       stateSyncStats
	syncStatsLock        sync.RWMutex // Lock protecting the sync stats fields锁定保护同步统计字段

	lightchain LightChain
	blockchain BlockChain

	// Callbacks
	dropPeer peerDropFn // Drops a peer for misbehaving因行为不端而堕落

	// Status
	synchroniseMock func(id string, hash common.Hash) error // Replacement for synchronise during testing在测试期间替换为同步
	synchronising   int32
	notified        int32
	committed       int32

	// Channels
	headerCh      chan dataPack        // [eth/62] Channel receiving inbound block headers  header的输入通道，从网络下载的header会被送到这个通道
	bodyCh        chan dataPack        // [eth/62] Channel receiving inbound block bodies   bodies的输入通道，从网络下载的bodies会被送到这个通道
	receiptCh     chan dataPack        // [eth/63] Channel receiving inbound receipts       receipts的输入通道，从网络下载的receipts会被送到这个通道
	bodyWakeCh    chan bool            // [eth/62] Channel to signal the block body fetcher of new tasks    用来传输body fetcher新任务的通道
	receiptWakeCh chan bool            // [eth/63] Channel to signal the receipt fetcher of new tasks	   用来传输receipt fetcher 新任务的通道
	headerProcCh  chan []*types.Header // [eth/62] Channel to feed the header processor new tasks		通道为header处理者提供新的任务

	// for stateFetcher
	stateSyncStart chan *stateSync //用来启动新的 state fetcher
	trackStateReq  chan *stateReq
	//state的输入通道，从网络下载的state会被送到这个通道
	stateCh chan dataPack // [eth/63] Channel receiving inbound node state data通道接收入站节点状态数据

	// Cancellation and termination取消和终止
	cancelPeer string        // Identifier of the peer currently being used as the master (cancel on drop)当前正在用作主设备的对等设备的标识符（取消放置）
	cancelCh   chan struct{} // Channel to cancel mid-flight syncs频道取消中途飞行同步
	cancelLock sync.RWMutex  // Lock to protect the cancel channel and peer in delivers锁定以保护取消频道和点播节目

	quitCh   chan struct{} // Quit channel to signal termination	//退出通道到信号终止
	quitLock sync.RWMutex  // Lock to prevent double closes锁定以防止双重关闭
	//测试钩子
	// Testing hooks
	syncInitHook     func(uint64, uint64)  // Method to call upon initiating a new sync run启动新同步运行的方法
	bodyFetchHook    func([]*types.Header) // Method to call upon starting a block body fetch调用开始块体提取的方法
	receiptFetchHook func([]*types.Header) // Method to call upon starting a receipt fetch调用开始收据提取的方法
	chainInsertHook  func([]*fetchResult)  // Method to call upon inserting a chain of blocks (possibly in multiple invocations)调用插入块链（可能在多个调用中）的方法
}

// LightChain封装同步轻链所需的功能。
// LightChain encapsulates functions required to synchronise a light chain.
type LightChain interface { // HasHeader在本地链中验证头部的存在。
	// HasHeader verifies a header's presence in the local chain.
	HasHeader(common.Hash, uint64) bool
	// GetHeaderByHash从本地链中检索一个头。
	// GetHeaderByHash retrieves a header from the local chain.
	GetHeaderByHash(common.Hash) *types.Header
	// CurrentHeader从本地链中检索头部标题
	// CurrentHeader retrieves the head header from the local chain.
	CurrentHeader() *types.Header
	// GetTd返回本地块的总难度。
	// GetTd returns the total difficulty of a local block.
	GetTd(common.Hash, uint64) *big.Int

	// InsertHeaderChain inserts a batch of headers into the local chain./ InsertHeaderChain将一批标题插入到本地链中
	InsertHeaderChain([]*types.Header, int) (int, error)

	// Rollback removes a few recently added elements from the local chain.//回滚会从本地链中删除一些最近添加的元素
	Rollback([]common.Hash)
}

// BlockChain封装了同步（全速或快速）区块链所需的功能。
// BlockChain encapsulates functions required to sync a (full or fast) blockchain.
type BlockChain interface {
	LightChain
	// HasBlock验证块在本地链中的存在。
	// HasBlock verifies a block's presence in the local chain.
	HasBlock(common.Hash, uint64) bool
	// GetBlockByHash从本地链中检索一个块。
	// GetBlockByHash retrieves a block from the local chain.
	GetBlockByHash(common.Hash) *types.Block
	// CurrentBlock从本地链中检索头部块。
	// CurrentBlock retrieves the head block from the local chain.
	CurrentBlock() *types.Block
	// CurrentFastBlock从本地链中检索头部快速块
	// CurrentFastBlock retrieves the head fast block from the local chain.
	CurrentFastBlock() *types.Block
	// FastSyncCommitHead直接将头块提交给某个实体。
	// FastSyncCommitHead directly commits the head block to a certain entity.
	FastSyncCommitHead(common.Hash) error
	// InsertChain将一批块插入到本地链中。
	// InsertChain inserts a batch of blocks into the local chain.
	InsertChain(types.Blocks) (int, error)
	// InsertReceiptChain将一批收据插入到本地链中。
	// InsertReceiptChain inserts a batch of receipts into the local chain.
	InsertReceiptChain(types.Blocks, []types.Receipts) (int, error)
}

// New创建一个新的下载器来从远程对等体获取散列和块。
// New creates a new downloader to fetch hashes and blocks from remote peers.
func New(mode SyncMode, stateDb ethdb.Database, mux *event.TypeMux, chain BlockChain, lightchain LightChain, dropPeer peerDropFn) *Downloader {
	if lightchain == nil {
		lightchain = chain
	}

	dl := &Downloader{
		mode:           mode,
		stateDB:        stateDb,
		mux:            mux,
		queue:          newQueue(),
		peers:          newPeerSet(),
		rttEstimate:    uint64(rttMaxEstimate),
		rttConfidence:  uint64(1000000),
		blockchain:     chain,
		lightchain:     lightchain,
		dropPeer:       dropPeer,
		headerCh:       make(chan dataPack, 1),
		bodyCh:         make(chan dataPack, 1),
		receiptCh:      make(chan dataPack, 1),
		bodyWakeCh:     make(chan bool, 1),
		receiptWakeCh:  make(chan bool, 1),
		headerProcCh:   make(chan []*types.Header, 1),
		quitCh:         make(chan struct{}),
		stateCh:        make(chan dataPack),
		stateSyncStart: make(chan *stateSync),
		syncStatsState: stateSyncStats{
			processed: core.GetTrieSyncProgress(stateDb),
		},
		trackStateReq: make(chan *stateReq),
	}
	go dl.qosTuner()     //简单 主要用来计算rttEstimate和rttConfidence
	go dl.stateFetcher() //启动stateFetcher的任务监听，但是这个时候还没有生成state fetcher的任务。
	return dl
}

// Progress会检索同步边界，特别是同步开始于（可能已失败/挂起）的原始块; 块或标题同步当前处于;
// 和同步目标的最新已知块。
//此外，在快速同步状态下载阶段，还会返回已处理的数量和已知状态的总数。 否则，这些都是零。
// Progress retrieves the synchronisation boundaries, specifically the origin
// block where synchronisation started at (may have failed/suspended); the block
// or header sync is currently at; and the latest known block which the sync targets.
//
// In addition, during the state download phase of fast synchronisation the number
// of processed and the total number of known states are also returned. Otherwise
// these are zero.
func (d *Downloader) Progress() ethereum.SyncProgress {
	// Lock the current stats and return the progress
	//锁定当前的统计数据并返回进度
	d.syncStatsLock.RLock()
	defer d.syncStatsLock.RUnlock()

	current := uint64(0)
	switch d.mode {
	case FullSync:
		current = d.blockchain.CurrentBlock().NumberU64()
	case FastSync:
		current = d.blockchain.CurrentFastBlock().NumberU64()
	case LightSync:
		current = d.lightchain.CurrentHeader().Number.Uint64()
	}
	return ethereum.SyncProgress{
		StartingBlock: d.syncStatsChainOrigin,
		CurrentBlock:  current,
		HighestBlock:  d.syncStatsChainHeight,
		PulledStates:  d.syncStatsState.processed,
		KnownStates:   d.syncStatsState.processed + d.syncStatsState.pending,
	}
}

// Synchronizing返回下载器是否正在检索块。
// Synchronising returns whether the downloader is currently retrieving blocks.
func (d *Downloader) Synchronising() bool {
	return atomic.LoadInt32(&d.synchronising) > 0
}

// RegisterPeer将一个新的下载对象注入到用于从中获取散列和块的块源的集合中。
// RegisterPeer injects a new download peer into the set of block source to be
// used for fetching hashes and blocks from.
func (d *Downloader) RegisterPeer(id string, version int, peer Peer) error {
	logger := log.New("peer", id)
	logger.Trace("Registering sync peer")
	if err := d.peers.Register(newPeerConnection(id, version, peer, logger)); err != nil {
		logger.Error("Failed to register sync peer", "err", err)
		return err
	}
	d.qosReduceConfidence()

	return nil
}

//注入一个轻客户端对象，将其包装成一个常规对等体。
// RegisterLightPeer injects a light client peer, wrapping it so it appears as a regular peer.
func (d *Downloader) RegisterLightPeer(id string, version int, peer LightPeer) error {
	return d.RegisterPeer(id, version, &lightPeerWrapper{peer})
}

// UnregisterPeer从已知列表中删除一个对等点，阻止来自指定对等点的任何操作。 还努力将任何挂起的提取返回到队列中。
// UnregisterPeer remove a peer from the known list, preventing any action from
// the specified peer. An effort is also made to return any pending fetches into
// the queue.
func (d *Downloader) UnregisterPeer(id string) error {
	// Unregister the peer from the active peer set and revoke any fetch tasks
	logger := log.New("peer", id)
	logger.Trace("Unregistering sync peer")
	if err := d.peers.Unregister(id); err != nil {
		logger.Error("Failed to unregister sync peer", "err", err)
		return err
	}
	d.queue.Revoke(id)
	//如果此对等方是主对等方，则立即中止同步
	// If this peer was the master peer, abort sync immediately
	d.cancelLock.RLock()
	master := id == d.cancelPeer
	d.cancelLock.RUnlock()

	if master {
		d.Cancel()
	}
	return nil
}

//Synchronise试图和一个peer来同步，如果同步过程中遇到一些错误，那么会删除掉Peer。然后会被重试。
// Synchronise tries to sync up our local block chain with a remote peer, both
// adding various sanity checks as well as wrapping it with various log entries.
func (d *Downloader) Synchronise(id string, head common.Hash, td *big.Int, mode SyncMode) error {
	err := d.synchronise(id, head, td, mode)
	switch err {
	case nil:
	case errBusy:

	case errTimeout, errBadPeer, errStallingPeer,
		errEmptyHeaderSet, errPeersUnavailable, errTooOld,
		errInvalidAncestor, errInvalidChain:
		log.Warn("Synchronisation failed, dropping peer", "peer", id, "err", err)
		if d.dropPeer == nil {
			//当`--copydb`用于本地副本时，dropPeer方法为零。
			//例如，如果例如超时 压实击中了错误的时间，并且可以忽略
			// The dropPeer method is nil when `--copydb` is used for a local copy.
			// Timeouts can occur if e.g. compaction hits at the wrong time, and can be ignored
			log.Warn("Downloader wants to drop peer, but peerdrop-function is not set", "peer", id)
		} else {
			d.dropPeer(id)
		}
	default:
		log.Warn("Synchronisation failed, retrying", "err", err)
	}
	return err
}

//同步将选择对等并将其用于同步。 如果给出一个空字符串，它将使用最好的对等方
// ，并且如果它的TD比我们自己的高，它就会同步。
// 如果任何检查失败，则会返回错误。 这种方法是同步的
// synchronise will select the peer and use it for synchronising. If an empty string is given
// it will use the best peer possible and synchronize if its TD is higher than our own. If any of the
// checks fail an error will be returned. This method is synchronous
func (d *Downloader) synchronise(id string, hash common.Hash, td *big.Int, mode SyncMode) error {
	// Mock out the synchronisation if testing
	if d.synchroniseMock != nil {
		return d.synchroniseMock(id, hash)
	}
	// Make sure only one goroutine is ever allowed past this point at once
	// 这个方法同时只能运行一个， 检查是否正在运行。
	if !atomic.CompareAndSwapInt32(&d.synchronising, 0, 1) {
		return errBusy
	}
	defer atomic.StoreInt32(&d.synchronising, 0)
	//发布用户的同步通知（每个会话只有一次）
	// Post a user notification of the sync (only once per session)
	if atomic.CompareAndSwapInt32(&d.notified, 0, 1) {
		log.Info("Block synchronisation started")
	}
	// 重置queue和peer的状态。
	// Reset the queue, peer set and wake channels to clean any internal leftover state
	d.queue.Reset()
	d.peers.Reset()
	// 清空d.bodyWakeCh, d.receiptWakeCh
	for _, ch := range []chan bool{d.bodyWakeCh, d.receiptWakeCh} {
		select {
		case <-ch:
		default:
		}
	}
	// 清空d.headerCh, d.bodyCh, d.receiptCh
	for _, ch := range []chan dataPack{d.headerCh, d.bodyCh, d.receiptCh} {
		for empty := false; !empty; {
			select {
			case <-ch:
			default:
				empty = true
			}
		}
	}
	// 清空headerProcCh
	for empty := false; !empty; {
		select {
		case <-d.headerProcCh:
		default:
			empty = true
		}
	} //创建取消频道以中途中止并标记主对等
	// Create cancel channel for aborting mid-flight and mark the master peer
	d.cancelLock.Lock()
	d.cancelCh = make(chan struct{})
	d.cancelPeer = id
	d.cancelLock.Unlock()
	//无论如何，我们不能将取消频道打开
	defer d.Cancel() // No matter what, we can't leave the cancel channel open
	//设置请求的同步模式，除非它被禁止
	// Set the requested sync mode, unless it's forbidden
	d.mode = mode
	//检索起始点并启动下载过程
	// Retrieve the origin peer and initiate the downloading process
	p := d.peers.Peer(id)
	if p == nil {
		return errUnknownPeer
	}
	return d.syncWithPeer(p, hash, td)
}

// syncWithPeer根据来自指定对等体和头散列的散列链启动块同步
// syncWithPeer starts a block synchronization based on the hash chain from the
// specified peer and head hash.
func (d *Downloader) syncWithPeer(p *peerConnection, hash common.Hash, td *big.Int) (err error) {
	d.mux.Post(StartEvent{})
	defer func() {
		// reset on error
		if err != nil {
			d.mux.Post(FailedEvent{err})
		} else {
			d.mux.Post(DoneEvent{})
		}
	}()
	if p.version < 62 {
		return errTooOld
	}

	log.Debug("Synchronising with the network", "peer", p.id, "eth", p.version, "head", hash, "td", td, "mode", d.mode)
	defer func(start time.Time) {
		log.Debug("Synchronisation terminated", "elapsed", time.Since(start))
	}(time.Now())
	// 使用hash指来获取区块头，这个方法里面会访问网络
	// Look up the sync boundaries: the common ancestor and the target block
	latest, err := d.fetchHeight(p)
	if err != nil {
		return err
	}
	height := latest.Number.Uint64()
	// findAncestor试图来获取大家共同的祖先，以便找到一个开始同步的点。
	origin, err := d.findAncestor(p, height)
	if err != nil {
		return err
	}
	d.syncStatsLock.Lock()
	if d.syncStatsChainHeight <= origin || d.syncStatsChainOrigin > origin {
		d.syncStatsChainOrigin = origin
	}
	d.syncStatsChainHeight = height
	d.syncStatsLock.Unlock()
	//确保我们的原点低于任何快速同步中心点
	// Ensure our origin point is below any fast sync pivot point
	pivot := uint64(0)
	if d.mode == FastSync {
		if height <= uint64(fsMinFullBlocks) {
			origin = 0
		} else {
			pivot = height - uint64(fsMinFullBlocks)
			if pivot <= origin {
				origin = pivot - 1
			}
		}
	}
	d.committed = 1
	if d.mode == FastSync && pivot != 0 {
		d.committed = 0
	} //使用并发头和内容检索算法启动同步
	// Initiate the sync using a concurrent header and content retrieval algorithm
	d.queue.Prepare(origin+1, d.mode)
	if d.syncInitHook != nil {
		d.syncInitHook(origin, height)
	}
	// 启动几个fetcher 分别负责header,bodies,receipts,处理headers
	fetchers := []func() error{
		func() error { return d.fetchHeaders(p, origin+1, pivot) }, // Headers are always retrieved//总是检索标题
		func() error { return d.fetchBodies(origin + 1) },          // Bodies are retrieved during normal and fast sync在正常和快速同步期间检索身体
		func() error { return d.fetchReceipts(origin + 1) },        // Receipts are retrieved during fast sync在快速同步期间检索收据
		func() error { return d.processHeaders(origin+1, pivot, td) },
	}
	if d.mode == FastSync { //根据模式的不同，增加新的处理逻辑
		fetchers = append(fetchers, func() error { return d.processFastSyncContent(latest) })
	} else if d.mode == FullSync {
		fetchers = append(fetchers, d.processFullSyncContent)
	}
	return d.spawnSync(fetchers)
}

//spawnSync给每个fetcher启动一个goroutine, 然后阻塞的等待fetcher出错。
// spawnSync runs d.process and all given fetcher functions to completion in
// separate goroutines, returning the first error that appears.
func (d *Downloader) spawnSync(fetchers []func() error) error {
	var wg sync.WaitGroup
	errc := make(chan error, len(fetchers))
	wg.Add(len(fetchers))
	for _, fn := range fetchers {
		fn := fn
		go func() { defer wg.Done(); errc <- fn() }()
	} //等待第一个错误，然后终止其他错误。
	// Wait for the first error, then terminate the others.
	var err error
	for i := 0; i < len(fetchers); i++ {
		if i == len(fetchers)-1 {
			//当所有fetchers退出时关闭队列。
			//这将导致块处理器在处理队列时结束。
			// Close the queue when all fetchers have exited.
			// This will cause the block processor to end when
			// it has processed the queue.
			d.queue.Close()
		}
		if err = <-errc; err != nil {
			break
		}
	}
	d.queue.Close()
	d.Cancel()
	wg.Wait()
	return err
}

//取消取消所有操作并重置队列。 它返回true如果取消操作完成。
// Cancel cancels all of the operations and resets the queue. It returns true
// if the cancel operation was completed.
func (d *Downloader) Cancel() {
	// Close the current cancel channel//关闭当前的取消频道
	d.cancelLock.Lock()
	if d.cancelCh != nil {
		select {
		case <-d.cancelCh:
			// Channel was already closed//频道已关闭
		default:
			close(d.cancelCh)
		}
	}
	d.cancelLock.Unlock()
}

//终止中断下载器，取消所有挂起的操作。调用终止后，下载程序不能重新使用。
// Terminate interrupts the downloader, canceling all pending operations.
// The downloader cannot be reused after calling Terminate.
func (d *Downloader) Terminate() {
	// Close the termination channel (make sure double close is allowed)
	//关闭终止信道（确保允许双关闭）
	d.quitLock.Lock()
	select {
	case <-d.quitCh:
	default:
		close(d.quitCh)
	}
	d.quitLock.Unlock()
	//取消所有待处理的下载请求
	// Cancel any pending download requests
	d.Cancel()
}

// fetchHeight检索远程对象的头标以帮助估计未决同步将花费的总时间。
// fetchHeight retrieves the head header of the remote peer to aid in estimating
// the total time a pending synchronisation would take.
func (d *Downloader) fetchHeight(p *peerConnection) (*types.Header, error) {
	p.log.Debug("Retrieving remote chain height")
	//请求广告的远程起始块并等待响应
	// Request the advertised remote head block and wait for the response
	head, _ := p.peer.Head()
	go p.peer.RequestHeadersByHash(head, 1, 0, false)

	ttl := d.requestTTL()
	timeout := time.After(ttl)
	for {
		select {
		case <-d.cancelCh:
			return nil, errCancelBlockFetch

		case packet := <-d.headerCh:
			//丢弃任何不是来自同辈的东西
			// Discard anything not from the origin peer
			if packet.PeerId() != p.id {
				log.Debug("Received headers from incorrect peer", "peer", packet.PeerId())
				break
			}
			//确保同伴实际上给了一些有效的东西
			// Make sure the peer actually gave something valid
			headers := packet.(*headerPack).headers
			if len(headers) != 1 {
				p.log.Debug("Multiple headers for single request", "headers", len(headers))
				return nil, errBadPeer
			}
			head := headers[0]
			p.log.Debug("Remote head header identified", "number", head.Number, "hash", head.Hash())
			return head, nil

		case <-timeout:
			p.log.Debug("Waiting for head header timed out", "elapsed", ttl)
			return nil, errTimeout

		case <-d.bodyCh:
		case <-d.receiptCh:
			// Out of bounds delivery, ignore
		}
	}
}

//findAncestor试图找到本地链和远程对等体区块链的共同祖先链接。
// 在一般情况下，当我们的节点同步并且在正确的链中时，检查前N个链接应该已经使我们匹配。
// 在极少数情况下，当我们结束长时间的重组时（即，没有头部链接匹配），我们做二进制搜索找到共同的祖先。
// findAncestor tries to locate the common ancestor link of the local chain and
// a remote peers blockchain. In the general case when our node was in sync and
// on the correct chain, checking the top N links should already get us a match.
// In the rare scenario when we ended up on a long reorganisation (i.e. none of
// the head links match), we do a binary search to find the common ancestor.
func (d *Downloader) findAncestor(p *peerConnection, height uint64) (uint64, error) {
	// Figure out the valid ancestor range to prevent rewrite attacks
	floor, ceil := int64(-1), d.lightchain.CurrentHeader().Number.Uint64()

	if d.mode == FullSync {
		ceil = d.blockchain.CurrentBlock().NumberU64()
	} else if d.mode == FastSync {
		ceil = d.blockchain.CurrentFastBlock().NumberU64()
	}
	if ceil >= MaxForkAncestry {
		floor = int64(ceil - MaxForkAncestry)
	}
	p.log.Debug("Looking for common ancestor", "local", ceil, "remote", height)
	//请求最上面的块来短路二进制祖先查找
	// Request the topmost blocks to short circuit binary ancestor lookup
	head := ceil
	if head > height {
		head = height
	}
	from := int64(head) - int64(MaxHeaderFetch)
	if from < 0 {
		from = 0
	} //跨越15个块的差距进入未来以收集糟糕的头部报告
	// Span out with 15 block gaps into the future to catch bad head reports
	limit := 2 * MaxHeaderFetch / 16
	count := 1 + int((int64(ceil)-from)/16)
	if count > limit {
		count = limit
	}
	go p.peer.RequestHeadersByNumber(uint64(from), count, 15, false)
	//等待头部获取的远程响应
	// Wait for the remote response to the head fetch
	number, hash := uint64(0), common.Hash{}

	ttl := d.requestTTL()
	timeout := time.After(ttl)

	for finished := false; !finished; {
		select {
		case <-d.cancelCh:
			return 0, errCancelHeaderFetch

		case packet := <-d.headerCh:
			//丢弃任何不是来自同辈的东西
			// Discard anything not from the origin peer
			if packet.PeerId() != p.id {
				log.Debug("Received headers from incorrect peer", "peer", packet.PeerId())
				break
			}
			//确保同伴实际上给了一些有效的东西
			// Make sure the peer actually gave something valid
			headers := packet.(*headerPack).headers
			if len(headers) == 0 {
				p.log.Warn("Empty head header set")
				return 0, errEmptyHeaderSet
			} //确保对方的回复符合请求
			// Make sure the peer's reply conforms to the request
			for i := 0; i < len(headers); i++ {
				if number := headers[i].Number.Int64(); number != from+int64(i)*16 {
					p.log.Warn("Head headers broke chain ordering", "index", i, "requested", from+int64(i)*16, "received", number)
					return 0, errInvalidChain
				}
			} //检查是否找到共同的祖先
			// Check if a common ancestor was found
			finished = true
			for i := len(headers) - 1; i >= 0; i-- {
				//跳过溢出/溢出请求集的任何头文件
				// Skip any headers that underflow/overflow our requested set
				if headers[i].Number.Int64() < from || headers[i].Number.Uint64() > ceil {
					continue
				} //否则检查我们是否已经知道标题
				// Otherwise check if we already know the header or not
				if (d.mode == FullSync && d.blockchain.HasBlock(headers[i].Hash(), headers[i].Number.Uint64())) || (d.mode != FullSync && d.lightchain.HasHeader(headers[i].Hash(), headers[i].Number.Uint64())) {
					number, hash = headers[i].Number.Uint64(), headers[i].Hash()
					//如果每个标题都是已知的，甚至未来的标题，对等人都会直接说谎
					// If every header is known, even future ones, the peer straight out lied about its head
					if number > height && i == limit-1 {
						p.log.Warn("Lied about chain head", "reported", height, "found", number)
						return 0, errStallingPeer
					}
					break
				}
			}

		case <-timeout:
			p.log.Debug("Waiting for head header timed out", "elapsed", ttl)
			return 0, errTimeout

		case <-d.bodyCh:
		case <-d.receiptCh:
			//越界交付，忽略
			// Out of bounds delivery, ignore
		}
	}
	//如果头部获取已经找到祖先，则返回
	// If the head fetch already found an ancestor, return
	if !common.EmptyHash(hash) {
		if int64(number) <= floor {
			p.log.Warn("Ancestor below allowance", "number", number, "hash", hash, "allowance", floor)
			return 0, errInvalidAncestor
		}
		p.log.Debug("Found common ancestor", "number", number, "hash", hash)
		return number, nil
	}
	//找不到祖先，我们需要在我们的连锁店进行二进制搜索
	// Ancestor not found, we need to binary search over our chain
	start, end := uint64(0), head
	if floor > 0 {
		start = uint64(floor)
	}
	for start+1 < end {
		//将我们的链间隔分成两部分，并请求散列进行交叉检查
		// Split our chain interval in two, and request the hash to cross check
		check := (start + end) / 2

		ttl := d.requestTTL()
		timeout := time.After(ttl)

		go p.peer.RequestHeadersByNumber(check, 1, 0, false)
		//等待回复到这个请求
		// Wait until a reply arrives to this request
		for arrived := false; !arrived; {
			select {
			case <-d.cancelCh:
				return 0, errCancelHeaderFetch

			case packer := <-d.headerCh:
				//丢弃任何不是来自同辈的东西
				// Discard anything not from the origin peer
				if packer.PeerId() != p.id {
					log.Debug("Received headers from incorrect peer", "peer", packer.PeerId())
					break
				} //确保同伴实际上给了一些有效的东西
				// Make sure the peer actually gave something valid
				headers := packer.(*headerPack).headers
				if len(headers) != 1 {
					p.log.Debug("Multiple headers for single request", "headers", len(headers))
					return 0, errBadPeer
				}
				arrived = true
				//根据响应修改搜索间隔
				// Modify the search interval based on the response
				if (d.mode == FullSync && !d.blockchain.HasBlock(headers[0].Hash(), headers[0].Number.Uint64())) || (d.mode != FullSync && !d.lightchain.HasHeader(headers[0].Hash(), headers[0].Number.Uint64())) {
					end = check
					break
				}
				header := d.lightchain.GetHeaderByHash(headers[0].Hash()) // Independent of sync mode, header surely exists
				if header.Number.Uint64() != check {
					p.log.Debug("Received non requested header", "number", header.Number, "hash", header.Hash(), "request", check)
					return 0, errBadPeer
				}
				start = check

			case <-timeout:
				p.log.Debug("Waiting for search header timed out", "elapsed", ttl)
				return 0, errTimeout

			case <-d.bodyCh:
			case <-d.receiptCh:
				// Out of bounds delivery, ignore
			}
		}
	} //确保有效的祖先和返回
	// Ensure valid ancestry and return
	if int64(start) <= floor {
		p.log.Warn("Ancestor below allowance", "number", start, "hash", hash, "allowance", floor)
		return 0, errInvalidAncestor
	}
	p.log.Debug("Found common ancestor", "number", start, "hash", hash)
	return start, nil
}

//etchHeaders方法用来获取header。 然后根据获取的header去获取body和receipt等信息。
// fetchHeaders keeps retrieving headers concurrently from the number
// requested, until no more are returned, potentially throttling on the way. To
// facilitate concurrency but still protect against malicious nodes sending bad
// headers, we construct a header chain skeleton using the "origin" peer we are
// syncing with, and fill in the missing headers using anyone else. Headers from
// other peers are only accepted if they map cleanly to the skeleton. If no one
// can fill in the skeleton - not even the origin peer - it's assumed invalid and
// the origin is dropped.
//fetchHeaders不断的重复这样的操作，发送header请求，等待所有的返回。直到完成所有的header请求。
// 为了提高并发性，同时仍然能够防止恶意节点发送错误的header，我们使用我们正在同步的“origin”peer构造一个头文件链骨架，
// 并使用其他人填充缺失的header。 其他peer的header只有在干净地映射到骨架上时才被接受。
// 如果没有人能够填充骨架 - 甚至origin peer也不能填充 - 它被认为是无效的，并且origin peer也被丢弃。
func (d *Downloader) fetchHeaders(p *peerConnection, from uint64, pivot uint64) error {
	p.log.Debug("Directing header downloads", "origin", from)
	defer p.log.Debug("Header download terminated")
	//创建一个超时定时器，以及相关的标题提取器
	// Create a timeout timer, and the associated header fetcher
	skeleton := true            // Skeleton assembly phase or finishing up//骨架装配阶段或完成
	request := time.Now()       // time of the last skeleton fetch request//最后一次骨架获取请求的时间
	timeout := time.NewTimer(0) // timer to dump a non-responsive active peer//定时器转储无响应的主动对等体
	<-timeout.C                 // timeout channel should be initially empty//超时通道应该初始为空
	defer timeout.Stop()

	var ttl time.Duration
	getHeaders := func(from uint64) {
		request = time.Now()

		ttl = d.requestTTL()
		timeout.Reset(ttl)

		if skeleton { //填充骨架
			p.log.Trace("Fetching skeleton headers", "count", MaxHeaderFetch, "from", from)
			go p.peer.RequestHeadersByNumber(from+uint64(MaxHeaderFetch)-1, MaxSkeletonSize, MaxHeaderFetch-1, false)
		} else { // 直接请求
			p.log.Trace("Fetching full headers", "count", MaxHeaderFetch, "from", from)
			go p.peer.RequestHeadersByNumber(from, MaxHeaderFetch, 0, false)
		}
	}
	//开始拉动标题链骨架，直到全部完成
	// Start pulling the header chain skeleton until all is done
	getHeaders(from)

	for {
		select {
		case <-d.cancelCh: //网络上返回的header会投递到headerCh这个通道
			return errCancelHeaderFetch

		case packet := <-d.headerCh:
			//确保主动对等体给我们的骨架头
			// Make sure the active peer is giving us the skeleton headers
			if packet.PeerId() != p.id {
				log.Debug("Received skeleton from incorrect peer", "peer", packet.PeerId())
				break
			}
			headerReqTimer.UpdateSince(request)
			timeout.Stop()
			//如果骨架已完成，请直接从原点拉出剩余的首部标题
			// If the skeleton's finished, pull any remaining head headers directly from the origin
			if packet.Items() == 0 && skeleton {
				skeleton = false
				getHeaders(from)
				continue
			}
			// 如果没有更多的返回了。 那么告诉headerProcCh通道
			// If no more headers are inbound, notify the content fetchers and return
			if packet.Items() == 0 {
				//在数据透视表下载时不要中止标题提取
				// Don't abort header fetches while the pivot is downloading
				if atomic.LoadInt32(&d.committed) == 0 && pivot <= from {
					p.log.Debug("No headers, waiting for pivot commit")
					select {
					case <-time.After(fsHeaderContCheck):
						getHeaders(from)
						continue
					case <-d.cancelCh:
						return errCancelHeaderFetch
					}
				} //完成数据透视（或不进行快速同步）并且没有更多标题，终止进程
				// Pivot done (or not in fast sync) and no more headers, terminate the process
				p.log.Debug("No more headers available")
				select {
				case d.headerProcCh <- nil:
					return nil
				case <-d.cancelCh:
					return errCancelHeaderFetch
				}
			}
			headers := packet.(*headerPack).headers
			// 如果是需要填充骨架，那么在这个方法里面填充好
			// If we received a skeleton batch, resolve internals concurrently
			if skeleton {
				filled, proced, err := d.fillHeaderSkeleton(from, headers)
				if err != nil {
					p.log.Debug("Skeleton chain invalid", "err", err)
					return errInvalidChain
				}
				headers = filled[proced:]
				// proced代表已经处理完了多少个了。  所以只需要proced:后面的headers了
				from += uint64(proced)
			} //插入所有新的标题并获取下一批
			// Insert all the new headers and fetch the next batch
			if len(headers) > 0 {
				p.log.Trace("Scheduling new headers", "count", len(headers), "from", from)
				//投递到headerProcCh 然后继续循环。
				select {
				case d.headerProcCh <- headers:
				case <-d.cancelCh:
					return errCancelHeaderFetch
				}
				from += uint64(len(headers))
			}
			getHeaders(from)

		case <-timeout.C:
			if d.dropPeer == nil {
				//当`--copydb`用于本地副本时，dropPeer方法为零。
				//例如，如果例如超时 压实击中了错误的时间，并且可以忽略
				// The dropPeer method is nil when `--copydb` is used for a local copy.
				// Timeouts can occur if e.g. compaction hits at the wrong time, and can be ignored
				p.log.Warn("Downloader wants to drop peer, but peerdrop-function is not set", "peer", p.id)
				break
			} //标题检索超时，请考虑对等体是否坏掉
			// Header retrieval timed out, consider the peer bad and drop
			p.log.Debug("Header request timed out", "elapsed", ttl)
			headerTimeoutMeter.Mark(1)
			d.dropPeer(p.id)
			//优雅地完成同步，而不是倾销收集的数据
			// Finish the sync gracefully instead of dumping the gathered data though
			for _, ch := range []chan bool{d.bodyWakeCh, d.receiptWakeCh} {
				select {
				case ch <- false:
				case <-d.cancelCh:
				}
			}
			select {
			case d.headerProcCh <- nil:
			case <-d.cancelCh:
			}
			return errBadPeer
		}
	}
}

// fillHeaderSkeleton并发地从我们所有可用的同伴中检索标题并将它们映射到提供的骨架标题链。
//从骨架开始的任何部分结果是（如果可能）转发
//立即到头部处理器，以保持管道的其余部分即使在头部停顿时也是如此。
//该方法修改整个填充的骨架以及已经转发处理的头部数量。
// fillHeaderSkeleton concurrently retrieves headers from all our available peers
// and maps them to the provided skeleton header chain.
//
// Any partial results from the beginning of the skeleton is (if possible) forwarded
// immediately to the header processor to keep the rest of the pipeline full even
// in the case of header stalls.
//
// The method returs the entire filled skeleton and also the number of headers
// already forwarded for processing.
func (d *Downloader) fillHeaderSkeleton(from uint64, skeleton []*types.Header) ([]*types.Header, int, error) {
	log.Debug("Filling up skeleton", "from", from)
	d.queue.ScheduleSkeleton(from, skeleton)

	var (
		deliver = func(packet dataPack) (int, error) {
			pack := packet.(*headerPack)
			return d.queue.DeliverHeaders(pack.peerId, pack.headers, d.headerProcCh)
		}
		expire   = func() map[string]int { return d.queue.ExpireHeaders(d.requestTTL()) }
		throttle = func() bool { return false }
		reserve  = func(p *peerConnection, count int) (*fetchRequest, bool, error) {
			return d.queue.ReserveHeaders(p, count), false, nil
		}
		fetch    = func(p *peerConnection, req *fetchRequest) error { return p.FetchHeaders(req.From, MaxHeaderFetch) }
		capacity = func(p *peerConnection) int { return p.HeaderCapacity(d.requestRTT()) }
		setIdle  = func(p *peerConnection, accepted int) { p.SetHeadersIdle(accepted) }
	)
	err := d.fetchParts(errCancelHeaderFetch, d.headerCh, deliver, d.queue.headerContCh, expire,
		d.queue.PendingHeaders, d.queue.InFlightHeaders, throttle, reserve,
		nil, fetch, d.queue.CancelHeaders, capacity, d.peers.HeaderIdlePeers, setIdle, "headers")

	log.Debug("Skeleton fill terminated", "err", err)

	filled, proced := d.queue.RetrieveHeaders()
	return filled, proced, err
}

//fetchBodies函数定义了一些闭包函数，然后调用了fetchParts函数
// fetchBodies iteratively downloads the scheduled block bodies, taking any
// available peers, reserving a chunk of blocks for each, waiting for delivery
// and also periodically checking for timeouts.
// fetchBodies 持续的下载区块体，中间会使用到任何可以用的链接，
// 为每一个链接保留一部分的区块体，等待区块被交付，并定期的检查是否超时。

func (d *Downloader) fetchBodies(from uint64) error {
	log.Debug("Downloading block bodies", "origin", from)

	var (
		deliver = func(packet dataPack) (int, error) { //下载完的区块体的交付函数
			pack := packet.(*bodyPack)
			return d.queue.DeliverBodies(pack.peerId, pack.transactions, pack.uncles)
		}
		expire   = func() map[string]int { return d.queue.ExpireBodies(d.requestTTL()) }          //超时
		fetch    = func(p *peerConnection, req *fetchRequest) error { return p.FetchBodies(req) } // fetch函数
		capacity = func(p *peerConnection) int { return p.BlockCapacity(d.requestRTT()) }         // 对端的吞吐量
		setIdle  = func(p *peerConnection, accepted int) { p.SetBodiesIdle(accepted) }            // 设置peer为idle
	)
	err := d.fetchParts(errCancelBodyFetch, d.bodyCh, deliver, d.bodyWakeCh, expire,
		d.queue.PendingBlocks, d.queue.InFlightBlocks, d.queue.ShouldThrottleBlocks, d.queue.ReserveBodies,
		d.bodyFetchHook, fetch, d.queue.CancelBodies, capacity, d.peers.BodyIdlePeers, setIdle, "bodies")

	log.Debug("Block body download terminated", "err", err)
	return err
}

//receipt的处理和body类似。
// fetchReceipts iteratively downloads the scheduled block receipts, taking any
// available peers, reserving a chunk of receipts for each, waiting for delivery
// and also periodically checking for timeouts.
// fetchReceipts迭代地下载计划的块收据，获取任何可用的同龄人，为每个收据预留一大块收据，等待交付并且还定期检查超时。
func (d *Downloader) fetchReceipts(from uint64) error {
	log.Debug("Downloading transaction receipts", "origin", from)

	var (
		deliver = func(packet dataPack) (int, error) {
			pack := packet.(*receiptPack)
			return d.queue.DeliverReceipts(pack.peerId, pack.receipts)
		}
		expire   = func() map[string]int { return d.queue.ExpireReceipts(d.requestTTL()) }
		fetch    = func(p *peerConnection, req *fetchRequest) error { return p.FetchReceipts(req) }
		capacity = func(p *peerConnection) int { return p.ReceiptCapacity(d.requestRTT()) }
		setIdle  = func(p *peerConnection, accepted int) { p.SetReceiptsIdle(accepted) }
	)
	err := d.fetchParts(errCancelReceiptFetch, d.receiptCh, deliver, d.receiptWakeCh, expire,
		d.queue.PendingReceipts, d.queue.InFlightReceipts, d.queue.ShouldThrottleReceipts, d.queue.ReserveReceipts,
		d.receiptFetchHook, fetch, d.queue.CancelReceipts, capacity, d.peers.ReceiptIdlePeers, setIdle, "receipts")

	log.Debug("Transaction receipt download terminated", "err", err)
	return err
}

// fetchParts iteratively downloads scheduled block parts, taking any available
// peers, reserving a chunk of fetch requests for each, waiting for delivery and
// also periodically checking for timeouts.
// fetchParts迭代地下载预定的块部分，取得任何可用的对等体，为每个部分预留大量的提取请求，等待交付并且还定期检查超时。
// As the scheduling/timeout logic mostly is the same for all downloaded data
// types, this method is used by each for data gathering and is instrumented with
// various callbacks to handle the slight differences between processing them.
// 由于调度/超时逻辑对于所有下载的数据类型大部分是相同的，所以这个方法被用于不同的区块类型的下载，并且用各种回调函数来处理它们之间的细微差别。
// The instrumentation parameters:
//  - errCancel:   error type to return if the fetch operation is cancelled (mostly makes logging nicer) 如果fetch操作被取消，会在这个通道上发送数据
//  - deliveryCh:  channel from which to retrieve downloaded data packets (merged from all concurrent peers) 数据被下载完成后投递的目的地
//  - deliver:     processing callback to deliver data packets into type specific download queues (usually within `queue`) 处理完成后数据被投递到哪个队列
//  - wakeCh:      notification channel for waking the fetcher when new tasks are available (or sync completed) 用来通知fetcher 新的任务到来，或者是同步完成
//  - expire:      task callback method to abort requests that took too long and return the faulty peers (traffic shaping)  因为超时来终止请求的回调函数。
//  - pending:     task callback for the number of requests still needing download (detect completion/non-completability) 还需要下载的任务的数量。
//  - inFlight:    task callback for the number of in-progress requests (wait for all active downloads to finish) 正在处理过程中的请求数量
//  - throttle:    task callback to check if the processing queue is full and activate throttling (bound memory use) 用来检查处理队列是否满的回调函数。
//  - reserve:     task callback to reserve new download tasks to a particular peer (also signals partial completions)  用来为某个peer来预定任务的回调函数
//  - fetchHook:   tester callback to notify of new tasks being initiated (allows testing the scheduling logic)
//  - fetch:       network callback to actually send a particular download request to a physical remote peer //发送网络请求的回调函数
//  - cancel:      task callback to abort an in-flight download request and allow rescheduling it (in case of lost peer)  用来取消正在处理的任务的回调函数
//  - capacity:    network callback to retrieve the estimated type-specific bandwidth capacity of a peer (traffic shaping) 网络容量或者是带宽。
//  - idle:        network callback to retrieve the currently (type specific) idle peers that can be assigned tasks  peer是否空闲的回调函数
//  - setIdle:     network callback to set a peer back to idle and update its estimated capacity (traffic shaping)  设置peer为空闲的回调函数
//  - kind:        textual label of the type being downloaded to display in log mesages   下载类型，用于日志
func (d *Downloader) fetchParts(errCancel error, deliveryCh chan dataPack, deliver func(dataPack) (int, error), wakeCh chan bool,
	expire func() map[string]int, pending func() int, inFlight func() bool, throttle func() bool, reserve func(*peerConnection, int) (*fetchRequest, bool, error),
	fetchHook func([]*types.Header), fetch func(*peerConnection, *fetchRequest) error, cancel func(*fetchRequest), capacity func(*peerConnection) int,
	idle func() ([]*peerConnection, int), setIdle func(*peerConnection, int), kind string) error {

	// Create a ticker to detect expired retrieval tasks
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	update := make(chan struct{}, 1)

	// Prepare the queue and fetch block parts until the block header fetcher's done
	finished := false
	for {
		select {
		case <-d.cancelCh:
			return errCancel

		case packet := <-deliveryCh:
			// If the peer was previously banned and failed to deliver its pack
			// in a reasonable time frame, ignore its message.
			// 如果peer在之前被禁止而且没有在合适的时间deliver它的数据，那么忽略这个数据
			if peer := d.peers.Peer(packet.PeerId()); peer != nil {
				//发送接收到的数据块并检查链路有效性
				// Deliver the received chunk of data and check chain validity
				accepted, err := deliver(packet)
				if err == errInvalidChain {
					return err
				}
				//除非对方完成除请求之外的其他事情（通常是由最终发生的超时请求引起的），否则将其设置为空闲。 如果交付过期，对方应该已经闲置。
				// Unless a peer delivered something completely else than requested (usually
				// caused by a timed out request which came through in the end), set it to
				// idle. If the delivery's stale, the peer should have already been idled.
				if err != errStaleDelivery {
					setIdle(peer, accepted)
				} //向用户发出日志以查看发生了什么
				// Issue a log to the user to see what's going on
				switch {
				case err == nil && packet.Items() == 0:
					peer.log.Trace("Requested data not delivered", "type", kind)
				case err == nil:
					peer.log.Trace("Delivered new batch of data", "type", kind, "count", packet.Stats())
				default:
					peer.log.Trace("Failed to deliver retrieved data", "type", kind, "err", err)
				}
			} //拼装块，尝试更新进度
			// Blocks assembled, try to update the progress
			select {
			case update <- struct{}{}:
			default:
			}
			// 当所有的任务完成的时候会写入这个队列。
		case cont := <-wakeCh:
			//标题提取器发送了一个继续标志，检查它是否完成
			// The header fetcher sent a continuation flag, check if it's done
			if !cont {
				finished = true
			} //标题到达，尝试更新进度
			// Headers arrive, try to update the progress
			select {
			case update <- struct{}{}:
			default:
			}

		case <-ticker.C:
			// Sanity check update the progress// Sanity检查更新进度
			select {
			case update <- struct{}{}:
			default:
			}

		case <-update: //如果我们失去了所有的同伴，就会造成短路
			// Short circuit if we lost all our peers
			if d.peers.Len() == 0 {
				return errNoPeers
			} //检查提取请求超时并降级负责的同级
			// Check for fetch request timeouts and demote the responsible peers
			for pid, fails := range expire() {
				if peer := d.peers.Peer(pid); peer != nil {
					// If a lot of retrieval elements expired, we might have overestimated the remote peer or perhaps
					// ourselves. Only reset to minimal throughput but don't drop just yet. If even the minimal times
					// out that sync wise we need to get rid of the peer.
					//如果很多检索元素过期，我们可能高估了远程对象或者我们自己。
					// 只能重置为最小的吞吐量，但不要丢弃。 如果即使最小的同步任然超时，我们需要删除peer。

					// The reason the minimum threshold is 2 is because the downloader tries to estimate the bandwidth
					// and latency of a peer separately, which requires pushing the measures capacity a bit and seeing
					// how response times reacts, to it always requests one more than the minimum (i.e. min 2).
					// 最小阈值为2的原因是因为下载器试图分别估计对等体的带宽和等待时间，
					// 这需要稍微推动测量容量并且看到响应时间如何反应，总是要求比最小值（即，最小值2）。

					if fails > 2 {
						peer.log.Trace("Data delivery timed out", "type", kind)
						setIdle(peer, 0)
					} else {
						peer.log.Debug("Stalling delivery, dropping", "type", kind)
						if d.dropPeer == nil {
							// The dropPeer method is nil when `--copydb` is used for a local copy.
							// Timeouts can occur if e.g. compaction hits at the wrong time, and can be ignored
							peer.log.Warn("Downloader wants to drop peer, but peerdrop-function is not set", "peer", pid)
						} else {
							d.dropPeer(pid)
						}
					}
				}
			}
			// 任务全部完成。 那么退出
			// If there's nothing more to fetch, wait or terminate
			if pending() == 0 { //如果没有等待分配的任务， 那么break。不用执行下面的代码了。
				if !inFlight() && finished {
					log.Debug("Data fetching completed", "type", kind)
					return nil
				}
				break
			} //向所有空闲的对等点发送下载请求，直到被限制
			// Send a download request to all idle peers, until throttled
			progressed, throttled, running := false, false, inFlight()
			idles, total := idle()

			for _, peer := range idles {
				//如果启用节流，则会短路
				// Short circuit if throttling activated
				if throttle() {
					throttled = true
					break
				} //如果没有更多可用任务，则短路。
				// Short circuit if there is no more available task.
				if pending() == 0 {
					break
				} //为对等体预留一大块提取。 一个零可能意味着没有更多的头可用，或者已知对等体没有它们
				// Reserve a chunk of fetches for a peer. A nil can mean either that
				// no more headers are available, or that the peer is known not to
				// have them.
				// 为某个peer请求分配任务。
				request, progress, err := reserve(peer, capacity(peer))
				if err != nil {
					return err
				}
				if progress {
					progressed = true
				}
				if request == nil {
					continue
				}
				if request.From > 0 {
					peer.log.Trace("Requesting new batch of data", "type", kind, "from", request.From)
				} else {
					peer.log.Trace("Requesting new batch of data", "type", kind, "count", len(request.Headers), "from", request.Headers[0].Number)
				} //获取块并确保任何错误将哈希值返回队列
				// Fetch the chunk and make sure any errors return the hashes to the queue
				if fetchHook != nil {
					fetchHook(request.Headers)
				}
				if err := fetch(peer, request); err != nil {
					//尽管我们可以尝试并尝试解决这个问题，但这个错误实际上意味着我们已经将一个提取任务分配给了一个对等体。
					// 如果是这样的话，下载器和队列的内部状态是非常错误的，所以更好的硬性崩溃并注意错误，而不是悄悄地积累到更大的问题中
					// Although we could try and make an attempt to fix this, this error really
					// means that we've double allocated a fetch task to a peer. If that is the
					// case, the internal state of the downloader and the queue is very wrong so
					// better hard crash and note the error instead of silently accumulating into
					// a much bigger issue.
					panic(fmt.Sprintf("%v: %s fetch assignment failed", peer, kind))
				}
				running = true
			} //确保我们有可用于获取的对等体。 如果所有同龄人已经尝试过，并且所有失败都会抛出错误
			// Make sure that we have peers available for fetching. If all peers have been tried
			// and all failed throw an error
			if !progressed && !throttled && !running && len(idles) == total && pending() > 0 {
				return errPeersUnavailable
			}
		}
	}
}

//processHeaders方法，这个方法从headerProcCh通道来获取header。
// 并把获取到的header丢入到queue来进行调度，
// 这样body fetcher或者是receipt fetcher就可以领取到fetch任务。
// processHeaders takes batches of retrieved headers from an input channel and
// keeps processing and scheduling them into the header chain and downloader's
// queue until the stream ends or a failure occurs.
// processHeaders批量的获取headers， 处理他们，并通过downloader的queue对象来调度他们。 直到错误发生或者处理结束。
func (d *Downloader) processHeaders(origin uint64, pivot uint64, td *big.Int) error {
	// Keep a count of uncertain headers to roll back
	// rollback 用来处理这种逻辑，如果某个点失败了。那么之前插入的2048个节点都要回滚。因为安全性达不到要求， 可以详细参考fast sync的文档。

	rollback := []*types.Header{}
	defer func() { // 这个函数用来错误退出的时候进行回滚。 TODO
		if len(rollback) > 0 {
			//展开标题并将其回滚
			// Flatten the headers and roll them back
			hashes := make([]common.Hash, len(rollback))
			for i, header := range rollback {
				hashes[i] = header.Hash()
			}
			lastHeader, lastFastBlock, lastBlock := d.lightchain.CurrentHeader().Number, common.Big0, common.Big0
			if d.mode != LightSync {
				lastFastBlock = d.blockchain.CurrentFastBlock().Number()
				lastBlock = d.blockchain.CurrentBlock().Number()
			}
			d.lightchain.Rollback(hashes)
			curFastBlock, curBlock := common.Big0, common.Big0
			if d.mode != LightSync {
				curFastBlock = d.blockchain.CurrentFastBlock().Number()
				curBlock = d.blockchain.CurrentBlock().Number()
			}
			log.Warn("Rolled back headers", "count", len(hashes),
				"header", fmt.Sprintf("%d->%d", lastHeader, d.lightchain.CurrentHeader().Number),
				"fast", fmt.Sprintf("%d->%d", lastFastBlock, curFastBlock),
				"block", fmt.Sprintf("%d->%d", lastBlock, curBlock))
		}
	}()
	//等待批量标题进行处理
	// Wait for batches of headers to process
	gotHeaders := false

	for {
		select {
		case <-d.cancelCh:
			return errCancelHeaderProcessing

		case headers := <-d.headerProcCh:
			// Terminate header processing if we synced up/ /如果我们同步了终止头处理
			if len(headers) == 0 { //处理完成
				//通知每个人头部已经完全处理
				// Notify everyone that headers are fully processed
				for _, ch := range []chan bool{d.bodyWakeCh, d.receiptWakeCh} {
					select {
					case ch <- false:
					case <-d.cancelCh:
					}
				}

				//如果根本没有找到标题，那么对等方违反了TD的承诺，即它与我们的链相比具有更好的链。
				// 唯一的例外是，如果其承诺的区块已经通过其他方式进口（例如fecher）：
				// R <remote peer>，L <本地节点>：两者都在框10
				// R：矿区块11，并传播给L
				// L：队列块11用于导入
				// L：注意R的头部和TD比我们增加了，开始同步
				// L：块11的导入结束
				// L：同步开始，并在11找到共同的祖先
				// L：从11开始申请新的标题（R的TD更高，它必须有东西）
				// R：没什么好说的
				// If no headers were retrieved at all, the peer violated its TD promise that it had a
				// better chain compared to ours. The only exception is if its promised blocks were
				// already imported by other means (e.g. fecher):
				//
				// R <remote peer>, L <local node>: Both at block 10
				// R: Mine block 11, and propagate it to L
				// L: Queue block 11 for import
				// L: Notice that R's head and TD increased compared to ours, start sync
				// L: Import of block 11 finishes
				// L: Sync begins, and finds common ancestor at 11
				// L: Request new headers up from 11 (R's TD was higher, it must have something)
				// R: Nothing to give
				if d.mode != LightSync { // 对方的TD比我们大，但是没有获取到任何东西。 那么认为对方是错误的对方。 会断开和对方的联系
					head := d.blockchain.CurrentBlock()
					if !gotHeaders && td.Cmp(d.blockchain.GetTd(head.Hash(), head.NumberU64())) > 0 {
						return errStallingPeer
					}
				}
				//如果快速或轻微同步，确保承诺的标题确实被传送。 这需要用于检测攻击者提供不良数据透视的情况，然后从提供标记无效内容的后透视数据块中释放出来。
				//对于完整导入，此检查无法“按原样”执行，因为在标题下载完成时，块仍可能排队等待处理。
				// 然而，只要同伴给我们一些有用的东西，我们就已经很高兴/进步（高于检查）。
				// If fast or light syncing, ensure promised headers are indeed delivered. This is
				// needed to detect scenarios where an attacker feeds a bad pivot and then bails out
				// of delivering the post-pivot blocks that would flag the invalid content.
				//
				// This check cannot be executed "as is" for full imports, since blocks may still be
				// queued for processing when the header download completes. However, as long as the
				// peer gave us something useful, we're already happy/progressed (above check).
				if d.mode == FastSync || d.mode == LightSync { //如果是快速同步模式，或者是轻量级同步模式(只下载区块头)
					head := d.lightchain.CurrentHeader()
					if td.Cmp(d.lightchain.GetTd(head.Hash(), head.Number.Uint64())) > 0 {
						return errStallingPeer
					}
				}
				//禁用任何回滚并返回
				// Disable any rollback and return
				rollback = nil
				return nil
			}
			//否则将标题块分成批并处理它们
			// Otherwise split the chunk of headers into batches and process them
			gotHeaders = true

			for len(headers) > 0 { //如果在处理块之间失败，终止
				// Terminate if something failed in between processing chunks
				select {
				case <-d.cancelCh:
					return errCancelHeaderProcessing
				default:
				} //选择要导入的下一个标题块
				// Select the next chunk of headers to import
				limit := maxHeadersProcess
				if limit > len(headers) {
					limit = len(headers)
				}
				chunk := headers[:limit]
				//在头只同步的情况下，立即验证块
				// In case of header only syncing, validate the chunk immediately
				if d.mode == FastSync || d.mode == LightSync {
					//收集尚未识别的标题，将它们标记为不确定
					// Collect the yet unknown headers to mark them as uncertain
					unknown := make([]*types.Header, 0, len(headers))
					for _, header := range chunk {
						if !d.lightchain.HasHeader(header.Hash(), header.Number.Uint64()) {
							unknown = append(unknown, header)
						}
					}
					// 每隔多少个区块验证一次
					// If we're importing pure headers, verify based on their recentness
					frequency := fsHeaderCheckFrequency
					if chunk[len(chunk)-1].Number.Uint64()+uint64(fsHeaderForceVerify) > pivot {
						frequency = 1
					}
					// lightchain默认是等于chain的。 插入区块头。如果失败那么需要回滚。
					if n, err := d.lightchain.InsertHeaderChain(chunk, frequency); err != nil {
						//如果插入了一些标题，则将它们也添加到回滚列表中
						// If some headers were inserted, add them too to the rollback list
						if n > 0 {
							rollback = append(rollback, chunk[:n]...)
						}
						log.Debug("Invalid header encountered", "number", chunk[n].Number, "hash", chunk[n].Hash(), "err", err)
						return errInvalidChain
					} //通过所有验证，存储新发现的不确定标题
					// All verifications passed, store newly found uncertain headers
					rollback = append(rollback, unknown...)
					if len(rollback) > fsHeaderSafetyNet {
						rollback = append(rollback[:0], rollback[len(rollback)-fsHeaderSafetyNet:]...)
					}
				}
				// 如果我们处理完轻量级链。 调度header来进行相关数据的获取。body，receipts
				// Unless we're doing light chains, schedule the headers for associated content retrieval
				if d.mode == FullSync || d.mode == FastSync {
					// If we've reached the allowed number of pending headers, stall a bit
					// 如果当前queue的容量容纳不下了。那么等待。
					for d.queue.PendingBlocks() >= maxQueuedHeaders || d.queue.PendingReceipts() >= maxQueuedHeaders {
						select {
						case <-d.cancelCh:
							return errCancelHeaderProcessing
						case <-time.After(time.Second):
						}
					}
					// 调用Queue进行调度，下载body和receipts
					// Otherwise insert the headers for content retrieval
					inserts := d.queue.Schedule(chunk, origin)
					if len(inserts) != len(chunk) {
						log.Debug("Stale headers")
						return errBadPeer
					}
				}
				headers = headers[limit:]
				origin += uint64(limit)
			}
			//更新我们知道的最高块号，如果找到更高的块号。
			// Update the highest block number we know if a higher one is found.
			d.syncStatsLock.Lock()
			if d.syncStatsChainHeight < origin {
				d.syncStatsChainHeight = origin - 1
			}
			d.syncStatsLock.Unlock()
			// 给通道d.bodyWakeCh, d.receiptWakeCh发送消息，唤醒处理线程。
			// Signal the content downloaders of the availablility of new tasks
			for _, ch := range []chan bool{d.bodyWakeCh, d.receiptWakeCh} {
				select {
				case ch <- true:
				default:
				}
			}
		}
	}
}

//processFullSyncContent,比较简单。 从队列里面获取区块然后插入。
// processFullSyncContent takes fetch results from the queue and imports them into the chain.
func (d *Downloader) processFullSyncContent() error {
	for {
		results := d.queue.Results(true)
		if len(results) == 0 {
			return nil
		}
		if d.chainInsertHook != nil {
			d.chainInsertHook(results)
		}
		if err := d.importBlockResults(results); err != nil {
			return err
		}
	}
}

func (d *Downloader) importBlockResults(results []*fetchResult) error {
	// Check for any early termination requests//检查是否有提前终止请求
	if len(results) == 0 {
		return nil
	}
	select {
	case <-d.quitCh:
		return errCancelContentProcessing
	default:
	} //检索要导入的一批结果
	// Retrieve the a batch of results to import
	first, last := results[0].Header, results[len(results)-1].Header
	log.Debug("Inserting downloaded chain", "items", len(results),
		"firstnum", first.Number, "firsthash", first.Hash(),
		"lastnum", last.Number, "lasthash", last.Hash(),
	)
	blocks := make([]*types.Block, len(results))
	for i, result := range results {
		blocks[i] = types.NewBlockWithHeader(result.Header).WithBody(result.Transactions, result.Uncles)
	}
	if index, err := d.blockchain.InsertChain(blocks); err != nil {
		log.Debug("Downloaded item processing failed", "number", results[index].Header.Number, "hash", results[index].Header.Hash(), "err", err)
		return errInvalidChain
	}
	return nil
}

//我们下面看看哪里会调用syncState()函数。processFastSyncContent这个函数会在最开始发现peer的时候启动
// processFastSyncContent takes fetch results from the queue and writes them to the
// database. It also controls the synchronisation of state nodes of the pivot block.
func (d *Downloader) processFastSyncContent(latest *types.Header) error {
	//开始同步报告的头块的状态。 这应该让我们了解大部分的数据透视表的状态。
	// Start syncing state of the reported head block. This should get us most of
	// the state of the pivot block.
	// 启动状态同步
	stateSync := d.syncState(latest.Root)
	defer stateSync.Cancel()
	go func() {
		if err := stateSync.Wait(); err != nil && err != errCancelStateFetch {
			d.queue.Close() // wake up WaitResults
		}
	}()
	//找出理想的枢轴块。 请注意，如果同步花费足够长时间以使链头显着移动，则此门柱可能会移动。
	// Figure out the ideal pivot block. Note, that this goalpost may move if the
	// sync takes long enough for the chain head to move significantly.
	pivot := uint64(0)
	if height := latest.Number.Uint64(); height > uint64(fsMinFullBlocks) {
		pivot = height - uint64(fsMinFullBlocks)
	} //为了迎合移动的枢轴点，跟踪数据透视块并随后分开累积下载结果。
	// To cater for moving pivot points, track the pivot block and subsequently
	// accumulated download results separatey.
	var (
		oldPivot *fetchResult   // Locked in pivot block, might change eventually//锁定在数据透视块中，最终可能会改变
		oldTail  []*fetchResult // Downloaded content after the pivot//在数据透视后下载内容
	)
	for {
		//等待下一批下载的数据可用，并且如果数据透视块变旧，则移动门柱
		// Wait for the next batch of downloaded data to be available, and if the pivot
		// block became stale, move the goalpost
		// 等待队列输出处理完成的区块
		results := d.queue.Results(oldPivot == nil) // Block if we're not monitoring pivot staleness
		if len(results) == 0 {
			// If pivot sync is done, stop//如果主轴同步完成，停止
			if oldPivot == nil {
				return stateSync.Cancel()
			}
			// If sync failed, stop//如果同步失败，请停止
			select {
			case <-d.cancelCh:
				return stateSync.Cancel()
			default:
			}
		}
		if d.chainInsertHook != nil {
			d.chainInsertHook(results)
		}
		if oldPivot != nil {
			results = append(append([]*fetchResult{oldPivot}, oldTail...), results...)
		} //围绕枢轴块分割并通过快速/完全同步处理双方
		// Split around the pivot block and process the two sides via fast/full sync
		if atomic.LoadInt32(&d.committed) == 0 {
			latest = results[len(results)-1].Header
			if height := latest.Number.Uint64(); height > pivot+2*uint64(fsMinFullBlocks) {
				log.Warn("Pivot became stale, moving", "old", pivot, "new", height-uint64(fsMinFullBlocks))
				pivot = height - uint64(fsMinFullBlocks)
			}
		}
		P, beforeP, afterP := splitAroundPivot(pivot, results)
		// 插入fast sync的数据
		if err := d.commitFastSyncData(beforeP, stateSync); err != nil {
			return err
		}
		if P != nil { //如果找到新的数据透视块，则取消旧的状态检索并重新启动
			// If new pivot block found, cancel old state retrieval and restart
			if oldPivot != P {
				stateSync.Cancel()
				// 如果已经达到了 pivot point 那么等待状态同步完成
				stateSync = d.syncState(P.Header.Root)
				defer stateSync.Cancel()
				go func() {
					if err := stateSync.Wait(); err != nil && err != errCancelStateFetch {
						d.queue.Close() // wake up WaitResults/唤醒WaitResults
					}
				}()
				oldPivot = P
			} //等待完成，偶尔检查枢轴过时
			// Wait for completion, occasionally checking for pivot staleness
			select {
			case <-stateSync.done:
				if stateSync.err != nil {
					return stateSync.err
				}
				if err := d.commitPivotBlock(P); err != nil {
					return err
				}
				oldPivot = nil

			case <-time.After(time.Second):
				oldTail = afterP
				continue
			}
		}
		// 对于pivot point 之后的所有节点，都需要按照完全的处理。
		// Fast sync done, pivot commit done, full import
		if err := d.importBlockResults(afterP); err != nil {
			return err
		}
	}
}

func splitAroundPivot(pivot uint64, results []*fetchResult) (p *fetchResult, before, after []*fetchResult) {
	for _, result := range results {
		num := result.Header.Number.Uint64()
		switch {
		case num < pivot:
			before = append(before, result)
		case num == pivot:
			p = result
		default:
			after = append(after, result)
		}
	}
	return p, before, after
}

func (d *Downloader) commitFastSyncData(results []*fetchResult, stateSync *stateSync) error {
	// Check for any early termination requests//检查是否有提前终止请求
	if len(results) == 0 {
		return nil
	}
	select {
	case <-d.quitCh:
		return errCancelContentProcessing
	case <-stateSync.done:
		if err := stateSync.Wait(); err != nil {
			return err
		}
	default:
	} //检索要导入的一批结果
	// Retrieve the a batch of results to import
	first, last := results[0].Header, results[len(results)-1].Header
	log.Debug("Inserting fast-sync blocks", "items", len(results),
		"firstnum", first.Number, "firsthash", first.Hash(),
		"lastnumn", last.Number, "lasthash", last.Hash(),
	)
	blocks := make([]*types.Block, len(results))
	receipts := make([]types.Receipts, len(results))
	for i, result := range results {
		blocks[i] = types.NewBlockWithHeader(result.Header).WithBody(result.Transactions, result.Uncles)
		receipts[i] = result.Receipts
	}
	if index, err := d.blockchain.InsertReceiptChain(blocks, receipts); err != nil {
		log.Debug("Downloaded item processing failed", "number", results[index].Header.Number, "hash", results[index].Header.Hash(), "err", err)
		return errInvalidChain
	}
	return nil
}

func (d *Downloader) commitPivotBlock(result *fetchResult) error {
	block := types.NewBlockWithHeader(result.Header).WithBody(result.Transactions, result.Uncles)
	log.Debug("Committing fast sync pivot as new head", "number", block.Number(), "hash", block.Hash())
	if _, err := d.blockchain.InsertReceiptChain([]*types.Block{block}, []types.Receipts{result.Receipts}); err != nil {
		return err
	}
	if err := d.blockchain.FastSyncCommitHead(block.Hash()); err != nil {
		return err
	}
	atomic.StoreInt32(&d.committed, 1)
	return nil
}

// DeliverHeaders将从远程节点接收到的新批块标题插入下载时间表。
// DeliverHeaders injects a new batch of block headers received from a remote
// node into the download schedule.
func (d *Downloader) DeliverHeaders(id string, headers []*types.Header) (err error) {
	return d.deliver(id, d.headerCh, &headerPack{id, headers}, headerInMeter, headerDropMeter)
}

// DeliverBody注入从远程节点接收到的新批块体。
// DeliverBodies injects a new batch of block bodies received from a remote node.
func (d *Downloader) DeliverBodies(id string, transactions [][]*types.Transaction, uncles [][]*types.Header) (err error) {
	return d.deliver(id, d.bodyCh, &bodyPack{id, transactions, uncles}, bodyInMeter, bodyDropMeter)
}

// DeliverReceipts注入从远程节点接收到的新批次收据。
// DeliverReceipts injects a new batch of receipts received from a remote node.
func (d *Downloader) DeliverReceipts(id string, receipts [][]*types.Receipt) (err error) {
	return d.deliver(id, d.receiptCh, &receiptPack{id, receipts}, receiptInMeter, receiptDropMeter)
}

// DeliverNodeData注入从远程节点接收的一批新的节点状态数据。
// DeliverNodeData injects a new batch of node state data received from a remote node.
func (d *Downloader) DeliverNodeData(id string, data [][]byte) (err error) {
	return d.deliver(id, d.stateCh, &statePack{id, data}, stateInMeter, stateDropMeter)
}

//交付注入从远程节点接收到的一批新数据。
// deliver injects a new batch of data received from a remote node.
func (d *Downloader) deliver(id string, destCh chan dataPack, packet dataPack, inMeter, dropMeter metrics.Meter) (err error) {
	// Update the delivery metrics for both good and failed deliveries
	//更新好的和失败的交付的交付指标
	inMeter.Mark(int64(packet.Items()))
	defer func() {
		if err != nil {
			dropMeter.Mark(int64(packet.Items()))
		}
	}()
	//如果同步在排队时被取消，则传递或中止
	// Deliver or abort if the sync is canceled while queuing
	d.cancelLock.RLock()
	cancel := d.cancelCh
	d.cancelLock.RUnlock()
	if cancel == nil {
		return errNoSyncActive
	}
	select {
	case destCh <- packet:
		return nil
	case <-cancel:
		return errNoSyncActive
	}
}

// qosTuner是服务调优循环的质量，偶尔会收集对等延迟统计信息并更新估计的请求往返时间。
// qosTuner is the quality of service tuning loop that occasionally gathers the
// peer latency statistics and updates the estimated request round trip time.
func (d *Downloader) qosTuner() {
	for { //检索当前的RTT中位数并融入前一个目标RTT
		// Retrieve the current median RTT and integrate into the previoust target RTT
		rtt := time.Duration((1-qosTuningImpact)*float64(atomic.LoadUint64(&d.rttEstimate)) + qosTuningImpact*float64(d.peers.medianRTT()))
		atomic.StoreUint64(&d.rttEstimate, uint64(rtt))
		//新的RTT周期过去了，增加了我们对估计RTT的信心
		// A new RTT cycle passed, increase our confidence in the estimated RTT
		conf := atomic.LoadUint64(&d.rttConfidence)
		conf = conf + (1000000-conf)/2
		atomic.StoreUint64(&d.rttConfidence, conf)
		//记录新的QoS值并休眠，直到下一个RTT
		// Log the new QoS values and sleep until the next RTT
		log.Debug("Recalculated downloader QoS values", "rtt", rtt, "confidence", float64(conf)/1000000.0, "ttl", d.requestTTL())
		select {
		case <-d.quitCh:
			return
		case <-time.After(rtt):
		}
	}
}

// qosReduceConfidence意味着当一个新对等体加入下载器的对等设置时需要调用，这需要降低我们对QoS估计的置信度。
// qosReduceConfidence is meant to be called when a new peer joins the downloader's
// peer set, needing to reduce the confidence we have in out QoS estimates.
func (d *Downloader) qosReduceConfidence() {
	// If we have a single peer, confidence is always 1
	peers := uint64(d.peers.Len())
	if peers == 0 {
		//确保对等连接比赛不会让我们措手不及
		// Ensure peer connectivity races don't catch us off guard
		return
	}
	if peers == 1 {
		atomic.StoreUint64(&d.rttConfidence, 1000000)
		return
	} //如果我们有很多同龄人，请不要放弃信心）
	// If we have a ton of peers, don't drop confidence)
	if peers >= uint64(qosConfidenceCap) {
		return
	} //否则放弃置信因子
	// Otherwise drop the confidence factor
	conf := atomic.LoadUint64(&d.rttConfidence) * (peers - 1) / peers
	if float64(conf)/1000000 < rttMinConfidence {
		conf = uint64(rttMinConfidence * 1000000)
	}
	atomic.StoreUint64(&d.rttConfidence, conf)

	rtt := time.Duration(atomic.LoadUint64(&d.rttEstimate))
	log.Debug("Relaxed downloader QoS values", "rtt", rtt, "confidence", float64(conf)/1000000.0, "ttl", d.requestTTL())
}

// requestRTT返回当前目标往返时间，以完成下载请求。
//注意，返回的RTT是实际估计的RTT的0.9。 原因是下载器试图将查询调整到RTT，因此可以适应多个RTT值，但较小的RTT值可以被优先使用（更稳定的下载流）。
// requestRTT returns the current target round trip time for a download request
// to complete in.
//
// Note, the returned RTT is .9 of the actually estimated RTT. The reason is that
// the downloader tries to adapt queries to the RTT, so multiple RTT values can
// be adapted to, but smaller ones are preffered (stabler download stream).
func (d *Downloader) requestRTT() time.Duration {
	return time.Duration(atomic.LoadUint64(&d.rttEstimate)) * 9 / 10
}

// requestTTL返回单个下载请求完成时的当前超时限额。
// requestTTL returns the current timeout allowance for a single download request
// to finish under.
func (d *Downloader) requestTTL() time.Duration {
	var (
		rtt  = time.Duration(atomic.LoadUint64(&d.rttEstimate))
		conf = float64(atomic.LoadUint64(&d.rttConfidence)) / 1000000.0
	)
	ttl := time.Duration(ttlScaling) * time.Duration(float64(rtt)/conf)
	if ttl > ttlLimit {
		ttl = ttlLimit
	}
	return ttl
}
