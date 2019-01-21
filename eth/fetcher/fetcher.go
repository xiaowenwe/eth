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

// Package fetcher contains the block announcement based synchronisation.
package fetcher

import (
	"errors"
	"math/rand"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/log"
	"gopkg.in/karalabe/cookiejar.v2/collections/prque"
)

//fetcher包含基于块通知的同步。当我们接收到NewBlockHashesMsg消息得时候，我们只收到了很多Block的hash值。
// 需要通过hash值来同步区块，然后更新本地区块链。 fetcher就提供了这样的功能。
const (
	arriveTimeout = 500 * time.Millisecond // Time allowance before an announced block is explicitly requested明确要求宣布的区块之前的时间限额
	gatherSlack   = 100 * time.Millisecond // Interval used to collate almost-expired announces with fetches用于通过提取将几乎到期的通告进行整理的时间间隔
	fetchTimeout  = 5 * time.Second        // Maximum allotted time to return an explicitly requested block最大分配时间返回明确请求的块
	maxUncleDist  = 7                      // Maximum allowed backward distance from the chain head链头最大允许后退距离
	maxQueueDist  = 32                     // Maximum allowed distance from the chain head to queue从链头到队列的最大允许距离
	hashLimit     = 256                    // Maximum number of unique blocks a peer may have announced对等方可能宣布的唯一块的最大数量
	blockLimit    = 64                     // Maximum number of unique blocks a peer may have delivered对等体可能传递的唯一块的最大数量
)

var (
	errTerminated = errors.New("terminated")
)

// blockRetrievalFn是用于从本地链中检索块的回调类型。
// blockRetrievalFn is a callback type for retrieving a block from the local chain.
type blockRetrievalFn func(common.Hash) *types.Block

// headerRequesterFn是用于发送标题检索请求的回调类型。
// headerRequesterFn is a callback type for sending a header retrieval request.
type headerRequesterFn func(common.Hash) error

// bodyRequesterFn是发送正文检索请求的回调类型。
// bodyRequesterFn is a callback type for sending a body retrieval request.
type bodyRequesterFn func([]common.Hash) error

// headerVerifierFn是一个回调类型，用于验证块快速传播的头。
// headerVerifierFn is a callback type to verify a block's header for fast propagation.
type headerVerifierFn func(header *types.Header) error

// blockBroadcasterFn是用于向连接的对等点广播块的回调类型。
// blockBroadcasterFn is a callback type for broadcasting a block to connected peers.
type blockBroadcasterFn func(block *types.Block, propagate bool)

// chainHeightFn是一个回调类型来检索当前链高度。
// chainHeightFn is a callback type to retrieve the current chain height.
type chainHeightFn func() uint64

// chainInsertFn是一个回调类型，用于将一批块插入到本地链中。
// chainInsertFn is a callback type to insert a batch of blocks into the local chain.
type chainInsertFn func(types.Blocks) (int, error)

// peerDropFn是一种回调类型，用于删除检测为恶意的对等方。
// peerDropFn is a callback type for dropping a peer detected as malicious.
type peerDropFn func(id string)

// announce 是一个hash通知，表示网络上有合适的新区块出现
// announce is the hash notification of the availability of a new block in the
// network.
type announce struct {
	hash   common.Hash   // Hash of the block being announced//新区块的hash值
	number uint64        // Number of the block being announced (0 = unknown | old protocol) 区块的高度值，
	header *types.Header // Header of the block partially reassembled (new protocol)重新组装的区块头
	time   time.Time     // Timestamp of the announcement

	origin string // Identifier of the peer originating the notification

	fetchHeader headerRequesterFn // Fetcher function to retrieve the header of an announced block 获取区块头的函数指针， 里面包含了peer的信息。就是说找谁要这个区块头
	fetchBodies bodyRequesterFn   // Fetcher function to retrieve the body of an announced block获取区块体的函数指针
}

// headerFilterTask表示一批需要fetcher过滤的头文件。
// headerFilterTask represents a batch of headers needing fetcher filtering.
type headerFilterTask struct {
	peer    string          // The source peer of block headers//块头的源对等体
	headers []*types.Header // Collection of headers to filter//收集标题进行过滤
	time    time.Time       // Arrival time of the headers//标题的到达时间
}

// headerFilterTask代表一批块体（交易和叔叔）
//需要收件人过滤。
// headerFilterTask represents a batch of block bodies (transactions and uncles)
// needing fetcher filtering.
type bodyFilterTask struct {
	peer         string                 // The source peer of block bodies//块体的源对象
	transactions [][]*types.Transaction // Collection of transactions per block bodies
	uncles       [][]*types.Header      // Collection of uncles per block bodies每个块体的叔叔的集合
	time         time.Time              // Arrival time of the blocks' contents块内容的到达时间
}

// 当节点收到NewBlockMsg的消息时候，会插入一个区块
// inject represents a schedules import operation.
type inject struct {
	origin string
	block  *types.Block
}

// Fetcher负责累积来自各个对等方的块公告并安排它们进行检索。
// Fetcher is responsible for accumulating block announcements from various peers
// and scheduling them for retrieval.
type Fetcher struct {
	// Various event channels//各种活动频道
	notify chan *announce //announce的通道，
	inject chan *inject   //inject的通道

	blockFilter  chan chan []*types.Block //通道的通道
	headerFilter chan chan *headerFilterTask
	bodyFilter   chan chan *bodyFilterTask

	done chan common.Hash
	quit chan struct{}

	// Announce states
	announces  map[string]int              // Per peer announce counts to prevent memory exhaustion key是peer的名字， value是announce的count， 为了避免内存占用太大。
	announced  map[common.Hash][]*announce // Announced blocks, scheduled for fetching等待调度fetching的announce
	fetching   map[common.Hash]*announce   // Announced blocks, currently fetching正在fetching的announce
	fetched    map[common.Hash][]*announce // Blocks with headers fetched, scheduled for body retrieval 已经获取区块头的，等待获取区块body
	completing map[common.Hash]*announce   // Blocks with headers, currently body-completing头和体都已经获取完成的announce

	// Block cache
	queue  *prque.Prque            // Queue containing the import operations (block number sorted)包含了import操作的队列(按照区块号排列)
	queues map[string]int          // Per peer block counts to prevent memory exhaustion key是peer，value是block数量。 避免内存消耗太多。
	queued map[common.Hash]*inject // Set of already queued blocks (to dedup imports)已经放入队列的区块。 为了去重。
	// Callbacks  依赖了一些回调函数。
	// Callbacks
	getBlock       blockRetrievalFn   // Retrieves a block from the local chain//从本地链中获取一个块
	verifyHeader   headerVerifierFn   // Checks if a block's headers have a valid proof of work//检查块的标题是否有有效的工作证明
	broadcastBlock blockBroadcasterFn // Broadcasts a block to connected peers// Broadcasts a block to connected peers
	chainHeight    chainHeightFn      // Retrieves the current chain's height//检索当前链的高度
	insertChain    chainInsertFn      // Injects a batch of blocks into the chain//将一批块注入链中
	dropPeer       peerDropFn         // Drops a peer for misbehaving//因行为不当而丢弃同伴

	// Testing hooks  仅供测试使用。
	// Testing hooks
	announceChangeHook func(common.Hash, bool) // Method to call upon adding or deleting a hash from the announce list//调用从公告列表中添加或删除散列的方法
	queueChangeHook    func(common.Hash, bool) // Method to call upon adding or deleting a block from the import queue//调用添加或删除导入队列中的块的方法
	fetchingHook       func([]common.Hash)     // Method to call upon starting a block (eth/61) or header (eth/62) fetch//启动一个块（eth / 61）或头（eth / 62）获取的方法
	completingHook     func([]common.Hash)     // Method to call upon starting a block body fetch (eth/62)//开始块体读取时调用的方法（eth / 62）
	importedHook       func(*types.Block)      // Method to call upon successful block import (both eth/61 and eth/62)调用成功块导入的方法（eth / 61和eth / 62）
}

// New创建块提取器以基于哈希公告检索块。
// New creates a block fetcher to retrieve blocks based on hash announcements.
func New(getBlock blockRetrievalFn, verifyHeader headerVerifierFn, broadcastBlock blockBroadcasterFn, chainHeight chainHeightFn, insertChain chainInsertFn, dropPeer peerDropFn) *Fetcher {
	return &Fetcher{
		notify:         make(chan *announce),
		inject:         make(chan *inject),
		blockFilter:    make(chan chan []*types.Block),
		headerFilter:   make(chan chan *headerFilterTask),
		bodyFilter:     make(chan chan *bodyFilterTask),
		done:           make(chan common.Hash),
		quit:           make(chan struct{}),
		announces:      make(map[string]int),
		announced:      make(map[common.Hash][]*announce),
		fetching:       make(map[common.Hash]*announce),
		fetched:        make(map[common.Hash][]*announce),
		completing:     make(map[common.Hash]*announce),
		queue:          prque.New(),
		queues:         make(map[string]int),
		queued:         make(map[common.Hash]*inject),
		getBlock:       getBlock,
		verifyHeader:   verifyHeader,
		broadcastBlock: broadcastBlock,
		chainHeight:    chainHeight,
		insertChain:    insertChain,
		dropPeer:       dropPeer,
	}
}

//启动fetcher， 直接启动了一个goroutine来处理。 这个函数有点长。 后续再分析。
// Start boots up the announcement based synchroniser, accepting and processing
// hash notifications and block fetches until termination requested.
func (f *Fetcher) Start() {
	go f.loop()
}

//停止终止基于通知的同步器，取消所有挂起的操作。
// Stop terminates the announcement based synchroniser, canceling all pending
// operations.
func (f *Fetcher) Stop() {
	close(f.quit)
}

//在接收到NewBlockHashesMsg的时候，对于本地区块链还没有的区块的hash值会调用fetcher的Notify方法发送到notify通道
// Notify announces the fetcher of the potential availability of a new block in
// the network.
func (f *Fetcher) Notify(peer string, hash common.Hash, number uint64, time time.Time,
	headerFetcher headerRequesterFn, bodyFetcher bodyRequesterFn) error {
	block := &announce{
		hash:        hash,
		number:      number,
		time:        time,
		origin:      peer,
		fetchHeader: headerFetcher,
		fetchBodies: bodyFetcher,
	}
	select {
	case f.notify <- block:
		return nil
	case <-f.quit:
		return errTerminated
	}
}

//在接收到NewBlockMsg的时候会调用fetcher的Enqueue方法，这个方法会把当前接收到的区块发送到inject通道。
// 可以看到这个方法生成了一个inject对象然后发送到inject通道
// Enqueue tries to fill gaps the the fetcher's future import queue.
func (f *Fetcher) Enqueue(peer string, block *types.Block) error {
	op := &inject{
		origin: peer,
		block:  block,
	}
	select {
	case f.inject <- op:
		return nil
	case <-f.quit:
		return errTerminated
	}
}

//FilterHeaders方法在接收到BlockHeadersMsg的时候被调用。这个方法首先投递了一个channel filter到headerFilter。
// 然后往filter投递了一个headerFilterTask的任务。然后阻塞等待filter队列返回消息。
// FilterHeaders extracts all the headers that were explicitly requested by the fetcher,
// returning those that should be handled differently.
func (f *Fetcher) FilterHeaders(peer string, headers []*types.Header, time time.Time) []*types.Header {
	log.Trace("Filtering headers", "peer", peer, "headers", len(headers))

	// Send the filter channel to the fetcher
	filter := make(chan *headerFilterTask)

	select {
	case f.headerFilter <- filter:
	case <-f.quit:
		return nil
	}
	// Request the filtering of the header list
	select {
	case filter <- &headerFilterTask{peer: peer, headers: headers, time: time}:
	case <-f.quit:
		return nil
	}
	// Retrieve the headers remaining after filtering
	select {
	case task := <-filter:
		return task.headers
	case <-f.quit:
		return nil
	}
}

// FilterBody提取fetcher显式请求的所有块体，并返回应以不同方式处理的块体。
// FilterBodies extracts all the block bodies that were explicitly requested by
// the fetcher, returning those that should be handled differently.
func (f *Fetcher) FilterBodies(peer string, transactions [][]*types.Transaction, uncles [][]*types.Header, time time.Time) ([][]*types.Transaction, [][]*types.Header) {
	log.Trace("Filtering bodies", "peer", peer, "txs", len(transactions), "uncles", len(uncles))
	//将过滤器通道发送到提取器
	// Send the filter channel to the fetcher
	filter := make(chan *bodyFilterTask)

	select {
	case f.bodyFilter <- filter:
	case <-f.quit:
		return nil, nil
	} //请求过滤正文列表
	// Request the filtering of the body list
	select {
	case filter <- &bodyFilterTask{peer: peer, transactions: transactions, uncles: uncles, time: time}:
	case <-f.quit:
		return nil, nil
	} //检索过滤后剩余的物体
	// Retrieve the bodies remaining after filtering
	select {
	case task := <-filter:
		return task.transactions, task.uncles
	case <-f.quit:
		return nil, nil
	}
}

//loop函数函数太长。 我先帖一个省略版本的出来。
// fetcher通过四个map(announced,fetching,fetched,completing )记录了announce的状态(等待fetch,正在fetch,fetch完头等待fetch body, fetch完成)。
// loop其实通过定时器和各种消息来对各种map里面的announce进行状态转换。
// Loop is the main fetcher loop, checking and processing various notification
// events.
func (f *Fetcher) loop() {
	// Iterate the block fetching until a quit is requested
	fetchTimer := time.NewTimer(0)    //fetch的定时器。
	completeTimer := time.NewTimer(0) // compelte的定时器。

	for {
		// 如果fetching的时间超过5秒，那么放弃掉这个fetching
		// Clean up any expired block fetches
		for hash, announce := range f.fetching {
			if time.Since(announce.time) > fetchTimeout {
				f.forgetHash(hash)
			}
		}
		// Import any queued blocks that could potentially fit
		// 这个fetcher.queue里面缓存了已经完成fetch的block等待按照顺序插入到本地的区块链中
		//fetcher.queue是一个优先级队列。 优先级别就是他们的区块号的负数，这样区块数小的排在最前面。
		height := f.chainHeight()
		for !f.queue.Empty() {
			op := f.queue.PopItem().(*inject)
			if f.queueChangeHook != nil {
				f.queueChangeHook(op.block.Hash(), false)
			} //如果链条或阶段太高，请稍后再继续
			// If too high up the chain or phase, continue later
			number := op.block.NumberU64()
			if number > height+1 { //当前的区块的高度太高，还不能import
				f.queue.Push(op, -float32(op.block.NumberU64()))
				if f.queueChangeHook != nil {
					f.queueChangeHook(op.block.Hash(), true)
				}
				break
			} //否则如果新鲜且仍然未知，请尝试导入
			// Otherwise if fresh and still unknown, try and import
			hash := op.block.Hash()
			if number+maxUncleDist < height || f.getBlock(hash) != nil {
				// 区块的高度太低 低于当前的height-maxUncleDist
				// 或者区块已经被import了
				f.forgetBlock(hash)
				continue
			}
			// 插入区块
			f.insert(op.origin, op.block)
		} //等待外部事件发生
		// Wait for an outside event to occur
		select {
		case <-f.quit:
			// Fetcher terminating, abort all operations
			return

		case notification := <-f.notify: //在接收到NewBlockHashesMsg的时候，对于本地区块链还没有的区块的hash值会调用fetcher的Notify方法发送到notify通道。
			// A block was announced, make sure the peer isn't DOSing us
			propAnnounceInMeter.Mark(1)

			count := f.announces[notification.origin] + 1
			if count > hashLimit { //hashLimit 256 一个远端最多只存在256个announces
				log.Debug("Peer exceeded outstanding announces", "peer", notification.origin, "limit", hashLimit)
				propAnnounceDOSMeter.Mark(1)
				break
			}
			// 查看是潜在是否有用。 根据这个区块号和本地区块链的距离， 太大和太小对于我们都没有意义。
			// If we have a valid block number, check that it's potentially useful
			if notification.number > 0 {
				if dist := int64(notification.number) - int64(f.chainHeight()); dist < -maxUncleDist || dist > maxQueueDist {
					log.Debug("Peer discarded announcement", "peer", notification.origin, "number", notification.number, "hash", notification.hash, "distance", dist)
					propAnnounceDropMeter.Mark(1)
					break
				}
			}
			// 检查我们是否已经存在了。
			// All is well, schedule the announce if block's not yet downloading
			if _, ok := f.fetching[notification.hash]; ok {
				break
			}
			if _, ok := f.completing[notification.hash]; ok {
				break
			}
			f.announces[notification.origin] = count
			f.announced[notification.hash] = append(f.announced[notification.hash], notification)
			if f.announceChangeHook != nil && len(f.announced[notification.hash]) == 1 {
				f.announceChangeHook(notification.hash, true)
			}
			if len(f.announced) == 1 {
				f.rescheduleFetch(fetchTimer)
			}
			// 在接收到NewBlockMsg的时候会调用fetcher的Enqueue方法，这个方法会把当前接收到的区块发送到inject通道。
		case op := <-f.inject:
			// A direct block insertion was requested, try and fill any pending gaps
			propBroadcastInMeter.Mark(1)
			f.enqueue(op.origin, op.block)
			//当完成一个区块的import的时候会发送该区块的hash值到done通道。
		case hash := <-f.done:
			// A pending import finished, remove all traces of the notification
			f.forgetHash(hash)
			f.forgetBlock(hash)
			// fetchTimer定时器，定期对需要fetch的区块头进行fetch
			//一共存在两个定时器。fetchTimer和completeTimer，分别负责获取区块头和获取区块body。
		case <-fetchTimer.C:
			// At least one block's timer ran out, check for needing retrieval
			request := make(map[string][]common.Hash)

			for hash, announces := range f.announced {
				// TODO 这里的时间限制是什么意思
				// 最早收到的announce，并经过arriveTimeout-gatherSlack这么长的时间
				if time.Since(announces[0].time) > arriveTimeout-gatherSlack {
					// Pick a random peer to retrieve from, reset all others
					// announces代表了同一个区块的来自多个peer的多个announce
					announce := announces[rand.Intn(len(announces))]
					f.forgetHash(hash)
					//如果块仍然没有到达，则排队进行抓取
					// If the block still didn't arrive, queue for fetching
					if f.getBlock(hash) == nil {
						request[announce.origin] = append(request[announce.origin], hash)
						f.fetching[hash] = announce
					}
				}
			}
			// 发送所有的请求。
			// Send out all block header requests
			for peer, hashes := range request {
				log.Trace("Fetching scheduled headers", "peer", peer, "list", hashes)

				// Create a closure of the fetch and schedule in on a new thread
				fetchHeader, hashes := f.fetching[hashes[0]].fetchHeader, hashes
				go func() {
					if f.fetchingHook != nil {
						f.fetchingHook(hashes)
					}
					for _, hash := range hashes {
						headerFetchMeter.Mark(1)
						fetchHeader(hash) // Suboptimal, but protocol doesn't allow batch header retrievals//不理想，但协议不允许批量标题检索
					}
				}()
			} //如果块仍处于挂起状态，则安排下一次抓取
			// Schedule the next fetch if blocks are still pending
			f.rescheduleFetch(fetchTimer)
			// completeTimer定时器定期对需要fetch的区块体进行fetch
		case <-completeTimer.C:
			//至少有一个标题的计时器耗尽，请检索所有内容
			// At least one header's timer ran out, retrieve everything
			request := make(map[string][]common.Hash)

			for hash, announces := range f.fetched {
				//选择一个随机点来检索，重置所有其他点
				// Pick a random peer to retrieve from, reset all others
				announce := announces[rand.Intn(len(announces))]
				f.forgetHash(hash)
				//如果该块仍未到达，请排队等待完成
				// If the block still didn't arrive, queue for completion
				if f.getBlock(hash) == nil {
					request[announce.origin] = append(request[announce.origin], hash)
					f.completing[hash] = announce
				}
			} //发出所有块体请求
			// Send out all block body requests
			for peer, hashes := range request {
				log.Trace("Fetching scheduled bodies", "peer", peer, "list", hashes)
				//在新线程中创建提取和计划的关闭
				// Create a closure of the fetch and schedule in on a new thread
				if f.completingHook != nil {
					f.completingHook(hashes)
				}
				bodyFetchMeter.Mark(int64(len(hashes)))
				go f.completing[hashes[0]].fetchBodies(hashes)
			} //如果块仍处于挂起状态，则安排下一次抓取
			// Schedule the next fetch if blocks are still pending
			f.rescheduleComplete(completeTimer)
			//当接收到BlockHeadersMsg的消息的时候(接收到一些区块头),会把这些消息投递到headerFilter队列。 这边会把属于fetcher请求的数据留下，其他的会返回出来，给其他系统使用。
		case filter := <-f.headerFilter:
			//标题从远程对等点到达。 提取fetcher显式请求的那些，并返回其他所有内容，以便将其交付给系统的其他部分。
			// Headers arrived from a remote peer. Extract those that were explicitly
			// requested by the fetcher, and return everything else so it's delivered
			// to other parts of the system.
			var task *headerFilterTask
			select {
			case task = <-filter:
			case <-f.quit:
				return
			}
			headerFilterInMeter.Mark(int64(len(task.headers)))
			//将一批标题拆分为未知的（返回给调用者），
			//已知不完整的（需要正文检索）和已完成的块。
			// Split the batch of headers into unknown ones (to return to the caller),
			// known incomplete ones (requiring body retrievals) and completed blocks.
			unknown, incomplete, complete := []*types.Header{}, []*announce{}, []*types.Block{}
			for _, header := range task.headers {
				hash := header.Hash()
				// 根据情况看这个是否是我们的请求返回的信息。
				// Filter fetcher-requested headers from other synchronisation algorithms
				if announce := f.fetching[hash]; announce != nil && announce.origin == task.peer && f.fetched[hash] == nil && f.completing[hash] == nil && f.queued[hash] == nil {
					// 如果返回的header的区块高度和我们请求的不同，那么删除掉返回这个header的peer。 并且忘记掉这个hash(以便于重新获取区块信息)
					// If the delivered header does not match the promised number, drop the announcer
					if header.Number.Uint64() != announce.number {
						log.Trace("Invalid block number fetched", "peer", announce.origin, "hash", header.Hash(), "announced", announce.number, "provided", header.Number)
						f.dropPeer(announce.origin)
						f.forgetHash(hash)
						continue
					} //只有保留，如果不是通过其他方式导入
					// Only keep if not imported by other means
					if f.getBlock(hash) == nil {
						announce.header = header
						announce.time = task.time
						// 根据区块头查看，如果这个区块不包含任何交易或者是Uncle区块。那么我们就不用获取区块的body了。 那么直接插入完成列表。
						// If the block is empty (header only), short circuit into the final import queue
						if header.TxHash == types.DeriveSha(types.Transactions{}) && header.UncleHash == types.CalcUncleHash([]*types.Header{}) {
							log.Trace("Block empty, skipping body retrieval", "peer", announce.origin, "number", header.Number, "hash", header.Hash())

							block := types.NewBlockWithHeader(header)
							block.ReceivedAt = task.time

							complete = append(complete, block)
							f.completing[hash] = announce
							continue
						}
						// 否则，插入到未完成列表等待fetch blockbody
						// Otherwise add to the list of blocks needing completion
						incomplete = append(incomplete, announce)
					} else {
						log.Trace("Block already imported, discarding header", "peer", announce.origin, "number", header.Number, "hash", header.Hash())
						f.forgetHash(hash)
					}
				} else {
					// Fetcher doesn't know about it, add to the return list
					// Fetcher并不知道这个header。 增加到返回列表等待返回。
					unknown = append(unknown, header)
				}
			}
			headerFilterOutMeter.Mark(int64(len(unknown)))
			select {
			//// 把返回结果返回。
			case filter <- &headerFilterTask{headers: unknown, time: task.time}:
			case <-f.quit:
				return
			}
			// Schedule the retrieved headers for body completion
			for _, announce := range incomplete {
				hash := announce.header.Hash()
				if _, ok := f.completing[hash]; ok { //如果已经在其他的地方完成
					continue
				}
				// 放到等待获取body的map等待处理。
				f.fetched[hash] = append(f.fetched[hash], announce)
				if len(f.fetched) == 1 { //如果fetched map只有刚刚加入的一个元素。 那么重置计时器。
					f.rescheduleComplete(completeTimer)
				}
			}
			// 这些只有header的区块放入queue等待import
			// Schedule the header-only blocks for import
			for _, block := range complete {
				if announce := f.completing[block.Hash()]; announce != nil {
					f.enqueue(announce.origin, block)
				}
			}
			//当接收到BlockBodiesMsg消息的时候，会把这些消息投递给bodyFilter队列。这边会把属于fetcher请求的数据留下，其他的会返回出来，给其他系统使用。
		case filter := <-f.bodyFilter:
			//块体到达，提取任何明确请求的块，然后返回其余块
			// Block bodies arrived, extract any explicitly requested blocks, return the rest
			var task *bodyFilterTask
			select {
			case task = <-filter:
			case <-f.quit:
				return
			}
			bodyFilterInMeter.Mark(int64(len(task.transactions)))

			blocks := []*types.Block{}
			for i := 0; i < len(task.transactions) && i < len(task.uncles); i++ {
				//匹配任何可能的完成请求的主体
				// Match up a body to any possible completion request
				matched := false

				for hash, announce := range f.completing {
					if f.queued[hash] == nil {
						txnHash := types.DeriveSha(types.Transactions(task.transactions[i]))
						uncleHash := types.CalcUncleHash(task.uncles[i])

						if txnHash == announce.header.TxHash && uncleHash == announce.header.UncleHash && announce.origin == task.peer {
							// Mark the body matched, reassemble if still unknown
							//标记匹配的身体，如果仍然未知，则重新组装
							matched = true

							if f.getBlock(hash) == nil {
								block := types.NewBlockWithHeader(announce.header).WithBody(task.transactions[i], task.uncles[i])
								block.ReceivedAt = task.time

								blocks = append(blocks, block)
							} else {
								f.forgetHash(hash)
							}
						}
					}
				}
				if matched {
					task.transactions = append(task.transactions[:i], task.transactions[i+1:]...)
					task.uncles = append(task.uncles[:i], task.uncles[i+1:]...)
					i--
					continue
				}
			}

			bodyFilterOutMeter.Mark(int64(len(task.transactions)))
			select {
			case filter <- task:
			case <-f.quit:
				return
			} //为排序导入调度检索到的块
			// Schedule the retrieved blocks for ordered import
			for _, block := range blocks {
				if announce := f.completing[block.Hash()]; announce != nil {
					f.enqueue(announce.origin, block)
				}
			}
		}
	}
}

// rescheduleFetch将指定的提取计时器重置为下一个通知超时。
// rescheduleFetch resets the specified fetch timer to the next announce timeout.
func (f *Fetcher) rescheduleFetch(fetch *time.Timer) {
	// Short circuit if no blocks are announced//如果没有通告块，则短路
	if len(f.announced) == 0 {
		return
	} //否则找到最早到期的公告
	// Otherwise find the earliest expiring announcement
	earliest := time.Now()
	for _, announces := range f.announced {
		if earliest.After(announces[0].time) {
			earliest = announces[0].time
		}
	}
	fetch.Reset(arriveTimeout - time.Since(earliest))
}

// rescheduleComplete将指定的完成计时器重置为下一个提取超时。
// rescheduleComplete resets the specified completion timer to the next fetch timeout.
func (f *Fetcher) rescheduleComplete(complete *time.Timer) {
	// Short circuit if no headers are fetched
	if len(f.fetched) == 0 {
		return
	}
	// Otherwise find the earliest expiring announcement
	earliest := time.Now()
	for _, announces := range f.fetched {
		if earliest.After(announces[0].time) {
			earliest = announces[0].time
		}
	}
	complete.Reset(gatherSlack - time.Since(earliest))
} //如果要导入的块尚未被看到，则排队执行新的未来导入操作。
// enqueue schedules a new future import operation, if the block to be imported
// has not yet been seen.
func (f *Fetcher) enqueue(peer string, block *types.Block) {
	hash := block.Hash()
	//确保对方没有给我们提供指令
	// Ensure the peer isn't DOSing us
	count := f.queues[peer] + 1
	if count > blockLimit { // blockLimit 64 如果缓存的对方的block太多。
		log.Debug("Discarded propagated block, exceeded allowance", "peer", peer, "number", block.Number(), "hash", hash, "limit", blockLimit)
		propBroadcastDOSMeter.Mark(1)
		f.forgetHash(hash)
		return
	}
	// 距离我们的区块链太远。
	// Discard any past or too distant blocks
	if dist := int64(block.NumberU64()) - int64(f.chainHeight()); dist < -maxUncleDist || dist > maxQueueDist {
		log.Debug("Discarded propagated block, too far away", "peer", peer, "number", block.Number(), "hash", hash, "distance", dist)
		propBroadcastDropMeter.Mark(1)
		f.forgetHash(hash)
		return
	}
	// 插入到队列。
	// Schedule the block for future importing
	if _, ok := f.queued[hash]; !ok {
		op := &inject{
			origin: peer,
			block:  block,
		}
		f.queues[peer] = count
		f.queued[hash] = op
		f.queue.Push(op, -float32(block.NumberU64()))
		if f.queueChangeHook != nil {
			f.queueChangeHook(op.block.Hash(), true)
		}
		log.Debug("Queued propagated block", "peer", peer, "number", block.Number(), "hash", hash, "queued", f.queue.Size())
	}
}

// 这个方法把给定的区块插入本地的区块链。
// insert spawns a new goroutine to run a block insertion into the chain. If the
// block's number is at the same height as the current import phase, it updates
// the phase states accordingly.
func (f *Fetcher) insert(peer string, block *types.Block) {
	hash := block.Hash()
	//在新线程上运行导入
	// Run the import on a new thread
	log.Debug("Importing propagated block", "peer", peer, "number", block.Number(), "hash", hash)
	go func() {
		defer func() { f.done <- hash }()
		//如果父级的未知，则中止插入
		// If the parent's unknown, abort insertion
		parent := f.getBlock(block.ParentHash())
		if parent == nil {
			log.Debug("Unknown parent of propagated block", "peer", peer, "number", block.Number(), "hash", hash, "parent", block.ParentHash())
			return
		}
		// 如果区块头通过验证，那么马上对区块进行广播。 NewBlockMsg
		// Quickly validate the header and propagate the block if it passes
		switch err := f.verifyHeader(block.Header()); err {
		case nil:
			// All ok, quickly propagate to our peers
			//一切都好，很快就会传播给我们的同行
			propBroadcastOutTimer.UpdateSince(block.ReceivedAt)
			go f.broadcastBlock(block, true)

		case consensus.ErrFutureBlock:
			// Weird future block, don't fail, but neither propagate
			//奇怪的未来模块，不会失败，但不会传播
		default:
			//发生了一些错误，请放弃同行
			// Something went very wrong, drop the peer
			log.Debug("Propagated block verification failed", "peer", peer, "number", block.Number(), "hash", hash, "err", err)
			f.dropPeer(peer)
			return
		} //运行实际导入并记录任何问题
		// Run the actual import and log any issues
		if _, err := f.insertChain(types.Blocks{block}); err != nil {
			log.Debug("Propagated block import failed", "peer", peer, "number", block.Number(), "hash", hash, "err", err)
			return
		}
		// 如果插入成功， 那么广播区块， 第二个参数为false。那么只会对区块的hash进行广播。NewBlockHashesMsg
		// If import succeeded, broadcast the block
		propAnnounceOutTimer.UpdateSince(block.ReceivedAt)
		go f.broadcastBlock(block, false)
		//根据需要调用测试钩子
		// Invoke the testing hook if needed
		if f.importedHook != nil {
			f.importedHook(block)
		}
	}()
}

// forgetHash从收件人的内部状态中删除块公告的所有痕迹。
// forgetHash removes all traces of a block announcement from the fetcher's
// internal state.
func (f *Fetcher) forgetHash(hash common.Hash) {
	// Remove all pending announces and decrement DOS counters//删除所有未决的通告并减少DOS计数器
	for _, announce := range f.announced[hash] {
		f.announces[announce.origin]--
		if f.announces[announce.origin] == 0 {
			delete(f.announces, announce.origin)
		}
	}
	delete(f.announced, hash)
	if f.announceChangeHook != nil {
		f.announceChangeHook(hash, false)
	} //删除任何挂起的提取并减少DOS计数器
	// Remove any pending fetches and decrement the DOS counters
	if announce := f.fetching[hash]; announce != nil {
		f.announces[announce.origin]--
		if f.announces[announce.origin] == 0 {
			delete(f.announces, announce.origin)
		}
		delete(f.fetching, hash)
	}
	//删除任何挂起的完成请求并减少DOS计数器
	// Remove any pending completion requests and decrement the DOS counters
	for _, announce := range f.fetched[hash] {
		f.announces[announce.origin]--
		if f.announces[announce.origin] == 0 {
			delete(f.announces, announce.origin)
		}
	}
	delete(f.fetched, hash)
	//删除所有挂起的完成并减少DOS计数器
	// Remove any pending completions and decrement the DOS counters
	if announce := f.completing[hash]; announce != nil {
		f.announces[announce.origin]--
		if f.announces[announce.origin] == 0 {
			delete(f.announces, announce.origin)
		}
		delete(f.completing, hash)
	}
}

// forgetBlock从收件人的内部状态中删除排队块的所有痕迹。
// forgetBlock removes all traces of a queued block from the fetcher's internal
// state.
func (f *Fetcher) forgetBlock(hash common.Hash) {
	if insert := f.queued[hash]; insert != nil {
		f.queues[insert.origin]--
		if f.queues[insert.origin] == 0 {
			delete(f.queues, insert.origin)
		}
		delete(f.queued, hash)
	}
}
