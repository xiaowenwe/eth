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

package downloader

import (
	"fmt"
	"hash"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/crypto/sha3"
	"github.com/ethereum/go-ethereum/ethdb"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/trie"
)

//statesync 用来获取pivot point所指定的区块的所有的state 的trie树，也就是所有的账号的信息，包括普通账号和合约账户。
// stateReq represents a batch of state fetch requests groupped together into
// a single data retrieval network packet.
//stateSync调度下载由给定state root所定义的特定state trie的请求。
type stateReq struct {
	items    []common.Hash              // Hashes of the state items to download
	tasks    map[common.Hash]*stateTask // Download tasks to track previous attempts
	timeout  time.Duration              // Maximum round trip time for this to complete
	timer    *time.Timer                // Timer to fire when the RTT timeout expires
	peer     *peerConnection            // Peer that we're requesting from
	response [][]byte                   // Response data of the peer (nil for timeouts)
	dropped  bool                       // Flag whether the peer dropped off early
}

//如果此请求超时，则返回timedOut。
// timedOut returns if this request timed out.
func (req *stateReq) timedOut() bool {
	return req.response == nil
}

// stateSyncStats是在状态trie同步到RPC请求期间报告的进度统计信息的集合，以及在用户日志中显示的状态。
// stateSyncStats is a collection of progress stats to report during a state trie
// sync to RPC requests as well as to display in user logs.
type stateSyncStats struct {
	processed  uint64 // Number of state entries processed
	duplicate  uint64 // Number of state entries downloaded twice
	unexpected uint64 // Number of non-requested state entries received
	pending    uint64 // Number of still pending state entries
}

// syncState开始使用给定的根哈希下载状态。
// syncState starts downloading state with the given root hash.
func (d *Downloader) syncState(root common.Hash) *stateSync {
	s := newStateSync(d, root)
	select {
	case d.stateSyncStart <- s:
	case <-d.quitCh:
		s.err = errCancelStateFetch
		close(s.done)
	}
	return s
}

//在downloader中启动了一个新的goroutine 来运行stateFetcher函数。
// 这个函数首先试图往stateSyncStart通道来以获取信息。 而syncState这个函数会给stateSyncStart通道发送数据。
// stateFetcher manages the active state sync and accepts requests
// on its behalf.
func (d *Downloader) stateFetcher() {
	for {
		select {
		case s := <-d.stateSyncStart:
			for next := s; next != nil; {
				next = d.runStateSync(next)
			}
		case <-d.stateCh:
			// Ignore state responses while no sync is running.
		case <-d.quitCh:
			return
		}
	}
}

//runStateSync,这个方法从stateCh获取已经下载好的状态，然后把他投递到deliver通道上等待别人处理。
// runStateSync runs a state synchronisation until it completes or another root
// hash is requested to be switched over to.
func (d *Downloader) runStateSync(s *stateSync) *stateSync {
	var (
		active   = make(map[string]*stateReq) // Currently in-flight requests
		finished []*stateReq                  // Completed or failed requests
		timeout  = make(chan *stateReq)       // Timed out active requests
	)
	defer func() {
		// Cancel active request timers on exit. Also set peers to idle so they're
		// available for the next sync.
		for _, req := range active {
			req.timer.Stop()
			req.peer.SetNodeDataIdle(len(req.items))
		}
	}()
	// Run the state sync.
	// 运行状态同步
	go s.run()
	defer s.Cancel()
	//收听对等离开事件以取消分配的任务
	// Listen for peer departure events to cancel assigned tasks
	peerDrop := make(chan *peerConnection, 1024)
	peerSub := s.d.peers.SubscribePeerDrops(peerDrop)
	defer peerSub.Unsubscribe()

	for {
		//如果有第一个缓冲元素，则启用第一个缓冲元素的发送。
		// Enable sending of the first buffered element if there is one.
		var (
			deliverReq   *stateReq
			deliverReqCh chan *stateReq
		)
		if len(finished) > 0 {
			deliverReq = finished[0]
			deliverReqCh = s.deliver
		}

		select {
		// The stateSync lifecycle:
		// 另外一个stateSync申请运行。 我们退出。
		case next := <-d.stateSyncStart:
			return next

		case <-s.done:
			return nil

		// Send the next finished request to the current sync:
		// 发送已经下载好的数据给sync
		case deliverReqCh <- deliverReq:
			// Shift out the first request, but also set the emptied slot to nil for GC
			copy(finished, finished[1:])
			finished[len(finished)-1] = nil
			finished = finished[:len(finished)-1]

		// Handle incoming state packs:
		// 处理进入的数据包。 downloader接收到state的数据会发送到这个通道上面。
		case pack := <-d.stateCh:
			// Discard any data not requested (or previsouly timed out)
			req := active[pack.PeerId()]
			if req == nil {
				log.Debug("Unrequested node data", "peer", pack.PeerId(), "len", pack.Items())
				continue
			} //完成请求并排队等待处理
			// Finalize the request and queue up for processing
			req.timer.Stop()
			req.response = pack.(*statePack).states

			finished = append(finished, req)
			delete(active, pack.PeerId())

			// Handle dropped peer connections://处理丢弃的对等连接：
		case p := <-peerDrop:
			// Skip if no request is currently pending
			req := active[p.id]
			if req == nil {
				continue
			} //完成请求并排队等待处理
			// Finalize the request and queue up for processing
			req.timer.Stop()
			req.dropped = true

			finished = append(finished, req)
			delete(active, p.id)

		// Handle timed-out requests://处理超时请求：
		case req := <-timeout:
			//如果对等方已经在请求其他内容，请忽略过时超时。
			//当超时和交付同时发生时，可能会发生这种情况，
			//导致两个路径都触发
			// If the peer is already requesting something else, ignore the stale timeout.
			// This can happen when the timeout and the delivery happens simultaneously,
			// causing both pathways to trigger.
			if active[req.peer.id] != req {
				continue
			} //将超时数据移回下载队列
			// Move the timed out data back into the download queue
			finished = append(finished, req)
			delete(active, req.peer.id)

		// Track outgoing state requests://跟踪传出状态请求：
		case req := <-d.trackStateReq:
			//如果此对等方已存在活动请求，则表示存在问题。 理论上，trie节点调度必须永远不要将两个请求分配给同一个对等体。
			// 然而，在practive中，对等体可能会在前一次超时之前收到请求，断开连接并立即重新连接。 在这种情况下，
			// 第一个请求永远不会被尊重，唉我们不能无声地覆盖它，因为这会导致有效请求丢失并同步以卡住。
			// If an active request already exists for this peer, we have a problem. In
			// theory the trie node schedule must never assign two requests to the same
			// peer. In practive however, a peer might receive a request, disconnect and
			// immediately reconnect before the previous times out. In this case the first
			// request is never honored, alas we must not silently overwrite it, as that
			// causes valid requests to go missing and sync to get stuck.
			if old := active[req.peer.id]; old != nil {
				log.Warn("Busy peer assigned new state fetch", "peer", old.peer.id)

				// Make sure the previous one doesn't get siletly lost//确保前一个不会丢失
				old.timer.Stop()
				old.dropped = true

				finished = append(finished, old)
			}
			// Start a timer to notify the sync loop if the peer stalled.//如果对等体停止，则启动计时器以通知同步循环。
			req.timer = time.AfterFunc(req.timeout, func() {
				select {
				case timeout <- req:
				case <-s.done:
					//防止在退出runStateSync之前触发计时器的不太可能的情况下泄漏计时器goroutine。
					// Prevent leaking of timer goroutines in the unlikely case where a
					// timer is fired just before exiting runStateSync.
				}
			})
			active[req.peer.id] = req
		}
	}
}

// stateSync计划下载由给定状态根定义的特定状态trie的请求。
// stateSync schedules requests for downloading a particular state trie defined
// by a given state root.
type stateSync struct {
	d *Downloader // Downloader instance to access and manage current peerset/ Downloader实例访问和管理当前的peeret

	sched  *trie.TrieSync             // State trie sync scheduler defining the tasks状态trie同步调度程序定义任务
	keccak hash.Hash                  // Keccak256 hasher to verify deliveries withKeccak256用于验证交付情况
	tasks  map[common.Hash]*stateTask // Set of tasks currently queued for retrieval当前排队等待检索的任务集

	numUncommitted   int
	bytesUncommitted int

	deliver    chan *stateReq // Delivery channel multiplexing peer responses传递信道多路复用同伴响应
	cancel     chan struct{}  // Channel to signal a termination request用于发出终止请求的信道
	cancelOnce sync.Once      // Ensures cancel only ever gets called once确保取消只被调用一次
	done       chan struct{}  // Channel to signal termination completion信号终止完成的通道
	err        error          // Any error hit during sync (set before completion)同步期间遇到任何错误（在完成之前设置）
}

// stateTask表示单个节点下载节点，其中包含已尝试从中检索的一组对等点，以检测停滞的同步和中止。
// stateTask represents a single trie node download taks, containing a set of
// peers already attempted retrieval from to detect stalled syncs and abort.
type stateTask struct {
	attempts map[string]struct{}
}

// newStateSync创建一个新的状态trie下载调度程序。 此方法尚未启动同步。 用户需要调用run来启动。
// newStateSync creates a new state trie download scheduler. This method does not
// yet start the sync. The user needs to call run to initiate.
func newStateSync(d *Downloader, root common.Hash) *stateSync {
	return &stateSync{
		d:       d,
		sched:   state.NewStateSync(root, d.stateDB),
		keccak:  sha3.NewKeccak256(),
		tasks:   make(map[common.Hash]*stateTask),
		deliver: make(chan *stateReq),
		cancel:  make(chan struct{}),
		done:    make(chan struct{}),
	}
}

//run和loop方法，获取任务，分配任务，获取结果。
// run starts the task assignment and response processing loop, blocking until
// it finishes, and finally notifying any goroutines waiting for the loop to
// finish.
// run启动任务分配和响应处理循环，阻塞直到完成，最后通知任何goroutines等待循环完成。
func (s *stateSync) run() {
	s.err = s.loop()
	close(s.done)
}

//等待阻止，直到同步完成或取消。
// Wait blocks until the sync is done or canceled.
func (s *stateSync) Wait() error {
	<-s.done
	return s.err
}

//取消取消同步并等待它关闭。
// Cancel cancels the sync and waits until it has shut down.
func (s *stateSync) Cancel() error {
	s.cancelOnce.Do(func() { close(s.cancel) })
	return s.Wait()
}

// loop是状态trie sync的主要事件循环。 它负责将新任务分配给对等方（包括将其发送给它们）以及处理入站数据。
// 注意，循环不直接从对等端接收数据，而是在下载器中缓冲并在此处将异步推送到对等端。 原因是将处理与数据接收和超时分离。
// loop is the main event loop of a state trie sync. It it responsible for the
// assignment of new tasks to peers (including sending it to them) as well as
// for the processing of inbound data. Note, that the loop does not directly
// receive data from peers, rather those are buffered up in the downloader and
// pushed here async. The reason is to decouple processing from data receipt
// and timeouts.
func (s *stateSync) loop() error {
	//侦听新的对等事件以向其分配任务
	// Listen for new peer events to assign tasks to them
	newPeer := make(chan *peerConnection, 1024)
	peerSub := s.d.peers.SubscribeNewPeers(newPeer)
	defer peerSub.Unsubscribe()
	// 一直等到 sync完成或者被被终止
	// Keep assigning new tasks until the sync completes or aborts
	for s.sched.Pending() > 0 {
		// 把数据从缓存里面刷新到持久化存储里面。 这也就是命令行 --cache指定的大小。
		if err := s.commit(false); err != nil {
			return err
		}
		// 指派任务，
		s.assignTasks()
		// Tasks assigned, wait for something to happen
		select {
		case <-newPeer:
			// New peer arrived, try to assign it download tasks

		case <-s.cancel:
			return errCancelStateFetch

		case <-s.d.cancelCh:
			return errCancelStateFetch

		case req := <-s.deliver:
			// 接收到runStateSync方法投递过来的返回信息，注意 返回信息里面包含了成功请求的也包含了未成功请求的。
			// Response, disconnect or timeout triggered, drop the peer if stalling
			log.Trace("Received node data response", "peer", req.peer.id, "count", len(req.response), "dropped", req.dropped, "timeout", !req.dropped && req.timedOut())
			if len(req.items) <= 2 && !req.dropped && req.timedOut() {
				// 2项是最低要求，即使超时，我们目前也没有使用此对等项。
				// 2 items are the minimum requested, if even that times out, we've no use of
				// this peer at the moment.
				log.Warn("Stalling state sync, dropping peer", "peer", req.peer.id)
				s.d.dropPeer(req.peer.id)
			} //处理所有收到的blob并检查过期交付
			// Process all the received blobs and check for stale delivery
			if err := s.process(req); err != nil {
				log.Warn("Node data write error", "err", err)
				return err
			}
			req.peer.SetNodeDataIdle(len(req.response))
		}
	}
	return s.commit(true)
}

func (s *stateSync) commit(force bool) error {
	if !force && s.bytesUncommitted < ethdb.IdealBatchSize {
		return nil
	}
	start := time.Now()
	b := s.d.stateDB.NewBatch()
	s.sched.Commit(b)
	if err := b.Write(); err != nil {
		return fmt.Errorf("DB write error: %v", err)
	}
	s.updateStats(s.numUncommitted, 0, 0, time.Since(start))
	s.numUncommitted = 0
	s.bytesUncommitted = 0
	return nil
}

// assign任务尝试将新任务分配给所有空闲对等方，可以是当前正在重试的批处理，也可以从trie同步本身获取新数据。
// assignTasks attempts to assing new tasks to all idle peers, either from the
// batch currently being retried, or fetching new data from the trie sync itself.
func (s *stateSync) assignTasks() {
	// Iterate over all idle peers and try to assign them state fetches//迭代所有空闲对等体并尝试为它们分配状态提取
	peers, _ := s.d.peers.NodeDataIdlePeers()
	for _, p := range peers {
		// Assign a batch of fetches proportional to the estimated latency/bandwidth//分配与估计的延迟/带宽成比例的一批提取
		cap := p.NodeDataCapacity(s.d.requestRTT())
		req := &stateReq{peer: p, timeout: s.d.requestTTL()}
		s.fillTasks(cap, req)
		//如果为对等体分配了要获取的任务，则发送网络请求
		// If the peer was assigned tasks to fetch, send the network request
		if len(req.items) > 0 {
			req.peer.log.Trace("Requesting new batch of data", "type", "state", "count", len(req.items))
			select {
			case s.d.trackStateReq <- req:
				req.peer.FetchNodeData(req.items)
			case <-s.cancel:
			case <-s.d.cancelCh:
			}
		}
	}
}

// fillTasks使用最多n个状态下载任务填充给定的请求对象以发送给远程对等方。
// fillTasks fills the given request object with a maximum of n state download
// tasks to send to the remote peer.
func (s *stateSync) fillTasks(n int, req *stateReq) {
	// Refill available tasks from the scheduler.//从调度程序中重新填充可用任务。
	if len(s.tasks) < n {
		new := s.sched.Missing(n - len(s.tasks))
		for _, hash := range new {
			s.tasks[hash] = &stateTask{make(map[string]struct{})}
		}
	} //查找未使用请求的对等方尝试过的任务。
	// Find tasks that haven't been tried with the request's peer.
	req.items = make([]common.Hash, 0, n)
	req.tasks = make(map[common.Hash]*stateTask, n)
	for hash, t := range s.tasks {
		// Stop when we've gathered enough requests//当我们收集到足够的请求时停止
		if len(req.items) == n {
			break
		}
		// Skip any requests we've already tried from this peer//跳过我们已经尝试过的任何请求
		if _, ok := t.attempts[req.peer.id]; ok {
			continue
		}
		// Assign the request to this peer//将请求分配给此对等方
		t.attempts[req.peer.id] = struct{}{}
		req.items = append(req.items, hash)
		req.tasks[hash] = t
		delete(s.tasks, hash)
	}
}

//进程迭代一批传递的状态数据，将每个项目注入运行状态sync，重新排队已请求但未传递的任何项目。
// process iterates over a batch of delivered state data, injecting each item
// into a running state sync, re-queuing any items that were requested but not
// delivered.
func (s *stateSync) process(req *stateReq) error {
	//如果收到有效数据，请收集处理统计信息并更新进度
	// Collect processing stats and update progress if valid data was received
	duplicate, unexpected := 0, 0

	defer func(start time.Time) {
		if duplicate > 0 || unexpected > 0 {
			s.updateStats(0, duplicate, unexpected, time.Since(start))
		}
	}(time.Now())
	//迭代所有传递的数据并逐个注入trie
	// Iterate over all the delivered data and inject one-by-one into the trie
	progress := false

	for _, blob := range req.response {
		prog, hash, err := s.processNodeData(blob)
		switch err {
		case nil:
			s.numUncommitted++
			s.bytesUncommitted += len(blob)
			progress = progress || prog
		case trie.ErrNotRequested:
			unexpected++
		case trie.ErrAlreadyProcessed:
			duplicate++
		default:
			return fmt.Errorf("invalid state node %s: %v", hash.TerminalString(), err)
		}
		if _, ok := req.tasks[hash]; ok {
			delete(req.tasks, hash)
		}
	} //将未完成的任务放回重试队列
	// Put unfulfilled tasks back into the retry queue
	npeers := s.d.peers.Len()
	for hash, task := range req.tasks {
		//如果节点确实传递了某些内容，则缺少的项目可能是由于协议限制或先前的超时+延迟传递。
		// 这两种情况都应该允许节点重试丢失的项目（以避免单点停顿）。
		// If the node did deliver something, missing items may be due to a protocol
		// limit or a previous timeout + delayed delivery. Both cases should permit
		// the node to retry the missing items (to avoid single-peer stalls).
		if len(req.response) > 0 || req.timedOut() {
			delete(task.attempts, req.peer.id)
		}
		//如果我们已经多次请求节点，则可能是恶意同步，其中没有人拥有正确的数据。中止。
		// If we've requested the node too many times already, it may be a malicious
		// sync where nobody has the right data. Abort.
		if len(task.attempts) >= npeers {
			return fmt.Errorf("state node %s failed with all peers (%d tries, %d peers)", hash.TerminalString(), len(task.attempts), npeers)
		} //缺少项目，放入重试队列。
		// Missing item, place into the retry queue.
		s.tasks[hash] = task
	}
	return nil
}

// processNodeData尝试将从远程对等方传递的trie节点数据blob注入状态trie，返回是否写入了任何有用的内容或发生了任何错误。
// processNodeData tries to inject a trie node data blob delivered from a remote
// peer into the state trie, returning whether anything useful was written or any
// error occurred.
func (s *stateSync) processNodeData(blob []byte) (bool, common.Hash, error) {
	res := trie.SyncResult{Data: blob}
	s.keccak.Reset()
	s.keccak.Write(blob)
	s.keccak.Sum(res.Hash[:0])
	committed, _, err := s.sched.Process([]trie.SyncResult{res})
	return committed, res.Hash, err
}

// updateStats颠覆各种状态同步进度计数器并显示一条日志消息供用户查看。
// updateStats bumps the various state sync progress counters and displays a log
// message for the user to see.
func (s *stateSync) updateStats(written, duplicate, unexpected int, duration time.Duration) {
	s.d.syncStatsLock.Lock()
	defer s.d.syncStatsLock.Unlock()

	s.d.syncStatsState.pending = uint64(s.sched.Pending())
	s.d.syncStatsState.processed += uint64(written)
	s.d.syncStatsState.duplicate += uint64(duplicate)
	s.d.syncStatsState.unexpected += uint64(unexpected)

	if written > 0 || duplicate > 0 || unexpected > 0 {
		log.Info("Imported new state entries", "count", written, "elapsed", common.PrettyDuration(duration), "processed", s.d.syncStatsState.processed, "pending", s.d.syncStatsState.pending, "retry", len(s.tasks), "duplicate", s.d.syncStatsState.duplicate, "unexpected", s.d.syncStatsState.unexpected)
	}
	if written > 0 {
		core.WriteTrieSyncProgress(s.d.stateDB, s.d.syncStatsState.processed)
	}
}
