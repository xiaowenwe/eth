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

// Package discover implements the Node Discovery Protocol.
//
// The Node Discovery protocol provides a way to find RLPx nodes that
// can be connected to. It uses a Kademlia-like protocol to maintain a
// distributed database of the IDs and endpoints of all listening
// nodes.
package discover

import (
	crand "crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	mrand "math/rand"
	"net"
	"sort"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/p2p/netutil"
)

const (
	alpha           = 3  // Kademlia concurrency factorKademlia并发因素
	bucketSize      = 16 // Kademlia bucket size // k桶容量
	maxReplacements = 10 // Size of per-bucket replacement list每桶替换列表的大小
	//我们为距离上限的1/15保留桶，因为我们不太可能会遇到更接近的节点。
	// We keep buckets for the upper 1/15 of distances because
	// it's very unlikely we'll ever encounter a node that's closer.
	hashBits          = len(common.Hash{}) * 8
	nBuckets          = hashBits / 15       // Number of buckets//桶的数量
	bucketMinDistance = hashBits - nBuckets // Log distance of closest bucket//最近桶的记录距离
	// IP地址限制。
	// IP address limits.
	bucketIPLimit, bucketSubnet = 2, 24 // at most 2 addresses from the same /24
	tableIPLimit, tableSubnet   = 10, 24
	//限制并发ping / pong交互的数量
	maxBondingPingPongs = 16 // Limit on the number of concurrent ping/pong interactions
	// 节点丢弃的限制，超过这些限制的节点将被丢弃
	maxFindnodeFailures = 5 // Nodes exceeding this limit are dropped//超出此限制的节点将被丢弃
	refreshInterval     = 30 * time.Minute
	revalidateInterval  = 10 * time.Second
	copyNodesInterval   = 30 * time.Second
	seedMinTableTime    = 5 * time.Minute
	seedCount           = 30
	seedMaxAge          = 5 * 24 * time.Hour
)

type Table struct {
	mutex   sync.Mutex        // protects buckets, bucket content, nursery, rand
	buckets [nBuckets]*bucket // index of known nodes by distance已知节点按距离的索引/ 引导节点
	nursery []*Node           // bootstrap nodes引导节点
	rand    *mrand.Rand       // source of randomness, periodically reseeded随机性来源，定期再次播种
	ips     netutil.DistinctNetSet

	db         *nodeDB // database of known nodes已知节点的数据库
	refreshReq chan chan struct{}
	initDone   chan struct{}
	closeReq   chan struct{}
	closed     chan struct{}

	bondmu    sync.Mutex
	bonding   map[NodeID]*bondproc
	bondslots chan struct{} // limits total number of active bonding processes限制活动绑定进程的总数

	nodeAddedHook func(*Node) // for testing

	net  transport
	self *Node // metadata of the local node本地节点的元数据
}

type bondproc struct {
	err  error
	n    *Node
	done chan struct{}
}

//通过UDP传输实现传输。
//它是一个接口，所以我们可以在不打开大量UDP套接字的情况下进行测试，也不需要生成私钥。
// transport is implemented by the UDP transport.
// it is an interface so we can test without opening lots of UDP
// sockets and without generating a private key.
type transport interface {
	ping(NodeID, *net.UDPAddr) error
	waitping(NodeID) error
	findnode(toid NodeID, addr *net.UDPAddr, target NodeID) ([]*Node, error)
	close()
}

//存储区包含节点，按其最后一次活动排序。 最近活动的条目是条目中的第一个元素。
// bucket contains nodes, ordered by their last activity. the entry
// that was most recently active is the first element in entries.
type bucket struct {
	entries []*Node // live entries, sorted by time of last contact实时条目，按上次联系时间排序
	// 候补节点，entries满了后之后的节点会存储到这
	replacements []*Node // recently seen nodes to be used if revalidation fails如果重新验证失败，最近会看到要使用的节点
	ips          netutil.DistinctNetSet
}

//每一个小时进行一次刷新工作(autoRefreshInterval)
//如果接收到refreshReq请求。那么进行刷新工作。
//如果接收到关闭消息。那么进行关闭。

func newTable(t transport, ourID NodeID, ourAddr *net.UDPAddr, nodeDBPath string, bootnodes []*Node) (*Table, error) {
	//这个在之前的database.go里面有介绍。 打开leveldb。如果path为空。那么打开一个基于内存的db
	// If no node database was given, use an in-memory one
	db, err := newNodeDB(nodeDBPath, Version, ourID)
	if err != nil {
		return nil, err
	} // 2.构造k桶表
	tab := &Table{
		net:        t,
		db:         db,
		self:       NewNode(ourID, ourAddr.IP, uint16(ourAddr.Port), uint16(ourAddr.Port)),
		bonding:    make(map[NodeID]*bondproc),
		bondslots:  make(chan struct{}, maxBondingPingPongs),
		refreshReq: make(chan chan struct{}),
		initDone:   make(chan struct{}),
		closeReq:   make(chan struct{}),
		closed:     make(chan struct{}),
		rand:       mrand.New(mrand.NewSource(0)),
		ips:        netutil.DistinctNetSet{Subnet: tableSubnet, Limit: tableIPLimit},
	} // 3.设置初始连接点
	if err := tab.setFallbackNodes(bootnodes); err != nil {
		return nil, err
	}
	for i := 0; i < cap(tab.bondslots); i++ {
		tab.bondslots <- struct{}{}
	} // 4.填充k桶
	for i := range tab.buckets {
		tab.buckets[i] = &bucket{
			ips: netutil.DistinctNetSet{Subnet: bucketSubnet, Limit: bucketIPLimit},
		}
	} // 4.加载种子节点
	tab.seedRand()
	tab.loadSeedNodes(false)
	//加载种子后启动后台过期goroutine，以便搜索种子节点也考虑过期的节点，否则该节点会在到期时被删除。
	// Start the background expiration goroutine after loading seeds so that the search for
	// seed nodes also considers older nodes that would otherwise be removed by the
	// expiration.
	tab.db.ensureExpirer()
	go tab.loop() // 5.开启协程循环更新
	return tab, nil
}

func (tab *Table) seedRand() {
	var b [8]byte
	crand.Read(b[:])

	tab.mutex.Lock()
	tab.rand.Seed(int64(binary.BigEndian.Uint64(b[:])))
	tab.mutex.Unlock()
}

//自己返回本地节点。
//返回的节点不应该被调用者修改。
// Self returns the local node.
// The returned node should not be modified by the caller.
func (tab *Table) Self() *Node {
	return tab.self
}

// ReadRandomNodes用来自表中的随机节点填充给定切片。 它不会多次写入同一个节点。 切片中的节点是副本，可以由调用者修改。
// ReadRandomNodes fills the given slice with random nodes from the
// table. It will not write the same node more than once. The nodes in
// the slice are copies and can be modified by the caller.
func (tab *Table) ReadRandomNodes(buf []*Node) (n int) {
	if !tab.isInitDone() {
		return 0
	}
	tab.mutex.Lock()
	defer tab.mutex.Unlock()
	//找到所有非空桶并获取新条目。
	// Find all non-empty buckets and get a fresh slice of their entries.
	var buckets [][]*Node
	for _, b := range tab.buckets {
		if len(b.entries) > 0 {
			buckets = append(buckets, b.entries[:])
		}
	}
	if len(buckets) == 0 {
		return 0
	}
	// Shuffle the buckets.
	for i := len(buckets) - 1; i > 0; i-- {
		j := tab.rand.Intn(len(buckets))
		buckets[i], buckets[j] = buckets[j], buckets[i]
	} //将每个桶的头部移入buf，移除变空的桶。
	// Move head of each bucket into buf, removing buckets that become empty.
	var i, j int
	for ; i < len(buf); i, j = i+1, (j+1)%len(buckets) {
		b := buckets[j]
		buf[i] = &(*b[0])
		buckets[j] = b[1:]
		if len(b) == 1 {
			buckets = append(buckets[:j], buckets[j+1:]...)
		}
		if len(buckets) == 0 {
			break
		}
	}
	return i + 1
}

//关闭终止网络监听器并刷新节点数据库。
// Close terminates the network listener and flushes the node database.
func (tab *Table) Close() {
	select {
	case <-tab.closed:
		// already closed.
	case tab.closeReq <- struct{}{}:
		<-tab.closed // wait for refreshLoop to end.
	}
}

//SetFallbackNodes方法，这个方法设置初始化的联系节点。 在table是空而且数据库里面也没有已知的节点，这些节点可以帮助连接上网络，
// setFallbackNodes sets the initial points of contact. These nodes
// are used to connect to the network if the table is empty and there
// are no known nodes in the database.
func (tab *Table) setFallbackNodes(nodes []*Node) error {
	for _, n := range nodes {
		if err := n.validateComplete(); err != nil {
			return fmt.Errorf("bad bootstrap/fallback node %q (%v)", n, err)
		}
	}
	tab.nursery = make([]*Node, 0, len(nodes))
	for _, n := range nodes {
		cpy := *n
		// Recompute cpy.sha because the node might not have been//重新计算cpy.sha，因为该节点可能没有
		////由NewNode或ParseNode创建。
		// created by NewNode or ParseNode.
		cpy.sha = crypto.Keccak256Hash(n.ID[:])
		tab.nursery = append(tab.nursery, &cpy)
	}
	return nil
}

// isInitDone returns whether the table's initial seeding procedure has completed.
func (tab *Table) isInitDone() bool {
	select {
	case <-tab.initDone:
		return true
	default:
		return false
	}
}

//Resolve方法用来获取一个指定ID的节点。 如果节点在本地。那么返回本地节点。 否则执行
//Lookup在网络上查询一次。 如果查询到节点。那么返回。否则返回nil
// Resolve searches for a specific node with the given ID.
// It returns nil if the node could not be found.
func (tab *Table) Resolve(targetID NodeID) *Node {
	// If the node is present in the local table, no
	// network interaction is required.
	hash := crypto.Keccak256Hash(targetID[:])
	tab.mutex.Lock()
	cl := tab.closest(hash, 1)
	tab.mutex.Unlock()
	if len(cl.entries) > 0 && cl.entries[0].ID == targetID {
		return cl.entries[0]
	}
	// Otherwise, do a network lookup.
	result := tab.Lookup(targetID)
	for _, n := range result {
		if n.ID == targetID {
			return n
		}
	}
	return nil
}

// Lookup performs a network search for nodes close
// to the given target. It approaches the target by querying
// nodes that are closer to it on each iteration.
// The given target does not need to be an actual node
// identifier.
func (tab *Table) Lookup(targetID NodeID) []*Node {
	return tab.lookup(targetID, true)
}

//在看看之前的Lookup函数。 这个函数用来查询一个指定节点的信息。 这个函数首先从本地拿到距离这个节点最近的所有16个节点。
// 然后给所有的节点发送findnode的请求。 然后对返回的界定进行bondall处理。 然后返回所有的节点。
func (tab *Table) lookup(targetID NodeID, refreshIfEmpty bool) []*Node {
	var (
		target         = crypto.Keccak256Hash(targetID[:])
		asked          = make(map[NodeID]bool)
		seen           = make(map[NodeID]bool)
		reply          = make(chan []*Node, alpha)
		pendingQueries = 0
		result         *nodesByDistance
	)
	// don't query further if we hit ourself.
	// unlikely to happen often in practice.
	asked[tab.self.ID] = true
	//不会询问我们自己
	for {
		tab.mutex.Lock()
		// generate initial result set// 获取k桶表中给定节点id的n个节点
		result = tab.closest(target, bucketSize)
		//求取和target最近的16个节点
		tab.mutex.Unlock()
		if len(result.entries) > 0 || !refreshIfEmpty { // k桶表不为空
			break
		}
		// The result set is empty, all nodes were dropped, refresh.
		// We actually wait for the refresh to complete here. The very
		// first query will hit this case and run the bootstrapping
		// logic.
		<-tab.refresh() // k桶表为空
		refreshIfEmpty = false
	}

	for { // 询问尚未询问的最接近的节点
		// ask the alpha closest nodes that we haven't asked yet
		// 这里会并发的查询，每次3个goroutine并发(通过pendingQueries参数进行控制)
		// 每次迭代会查询result中和target距离最近的三个节点。
		for i := 0; i < len(result.entries) && pendingQueries < alpha; i++ {
			n := result.entries[i]
			// 未被查询的节点
			if !asked[n.ID] { //如果没有查询过 //因为这个result.entries会被重复循环很多次。 所以用这个变量控制那些已经处理过了。
				asked[n.ID] = true
				pendingQueries++
				go func() { // 查找节点
					// Find potential neighbors to bond with
					r, err := tab.net.findnode(n.ID, n.addr(), targetID)
					if err != nil {
						// Bump the failure counter to detect and evacuate non-bonded entries
						fails := tab.db.findFails(n.ID) + 1
						tab.db.updateFindFails(n.ID, fails)
						log.Trace("Bumping findnode failure counter", "id", n.ID, "failcount", fails)

						if fails >= maxFindnodeFailures {
							log.Trace("Too many findnode failures, dropping", "id", n.ID, "failcount", fails)
							tab.delete(n)
						}
					}
					reply <- tab.bondall(r)
				}()
			}
		}
		if pendingQueries == 0 {
			// we have asked all closest nodes, stop the search
			break
		}
		// wait for the next reply
		for _, n := range <-reply {
			if n != nil && !seen[n.ID] { //因为不同的远方节点可能返回相同的节点。所有用seen[]来做排重。
				seen[n.ID] = true
				//这个地方需要注意的是, 查找出来的结果又会加入result这个队列。也就是说这是一个循环查找的过程， 只要result里面不断加入新的节点。这个循环就不会终止。
				result.push(n, bucketSize)
			}
		}
		pendingQueries--
	}
	return result.entries
}

func (tab *Table) refresh() <-chan struct{} {
	done := make(chan struct{})
	select {
	case tab.refreshReq <- done:
	case <-tab.closed:
		close(done)
	}
	return done
}

//所以函数主要的工作就是启动刷新工作。doRefresh
// loop schedules refresh, revalidate runs and coordinates shutdown.
func (tab *Table) loop() {
	var (
		revalidate     = time.NewTimer(tab.nextRevalidateTime())
		refresh        = time.NewTicker(refreshInterval)
		copyNodes      = time.NewTicker(copyNodesInterval)
		revalidateDone = make(chan struct{})
		refreshDone    = make(chan struct{})           // where doRefresh reports completion
		waiting        = []chan struct{}{tab.initDone} // holds waiting callers while doRefresh runs
	)
	defer refresh.Stop()
	defer revalidate.Stop()
	defer copyNodes.Stop()
	// 初始化刷新
	// Start initial refresh.
	go tab.doRefresh(refreshDone)

loop:
	for {
		select {
		case <-refresh.C:
			tab.seedRand()
			if refreshDone == nil {
				refreshDone = make(chan struct{})
				go tab.doRefresh(refreshDone)
			}
		case req := <-tab.refreshReq:
			// 将刷新请求加入通道数组
			waiting = append(waiting, req)
			if refreshDone == nil {
				refreshDone = make(chan struct{})
				go tab.doRefresh(refreshDone)
			}
		case <-refreshDone:
			// 刷新完毕，关闭通道
			for _, ch := range waiting {
				close(ch)
			}
			waiting, refreshDone = nil, nil
		case <-revalidate.C:
			// 重新验证
			go tab.doRevalidate(revalidateDone)
		case <-revalidateDone:
			revalidate.Reset(tab.nextRevalidateTime())
		case <-copyNodes.C:
			// 拷贝活跃节点
			go tab.copyBondedNodes()
		case <-tab.closeReq:
			break loop
		}
	}

	if tab.net != nil {
		tab.net.close()
	}
	if refreshDone != nil {
		<-refreshDone
	}
	for _, ch := range waiting {
		close(ch)
	}
	tab.db.close()
	close(tab.closed)
}

//随机查找一个目标，以便保持buckets是满的。如果table是空的，那么种子节点会插入。 （比如最开始的启动或者是删除错误的节点之后）
// doRefresh performs a lookup for a random target to keep buckets
// full. seed nodes are inserted if the table is empty (initial
// bootstrap or discarded faulty peers).
func (tab *Table) doRefresh(done chan struct{}) {
	defer close(done)
	//从数据库加载节点并插入它们。 这应该会产生一些以前看到的节点（希望）仍然存在。
	// Load nodes from the database and insert
	// them. This should yield a few previously seen nodes that are
	// (hopefully) still alive.
	tab.loadSeedNodes(true)

	// Run self lookup to discover new neighbor nodes.运行自查找以发现新的邻居节点。
	tab.lookup(tab.self.ID, false)
	// Kademlia文件指定存储桶刷新应在最近最少使用的存储桶中执行查找。 我们不能坚持这一点，因为findnode目标是一个512位的值（不是散列大小），并且不容易生成落入所选存储桶的sha3前映像。
	//我们用随机目标执行一些查找。
	// The Kademlia paper specifies that the bucket refresh should
	// perform a lookup in the least recently used bucket. We cannot
	// adhere to this because the findnode target is a 512bit value
	// (not hash-sized) and it is not easily possible to generate a
	// sha3 preimage that falls into a chosen bucket.
	// We perform a few lookups with a random target instead.
	for i := 0; i < 3; i++ {
		var target NodeID
		crand.Read(target[:])
		tab.lookup(target, false) //lookup是查找距离target最近的k个节点
	}
}

func (tab *Table) loadSeedNodes(bond bool) {
	//querySeeds函数在database.go章节有介绍，从数据库里面随机的查找可用的种子节点。
	//在最开始启动的时候数据库是空白的。也就是最开始的时候这个seeds返回的是空的。
	seeds := tab.db.querySeeds(seedCount, seedMaxAge)
	//调用bondall函数。会尝试联系这些节点，并插入到表中。
	//tab.nursery是在命令行中指定的种子节点。
	//最开始启动的时候。 tab.nursery的值是内置在代码里面的。 这里是有值的。
	//C:\GOPATH\src\github.com\ethereum\go-ethereum\mobile\params.go
	//这里面写死了值。 这个值是通过SetFallbackNodes方法写入的。 这个方法后续会分析。
	//这里会进行双向的pingpong交流。 然后把结果存储在数据库。
	seeds = append(seeds, tab.nursery...)
	if bond {
		seeds = tab.bondall(seeds)
	}
	for i := range seeds {
		seed := seeds[i]
		age := log.Lazy{Fn: func() interface{} { return time.Since(tab.db.bondTime(seed.ID)) }}
		log.Debug("Found seed node in database", "id", seed.ID, "addr", seed.addr(), "age", age)
		tab.add(seed)
	}
}

// 检查最后一个节点是否在线，不在线就更换或删除
// doRevalidate checks that the last node in a random bucket is still live
// and replaces or deletes the node if it isn't.
func (tab *Table) doRevalidate(done chan<- struct{}) {
	defer func() { done <- struct{}{} }()

	last, bi := tab.nodeToRevalidate()
	if last == nil {
		// No non-empty bucket found.
		return
	}
	// 发出ping指令，等待pong回复
	// Ping the selected node and wait for a pong.
	err := tab.ping(last.ID, last.addr())

	tab.mutex.Lock()
	defer tab.mutex.Unlock()
	b := tab.buckets[bi]
	if err == nil {
		// The node responded, move it to the front.
		log.Debug("Revalidated node", "b", bi, "id", last.ID)
		b.bump(last)
		return
	} // 没有事收到ping的回复，更换或删除节点
	// No reply received, pick a replacement or delete the node if there aren't
	// any replacements.
	if r := tab.replace(b, last); r != nil {
		log.Debug("Replaced dead node", "b", bi, "id", last.ID, "ip", last.IP, "r", r.ID, "rip", r.IP)
	} else {
		log.Debug("Removed dead node", "b", bi, "id", last.ID, "ip", last.IP)
	}
}

// 获取随机非空k桶的最后一个节点
// nodeToRevalidate returns the last node in a random, non-empty bucket.
func (tab *Table) nodeToRevalidate() (n *Node, bi int) {
	tab.mutex.Lock()
	defer tab.mutex.Unlock()

	for _, bi = range tab.rand.Perm(len(tab.buckets)) {
		b := tab.buckets[bi]
		if len(b.entries) > 0 {
			last := b.entries[len(b.entries)-1]
			return last, bi
		}
	}
	return nil, 0
}

func (tab *Table) nextRevalidateTime() time.Duration {
	tab.mutex.Lock()
	defer tab.mutex.Unlock()

	return time.Duration(tab.rand.Int63n(int64(revalidateInterval)))
}

// copyBondedNodes adds nodes from the table to the database if they have been in the table
// longer then minTableTime.
func (tab *Table) copyBondedNodes() {
	tab.mutex.Lock()
	defer tab.mutex.Unlock()

	now := time.Now()
	for _, b := range tab.buckets {
		for _, n := range b.entries {
			if now.Sub(n.addedAt) >= seedMinTableTime {
				tab.db.updateNode(n)
			}
		}
	}
}

// closest returns the n nodes in the table that are closest to the
// given id. The caller must hold tab.mutex.
func (tab *Table) closest(target common.Hash, nresults int) *nodesByDistance {
	// This is a very wasteful way to find the closest nodes but
	// obviously correct. I believe that tree-based buckets would make
	// this easier to implement efficiently.
	close := &nodesByDistance{target: target}
	for _, b := range tab.buckets {
		for _, n := range b.entries {
			close.push(n, nresults)
		}
	}
	return close
}

func (tab *Table) len() (n int) {
	for _, b := range tab.buckets {
		n += len(b.entries)
	}
	return n
}

//
//bondall方法，这个方法就是多线程的调用bond方法
// bondall bonds with all given nodes concurrently and returns
// those nodes for which bonding has probably succeeded.
func (tab *Table) bondall(nodes []*Node) (result []*Node) {
	rc := make(chan *Node, len(nodes))
	for i := range nodes {
		go func(n *Node) {
			nn, _ := tab.bond(false, n.ID, n.addr(), n.TCP)
			rc <- nn
		}(nodes[i])
	}
	for range nodes {
		if n := <-rc; n != nil {
			result = append(result, n)
		}
	}
	return result
}

// bond ensures the local node has a bond with the given remote node.
// It also attempts to insert the node into the table if bonding succeeds.
// The caller must not hold tab.mutex.
//
// A bond is must be established before sending findnode requests.
// Both sides must have completed a ping/pong exchange for a bond to
// exist. The total number of active bonding processes is limited in
// order to restrain network use.
//
// bond is meant to operate idempotently in that bonding with a remote
// node which still remembers a previously established bond will work.
// The remote node will simply not send a ping back, causing waitping
// to time out.
//
// If pinged is true, the remote node has just pinged us and one half
// of the process can be skipped.
//bond方法。记得在udp.go中。当我们收到一个ping方法的时候，也有可能会调用这个方法
//// bond确保本地节点与给定的远程节点具有绑定。(远端的ID和远端的IP)。
// 如果绑定成功，它也会尝试将节点插入表中。调用者必须持有tab.mutex锁
// 发送findnode请求之前必须建立一个绑定。	双方为了完成一个bond必须完成双向的ping/pong过程。
// 为了节约网路资源。 同时存在的bonding处理流程的总数量是受限的。
//bond 是幂等的操作，跟一个任然记得之前的bond的远程节点进行bond也可以完成。 远程节点会简单的不会发送ping。 等待waitping超时。
//如果pinged是true。 那么远端节点已经给我们发送了ping消息。这样一半的流程可以跳过。
func (tab *Table) bond(pinged bool, id NodeID, addr *net.UDPAddr, tcpPort uint16) (*Node, error) {
	if id == tab.self.ID {
		return nil, errors.New("is self")
	}
	if pinged && !tab.isInitDone() {
		return nil, errors.New("still initializing")
	}
	// Start bonding if we haven't seen this node for a while or if it failed findnode too often.
	node, fails := tab.db.node(id), tab.db.findFails(id)
	age := time.Since(tab.db.bondTime(id))
	var result error
	//如果数据库没有这个节点。 或者错误数量大于0或者节点超时。
	if fails > 0 || age > nodeDBNodeExpiration {

		log.Trace("Starting bonding ping/pong", "id", id, "known", node != nil, "failcount", fails, "age", age)

		tab.bondmu.Lock()
		w := tab.bonding[id]
		if w != nil {
			// Wait for an existing bonding process to complete.
			tab.bondmu.Unlock()
			<-w.done
		} else {
			// Register a new bonding process.
			w = &bondproc{done: make(chan struct{})}
			tab.bonding[id] = w
			tab.bondmu.Unlock()
			// Do the ping/pong. The result goes into w.
			tab.pingpong(w, pinged, id, addr, tcpPort)
			// Unregister the process after it's done.
			tab.bondmu.Lock()
			delete(tab.bonding, id)
			tab.bondmu.Unlock()
		}
		// Retrieve the bonding results
		result = w.err
		if result == nil {
			node = w.n
		}
	}
	// Add the node to the table even if the bonding ping/pong
	// fails. It will be relaced quickly if it continues to be
	// unresponsive.
	if node != nil {
		//这个方法比较重要。 如果对应的bucket有空间，会直接插入buckets。如果buckets满了。 会用ping操作来测试buckets中的节点试图腾出空间。
		tab.add(node)
		tab.db.updateFindFails(id, 0)
	}
	return node, result
}

func (tab *Table) pingpong(w *bondproc, pinged bool, id NodeID, addr *net.UDPAddr, tcpPort uint16) {
	// Request a bonding slot to limit network usage请求绑定槽以限制网络使用
	<-tab.bondslots
	defer func() { tab.bondslots <- struct{}{} }()
	// Ping远程节点。并等待一个pong消息
	// Ping the remote side and wait for a pong.
	if w.err = tab.ping(id, addr); w.err != nil {
		close(w.done)
		return
	}
	if !pinged {
		//这个在udp收到一个ping消息的时候被设置为真。这个时候我们已经收到对方的ping消息了。
		//那么我们就不同等待ping消息了。 否则需要等待对方发送过来的ping消息(我们主动发起ping消息)。
		// Give the remote node a chance to ping us before we start
		// sending findnode requests. If they still remember us,
		// waitping will simply time out.
		tab.net.waitping(id)
	}
	// 完成bond过程。 把节点插入数据库。 数据库操作在这里完成。 bucket的操作在tab.add里面完成。 buckets是内存的操作。 数据库是持久化的seeds节点。用来加速启动过程的。
	// Bonding succeeded, update the node database.
	w.n = NewNode(id, addr.IP, uint16(addr.Port), tcpPort)
	close(w.done)
}

// ping远程端点并等待回复，并相应地更新节点数据库。
// ping a remote endpoint and wait for a reply, also updating the node
// database accordingly.
func (tab *Table) ping(id NodeID, addr *net.UDPAddr) error {
	tab.db.updateLastPing(id, time.Now())
	if err := tab.net.ping(id, addr); err != nil {
		return err
	}
	tab.db.updateBondTime(id, time.Now())
	return nil
}

// bucket returns the bucket for the given node ID hash.
func (tab *Table) bucket(sha common.Hash) *bucket {
	d := logdist(tab.self.sha, sha)
	if d <= bucketMinDistance {
		return tab.buckets[0]
	}
	return tab.buckets[d-bucketMinDistance-1]
}

// add attempts to add the given node its corresponding bucket. If the
// bucket has space available, adding the node succeeds immediately.
// Otherwise, the node is added if the least recently active node in
// the bucket does not respond to a ping packet.
//add试图把给定的节点插入对应的bucket。 如果bucket有空间，那么直接插入。 否则，如果bucket中最近活动的节点没有响应ping操作，那么我们就使用这个节点替换它。
// The caller must not hold tab.mutex.
func (tab *Table) add(new *Node) {
	tab.mutex.Lock()
	defer tab.mutex.Unlock()

	b := tab.bucket(new.sha)
	if !tab.bumpOrAdd(b, new) { //如果节点存在。那么更新它的值。然后退出。
		// Node is not in table. Add it to the replacement list.
		tab.addReplacement(b, new)
	}
}

//stuff方法比较简单。 找到对应节点应该插入的bucket。 如果这个bucket没有满，那么就插入这个bucket。否则什么也不做。 需要说一下的是logdist()这个方法。这个方法对两个值进行按照位置异或，然后返回最高位的下标。 比如 logdist(101,010) = 3 logdist(100, 100) = 0 logdist(100,110) = 2
// stuff adds nodes the table to the end of their corresponding bucket
// if the bucket is not full. The caller must not hold tab.mutex.
func (tab *Table) stuff(nodes []*Node) {
	tab.mutex.Lock()
	defer tab.mutex.Unlock()

	for _, n := range nodes {
		if n.ID == tab.self.ID {
			continue // don't add self
		}
		b := tab.bucket(n.sha)
		if len(b.entries) < bucketSize {
			tab.bumpOrAdd(b, n)
		}
	}
}

// delete removes an entry from the node table (used to evacuate
// failed/non-bonded discovery peers).
func (tab *Table) delete(node *Node) {
	tab.mutex.Lock()
	defer tab.mutex.Unlock()

	tab.deleteInBucket(tab.bucket(node.sha), node)
}

func (tab *Table) addIP(b *bucket, ip net.IP) bool {
	if netutil.IsLAN(ip) {
		return true
	}
	if !tab.ips.Add(ip) {
		log.Debug("IP exceeds table limit", "ip", ip)
		return false
	}
	if !b.ips.Add(ip) {
		log.Debug("IP exceeds bucket limit", "ip", ip)
		tab.ips.Remove(ip)
		return false
	}
	return true
}

func (tab *Table) removeIP(b *bucket, ip net.IP) {
	if netutil.IsLAN(ip) {
		return
	}
	tab.ips.Remove(ip)
	b.ips.Remove(ip)
}

func (tab *Table) addReplacement(b *bucket, n *Node) {
	for _, e := range b.replacements {
		if e.ID == n.ID {
			return // already in list
		}
	}
	if !tab.addIP(b, n.IP) {
		return
	}
	var removed *Node
	b.replacements, removed = pushNode(b.replacements, n, maxReplacements)
	if removed != nil {
		tab.removeIP(b, removed.IP)
	}
}

//替换时，将n从替换列表中删除，如果它是存储桶中的最后一个条目，则用它替换“last”。
// 如果“最后”不是最后一个条目，则它已被替换为其他人或变为活动状态。
// replace removes n from the replacement list and replaces 'last' with it if it is the
// last entry in the bucket. If 'last' isn't the last entry, it has either been replaced
// with someone else or became active.// 这里验证失败后会把前面buket结构里replacements里的节点候补到entries
func (tab *Table) replace(b *bucket, last *Node) *Node {
	if len(b.entries) == 0 || b.entries[len(b.entries)-1].ID != last.ID {
		// Entry has moved, don't replace it.
		return nil
	}
	// Still the last entry.
	if len(b.replacements) == 0 {
		tab.deleteInBucket(b, last)
		return nil
	}
	r := b.replacements[tab.rand.Intn(len(b.replacements))]
	b.replacements = deleteNode(b.replacements, r)
	b.entries[len(b.entries)-1] = r
	tab.removeIP(b, last.IP)
	return r
}

// bump将给定节点移动到存储区条目列表的前面（如果它包含在该列表中）。
// bump moves the given node to the front of the bucket entry list
// if it is contained in that list.
func (b *bucket) bump(n *Node) bool {
	for i := range b.entries {
		if b.entries[i].ID == n.ID {
			// move it to the front
			copy(b.entries[1:], b.entries[:i])
			b.entries[0] = n
			return true
		}
	}
	return false
}

// bumpOrAdd将n移动到存储区条目列表的前面，或者如果列表未满，则添加它。 如果n在桶中，返回值为true。
// bumpOrAdd moves n to the front of the bucket entry list or adds it if the list isn't
// full. The return value is true if n is in the bucket.
func (tab *Table) bumpOrAdd(b *bucket, n *Node) bool {
	if b.bump(n) {
		return true
	}
	if len(b.entries) >= bucketSize || !tab.addIP(b, n.IP) {
		return false
	}
	b.entries, _ = pushNode(b.entries, n, bucketSize)
	b.replacements = deleteNode(b.replacements, n)
	n.addedAt = time.Now()
	if tab.nodeAddedHook != nil {
		tab.nodeAddedHook(n)
	}
	return true
}

func (tab *Table) deleteInBucket(b *bucket, n *Node) {
	b.entries = deleteNode(b.entries, n)
	tab.removeIP(b, n.IP)
}

// pushNode将n添加到列表的前面，最多保留最多的项目。
// pushNode adds n to the front of list, keeping at most max items.
func pushNode(list []*Node, n *Node, max int) ([]*Node, *Node) {
	if len(list) < max {
		list = append(list, nil)
	}
	removed := list[len(list)-1]
	copy(list[1:], list)
	list[0] = n
	return list, removed
}

// deleteNode removes n from list.
func deleteNode(list []*Node, n *Node) []*Node {
	for i := range list {
		if list[i].ID == n.ID {
			return append(list[:i], list[i+1:]...)
		}
	}
	return list
}

// nodesByDistance是一个节点列表，按照排序
//距目标距离。
// nodesByDistance is a list of nodes, ordered by
// distance to target.
type nodesByDistance struct {
	entries []*Node
	target  common.Hash
} // 将节点添加到给定列表并维持大小小于maxElems
//result.push方法，这个方法会根据 所有的节点对于target的距离进行排序。 按照从近到远的方式决定新节点的插入顺序。(队列中最大会包含16个元素)。
// 这样会导致队列里面的元素和target的距离越来越近。距离相对远的会被踢出队列。
// push adds the given node to the list, keeping the total size below maxElems.
func (h *nodesByDistance) push(n *Node, maxElems int) {
	ix := sort.Search(len(h.entries), func(i int) bool { // 根据节点到target的距离排序
		return distcmp(h.target, h.entries[i].sha, n.sha) > 0
	})
	if len(h.entries) < maxElems { // 大小未超出限制,将节点加入
		h.entries = append(h.entries, n)
	}
	if ix == len(h.entries) {
		//比我们已经拥有的所有节点更远。
		//如果有空间，节点现在是最后一个元素。
		// farther away than all nodes we already have.
		// if there was room for it, the node is now the last element.
	} else {
		//将现有条目向下滑动以腾出空间，这将覆盖我们刚添加的条目。
		// slide existing entries down to make room
		// this will overwrite the entry we just appended.
		copy(h.entries[ix+1:], h.entries[ix:])
		h.entries[ix] = n
	}
}
