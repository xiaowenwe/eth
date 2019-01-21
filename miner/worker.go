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

package miner

import (
	"bytes"
	"fmt"
	"math/big"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus"
	"github.com/ethereum/go-ethereum/consensus/misc"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/ethdb"
	"github.com/ethereum/go-ethereum/event"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/params"
	"gopkg.in/fatih/set.v0"
	//"gopkg.in/fatih/set.v0"
	//"github.com/ethereum/go-ethereum/vendor/gopkg.in/fatih/set.v0"
)

//worker 内部包含了很多agent，可以包含之前提到的agent和remote_agent。
// worker同时负责构建区块和对象。同时把任务提供给agent。
const (
	resultQueueSize  = 10
	miningLogAtDepth = 5

	// txChanSize is the size of channel listening to TxPreEvent.
	// The number is referenced from the size of tx pool.
	// txChanSize是侦听TxPreEvent的通道的大小。
	//该数字是从tx池的大小引用的。
	txChanSize = 4096
	// chainHeadChanSize is the size of channel listening to ChainHeadEvent.监听ChainHeadEvent的通道的大小
	chainHeadChanSize = 10
	// chainSideChanSize is the size of channel listening to ChainSideEvent.监听 ChainSideEvent的通道的大小
	chainSideChanSize = 10
)

//Agent接口
// Agent can register themself with the worker
type Agent interface {
	Work() chan<- *Work
	SetReturnCh(chan<- *Result)
	Stop()
	Start()
	GetHashRate() int64
}

// Work is the workers current environment and holds
// all of the current state information
//work 当前的环境并保存所有当前的状态信息
type Work struct {
	config *params.ChainConfig
	signer types.Signer // 签名者

	state     *state.StateDB // apply state changes here状态数据库
	ancestors *set.Set       // ancestor set (used for checking uncle parent validity)祖先集合，用来检查祖先是否有效
	family    *set.Set       // family set (used for checking uncle invalidity)家族集合，用来检查祖先的无效性
	uncles    *set.Set       // uncle set集合
	tcount    int            // tx count in cycle这个周期的交易数量

	Block *types.Block // the new block //新的区块

	header   *types.Header        // 区块头
	txs      []*types.Transaction // 交易
	receipts []*types.Receipt     // 收据

	createdAt time.Time // 创建时间
}

type Result struct { //结果
	Work  *Work
	Block *types.Block
}

//工作者是负责将消息应用到新状态的主要对象
// worker is the main object which takes care of applying messages to the new state
type worker struct {
	config *params.ChainConfig
	engine consensus.Engine

	mu sync.Mutex

	// update loop
	mux          *event.TypeMux
	txCh         chan core.TxPreEvent     // 用来接受txPool里面的交易的通道
	txSub        event.Subscription       // 用来接受txPool里面的交易的订阅器
	chainHeadCh  chan core.ChainHeadEvent // 用来接受区块头的通道
	chainHeadSub event.Subscription
	chainSideCh  chan core.ChainSideEvent // 用来接受一个区块链从规范区块链移出的通道
	chainSideSub event.Subscription
	wg           sync.WaitGroup

	agents map[Agent]struct{} // 所有的agent
	recv   chan *Result       // agent会把结果发送到这个通道

	eth     Backend          // eth的协议
	chain   *core.BlockChain // 区块链
	proc    core.Validator   // 区块链验证器
	chainDb ethdb.Database   // 区块链数据库

	coinbase common.Address // 挖矿者的地址
	extra    []byte

	currentMu sync.Mutex
	current   *Work

	uncleMu        sync.Mutex
	possibleUncles map[common.Hash]*types.Block //可能的叔父节点

	unconfirmed *unconfirmedBlocks // set of locally mined blocks pending canonicalness confirmations一组本地开采的块正在等待canonicalness确认

	// atomic status counters
	mining int32
	atWork int32
}

//构造
func newWorker(config *params.ChainConfig, engine consensus.Engine, coinbase common.Address, eth Backend, mux *event.TypeMux) *worker {
	worker := &worker{
		config: config,
		engine: engine,
		eth:    eth,
		mux:    mux,
		txCh:   make(chan core.TxPreEvent, txChanSize), // TxPreEvent事件是TxPool发出的事件，
		// 代表一个新交易tx加入到了交易池中，这时候如果work空闲会将该笔交易收进work.txs，准备下一次打包进块。
		chainHeadCh: make(chan core.ChainHeadEvent, chainHeadChanSize), // ChainHeadEvent事件，代表已经有一个块作为链头，
		// 此时work.update函数会监听到这个事件，则会继续挖新的区块
		chainSideCh: make(chan core.ChainSideEvent, chainSideChanSize), // ChainSideEvent事件，代表有一个新块作为链的旁支，
		// 会被放到possibleUncles数组中，可能称为叔块。
		chainDb:        eth.ChainDb(), // 区块链数据库
		recv:           make(chan *Result, resultQueueSize),
		chain:          eth.BlockChain(),
		proc:           eth.BlockChain().Validator(),
		possibleUncles: make(map[common.Hash]*types.Block), // 存放可能称为下一个块的叔块数组
		coinbase:       coinbase,
		agents:         make(map[Agent]struct{}),
		unconfirmed:    newUnconfirmedBlocks(eth.BlockChain(), miningLogAtDepth), // 返回一个数据结构，包括追踪当前未被确认的区块。
	}
	// Subscribe TxPreEvent for tx pool为tx池订阅TxPreEvent
	worker.txSub = eth.TxPool().SubscribeTxPreEvent(worker.txCh)
	// Subscribe events for blockchain	订阅区块链事件
	worker.chainHeadSub = eth.BlockChain().SubscribeChainHeadEvent(worker.chainHeadCh)
	worker.chainSideSub = eth.BlockChain().SubscribeChainSideEvent(worker.chainSideCh)
	go worker.update() //接收管道消息，做出反应，包括挖矿，处理叔块，处理交易

	go worker.wait() //wait函数用来接受挖矿的结果然后写入本地区块链，同时通过eth协议广播出去。
	worker.commitNewWork()

	return worker
}

//设置挖矿者地址
func (self *worker) setEtherbase(addr common.Address) {
	self.mu.Lock()
	defer self.mu.Unlock()
	self.coinbase = addr
}

//设置额外数据
func (self *worker) setExtra(extra []byte) {
	self.mu.Lock()
	defer self.mu.Unlock()
	self.extra = extra
}

//未决定的*types.Block, *state.StateDB
func (self *worker) pending() (*types.Block, *state.StateDB) {
	self.currentMu.Lock()
	defer self.currentMu.Unlock()

	if atomic.LoadInt32(&self.mining) == 0 {
		return types.NewBlock(
			self.current.header,
			self.current.txs,
			nil,
			self.current.receipts,
		), self.current.state.Copy()
	}
	return self.current.Block, self.current.state.Copy()
}

//未决定的*types.Block
func (self *worker) pendingBlock() *types.Block {
	self.currentMu.Lock()
	defer self.currentMu.Unlock()

	if atomic.LoadInt32(&self.mining) == 0 {
		return types.NewBlock(
			self.current.header,
			self.current.txs,
			nil,
			self.current.receipts,
		)
	}
	return self.current.Block
}

// 启动worker 开始挖矿,实际上遍历启动了它所有的agent。上面提到了，这里走的是CpuAgent的实现。
func (self *worker) start() {
	self.mu.Lock()
	defer self.mu.Unlock()

	atomic.StoreInt32(&self.mining, 1)

	// spin up agents
	for agent := range self.agents {
		agent.Start()
	}
}

//停止挖矿
func (self *worker) stop() {
	self.wg.Wait()

	self.mu.Lock()
	defer self.mu.Unlock()
	if atomic.LoadInt32(&self.mining) == 1 {
		for agent := range self.agents {
			agent.Stop()
		}
	}
	atomic.StoreInt32(&self.mining, 0)
	atomic.StoreInt32(&self.atWork, 0)
}

//agent注册
func (self *worker) register(agent Agent) {
	self.mu.Lock()
	defer self.mu.Unlock()
	self.agents[agent] = struct{}{}
	agent.SetReturnCh(self.recv)
}

//删除
func (self *worker) unregister(agent Agent) {
	self.mu.Lock()
	defer self.mu.Unlock()
	delete(self.agents, agent)
	agent.Stop()
}

//接收管道消息，做出反应，包括挖矿，处理叔块，处理交易
func (self *worker) update() {
	defer self.txSub.Unsubscribe()
	defer self.chainHeadSub.Unsubscribe()
	defer self.chainSideSub.Unsubscribe()

	for {
		// A real event arrived, process interesting content一个真实的事件到达，处理有趣的内容
		select {
		// Handle ChainHeadEvent当接收到一个区块头的信息的时候，马上开启挖矿服务。
		//是指区块链中已经加入了一个新的区块作为整个链的链头，这时worker的回应是立即开始准备挖掘下一个新区块
		case <-self.chainHeadCh:
			self.commitNewWork()

		// Handle ChainSideEvent 接收不在规范的区块链的区块，加入到潜在的叔父集合
		case ev := <-self.chainSideCh:
			self.uncleMu.Lock()
			self.possibleUncles[ev.Block.Hash()] = ev.Block
			self.uncleMu.Unlock()

		// Handle TxPreEvent接收到txPool里面的交易信息的时候。
		case ev := <-self.txCh:
			// Apply transaction to the pending state if we're not mining如果我们不挖掘，则将事务应用于待处理状态
			if atomic.LoadInt32(&self.mining) == 0 {
				self.currentMu.Lock()
				acc, _ := types.Sender(self.current.signer, ev.Tx)
				txs := map[common.Address]types.Transactions{acc: {ev.Tx}}
				txset := types.NewTransactionsByPriceAndNonce(self.current.signer, txs)

				self.current.commitTransactions(self.mux, txset, self.chain, self.coinbase)
				self.currentMu.Unlock()
			} else {
				// If we're mining, but nothing is being processed, wake on new transactions
				if self.config.Clique != nil && self.config.Clique.Period == 0 {
					self.commitNewWork()
				}
			}

		// System stopped
		case <-self.txSub.Err():
			return
		case <-self.chainHeadSub.Err():
			return
		case <-self.chainSideSub.Err():
			return
		}
	}
}

//wait函数用来接受挖矿的结果然后写入本地区块链，同时通过eth协议广播出去。
//会在一个channel处一直等待Agent完成挖掘发送回来的新Block和Work对象。
// 这个Block会被写入数据库，加入本地的区块链试图成为最新的链头。
// 注意，此时区块中的所有交易，假设都已经被执行过了，所以这里的操作，
// 不会再去执行这些交易对象。
func (self *worker) wait() {
	for {
		mustCommitNewWork := true
		for result := range self.recv {
			atomic.AddInt32(&self.atWork, -1)

			if result == nil {
				continue
			}
			block := result.Block
			work := result.Work
			//更新所有日志中的块哈希，因为它现在可用，而不是当时
			//创建了单个交易的收据/日志。
			// Update the block hash in all logs since it is now available and not when the
			// receipt/log of individual transactions were created.
			for _, r := range work.receipts {
				for _, l := range r.Logs {
					l.BlockHash = block.Hash()
				}
			}
			for _, log := range work.state.Logs() {
				log.BlockHash = block.Hash()
			}
			stat, err := self.chain.WriteBlockWithState(block, work.receipts, work.state)
			if err != nil {
				log.Error("Failed writing block to chain", "err", err)
				continue
			}
			// check if canon block and write transactions检查canon是否阻塞并写入交易列表
			if stat == core.CanonStatTy { // 说明已经插入到规范的区块链
				// implicit by posting ChainHeadEvent
				//因为这种状态下，会发送ChainHeadEvent，会触发上面的update里面的代码，这部分代码会commitNewWork，所以在这里就不需要commit了。
				mustCommitNewWork = false
			}
			// 广播区块，并且申明区块链插入事件。
			// Broadcast the block and announce chain insertion event
			self.mux.Post(core.NewMinedBlockEvent{Block: block})
			var (
				events []interface{}
				logs   = work.state.Logs()
			)
			events = append(events, core.ChainEvent{Block: block, Hash: block.Hash(), Logs: logs})
			if stat == core.CanonStatTy {
				events = append(events, core.ChainHeadEvent{Block: block})
			}
			self.chain.PostChainEvents(events, logs)
			// 插入本地跟踪列表， 查看后续的确认状态。
			// Insert the block into the set of pending ones to wait for confirmations
			self.unconfirmed.Insert(block.NumberU64(), block.Hash())

			if mustCommitNewWork {
				self.commitNewWork()
			}
		}
	}
}

//如果我们没有在挖矿，那么直接返回，否则把任务送给每一个agent
// push sends a new work task to currently live miner agents.
func (self *worker) push(work *Work) {
	if atomic.LoadInt32(&self.mining) != 1 {
		return
	}
	for agent := range self.agents {
		atomic.AddInt32(&self.atWork, 1)
		if ch := agent.Work(); ch != nil {
			ch <- work
		}
	}
}

//为当前的周期创建一个新的环境。
// makeCurrent creates a new environment for the current cycle.
func (self *worker) makeCurrent(parent *types.Block, header *types.Header) error {
	state, err := self.chain.StateAt(parent.Root())
	if err != nil {
		return err
	}
	work := &Work{
		config:    self.config,
		signer:    types.NewEIP155Signer(self.config.ChainId),
		state:     state,
		ancestors: set.New(),
		family:    set.New(),
		uncles:    set.New(),
		header:    header,
		createdAt: time.Now(),
	}

	// when 08 is processed ancestors contain 07 (quick block)
	for _, ancestor := range self.chain.GetBlocksFromHash(parent.Hash(), 7) {
		for _, uncle := range ancestor.Uncles() {
			work.family.Add(uncle.Hash())
		}
		work.family.Add(ancestor.Hash())
		work.ancestors.Add(ancestor.Hash())
	}

	// Keep track of transactions which return errors so they can be removed
	work.tcount = 0
	self.current = work
	return nil
}

// 提交新的任务,为新块准备基本数据，包括header，txs，uncles等。
func (self *worker) commitNewWork() {
	self.mu.Lock()
	defer self.mu.Unlock()
	self.uncleMu.Lock()
	defer self.uncleMu.Unlock()
	self.currentMu.Lock()
	defer self.currentMu.Unlock()

	tstart := time.Now()
	parent := self.chain.CurrentBlock()
	//1准备新区块的时间属性Header.Time，一般均等于系统当前时间，不过要确保父区块的时间(parentBlock.Time())要早于新区块的时间，父区块当然来自当前区块链的链头了
	tstamp := tstart.Unix()
	if parent.Time().Cmp(new(big.Int).SetInt64(tstamp)) >= 0 { //// 不能出现比parent的时间还少的情况
		tstamp = parent.Time().Int64() + 1
	}
	// 我们的时间不要超过现在的时间太远， 那么等待一段时间，
	// 感觉这个功能完全是为了测试实现的， 如果是真实的挖矿程序，应该不会等待
	// this will ensure we're not going off too far in the future
	if now := time.Now().Unix(); tstamp > now+1 {
		wait := time.Duration(tstamp-now) * time.Second
		log.Info("Mining too far in the future", "wait", common.PrettyDuration(wait))
		time.Sleep(wait)
	}
	//2创建新区块的Header对象，其各属性中：Num可确定(父区块Num +1)；
	// Time可确定；ParentHash可确定;其余诸如Difficulty，GasLimit等，均留待之后共识算法中确定。
	num := parent.Number()
	header := &types.Header{
		ParentHash: parent.Hash(),
		Number:     num.Add(num, common.Big1),
		GasLimit:   core.CalcGasLimit(parent),
		Extra:      self.extra,
		Time:       big.NewInt(tstamp),
	}
	//只有当我们挖矿的时候才设置coinbase(避免虚假的块奖励？ TODO 没懂)
	// Only set the coinbase if we are mining (avoid spurious block rewards)
	if atomic.LoadInt32(&self.mining) == 1 {
		header.Coinbase = self.coinbase
	} //3调用Engine.Prepare()函数，完成Header对象的准备。
	if err := self.engine.Prepare(self.chain, header); err != nil {
		log.Error("Failed to prepare header for mining", "err", err)
		return
	}
	// 4根据我们是否关心DAO硬分叉来决定是否覆盖额外的数据。
	// If we are care about TheDAO hard-fork check whether to override the extra-data or not
	if daoBlock := self.config.DAOForkBlock; daoBlock != nil {
		// Check whether the block is among the fork extra-override range
		// 检查区块是否在 DAO硬分叉的范围内   [daoblock,daoblock+limit]
		limit := new(big.Int).Add(daoBlock, params.DAOForkExtraRange)
		if header.Number.Cmp(daoBlock) >= 0 && header.Number.Cmp(limit) < 0 { // 如果我们支持DAO 那么设置保留的额外的数据
			// Depending whether we support or oppose the fork, override differently
			if self.config.DAOForkSupport {
				header.Extra = common.CopyBytes(params.DAOForkBlockExtra)
			} else if bytes.Equal(header.Extra, params.DAOForkBlockExtra) {
				header.Extra = []byte{} // If miner opposes, don't let it use the reserved extra-data
			}
		}
	}
	// Could potentially happen if starting to mine in an odd state.
	err := self.makeCurrent(parent, header) // 用新的区块头来设置当前的状态
	if err != nil {
		log.Error("Failed to create mining context", "err", err)
		return
	}
	// Create the current work task and check any fork transitions needed
	//	5根据已有的Header对象，创建一个新的Work对象，并用其更新worker.current成员变量。
	work := self.current
	if self.config.DAOForkSupport && self.config.DAOForkBlock != nil && self.config.DAOForkBlock.Cmp(header.Number) == 0 {
		misc.ApplyDAOHardFork(work.state) // 6把DAO里面的资金转移到指定的账户。如果配置信息中支持硬分叉，在Work对象的StateDB里应用硬分叉。
	}
	//7准备新区块的交易列表，来源是TxPool中那些最近加入的tx，并执行这些交易。
	pending, err := self.eth.TxPool().Pending() //得到阻塞的资金
	if err != nil {
		log.Error("Failed to fetch pending transactions", "err", err)
		return
	}
	// 创建交易。
	txs := types.NewTransactionsByPriceAndNonce(self.current.signer, pending)
	// 提交交易
	work.commitTransactions(self.mux, txs, self.chain, self.coinbase)

	// compute uncles for the new block.
	var (
		uncles    []*types.Header
		badUncles []common.Hash
	)
	//8准备新区块的叔区块uncles[]，来源是worker.possibleUncles[]，
	//而possibleUncles[]中的每个区块都从事件ChainSideEvent中搜集得到。注意叔区块最多有两个。
	for hash, uncle := range self.possibleUncles {
		if len(uncles) == 2 {
			break
		}
		if err := self.commitUncle(work, uncle.Header()); err != nil {
			log.Trace("Bad uncle found and will be removed", "hash", hash)
			log.Trace(fmt.Sprint(uncle))

			badUncles = append(badUncles, hash)
		} else {
			log.Debug("Committing new uncle to block", "hash", hash)
			uncles = append(uncles, uncle.Header())
		}
	}
	for _, hash := range badUncles {
		delete(self.possibleUncles, hash)
	}
	// Create the new block to seal with the consensus engine
	//9调用Engine.Finalize()函数，对新区块“定型”，填充上Header.Root, TxHash, ReceiptHash,
	// UncleHash等几个属性。
	// 使用给定的状态来创建新的区块，Finalize会进行区块奖励等操作
	if work.Block, err = self.engine.Finalize(self.chain, header, work.state, work.txs, uncles, work.receipts); err != nil {
		log.Error("Failed to finalize block for sealing", "err", err)
		return
	}
	// We only care about logging if we're actually mining.
	//10如果上一个区块(即旧的链头区块)处于unconfirmedBlocks中，意味着它也是由本节点挖掘出来的，尝试去验证它已经被吸纳进主干链中。
	if atomic.LoadInt32(&self.mining) == 1 {
		log.Info("Commit new mining work", "number", work.Block.Number(), "txs", work.tcount, "uncles", len(uncles), "elapsed", common.PrettyDuration(time.Since(tstart)))
		self.unconfirmed.Shift(work.Block.NumberU64() - 1)
	}
	//11把创建的Work对象，通过channel发送给每一个登记过的Agent，进行后续的挖掘
	self.push(work)
}

func (self *worker) commitUncle(work *Work, uncle *types.Header) error {
	hash := uncle.Hash()
	if work.uncles.Has(hash) {
		return fmt.Errorf("uncle not unique")
	}
	if !work.ancestors.Has(uncle.ParentHash) {
		return fmt.Errorf("uncle's parent unknown (%x)", uncle.ParentHash[0:4])
	}
	if work.family.Has(hash) {
		return fmt.Errorf("uncle already in family (%x)", hash)
	}
	work.uncles.Add(uncle.Hash())
	return nil
}

func (env *Work) commitTransactions(mux *event.TypeMux, txs *types.TransactionsByPriceAndNonce, bc *core.BlockChain, coinbase common.Address) {
	gp := new(core.GasPool).AddGas(env.header.GasLimit)

	var coalescedLogs []*types.Log

	for {
		//如果我们没有足够的gas进行任何进一步的交易，那么我们就结束了
		// If we don't have enough gas for any further transactions then we're done
		if gp.Gas() < params.TxGas {
			log.Trace("Not enough gas for further transactions", "gp", gp)
			break
		}
		// Retrieve the next transaction and abort if all done		检索下一个事务并中止全部完成
		tx := txs.Peek()
		if tx == nil {
			break
		}

		// Error may be ignored here. The error has already been checked
		// during transaction acceptance is the transaction pool.
		///错误可能在此处被忽略。 在交易接受期间，错误已经被检查为交易池。 不管当前的hf如何，使用eip155签名者。
		// We use the eip155 signer regardless of the current hf.
		from, _ := types.Sender(env.signer, tx)
		// Check whether the tx is replay protected. If we're not in the EIP155 hf
		// phase, start ignoring the sender until we do.
		//请参考 https://github.com/ethereum/EIPs/blob/master/EIPS/eip-155.md
		//上面发生的交易可以拿到ETH上面进行重放， 反之亦然。 所以Vitalik提出了EIP155来避免这种情况。
		if tx.Protected() && !env.config.IsEIP155(env.header.Number) {
			log.Trace("Ignoring reply protected transaction", "hash", tx.Hash(), "eip155", env.config.EIP155Block)

			txs.Pop()
			continue
		}
		// Start executing the transaction开始执行交易。
		env.state.Prepare(tx.Hash(), common.Hash{}, env.tcount)
		// 执行交易
		err, logs := env.commitTransaction(tx, bc, coinbase, gp)
		switch err {
		case core.ErrGasLimitReached:
			// 弹出整个账户的所有交易， 不处理用户的下一个交易。
			// Pop the current out-of-gas transaction without shifting in the next from the account
			log.Trace("Gas limit exceeded for current block", "sender", from)
			txs.Pop()

		case core.ErrNonceTooLow:
			// 移动到用户的下一个交易
			// New head notification data race between the transaction pool and miner, shift
			log.Trace("Skipping transaction with low nonce", "sender", from, "nonce", tx.Nonce())
			txs.Shift()

		case core.ErrNonceTooHigh:
			// 跳过这个账户
			// Reorg notification data race between the transaction pool and miner, skip account =
			log.Trace("Skipping account with hight nonce", "sender", from, "nonce", tx.Nonce())
			txs.Pop()

		case nil:
			//一切正常，收集日志并从同一个账户转入下一笔交易
			// Everything ok, collect the logs and shift in the next transaction from the same account
			coalescedLogs = append(coalescedLogs, logs...)
			env.tcount++
			txs.Shift()

		default:
			// 其他奇怪的错误，跳过这个交易。
			// Strange error, discard the transaction and get the next in line (note, the
			// nonce-too-high clause will prevent us from executing in vain).
			log.Debug("Transaction failed, account skipped", "hash", tx.Hash(), "err", err)
			txs.Shift()
		}
	}

	if len(coalescedLogs) > 0 || env.tcount > 0 {
		// make a copy, the state caches the logs and these logs get "upgraded" from pending to mined
		// logs by filling in the block hash when the block was mined by the local miner. This can
		// cause a race condition if a log was "upgraded" before the PendingLogsEvent is processed.
		// 因为需要把log发送出去，而这边在挖矿完成后需要对log进行修改，所以拷贝一份发送出去，避免争用。
		cpy := make([]*types.Log, len(coalescedLogs))
		for i, l := range coalescedLogs {
			cpy[i] = new(types.Log)
			*cpy[i] = *l
		}
		go func(logs []*types.Log, tcount int) {
			if len(logs) > 0 {
				mux.Post(core.PendingLogsEvent{Logs: logs})
			}
			if tcount > 0 {
				mux.Post(core.PendingStateEvent{})
			}
		}(cpy, env.tcount)
	}
}

//ApplyTransaction
func (env *Work) commitTransaction(tx *types.Transaction, bc *core.BlockChain, coinbase common.Address, gp *core.GasPool) (error, []*types.Log) {
	snap := env.state.Snapshot()

	receipt, _, err := core.ApplyTransaction(env.config, bc, &coinbase, gp, env.state, env.header, tx, &env.header.GasUsed, vm.Config{})
	if err != nil {
		env.state.RevertToSnapshot(snap)
		return err, nil
	}
	env.txs = append(env.txs, tx)
	env.receipts = append(env.receipts, receipt)

	return nil, receipt.Logs
}
