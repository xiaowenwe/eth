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

// Contains the active peer-set of the downloader, maintaining both failures
// as well as reputation metrics to prioritize the block retrievals.

package downloader

import (
	"errors"
	"fmt"
	"math"
	"math/big"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/event"
	"github.com/ethereum/go-ethereum/log"
)

//peer模块包含了downloader使用的peer节点，封装了吞吐量，是否空闲，并记录了之前失败的信息。
const (
	maxLackingHashes  = 4096 // Maximum number of entries allowed on the list or lacking items列表中允许的最大条目数或缺少项目
	measurementImpact = 0.1  // The impact a single measurement has on a peer's final throughput value.单一测量对同级最终吞吐量值的影响。
)

var (
	errAlreadyFetching   = errors.New("already fetching blocks from peer")
	errAlreadyRegistered = errors.New("peer is already registered")
	errNotRegistered     = errors.New("peer is not registered")
)

// peerConnection表示从中检索散列和块的主动对等体。
// peerConnection represents an active peer from which hashes and blocks are retrieved.
type peerConnection struct {
	id string // Unique identifier of the peer对等体的唯一标识符

	headerIdle  int32 // Current header activity state of the peer (idle = 0, active = 1) 当前的header获取的工作状态。
	blockIdle   int32 // Current block activity state of the peer (idle = 0, active = 1)	当前的区块获取的工作状态
	receiptIdle int32 // Current receipt activity state of the peer (idle = 0, active = 1) 当前的收据获取的工作状态
	stateIdle   int32 // Current node data activity state of the peer (idle = 0, active = 1) 当前节点状态的工作状态

	headerThroughput  float64 // Number of headers measured to be retrievable per second	//记录每秒能够接收多少个区块头的度量值
	blockThroughput   float64 // Number of blocks (bodies) measured to be retrievable per second  //记录每秒能够接收多少个区块的度量值
	receiptThroughput float64 // Number of receipts measured to be retrievable per second 记录每秒能够接收多少个收据的度量值
	stateThroughput   float64 // Number of node data pieces measured to be retrievable per second  记录每秒能够接收多少个账户状态的度量值

	rtt time.Duration // Request round trip time to track responsiveness (QoS)  请求回应时间

	headerStarted  time.Time // Time instance when the last header fetch was started	记录最后一个header fetch的请求时间
	blockStarted   time.Time // Time instance when the last block (body) fetch was started最后一个块（主体）获取开始时的时间实例
	receiptStarted time.Time // Time instance when the last receipt fetch was started最后一次收据提取开始时的时间实例
	stateStarted   time.Time // Time instance when the last node data fetch was started最后一个节点数据获取开始的时间实例

	lacking map[common.Hash]struct{} // Set of hashes not to request (didn't have previously)  记录的Hash值不会去请求，一般是因为之前的请求失败

	peer Peer //eth的peer

	version int        // Eth protocol version number to switch strategies// Eth协议版本号切换策略
	log     log.Logger // Contextual logger to add extra infos to peer logs上下文记录器为对等记录添加额外的信息
	lock    sync.RWMutex
}

// LightPeer封装了与远程轻点同步所需的方法
// LightPeer encapsulates the methods required to synchronise with a remote light peer.
type LightPeer interface {
	Head() (common.Hash, *big.Int)
	RequestHeadersByHash(common.Hash, int, int, bool) error
	RequestHeadersByNumber(uint64, int, int, bool) error
}

// Peer封装了与远程完整对等进行同步所需的方法。
// Peer encapsulates the methods required to synchronise with a remote full peer.
type Peer interface {
	LightPeer
	RequestBodies([]common.Hash) error
	RequestReceipts([]common.Hash) error //RequestReceipts从远程节点获取一批交易收据
	RequestNodeData([]common.Hash) error
}

// lightPeerWrapper封装了一个LightPeer结构，用于存储Peer-only方法。
// lightPeerWrapper wraps a LightPeer struct, stubbing out the Peer-only methods.
type lightPeerWrapper struct {
	peer LightPeer
}

func (w *lightPeerWrapper) Head() (common.Hash, *big.Int) { return w.peer.Head() }
func (w *lightPeerWrapper) RequestHeadersByHash(h common.Hash, amount int, skip int, reverse bool) error {
	return w.peer.RequestHeadersByHash(h, amount, skip, reverse)
}
func (w *lightPeerWrapper) RequestHeadersByNumber(i uint64, amount int, skip int, reverse bool) error {
	return w.peer.RequestHeadersByNumber(i, amount, skip, reverse)
}
func (w *lightPeerWrapper) RequestBodies([]common.Hash) error {
	panic("RequestBodies not supported in light client mode sync")
}
func (w *lightPeerWrapper) RequestReceipts([]common.Hash) error {
	panic("RequestReceipts not supported in light client mode sync")
}
func (w *lightPeerWrapper) RequestNodeData([]common.Hash) error {
	panic("RequestNodeData not supported in light client mode sync")
}

// newPeerConnection创建一个新的下载器对等体。
// newPeerConnection creates a new downloader peer.
func newPeerConnection(id string, version int, peer Peer, logger log.Logger) *peerConnection {
	return &peerConnection{
		id:      id,
		lacking: make(map[common.Hash]struct{}),

		peer: peer,

		version: version,
		log:     logger,
	}
}

//重置清除对等实体的内部状态。
// Reset clears the internal state of a peer entity.
func (p *peerConnection) Reset() {
	p.lock.Lock()
	defer p.lock.Unlock()

	atomic.StoreInt32(&p.headerIdle, 0)
	atomic.StoreInt32(&p.blockIdle, 0)
	atomic.StoreInt32(&p.receiptIdle, 0)
	atomic.StoreInt32(&p.stateIdle, 0)

	p.headerThroughput = 0
	p.blockThroughput = 0
	p.receiptThroughput = 0
	p.stateThroughput = 0

	p.lacking = make(map[common.Hash]struct{})
}

//FetchXXX FetchHeaders FetchBodies等函数 主要调用了eth.peer的功能来进行发送数据请求。
// FetchHeaders sends a header retrieval request to the remote peer.
func (p *peerConnection) FetchHeaders(from uint64, count int) error {
	// Sanity check the protocol version
	if p.version < 62 {
		panic(fmt.Sprintf("header fetch [eth/62+] requested on eth/%d", p.version))
	} //如果对等体已经获取，则短路
	// Short circuit if the peer is already fetching
	if !atomic.CompareAndSwapInt32(&p.headerIdle, 0, 1) {
		return errAlreadyFetching
	}
	p.headerStarted = time.Now()
	//发出头部检索请求（absolut向上无间隙
	// Issue the header retrieval request (absolut upwards without gaps)
	go p.peer.RequestHeadersByNumber(from, count, 0, false)

	return nil
}

// FetchBody将块体检索请求发送给远程对等体
// FetchBodies sends a block body retrieval request to the remote peer.
func (p *peerConnection) FetchBodies(request *fetchRequest) error {
	// Sanity check the protocol version
	if p.version < 62 {
		panic(fmt.Sprintf("body fetch [eth/62+] requested on eth/%d", p.version))
	} //如果对等体已经获取，则短路
	// Short circuit if the peer is already fetching
	if !atomic.CompareAndSwapInt32(&p.blockIdle, 0, 1) {
		return errAlreadyFetching
	}
	p.blockStarted = time.Now()
	//将头部集合转换为可检索的切片
	// Convert the header set to a retrievable slice
	hashes := make([]common.Hash, 0, len(request.Headers))
	for _, header := range request.Headers {
		hashes = append(hashes, header.Hash())
	}
	go p.peer.RequestBodies(hashes)

	return nil
}

// FetchReceipts向远程节点发送收据检索请求。
// FetchReceipts sends a receipt retrieval request to the remote peer.
func (p *peerConnection) FetchReceipts(request *fetchRequest) error {
	// Sanity check the protocol version
	if p.version < 63 {
		panic(fmt.Sprintf("body fetch [eth/63+] requested on eth/%d", p.version))
	} //如果对等体已经获取，则短路
	// Short circuit if the peer is already fetching
	if !atomic.CompareAndSwapInt32(&p.receiptIdle, 0, 1) {
		return errAlreadyFetching
	}
	p.receiptStarted = time.Now()

	// Convert the header set to a retrievable slice/将标题集转换为可检索的片
	hashes := make([]common.Hash, 0, len(request.Headers))
	for _, header := range request.Headers {
		hashes = append(hashes, header.Hash())
	}
	go p.peer.RequestReceipts(hashes)

	return nil
}

// FetchNodeData向远程节点发送节点状态数据检索请求。
// FetchNodeData sends a node state data retrieval request to the remote peer.
func (p *peerConnection) FetchNodeData(hashes []common.Hash) error {
	// Sanity check the protocol version// Sanity检查协议版本
	if p.version < 63 {
		panic(fmt.Sprintf("node data fetch [eth/63+] requested on eth/%d", p.version))
	} //如果对等体已经获取，则短路
	// Short circuit if the peer is already fetching
	if !atomic.CompareAndSwapInt32(&p.stateIdle, 0, 1) {
		return errAlreadyFetching
	}
	p.stateStarted = time.Now()

	go p.peer.RequestNodeData(hashes)

	return nil
}

//SetXXXIdle函数 SetHeadersIdle, SetBlocksIdle 等函数 设置peer的状态为空闲状态，允许它执行新的请求。 同时还会通过本次传输的数据的多少来重新评估链路的吞吐量。
// SetHeadersIdle sets the peer to idle, allowing it to execute new header retrieval
// requests. Its estimated header retrieval throughput is updated with that measured
// just now.
// SetHeadersIdle将对等设置为空闲，允许它执行新的头部检索请求。 其估计的头部检索吞吐量用刚刚测量的值进行更新。
func (p *peerConnection) SetHeadersIdle(delivered int) {
	p.setIdle(p.headerStarted, delivered, &p.headerThroughput, &p.headerIdle)
}

// SetBlocksIdle将对等设置为空闲，允许它执行新的块检索请求。 其估计的块检索吞吐量用刚刚测量的那个更新。
// SetBlocksIdle sets the peer to idle, allowing it to execute new block retrieval
// requests. Its estimated block retrieval throughput is updated with that measured
// just now.
func (p *peerConnection) SetBlocksIdle(delivered int) {
	p.setIdle(p.blockStarted, delivered, &p.blockThroughput, &p.blockIdle)
}

// SetBodiesIdle将对等体设置为空闲，允许它执行块体检索请求。 它的估计身体检索吞吐量用刚刚测量的更新。
// SetBodiesIdle sets the peer to idle, allowing it to execute block body retrieval
// requests. Its estimated body retrieval throughput is updated with that measured
// just now.
func (p *peerConnection) SetBodiesIdle(delivered int) {
	p.setIdle(p.blockStarted, delivered, &p.blockThroughput, &p.blockIdle)
}

// SetReceiptsIdle将对等设置为空闲状态，允许它执行新的收据检索请求。 其预计的收据检索吞吐量将随着刚刚测量的数据更新。
// SetReceiptsIdle sets the peer to idle, allowing it to execute new receipt
// retrieval requests. Its estimated receipt retrieval throughput is updated
// with that measured just now.
func (p *peerConnection) SetReceiptsIdle(delivered int) {
	p.setIdle(p.receiptStarted, delivered, &p.receiptThroughput, &p.receiptIdle)
}

// SetNodeDataIdle将对等体设置为空闲，允许它执行新的状态trie数据检索请求。 其估计的状态检索吞吐量使用刚才测量的更新。
// SetNodeDataIdle sets the peer to idle, allowing it to execute new state trie
// data retrieval requests. Its estimated state retrieval throughput is updated
// with that measured just now.
func (p *peerConnection) SetNodeDataIdle(delivered int) {
	p.setIdle(p.stateStarted, delivered, &p.stateThroughput, &p.stateIdle)
}

// setIdle将对等体设置为空闲，允许它执行新的检索请求。
//它的估计检索吞吐量用刚刚测量的更新。
// setIdle sets the peer to idle, allowing it to execute new retrieval requests.
// Its estimated retrieval throughput is updated with that measured just now.
func (p *peerConnection) setIdle(started time.Time, delivered int, throughput *float64, idle *int32) {
	// Irrelevant of the scaling, make sure the peer ends up idle
	//无关缩放，请确保对等结束闲置
	defer atomic.StoreInt32(idle, 0)

	p.lock.Lock()
	defer p.lock.Unlock()
	//如果没有提供任何内容（硬超时/不可用数据），请将吞吐量降至最低
	// If nothing was delivered (hard timeout / unavailable data), reduce throughput to minimum
	if delivered == 0 {
		*throughput = 0
		return
	} //否则用新的度量更新吞吐量
	// Otherwise update the throughput with a new measurement
	elapsed := time.Since(started) + 1 // +1 (ns) to ensure non-zero divisor/ +1（ns）以确保非零除数
	measured := float64(delivered) / (float64(elapsed) / float64(time.Second))
	// measurementImpact = 0.1 , 新的吞吐量=老的吞吐量*0.9 + 这次的吞吐量*0.1
	*throughput = (1-measurementImpact)*(*throughput) + measurementImpact*measured
	// 更新RTT
	p.rtt = time.Duration((1-measurementImpact)*float64(p.rtt) + measurementImpact*float64(elapsed))

	p.log.Trace("Peer throughput measurements updated",
		"hps", p.headerThroughput, "bps", p.blockThroughput,
		"rps", p.receiptThroughput, "sps", p.stateThroughput,
		"miss", len(p.lacking), "rtt", p.rtt)
}

//XXXCapacity函数，用来返回当前的链接允许的吞吐量。
// HeaderCapacity retrieves the peers header download allowance based on its
// previously discovered throughput.
func (p *peerConnection) HeaderCapacity(targetRTT time.Duration) int {
	p.lock.RLock()
	defer p.lock.RUnlock()
	// 这里有点奇怪，targetRTT越大，请求的数量就越多。
	return int(math.Min(1+math.Max(1, p.headerThroughput*float64(targetRTT)/float64(time.Second)), float64(MaxHeaderFetch)))
}

// BlockCapacity根据它检索对等块的下载限额先前发现吞吐量。
// BlockCapacity retrieves the peers block download allowance based on its
// previously discovered throughput.
func (p *peerConnection) BlockCapacity(targetRTT time.Duration) int {
	p.lock.RLock()
	defer p.lock.RUnlock()

	return int(math.Min(1+math.Max(1, p.blockThroughput*float64(targetRTT)/float64(time.Second)), float64(MaxBlockFetch)))
}

// ReceiptCapacity根据先前发现的吞吐量检索同等收据下载限额。
// ReceiptCapacity retrieves the peers receipt download allowance based on its
// previously discovered throughput.
func (p *peerConnection) ReceiptCapacity(targetRTT time.Duration) int {
	p.lock.RLock()
	defer p.lock.RUnlock()

	return int(math.Min(1+math.Max(1, p.receiptThroughput*float64(targetRTT)/float64(time.Second)), float64(MaxReceiptFetch)))
}

// NodeDataCapacity根据先前发现的吞吐量检索对等体状态下载容限。
// NodeDataCapacity retrieves the peers state download allowance based on its
// previously discovered throughput.
func (p *peerConnection) NodeDataCapacity(targetRTT time.Duration) int {
	p.lock.RLock()
	defer p.lock.RUnlock()

	return int(math.Min(1+math.Max(1, p.stateThroughput*float64(targetRTT)/float64(time.Second)), float64(MaxStateFetch)))
}

//Lacks 用来标记上次是否失败，以便下次同样的请求不通过这个peer
// MarkLacking appends a new entity to the set of items (blocks, receipts, states)
// that a peer is known not to have (i.e. have been requested before). If the
// set reaches its maximum allowed capacity, items are randomly dropped off.
// MarkLacking将一个新实体附加到一组项目（块，收据，状态）
//已知对等体没有（即之前被请求过）。 如果该组达到其最大允许容量，则物品将被随机丢弃。
func (p *peerConnection) MarkLacking(hash common.Hash) {
	p.lock.Lock()
	defer p.lock.Unlock()

	for len(p.lacking) >= maxLackingHashes {
		for drop := range p.lacking {
			delete(p.lacking, drop)
			break
		}
	}
	p.lacking[hash] = struct{}{}
}

//缺乏检索区块链项目的散列是否在缺少列表的对等体上（即，我们是否知道对等体没有它）。
// Lacks retrieves whether the hash of a blockchain item is on the peers lacking
// list (i.e. whether we know that the peer does not have it).
func (p *peerConnection) Lacks(hash common.Hash) bool {
	p.lock.RLock()
	defer p.lock.RUnlock()

	_, ok := p.lacking[hash]
	return ok
}

// peerSet表示参与链下载过程的活动对等的集合。
// peerSet represents the collection of active peer participating in the chain
// download procedure.
type peerSet struct {
	peers        map[string]*peerConnection
	newPeerFeed  event.Feed
	peerDropFeed event.Feed
	lock         sync.RWMutex
}

// newPeerSet创建一个新的对等机顶盒跟踪活动的下载源。
// newPeerSet creates a new peer set top track the active download sources.
func newPeerSet() *peerSet {
	return &peerSet{
		peers: make(map[string]*peerConnection),
	}
}

// SubscribeNewPeers订阅同行到达事件。
// SubscribeNewPeers subscribes to peer arrival events.
func (ps *peerSet) SubscribeNewPeers(ch chan<- *peerConnection) event.Subscription {
	return ps.newPeerFeed.Subscribe(ch)
}

// SubscribePeerDrops订阅同行离开事件
// SubscribePeerDrops subscribes to peer departure events.
func (ps *peerSet) SubscribePeerDrops(ch chan<- *peerConnection) event.Subscription {
	return ps.peerDropFeed.Subscribe(ch)
}

//重置对当前对等设置的迭代，并重置每个已知对等设备以准备下一批块检索。
// Reset iterates over the current peer set, and resets each of the known peers
// to prepare for a next batch of block retrieval.
func (ps *peerSet) Reset() {
	ps.lock.RLock()
	defer ps.lock.RUnlock()

	for _, peer := range ps.peers {
		peer.Reset()
	}
}

//注册将新对等点注入工作集，或者如果对等点已知，则返回错误。
//该方法还将新对等端的起始吞吐量值设置为所有现有对等端的平均值，以便为其提供用于数据检索的实际机会。
// Register injects a new peer into the working set, or returns an error if the
// peer is already known.
//
// The method also sets the starting throughput values of the new peer to the
// average of all existing peers, to give it a realistic chance of being used
// for data retrievals.
func (ps *peerSet) Register(p *peerConnection) error {
	// Retrieve the current median RTT as a sane default
	//检索当前的RTT中位数作为理智的默认值
	p.rtt = ps.medianRTT()
	//用一些有意义的默认值注册新对等体
	// Register the new peer with some meaningful defaults
	ps.lock.Lock()
	if _, ok := ps.peers[p.id]; ok {
		ps.lock.Unlock()
		return errAlreadyRegistered
	}
	if len(ps.peers) > 0 {
		p.headerThroughput, p.blockThroughput, p.receiptThroughput, p.stateThroughput = 0, 0, 0, 0

		for _, peer := range ps.peers {
			peer.lock.RLock()
			p.headerThroughput += peer.headerThroughput
			p.blockThroughput += peer.blockThroughput
			p.receiptThroughput += peer.receiptThroughput
			p.stateThroughput += peer.stateThroughput
			peer.lock.RUnlock()
		}
		p.headerThroughput /= float64(len(ps.peers))
		p.blockThroughput /= float64(len(ps.peers))
		p.receiptThroughput /= float64(len(ps.peers))
		p.stateThroughput /= float64(len(ps.peers))
	}
	ps.peers[p.id] = p
	ps.lock.Unlock()

	ps.newPeerFeed.Send(p)
	return nil
}

//取消注册将远程对象从活动集中移除，禁用对该特定实体的任何进一步操作
// Unregister removes a remote peer from the active set, disabling any further
// actions to/from that particular entity.
func (ps *peerSet) Unregister(id string) error {
	ps.lock.Lock()
	p, ok := ps.peers[id]
	if !ok {
		defer ps.lock.Unlock()
		return errNotRegistered
	}
	delete(ps.peers, id)
	ps.lock.Unlock()

	ps.peerDropFeed.Send(p)
	return nil
}

// Peer使用给定的ID检索注册的对等体
// Peer retrieves the registered peer with the given id.
func (ps *peerSet) Peer(id string) *peerConnection {
	ps.lock.RLock()
	defer ps.lock.RUnlock()

	return ps.peers[id]
}

// Len返回集合中当前的对等数量。
// Len returns if the current number of peers in the set.
func (ps *peerSet) Len() int {
	ps.lock.RLock()
	defer ps.lock.RUnlock()

	return len(ps.peers)
}

// AllPeers检索se中所有对等的平面列表
// AllPeers retrieves a flat list of all the peers within the set.
func (ps *peerSet) AllPeers() []*peerConnection {
	ps.lock.RLock()
	defer ps.lock.RUnlock()

	list := make([]*peerConnection, 0, len(ps.peers))
	for _, p := range ps.peers {
		list = append(list, p)
	}
	return list
}

// HeaderIdlePeers检索活动对等集中所有当前标头空闲对等的平面列表，按其声誉排序。
// HeaderIdlePeers retrieves a flat list of all the currently header-idle peers
// within the active peer set, ordered by their reputation.
func (ps *peerSet) HeaderIdlePeers() ([]*peerConnection, int) {
	idle := func(p *peerConnection) bool {
		return atomic.LoadInt32(&p.headerIdle) == 0
	}
	throughput := func(p *peerConnection) float64 {
		p.lock.RLock()
		defer p.lock.RUnlock()
		return p.headerThroughput
	}
	return ps.idlePeers(62, 64, idle, throughput)
}

// BodyIdlePeers根据其声誉排序，检索活动对等集内所有当前正文空闲对等的平面列表。
// BodyIdlePeers retrieves a flat list of all the currently body-idle peers within
// the active peer set, ordered by their reputation.
func (ps *peerSet) BodyIdlePeers() ([]*peerConnection, int) {
	idle := func(p *peerConnection) bool {
		return atomic.LoadInt32(&p.blockIdle) == 0
	}
	throughput := func(p *peerConnection) float64 {
		p.lock.RLock()
		defer p.lock.RUnlock()
		return p.blockThroughput
	}
	return ps.idlePeers(62, 64, idle, throughput)
}

// ReceiptIdlePeers检索活动对等集内所有当前接收空闲对等的平坦列表，按其声誉排序。
// ReceiptIdlePeers retrieves a flat list of all the currently receipt-idle peers
// within the active peer set, ordered by their reputation.
func (ps *peerSet) ReceiptIdlePeers() ([]*peerConnection, int) {
	idle := func(p *peerConnection) bool {
		return atomic.LoadInt32(&p.receiptIdle) == 0
	}
	throughput := func(p *peerConnection) float64 {
		p.lock.RLock()
		defer p.lock.RUnlock()
		return p.receiptThroughput
	}
	return ps.idlePeers(63, 64, idle, throughput)
}

// NodeDataIdlePeers检索活动对等集内所有当前节点数据空闲对等的平坦列表，按其声誉排序。
// NodeDataIdlePeers retrieves a flat list of all the currently node-data-idle
// peers within the active peer set, ordered by their reputation.
func (ps *peerSet) NodeDataIdlePeers() ([]*peerConnection, int) {
	idle := func(p *peerConnection) bool {
		return atomic.LoadInt32(&p.stateIdle) == 0
	}
	throughput := func(p *peerConnection) float64 {
		p.lock.RLock()
		defer p.lock.RUnlock()
		return p.stateThroughput
	}
	return ps.idlePeers(63, 64, idle, throughput)
}

// idlePeers检索一个满足协议版本约束条件的所有当前空闲对象的平面列表，使用提供的功能来检查空闲状态。
//所得到的同伴组按其测量吞吐量排序。
// idlePeers retrieves a flat list of all currently idle peers satisfying the
// protocol version constraints, using the provided function to check idleness.
// The resulting set of peers are sorted by their measure throughput.
func (ps *peerSet) idlePeers(minProtocol, maxProtocol int, idleCheck func(*peerConnection) bool, throughput func(*peerConnection) float64) ([]*peerConnection, int) {
	ps.lock.RLock()
	defer ps.lock.RUnlock()

	idle, total := make([]*peerConnection, 0, len(ps.peers)), 0
	for _, p := range ps.peers { //首先抽取idle的peer
		if p.version >= minProtocol && p.version <= maxProtocol {
			if idleCheck(p) {
				idle = append(idle, p)
			}
			total++
		}
	}
	for i := 0; i < len(idle); i++ { // 冒泡排序， 从吞吐量大到吞吐量小。
		for j := i + 1; j < len(idle); j++ {
			if throughput(idle[i]) < throughput(idle[j]) {
				idle[i], idle[j] = idle[j], idle[i]
			}
		}
	}
	return idle, total
}

//medianRTT,求得peerset的RTT的中位数，
// medianRTT returns the median RTT of the peerset, considering only the tuning
// peers if there are more peers available.
// medianRTT返回对等体的RTT中值，如果有更多对等体可用，则只考虑调整对等体。
func (ps *peerSet) medianRTT() time.Duration {
	//收集所有当前测量的往返时间
	// Gather all the currnetly measured round trip times
	ps.lock.RLock()
	defer ps.lock.RUnlock()

	rtts := make([]float64, 0, len(ps.peers))
	for _, p := range ps.peers {
		p.lock.RLock()
		rtts = append(rtts, float64(p.rtt))
		p.lock.RUnlock()
	}
	sort.Float64s(rtts)

	median := rttMaxEstimate
	if qosTuningPeers <= len(rtts) {
		median = time.Duration(rtts[qosTuningPeers/2]) // Median of our tuning peers//我们的调整同伴的中位数
	} else if len(rtts) > 0 { //我们连接的同行的中位数（即使像这样保持一些基线质量）
		median = time.Duration(rtts[len(rtts)/2]) // Median of our connected peers (maintain even like this some baseline qos)
	} //将RTT限制为某些QoS默认值，与真正的RTT无关
	// Restrict the RTT into some QoS defaults, irrelevant of true RTT
	if median < rttMinEstimate {
		median = rttMinEstimate
	}
	if median > rttMaxEstimate {
		median = rttMaxEstimate
	}
	return median
}
