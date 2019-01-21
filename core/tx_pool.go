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
/*
txpool主要用来存放当前提交的等待写入区块的交易，有远端和本地的。

txpool里面的交易分为两种，

提交但是还不能执行的，放在queue里面等待能够执行(比如说nonce太高)。
等待执行的，放在pending里面等待执行。

从txpool的测试案例来看，txpool主要功能有下面几点。

交易验证的功能，包括余额不足，Gas不足，Nonce太低, value值是合法的，不能为负数。
能够缓存Nonce比当前本地账号状态高的交易。 存放在queue字段。 如果是能够执行的交易存放在pending字段
相同用户的相同Nonce的交易只会保留一个GasPrice最大的那个。 其他的插入不成功。
如果账号没有钱了，那么queue和pending中对应账号的交易会被删除。
如果账号的余额小于一些交易的额度，那么对应的交易会被删除，同时有效的交易会从pending移动到queue里面。防止被广播。
txPool支持一些限制PriceLimit(remove的最低GasPrice限制)，PriceBump(替换相同Nonce的交易的价格的百分比) AccountSlots(每个账户的pending的槽位的最小值) GlobalSlots(全局pending队列的最大值)AccountQueue(每个账户的queueing的槽位的最小值) GlobalQueue(全局queueing的最大值) Lifetime(在queue队列的最长等待时间)
有限的资源情况下按照GasPrice的优先级进行替换。
本地的交易会使用journal的功能存放在磁盘上，重启之后会重新导入。 远程的交易不会。
*/

package core

import (
	"errors"
	"fmt"
	"math"
	"math/big"
	"sort"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/event"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/metrics"
	"github.com/ethereum/go-ethereum/params"
	"gopkg.in/karalabe/cookiejar.v2/collections/prque"
)

const (
	// chainHeadChanSize是侦听ChainHeadEvent的通道的大小
	// chainHeadChanSize is the size of channel listening to ChainHeadEvent.
	chainHeadChanSize = 10
	// rmTxChanSize是侦听RemovedTransactionEvent的通道大小。
	// rmTxChanSize is the size of channel listening to RemovedTransactionEvent.
	rmTxChanSize = 10
)

var (
	//如果交易包含无效签名，则返回ErrInvalidSender。
	// ErrInvalidSender is returned if the transaction contains an invalid signature.
	ErrInvalidSender = errors.New("invalid sender")
	//如果交易的nonce低于本地链中存在的nonce，则返回ErrNonceTooLow。
	// ErrNonceTooLow is returned if the nonce of a transaction is lower than the
	// one present in the local chain.
	ErrNonceTooLow = errors.New("nonce too low")
	//如果交易的汽油价格低于为交易池配置的最低价格，则返回ErrUnderpriced。
	// ErrUnderpriced is returned if a transaction's gas price is below the minimum
	// configured for the transaction pool.
	ErrUnderpriced = errors.New("transaction underpriced")
	//如果尝试用不同的价格碰撞替换另一个交易，则返回ErrReplaceUnderpriced。
	// ErrReplaceUnderpriced is returned if a transaction is attempted to be replaced
	// with a different one without the required price bump.
	ErrReplaceUnderpriced = errors.New("replacement transaction underpriced")
	//如果执行交易的总成本高于用户帐户的余额，则返回ErrInsufficientFunds。
	// ErrInsufficientFunds is returned if the total cost of executing a transaction
	// is higher than the balance of the user's account.
	ErrInsufficientFunds = errors.New("insufficient funds for gas * price + value")
	//如果指定交易使用的气体少于启动调用所需的气体，则返回ErrIntrinsicGas。
	// ErrIntrinsicGas is returned if the transaction is specified to use less gas
	// than required to start the invocation.
	ErrIntrinsicGas = errors.New("intrinsic gas too low")
	//如果交易的请求气体限制超过当前块的最大容限，则返回ErrGasLimit。
	// ErrGasLimit is returned if a transaction's requested gas limit exceeds the
	// maximum allowance of the current block.
	ErrGasLimit = errors.New("exceeds block gas limit")
	// Er Negative Value是一个完整性错误，用于确保没有人能够指定具有负值的事务。
	// ErrNegativeValue is a sanity error to ensure noone is able to specify a
	// transaction with a negative value.
	ErrNegativeValue = errors.New("negative value")
	//如果交易的输入数据大于用户可能使用的某个有意义的限制，则返回ErrOversizedData。 这不是共识错误使交易无效，而不是DOS保护。
	// ErrOversizedData is returned if the input data of a transaction is greater
	// than some meaningful limit a user might use. This is not a consensus error
	// making the transaction invalid, rather a DOS protection.
	ErrOversizedData = errors.New("oversized data")
)

var (
	evictionInterval    = time.Minute     // Time interval to check for evictable transactions检查可撤销交易的时间间隔
	statsReportInterval = 8 * time.Second // Time interval to report transaction pool stats报告事务池统计信息的时间间隔
)

var (
	// Metrics for the pending pool待处理池的度量标准
	pendingDiscardCounter   = metrics.NewRegisteredCounter("txpool/pending/discard", nil)
	pendingReplaceCounter   = metrics.NewRegisteredCounter("txpool/pending/replace", nil)
	pendingRateLimitCounter = metrics.NewRegisteredCounter("txpool/pending/ratelimit", nil) // Dropped due to rate limiting由于速率限制而下降
	pendingNofundsCounter   = metrics.NewRegisteredCounter("txpool/pending/nofunds", nil)   // Dropped due to out-of-funds由于资金不足而下跌

	// Metrics for the queued pool排队池的度量标准
	queuedDiscardCounter   = metrics.NewRegisteredCounter("txpool/queued/discard", nil)//丢弃
	queuedReplaceCounter   = metrics.NewRegisteredCounter("txpool/queued/replace", nil)//更换
	queuedRateLimitCounter = metrics.NewRegisteredCounter("txpool/queued/ratelimit", nil) // Dropped due to rate limiting由于速率限制而下降
	queuedNofundsCounter   = metrics.NewRegisteredCounter("txpool/queued/nofunds", nil)   // Dropped due to out-of-funds由于资金不足而下跌

	// General tx metrics一般tx指标
	invalidTxCounter     = metrics.NewRegisteredCounter("txpool/invalid", nil)//无效
	underpricedTxCounter = metrics.NewRegisteredCounter("txpool/underpriced", nil)//低估
)
// TxStatus是池看到的交易的当前状态。
// TxStatus is the current status of a transaction as seen by the pool.
type TxStatus uint

const (
	TxStatusUnknown TxStatus = iota
	TxStatusQueued
	TxStatusPending
	TxStatusIncluded
)
//区块链提供区块链的状态和当前气体限制，以便在TX池和事件订阅者中执行/一些预检查。
// blockChain provides the state of blockchain and current gas limit to do
// some pre checks in tx pool and event subscribers.
type blockChain interface {
	CurrentBlock() *types.Block
	GetBlock(hash common.Hash, number uint64) *types.Block
	StateAt(root common.Hash) (*state.StateDB, error)

	SubscribeChainHeadEvent(ch chan<- ChainHeadEvent) event.Subscription
}
//TxPoolConfig是交易池的配置参数。
// TxPoolConfig are the configuration parameters of the transaction pool.
type TxPoolConfig struct {
	NoLocals  bool          // Whether local transaction handling should be disabled//是否应禁用本地交易处理
	Journal   string        // Journal of local transactions to survive node restarts本地交易日志以生存节点重新启动
	Rejournal time.Duration // Time interval to regenerate the local transaction journal重新生成本地交易日志的时间间隔

	PriceLimit uint64 // Minimum gas price to enforce for acceptance into the pool最低汽油价格，以供入库使用
	PriceBump  uint64 // Minimum price bump percentage to replace an already existing transaction (nonce)最低价格上涨百分比，以取代现有的交易(现在)

	AccountSlots uint64 // Minimum number of executable transaction slots guaranteed per account每个帐户保证的可执行交易槽的最小数目
	GlobalSlots  uint64 // Maximum number of executable transaction slots for all accounts所有帐户的最大可执行交易槽数
	AccountQueue uint64 // Maximum number of non-executable transaction slots permitted per account每个帐户允许的最大不可执行交易槽数
	GlobalQueue  uint64 // Maximum number of non-executable transaction slots for all accounts所有帐户的最大不可执行交易槽数

	Lifetime time.Duration // Maximum amount of time non-executable transaction are queued非可执行交易的最大排队时间
}
//DefaultTxPoolConfig包含事务/池的默认配置。
// DefaultTxPoolConfig contains the default configurations for the transaction
// pool.
var DefaultTxPoolConfig = TxPoolConfig{
	Journal:   "transactions.rlp",
	Rejournal: time.Hour,

	PriceLimit: 1,
	PriceBump:  10,

	AccountSlots: 16,
	GlobalSlots:  4096,
	AccountQueue: 64,
	GlobalQueue:  1024,

	Lifetime: 3 * time.Hour,
}
//sanitize检查提供的用户配置，并更改/不合理或不可行的任何内容。
// sanitize checks the provided user configurations and changes anything that's
// unreasonable or unworkable.
func (config *TxPoolConfig) sanitize() TxPoolConfig {
	conf := *config
	if conf.Rejournal < time.Second {
		log.Warn("Sanitizing invalid txpool journal time", "provided", conf.Rejournal, "updated", time.Second)
		conf.Rejournal = time.Second
	}
	if conf.PriceLimit < 1 {
		log.Warn("Sanitizing invalid txpool price limit", "provided", conf.PriceLimit, "updated", DefaultTxPoolConfig.PriceLimit)
		conf.PriceLimit = DefaultTxPoolConfig.PriceLimit
	}
	if conf.PriceBump < 1 {
		log.Warn("Sanitizing invalid txpool price bump", "provided", conf.PriceBump, "updated", DefaultTxPoolConfig.PriceBump)
		conf.PriceBump = DefaultTxPoolConfig.PriceBump
	}
	return conf
}

// TxPool contains all currently known transactions. Transactions
// enter the pool when they are received from the network or submitted
// locally. They exit the pool when they are included in the blockchain.
// TxPool 包含了当前知的交易， 当前网络接收到交易，或者本地提交的交易会加入到TxPool。
// 当他们已经被添加到区块链的时候被移除。
// The pool separates processable transactions (which can be applied to the
// current state) and future transactions. Transactions move between those
// two states over time as they are received and processed.
// TxPool分为可执行的交易(可以应用到当前的状态)和未来的交易。 交易在这两种状态之间转换，
type TxPool struct {
	config       TxPoolConfig
	chainconfig  *params.ChainConfig
	chain        blockChain
	gasPrice     *big.Int  //最低的GasPrice限制
	txFeed       event.Feed //通过txFeed来订阅TxPool的消息
	scope        event.SubscriptionScope
	chainHeadCh  chan ChainHeadEvent  // 订阅了区块头的消息，当有了新的区块头生成的时候会在这里收到通知
	chainHeadSub event.Subscription // 区块头消息的订阅器。
	signer       types.Signer // 封装了交易签名处理。
	mu           sync.RWMutex

	currentState  *state.StateDB      // Current state in the blockchain head区块链头部的当前状态
	pendingState  *state.ManagedState // Pending state tracking virtual nonces挂起状态跟踪虚拟随机数
	currentMaxGas uint64              // Current gas limit for transaction caps 目前交易上限的GasLimit


	locals  *accountSet // Set of local transaction to exempt from eviction rules本地交易免除驱逐规则
	journal *txJournal  // Journal of local transaction to back up to disk本地交易会写入磁盘

	pending map[common.Address]*txList         // All currently processable transactions 所有当前可以处理的交易
	queue   map[common.Address]*txList         // Queued but non-processable transactions 当前还不能处理的交易
	beats   map[common.Address]time.Time       // Last heartbeat from each known account 每一个已知账号的最后一次心跳信息的时间
	all     map[common.Hash]*types.Transaction // All transactions to allow lookups 可以查找到所有交易
	priced  *txPricedList                      // All transactions sorted by price 按照价格排序的交易

	wg sync.WaitGroup // for shutdown sync用于关机同步

	homestead bool// 家园版本
}
// NewTxPool创建一个新的交易池来收集，排序和过滤来自网络的入站交易。
// NewTxPool creates a new transaction pool to gather, sort and filter inbound
// transactions from the network.
func NewTxPool(config TxPoolConfig, chainconfig *params.ChainConfig, chain blockChain) *TxPool {
	// Sanitize the input to ensure no vulnerable gas prices are set/消毒输入以确保没有设定脆弱的天然气价格
	config = (&config).sanitize()
	//使用初始设置创建交易池
	// Create the transaction pool with its initial settings
	pool := &TxPool{
		config:      config,
		chainconfig: chainconfig,
		chain:       chain,
		signer:      types.NewEIP155Signer(chainconfig.ChainId),
		pending:     make(map[common.Address]*txList),
		queue:       make(map[common.Address]*txList),
		beats:       make(map[common.Address]time.Time),
		all:         make(map[common.Hash]*types.Transaction),
		chainHeadCh: make(chan ChainHeadEvent, chainHeadChanSize),
		gasPrice:    new(big.Int).SetUint64(config.PriceLimit),
	}
	pool.locals = newAccountSet(pool.signer)
	pool.priced = newTxPricedList(&pool.all)
	pool.reset(nil, chain.CurrentBlock().Header())

	// If local transactions and journaling is enabled, load from disk
	// 如果本地交易被允许,而且配置的Journal目录不为空,那么从指定的目录加载日志.
	// 然后rotate交易日志. 因为老的交易可能已经失效了, 所以调用add方法之后再把被接收的交易写入日志.
	if !config.NoLocals && config.Journal != "" {
		pool.journal = newTxJournal(config.Journal)

		if err := pool.journal.load(pool.AddLocal); err != nil {
			log.Warn("Failed to load transaction journal", "err", err)
		}
		if err := pool.journal.rotate(pool.local()); err != nil {
			log.Warn("Failed to rotate transaction journal", "err", err)
		}
	}
	// Subscribe events from blockchain//从区块链订阅事件
	pool.chainHeadSub = pool.chain.SubscribeChainHeadEvent(pool.chainHeadCh)

	// Start the event loop and return//启动事件循环并返回
	pool.wg.Add(1)
	go pool.loop()

	return pool
}
//loop是txPool的一个goroutine.也是主要的事件循环.等待和响应外部区块链事件以及各种报告和交易驱逐事件。
// loop is the transaction pool's main event loop, waiting for and reacting to
// outside blockchain events as well as for various reporting and transaction
// eviction events.
func (pool *TxPool) loop() {
	defer pool.wg.Done()
	//启动统计报告和交易逐出代码
	// Start the stats reporting and transaction eviction tickers
	var prevPending, prevQueued, prevStales int

	report := time.NewTicker(statsReportInterval)
	defer report.Stop()

	evict := time.NewTicker(evictionInterval)
	defer evict.Stop()

	journal := time.NewTicker(pool.config.Rejournal)
	defer journal.Stop()
	//跟踪事务重组的前一个头标题
	// Track the previous head headers for transaction reorgs
	head := pool.chain.CurrentBlock()
	//继续等待各种事件并做出反应
	// Keep waiting for and reacting to the various events
	for {
		select {
		// 监听到区块头的事件, 获取到新的区块头.
		// 调用reset方法
		// Handle ChainHeadEvent
		case ev := <-pool.chainHeadCh:
			if ev.Block != nil {
				pool.mu.Lock()
				if pool.chainconfig.IsHomestead(ev.Block.Number()) {
					pool.homestead = true
				}
				pool.reset(head.Header(), ev.Block.Header())
				head = ev.Block

				pool.mu.Unlock()
			}
			//由于系统停止而取消订阅
		// Be unsubscribed due to system stopped
		case <-pool.chainHeadSub.Err():
			return
			// Handle stats reporting ticks 报告就是打印了一些日志
		// Handle stats reporting ticks
		case <-report.C:
			pool.mu.RLock()
			pending, queued := pool.stats()
			stales := pool.priced.stales
			pool.mu.RUnlock()

			if pending != prevPending || queued != prevQueued || stales != prevStales {
				log.Debug("Transaction pool status report", "executable", pending, "queued", queued, "stales", stales)
				prevPending, prevQueued, prevStales = pending, queued, stales
			}
			// 处理超时的交易信息,
		// Handle inactive account transaction eviction
		case <-evict.C:
			pool.mu.Lock()
			for addr := range pool.queue {
				//从驱逐机制中跳过本地事务
				// Skip local transactions from the eviction mechanism
				if pool.locals.contains(addr) {
					continue
				}
				// Any non-locals old enough should be removed
				if time.Since(pool.beats[addr]) > pool.config.Lifetime {
					for _, tx := range pool.queue[addr].Flatten() {
						pool.removeTx(tx.Hash())
					}
				}
			}
			pool.mu.Unlock()
			// Handle local transaction journal rotation 处理定时写交易日志的信息.
		// Handle local transaction journal rotation
		case <-journal.C:
			if pool.journal != nil {
				pool.mu.Lock()
				if err := pool.journal.rotate(pool.local()); err != nil {
					log.Warn("Failed to rotate local tx journal", "err", err)
				}
				pool.mu.Unlock()
			}
		}
	}
}
// lockedReset是一个重置的包装器，允许在线程安全中调用它
//方式 这种方法只在测试仪中使用过！
// lockedReset is a wrapper around reset to allow calling it in a thread safe
// manner. This method is only ever used in the tester!
func (pool *TxPool) lockedReset(oldHead, newHead *types.Header) {
	pool.mu.Lock()
	defer pool.mu.Unlock()

	pool.reset(oldHead, newHead)
}
/*reset方法检索区块链的当前状态并且确保事务池的内容关于当前的区块链状态是有效的。主要功能包括：

因为更换了区块头，所以原有的区块中有一些交易因为区块头的更换而作废，这部分交易需要重新加入到txPool里面等待插入新的区块
生成新的currentState和pendingState
因为状态的改变。将pending中的部分交易移到queue里面
因为状态的改变，将queue里面的交易移入到pending里面。*/

// reset retrieves the current state of the blockchain and ensures the content
// of the transaction pool is valid with regard to the chain state.
func (pool *TxPool) reset(oldHead, newHead *types.Header) {
	// If we're reorging an old state, reinject all dropped transactions
	var reinject types.Transactions

	if oldHead != nil && oldHead.Hash() != newHead.ParentHash {
		// If the reorg is too deep, avoid doing it (will happen during fast sync)
		oldNum := oldHead.Number.Uint64()
		newNum := newHead.Number.Uint64()

		if depth := uint64(math.Abs(float64(oldNum) - float64(newNum))); depth > 64 { //如果老的头和新的头差距太远, 那么取消重建
			log.Debug("Skipping deep transaction reorg", "depth", depth)
		} else {
			// Reorg seems shallow enough to pull in all transactions into memory
			var discarded, included types.Transactions

			var (
				rem = pool.chain.GetBlock(oldHead.Hash(), oldHead.Number.Uint64())
				add = pool.chain.GetBlock(newHead.Hash(), newHead.Number.Uint64())
			)// 如果老的高度大于新的.那么需要把多的全部删除.
			for rem.NumberU64() > add.NumberU64() {
				discarded = append(discarded, rem.Transactions()...)
				if rem = pool.chain.GetBlock(rem.ParentHash(), rem.NumberU64()-1); rem == nil {
					log.Error("Unrooted old chain seen by tx pool", "block", oldHead.Number, "hash", oldHead.Hash())
					return
				}
			}// 如果新的高度大于老的, 那么需要增加.
			for add.NumberU64() > rem.NumberU64() {
				included = append(included, add.Transactions()...)
				if add = pool.chain.GetBlock(add.ParentHash(), add.NumberU64()-1); add == nil {
					log.Error("Unrooted new chain seen by tx pool", "block", newHead.Number, "hash", newHead.Hash())
					return
				}
			}// 高度相同了.如果hash不同,那么需要往后找,一直找到他们相同hash根的节点.
			for rem.Hash() != add.Hash() {
				discarded = append(discarded, rem.Transactions()...)
				if rem = pool.chain.GetBlock(rem.ParentHash(), rem.NumberU64()-1); rem == nil {
					log.Error("Unrooted old chain seen by tx pool", "block", oldHead.Number, "hash", oldHead.Hash())
					return
				}
				included = append(included, add.Transactions()...)
				if add = pool.chain.GetBlock(add.ParentHash(), add.NumberU64()-1); add == nil {
					log.Error("Unrooted new chain seen by tx pool", "block", newHead.Number, "hash", newHead.Hash())
					return
				}
			}
			// 找出所有存在discard里面,但是不在included里面的值.
			// 需要等下把这些交易重新插入到pool里面。
			reinject = types.TxDifference(discarded, included)
		}
	}
	// Initialize the internal state to the current head//将内部状态初始化为当前头部
	if newHead == nil {
		newHead = pool.chain.CurrentBlock().Header() // Special case during testing/测试期间的特殊情况
	}
	statedb, err := pool.chain.StateAt(newHead.Root)
	if err != nil {
		log.Error("Failed to reset txpool state", "err", err)
		return
	}
	pool.currentState = statedb
	pool.pendingState = state.ManageState(statedb)
	pool.currentMaxGas = newHead.GasLimit
	//注入因reorgs而丢弃的所有事务
	// Inject any transactions discarded due to reorgs
	log.Debug("Reinjecting stale transactions", "count", len(reinject))
	pool.addTxsLocked(reinject, false)
	//验证待处理事务池，这将删除
	//块中包含的任何事务或
	//由于其他交易而失效（例如
	//更高的汽油价格）
	// validate the pool of pending transactions, this will remove
	// any transactions that have been included in the block or
	// have been invalidated because of another transaction (e.g.
	// higher gas price)
	// 验证pending transaction池里面的交易， 会移除所有已经存在区块链里面的交易，或者是因为其他交易导致不可用的交易(比如有一个更高的gasPrice)
	// demote 降级 将pending中的一些交易降级到queue里面。
	pool.demoteUnexecutables()

	// Update all accounts to the latest known pending nonce
	// 根据pending队列的nonce更新所有账号的nonce
	for addr, list := range pool.pending {
		txs := list.Flatten() // Heavy but will be cached and is needed by the miner anyway
		pool.pendingState.SetNonce(addr, txs[len(txs)-1].Nonce()+1)
	}
	// 检查队列并尽可能地将事务移到pending，或删除那些已经失效的事务
	// promote 升级
	// Check the queue and move transactions over to the pending if possible
	// or remove those that have become invalid
	pool.promoteExecutables(nil)
}
//停止终止事务池
// Stop terminates the transaction pool.
func (pool *TxPool) Stop() {
	//取消订阅从txpool注册的所有订阅
	// Unsubscribe all subscriptions registered from txpool
	pool.scope.Close()
	//取消订阅区块链注册的订阅
	// Unsubscribe subscriptions registered from blockchain
	pool.chainHeadSub.Unsubscribe()
	pool.wg.Wait()

	if pool.journal != nil {
		pool.journal.close()
	}
	log.Info("Transaction pool stopped")
}

// SubscribeTxPreEvent registers a subscription of TxPreEvent and
// starts sending event to the given channel.
//SubscribeTxPreEvent注册TxPreEvent和//开始向给定信道发送事件。
func (pool *TxPool) SubscribeTxPreEvent(ch chan<- TxPreEvent) event.Subscription {
	return pool.scope.Track(pool.txFeed.Subscribe(ch))
}
// GasPrice返回事务池强制执行的当前汽油价格。
// GasPrice returns the current gas price enforced by the transaction pool.
func (pool *TxPool) GasPrice() *big.Int {
	pool.mu.RLock()
	defer pool.mu.RUnlock()

	return new(big.Int).Set(pool.gasPrice)
}
// SetGasPrice更新事务池为新事务所需的最低价格，并丢弃低于此阈值的所有事务。
// SetGasPrice updates the minimum price required by the transaction pool for a
// new transaction, and drops all transactions below this threshold.
func (pool *TxPool) SetGasPrice(price *big.Int) {
	pool.mu.Lock()
	defer pool.mu.Unlock()

	pool.gasPrice = price
	for _, tx := range pool.priced.Cap(price, pool.locals) {
		pool.removeTx(tx.Hash())
	}
	log.Info("Transaction pool price threshold updated", "price", price)
}
// State返回事务池的虚拟托管状态。
// State returns the virtual managed state of the transaction pool.
func (pool *TxPool) State() *state.ManagedState {
	pool.mu.RLock()
	defer pool.mu.RUnlock()

	return pool.pendingState
}
// Stats检索当前池的统计信息，即待处理的数量和
//排队（非可执行）事务的数量。
// Stats retrieves the current pool stats, namely the number of pending and the
// number of queued (non-executable) transactions.
func (pool *TxPool) Stats() (int, int) {
	pool.mu.RLock()
	defer pool.mu.RUnlock()

	return pool.stats()
}
// stats检索当前池统计信息，即挂起的数量和排队（不可执行）的事务数量。
// stats retrieves the current pool stats, namely the number of pending and the
// number of queued (non-executable) transactions.
func (pool *TxPool) stats() (int, int) {
	pending := 0
	for _, list := range pool.pending {
		pending += list.Len()
	}
	queued := 0
	for _, list := range pool.queue {
		queued += list.Len()
	}
	return pending, queued
}
// Content检索事务池的数据内容，返回所有挂起和排队的事务，按帐户分组并按nonce排序。
// Content retrieves the data content of the transaction pool, returning all the
// pending as well as queued transactions, grouped by account and sorted by nonce.
func (pool *TxPool) Content() (map[common.Address]types.Transactions, map[common.Address]types.Transactions) {
	pool.mu.Lock()
	defer pool.mu.Unlock()

	pending := make(map[common.Address]types.Transactions)
	for addr, list := range pool.pending {
		pending[addr] = list.Flatten()
	}
	queued := make(map[common.Address]types.Transactions)
	for addr, list := range pool.queue {
		queued[addr] = list.Flatten()
	}
	return pending, queued
}
//待定检索所有当前可处理的事务，按原始帐户分组并按nonce排序。 返回的事务集是一个副本，可以通过调用代码自由修改。
// Pending retrieves all currently processable transactions, groupped by origin
// account and sorted by nonce. The returned transaction set is a copy and can be
// freely modified by calling code.
func (pool *TxPool) Pending() (map[common.Address]types.Transactions, error) {
	pool.mu.Lock()
	defer pool.mu.Unlock()

	pending := make(map[common.Address]types.Transactions)
	for addr, list := range pool.pending {
		pending[addr] = list.Flatten()
	}
	return pending, nil
}
// local检索所有当前已知的本地事务，按原始帐户分组并按nonce排序。 返回的事务集是一个副本，可以通过调用代码自由修改。
// local retrieves all currently known local transactions, groupped by origin
// account and sorted by nonce. The returned transaction set is a copy and can be
// freely modified by calling code.
func (pool *TxPool) local() map[common.Address]types.Transactions {
	txs := make(map[common.Address]types.Transactions)
	for addr := range pool.locals.accounts {
		if pending := pool.pending[addr]; pending != nil {
			txs[addr] = append(txs[addr], pending.Flatten()...)
		}
		if queued := pool.queue[addr]; queued != nil {
			txs[addr] = append(txs[addr], queued.Flatten()...)
		}
	}
	return txs
}
//validateTx 使用一致性规则来检查一个交易是否有效,并采用本地节点的一些启发式的限制.
// validateTx checks whether a transaction is valid according to the consensus
// rules and adheres to some heuristic limits of the local node (price and size).
func (pool *TxPool) validateTx(tx *types.Transaction, local bool) error {
	//启发式限制，拒绝超过32KB的事务以防止DOS攻击
	// Heuristic limit, reject transactions over 32KB to prevent DOS attacks
	if tx.Size() > 32*1024 {
		return ErrOversizedData
	}//交易不能是否定的。 使用RLP解码的事务可能永远不会发生这种情况，但如果使用RPC创建事务可能会发生这种情况。
	// Transactions can't be negative. This may never happen using RLP decoded
	// transactions but may occur if you create a transaction using the RPC.
	if tx.Value().Sign() < 0 {
		return ErrNegativeValue
	}//确保交易不超过当前的限额限制气体。
	// Ensure the transaction doesn't exceed the current block limit gas.
	if pool.currentMaxGas < tx.Gas() {
		return ErrGasLimit
	}
	// 确保交易被正确签名.
	// Make sure the transaction is signed properly
	from, err := types.Sender(pool.signer, tx)
	if err != nil {
		return ErrInvalidSender
	}
	// 如果不是本地的交易,并且GasPrice低于我们的设置,那么也不会接收.
	// Drop non-local transactions under our own minimal accepted gas price
	local = local || pool.locals.contains(from) // account may be local even if the transaction arrived from the network
	if !local && pool.gasPrice.Cmp(tx.GasPrice()) > 0 {
		return ErrUnderpriced
	}
	// 确保交易遵守了Nonce的顺序
	// Ensure the transaction adheres to nonce ordering
	if pool.currentState.GetNonce(from) > tx.Nonce() {
		return ErrNonceTooLow
	}
	// 确保用户有足够的余额来支付.
	// Transactor should have enough funds to cover the costs
	// cost == V + GP * GL
	if pool.currentState.GetBalance(from).Cmp(tx.Cost()) < 0 {
		return ErrInsufficientFunds
	}
	intrGas, err := IntrinsicGas(tx.Data(), tx.To() == nil, pool.homestead)
	if err != nil {
		return err
	}
	// 如果交易是一个合约创建或者调用. 那么看看是否有足够的 初始Gas.
	if tx.Gas() < intrGas {
		return ErrIntrinsicGas
	}
	return nil
}
//add 方法, 验证交易并将其插入到future queue. 如果这个交易是替换了当前存在的某个交易,那么会返回之前的那个交易,这样外部就不用调用promote方法. 如果某个新增加的交易被标记为local,
//那么它的发送账户会进入白名单,这个账户的关联的交易将不会因为价格的限制或者其他的一些限制被删除.
// add validates a transaction and inserts it into the non-executable queue for
// later pending promotion and execution. If the transaction is a replacement for
// an already pending or queued one, it overwrites the previous and returns this
// so outer code doesn't uselessly call promote.
//
// If a newly added transaction is marked as local, its sending account will be
// whitelisted, preventing any associated transaction from being dropped out of
// the pool due to pricing constraints.
func (pool *TxPool) add(tx *types.Transaction, local bool) (bool, error) {
	// If the transaction is already known, discard it
	hash := tx.Hash()
	if pool.all[hash] != nil {
		log.Trace("Discarding already known transaction", "hash", hash)
		return false, fmt.Errorf("known transaction: %x", hash)
	}
	// If the transaction fails basic validation, discard it
	// 如果交易不能通过基本的验证,那么丢弃它
	if err := pool.validateTx(tx, local); err != nil {
		log.Trace("Discarding invalid transaction", "hash", hash, "err", err)
		invalidTxCounter.Inc(1)
		return false, err
	}
	// 如果交易池满了. 那么删除一些低价的交易.
	// If the transaction pool is full, discard underpriced transactions
	if uint64(len(pool.all)) >= pool.config.GlobalSlots+pool.config.GlobalQueue {
		// If the new transaction is underpriced, don't accept it
		// 如果新交易本身就是低价的.那么不接收它
		if pool.priced.Underpriced(tx, pool.locals) {
			log.Trace("Discarding underpriced transaction", "hash", hash, "price", tx.GasPrice())
			underpricedTxCounter.Inc(1)
			return false, ErrUnderpriced
		}
		// 否则删除低价值的给他腾空间.
		// New transaction is better than our worse ones, make room for it
		drop := pool.priced.Discard(len(pool.all)-int(pool.config.GlobalSlots+pool.config.GlobalQueue-1), pool.locals)
		for _, tx := range drop {
			log.Trace("Discarding freshly underpriced transaction", "hash", tx.Hash(), "price", tx.GasPrice())
			underpricedTxCounter.Inc(1)
			pool.removeTx(tx.Hash())
		}
	}//如果事务正在替换已经挂起的事务，请直接执行
	// If the transaction is replacing an already pending one, do directly
	from, _ := types.Sender(pool.signer, tx) // already validated//已经过验证
	if list := pool.pending[from]; list != nil && list.Overlaps(tx) {
		// Nonce already pending, check if required price bump is met
		// 如果交易对应的Nonce已经在pending队列了,那么产看是否能够替换.
		inserted, old := list.Add(tx, pool.config.PriceBump)
		if !inserted {
			pendingDiscardCounter.Inc(1)
			return false, ErrReplaceUnderpriced
		}//新事务更好，替换旧事务
		// New transaction is better, replace old one
		if old != nil {
			delete(pool.all, old.Hash())
			pool.priced.Removed()
			pendingReplaceCounter.Inc(1)
		}
		pool.all[tx.Hash()] = tx
		pool.priced.Put(tx)
		pool.journalTx(from, tx)

		log.Trace("Pooled new executable transaction", "hash", hash, "from", from, "to", tx.To())
		//我们直接注入替换事务，通知子系统
		// We've directly injected a replacement transaction, notify subsystems
		go pool.txFeed.Send(TxPreEvent{tx})

		return old != nil, nil
	}
	// New transaction isn't replacing a pending one, push into queue
	// 新交易不能替换pending里面的任意一个交易,那么把他push到futuren 队列里面.
	replace, err := pool.enqueueTx(hash, tx)
	if err != nil {
		return false, err
	}//标记本地地址和日记本地事务
	// Mark local addresses and journal local transactions
	if local {
		pool.locals.add(from)
	}
	// 如果是本地的交易,会被记录进入journalTx
	pool.journalTx(from, tx)

	log.Trace("Pooled new future transaction", "hash", hash, "from", from, "to", tx.To())
	return replace, nil
}

// enqueueTx inserts a new transaction into the non-executable transaction queue.
//enqueueTx 把一个新的交易插入到future queue。 这个方法假设已经获取了池的锁。
// Note, this method assumes the pool lock is held!
func (pool *TxPool) enqueueTx(hash common.Hash, tx *types.Transaction) (bool, error) {
	// Try to insert the transaction into the future queue//尝试将事务插入到将来的队列中
	from, _ := types.Sender(pool.signer, tx) // already validated已经过验证
	if pool.queue[from] == nil {
		pool.queue[from] = newTxList(false)
	}
	inserted, old := pool.queue[from].Add(tx, pool.config.PriceBump)
	if !inserted {
		// An older transaction was better, discard this//旧的交易更好，丢弃这个
		queuedDiscardCounter.Inc(1)
		return false, ErrReplaceUnderpriced
	}//丢弃之前的任何交易并标记此信息
	// Discard any previous transaction and mark this
	if old != nil {
		delete(pool.all, old.Hash())
		pool.priced.Removed()
		queuedReplaceCounter.Inc(1)
	}
	pool.all[hash] = tx
	pool.priced.Put(tx)
	return old != nil, nil
}
// journalTx将指定的事务添加到本地磁盘日志中，如果它被认为是从本地帐户发送的。
// journalTx adds the specified transaction to the local disk journal if it is
// deemed to have been sent from a local account.
func (pool *TxPool) journalTx(from common.Address, tx *types.Transaction) {
	// Only journal if it's enabled and the transaction is local只有日志，如果它已启用且交易是本地的
	if pool.journal == nil || !pool.locals.contains(from) {
		return
	}
	if err := pool.journal.insert(tx); err != nil {
		log.Warn("Failed to journal local transaction", "err", err)
	}
}

// promoteTx adds a transaction to the pending (processable) list of transactions.
//promoteTx把某个交易加入到pending 队列. 这个方法假设已经获取到了锁.
// Note, this method assumes the pool lock is held!
func (pool *TxPool) promoteTx(addr common.Address, hash common.Hash, tx *types.Transaction) {
	// Try to insert the transaction into the pending queue//尝试将事务插入挂起队列
	if pool.pending[addr] == nil {
		pool.pending[addr] = newTxList(true)
	}
	list := pool.pending[addr]

	inserted, old := list.Add(tx, pool.config.PriceBump)
	if !inserted {// 如果不能替换, 已经存在一个老的交易了. 删除.
		// An older transaction was better, discard this
		delete(pool.all, hash)
		pool.priced.Removed()

		pendingDiscardCounter.Inc(1)
		return
	}//否则丢弃任何先前的交易并标记
	// Otherwise discard any previous transaction and mark this
	if old != nil {
		delete(pool.all, old.Hash())
		pool.priced.Removed()

		pendingReplaceCounter.Inc(1)
	}//无法解决直接挂起的插入（测试）
	// Failsafe to work around direct pending inserts (tests)
	if pool.all[hash] == nil {
		pool.all[hash] = tx
		pool.priced.Put(tx)
	}
	// 把交易加入到队列,并发送消息告诉所有的订阅者, 这个订阅者在eth协议内部. 会接收这个消息并把这个消息通过网路广播出去.
	// Set the potentially new pending nonce and notify any subsystems of the new tx
	pool.beats[addr] = time.Now()
	pool.pendingState.SetNonce(addr, tx.Nonce()+1)

	go pool.txFeed.Send(TxPreEvent{tx})
}
// AddLocal将单个事务排入池中（如果它有效），同时将发送者标记为本地事务，确保它绕过本地定价约束。
// AddLocal enqueues a single transaction into the pool if it is valid, marking
// the sender as a local one in the mean time, ensuring it goes around the local
// pricing constraints.
func (pool *TxPool) AddLocal(tx *types.Transaction) error {
	return pool.addTx(tx, !pool.config.NoLocals)
}
// AddRemote将单个事务排入池中（如果它有效）。 如果发件人不在本地跟踪的发件人中，则将适用完全定价约束。
// AddRemote enqueues a single transaction into the pool if it is valid. If the
// sender is not among the locally tracked ones, full pricing constraints will
// apply.
func (pool *TxPool) AddRemote(tx *types.Transaction) error {
	return pool.addTx(tx, false)
}
// AddLocals将一批事务排入队列（如果它们有效）
//同时将发件人标记为本地发件人，确保他们绕过本地定价约束。
// AddLocals enqueues a batch of transactions into the pool if they are valid,
// marking the senders as a local ones in the mean time, ensuring they go around
// the local pricing constraints.
func (pool *TxPool) AddLocals(txs []*types.Transaction) []error {
	return pool.addTxs(txs, !pool.config.NoLocals)
}
// AddRemotes将一批事务排入池中（如果它们有效）。
//如果发件人不在本地跟踪的发件人中，则将适用完全定价约束。
// AddRemotes enqueues a batch of transactions into the pool if they are valid.
// If the senders are not among the locally tracked ones, full pricing constraints
// will apply.
func (pool *TxPool) AddRemotes(txs []*types.Transaction) []error {
	return pool.addTxs(txs, false)
}
//如果有效，则addTx会将单个事务排入池中。
// addTx enqueues a single transaction into the pool if it is valid.
func (pool *TxPool) addTx(tx *types.Transaction, local bool) error {
	pool.mu.Lock()
	defer pool.mu.Unlock()
	//尝试注入事务并更新任何状态
	// Try to inject the transaction and update any state
	replace, err := pool.add(tx, local)
	if err != nil {
		return err
	}//如果我们添加了新的交易，请运行促销检查并返回
	// If we added a new transaction, run promotion checks and return
	if !replace {
		from, _ := types.Sender(pool.signer, tx) // already validated
		pool.promoteExecutables([]common.Address{from})
	}
	return nil
}
// addTxs尝试将一批事务排队，如果它们有效。
// addTxs attempts to queue a batch of transactions if they are valid.
func (pool *TxPool) addTxs(txs []*types.Transaction, local bool) []error {
	pool.mu.Lock()
	defer pool.mu.Unlock()

	return pool.addTxsLocked(txs, local)
}
// addTxsLocked尝试把有效的交易放入queue队列，调用这个函数的时候假设已经获取到锁
// addTxsLocked attempts to queue a batch of transactions if they are valid,
// whilst assuming the transaction pool lock is already held.
func (pool *TxPool) addTxsLocked(txs []*types.Transaction, local bool) []error {
	// Add the batch of transaction, tracking the accepted ones
	dirty := make(map[common.Address]struct{})
	errs := make([]error, len(txs))

	for i, tx := range txs {
		var replace bool
		if replace, errs[i] = pool.add(tx, local); errs[i] == nil {
			if !replace {// replace 是替换的意思， 如果不是替换，那么就说明状态有更新，有可以下一步处理的可能。
				from, _ := types.Sender(pool.signer, tx) // already validated
				dirty[from] = struct{}{}
			}
		}
	}//如果实际添加了某些内容，则仅重新处理内部状态
	// Only reprocess the internal state if something was actually added
	if len(dirty) > 0 {
		addrs := make([]common.Address, 0, len(dirty))
		for addr := range dirty {
			addrs = append(addrs, addr)
		}
		// 传入了被修改的地址，
		pool.promoteExecutables(addrs)
	}
	return errs
}
// Status返回由其哈希标识的一批事务的状态（未知/挂起/排队）。
// Status returns the status (unknown/pending/queued) of a batch of transactions
// identified by their hashes.
func (pool *TxPool) Status(hashes []common.Hash) []TxStatus {
	pool.mu.RLock()
	defer pool.mu.RUnlock()

	status := make([]TxStatus, len(hashes))
	for i, hash := range hashes {
		if tx := pool.all[hash]; tx != nil {
			from, _ := types.Sender(pool.signer, tx) // already validated已经过验证
			if pool.pending[from] != nil && pool.pending[from].txs.items[tx.Nonce()] != nil {
				status[i] = TxStatusPending
			} else {
				status[i] = TxStatusQueued
			}
		}
	}
	return status
}
//如果事务包含在池中，则返回返回事务否则没有。
// Get returns a transaction if it is contained in the pool
// and nil otherwise.
func (pool *TxPool) Get(hash common.Hash) *types.Transaction {
	pool.mu.RLock()
	defer pool.mu.RUnlock()

	return pool.all[hash]
}
//removeTx，删除某个交易， 并把所有后续的交易移动到future queue
// removeTx removes a single transaction from the queue, moving all subsequent
// transactions back to the future queue.
func (pool *TxPool) removeTx(hash common.Hash) {
	// Fetch the transaction we wish to delete
	tx, ok := pool.all[hash]
	if !ok {
		return
	}
	addr, _ := types.Sender(pool.signer, tx) // already validated during insertion已在插入期间验证
	//从已知交易列表中删除它
	// Remove it from the list of known transactions
	delete(pool.all, hash)
	pool.priced.Removed()

	// Remove the transaction from the pending lists and reset the account nonce
	// 把交易从pending删除， 并把因为这个交易的删除而变得无效的交易放到future queue
	// 然后更新pendingState的状态
	if pending := pool.pending[addr]; pending != nil {
		if removed, invalids := pending.Remove(tx); removed {
			// If no more pending transactions are left, remove the list
			if pending.Empty() {
				delete(pool.pending, addr)
				delete(pool.beats, addr)
			}//推迟任何无效的交易
			// Postpone any invalidated transactions
			for _, tx := range invalids {
				pool.enqueueTx(tx.Hash(), tx)
			}//如果需要，请更新帐户nonce
			// Update the account nonce if needed
			if nonce := tx.Nonce(); pool.pendingState.GetNonce(addr) > nonce {
				pool.pendingState.SetNonce(addr, nonce)
			}
			return
		}
	}
	// 把交易从future queue删除.
	// Transaction is in the future queue
	if future := pool.queue[addr]; future != nil {
		future.Remove(tx)
		if future.Empty() {
			delete(pool.queue, addr)
		}
	}
}
//promoteExecutables方法把 已经变得可以执行的交易从future queue 插入到pending queue。通过这个处理过程，所有的无效的交易(nonce太低，余额不足)会被删除。
// promoteExecutables moves transactions that have become processable from the
// future queue to the set of pending transactions. During this process, all
// invalidated transactions (low nonce, low balance) are deleted.
func (pool *TxPool) promoteExecutables(accounts []common.Address) {
	// Gather all the accounts potentially needing updates
	// accounts存储了所有潜在需要更新的账户。 如果账户传入为nil，代表所有已知的账户。
	if accounts == nil {
		accounts = make([]common.Address, 0, len(pool.queue))
		for addr := range pool.queue {
			accounts = append(accounts, addr)
		}
	}//迭代所有帐户并推广任何可执行事务
	// Iterate over all accounts and promote any executable transactions
	for _, addr := range accounts {
		list := pool.queue[addr]
		if list == nil {
			continue // Just in case someone calls with a non existing account以防有人用非现有帐户打电话
		}
		// 删除所有的nonce太低的交易
		// Drop all transactions that are deemed too old (low nonce)
		for _, tx := range list.Forward(pool.currentState.GetNonce(addr)) {
			hash := tx.Hash()
			log.Trace("Removed old queued transaction", "hash", hash)
			delete(pool.all, hash)
			pool.priced.Removed()
		}
		// 删除所有余额不足的交易。
		// Drop all transactions that are too costly (low balance or out of gas)
		drops, _ := list.Filter(pool.currentState.GetBalance(addr), pool.currentMaxGas)
		for _, tx := range drops {
			hash := tx.Hash()
			log.Trace("Removed unpayable queued transaction", "hash", hash)
			delete(pool.all, hash)
			pool.priced.Removed()
			queuedNofundsCounter.Inc(1)
		}
		// 得到所有的可以执行的交易，并promoteTx加入pending
		// Gather all executable transactions and promote them
		for _, tx := range list.Ready(pool.pendingState.GetNonce(addr)) {
			hash := tx.Hash()
			log.Trace("Promoting queued transaction", "hash", hash)
			pool.promoteTx(addr, hash, tx)
		}
		// 删除所有超过限制的交易。
		// Drop all transactions over the allowed limit
		if !pool.locals.contains(addr) {
			for _, tx := range list.Cap(int(pool.config.AccountQueue)) {
				hash := tx.Hash()
				delete(pool.all, hash)
				pool.priced.Removed()
				queuedRateLimitCounter.Inc(1)
				log.Trace("Removed cap-exceeding queued transaction", "hash", hash)
			}
		}//如果整个队列条目变空，则删除它。
		// Delete the entire queue entry if it became empty.
		if list.Empty() {
			delete(pool.queue, addr)
		}
	}
	// If the pending limit is overflown, start equalizing allowances
	pending := uint64(0)
	for _, list := range pool.pending {
		pending += uint64(list.Len())
	}
	// 如果pending的总数超过系统的配置。
	if pending > pool.config.GlobalSlots {
		pendingBeforeCap := pending
		// Assemble a spam order to penalize large transactors first//首先汇编垃圾邮件订单以惩罚大型交易者
		spammers := prque.New()
		for addr, list := range pool.pending {
			// Only evict transactions from high rollers
			// 首先把所有大于AccountSlots最小值的账户记录下来， 会从这些账户里面剔除一些交易。
			// 注意spammers是一个优先级队列，也就是说是按照交易的多少从大到小排序的。
			if !pool.locals.contains(addr) && uint64(list.Len()) > pool.config.AccountSlots {
				spammers.Push(addr, float32(list.Len()))
			}
		}//逐渐放弃犯罪者的交易
		// Gradually drop transactions from offenders
		offenders := []common.Address{}
		for pending > pool.config.GlobalSlots && !spammers.Empty() {
			/*
			模拟一下offenders队列的账户交易数量的变化情况。
				第一次循环   [10]    循环结束  [10]
				第二次循环   [10, 9] 循环结束  [9,9]
				第三次循环   [9, 9, 7] 循环结束 [7, 7, 7]
				第四次循环   [7, 7 , 7 ,2] 循环结束 [2, 2 ,2, 2]
			*/
			// Retrieve the next offender if not local address//如果不是本地地址，则检索下一个违规者
			offender, _ := spammers.Pop()
			offenders = append(offenders, offender.(common.Address))
			//均衡余额，直到达到所有相同或更低的阈值
			// Equalize balances until all the same or below threshold
			if len(offenders) > 1 {// 第一次进入这个循环的时候， offenders队列里面有交易数量最大的两个账户
				// Calculate the equalization threshold for all current offenders
				// 把最后加入的账户的交易数量当成本次的阈值
				threshold := pool.pending[offender.(common.Address)].Len()

				// Iteratively reduce all offenders until below limit or threshold reached
				// 遍历直到pending有效，或者是倒数第二个的交易数量等于最后一个的交易数量
				for pending > pool.config.GlobalSlots && pool.pending[offenders[len(offenders)-2]].Len() > threshold {
					// 遍历除了最后一个账户以外的所有账户， 把他们的交易数量减去1.
					for i := 0; i < len(offenders)-1; i++ {
						list := pool.pending[offenders[i]]
						for _, tx := range list.Cap(list.Len() - 1) {
							// Drop the transaction from the global pools too//也从全局池中删除事务
							hash := tx.Hash()
							delete(pool.all, hash)
							pool.priced.Removed()

							// Update the account nonce to the dropped transaction//将帐户随机数更新为已删除的事务
							if nonce := tx.Nonce(); pool.pendingState.GetNonce(offenders[i]) > nonce {
								pool.pendingState.SetNonce(offenders[i], nonce)
							}
							log.Trace("Removed fairness-exceeding pending transaction", "hash", hash)
						}
						pending--
					}
				}
			}
		}
		// 经过上面的循环，所有的超过AccountSlots的账户的交易数量都变成了之前的最小值。
		// 如果还是超过阈值，那么在继续从offenders里面每次删除一个。
		// If still above threshold, reduce to limit or min allowance
		if pending > pool.config.GlobalSlots && len(offenders) > 0 {
			for pending > pool.config.GlobalSlots && uint64(pool.pending[offenders[len(offenders)-1]].Len()) > pool.config.AccountSlots {
				for _, addr := range offenders {
					list := pool.pending[addr]
					for _, tx := range list.Cap(list.Len() - 1) {
						// Drop the transaction from the global pools too//也从全局池中删除事务
						hash := tx.Hash()
						delete(pool.all, hash)
						pool.priced.Removed()

						// Update the account nonce to the dropped transaction/将帐户随机数更新为已删除的事务
						if nonce := tx.Nonce(); pool.pendingState.GetNonce(addr) > nonce {
							pool.pendingState.SetNonce(addr, nonce)
						}
						log.Trace("Removed fairness-exceeding pending transaction", "hash", hash)
					}
					pending--
				}
			}
		}
		pendingRateLimitCounter.Inc(int64(pendingBeforeCap - pending))
	}
	// 我们处理了pending的限制， 下面需要处理future queue的限制了。
	// If we've queued more transactions than the hard limit, drop oldest ones
	queued := uint64(0)
	for _, list := range pool.queue {
		queued += uint64(list.Len())
	}
	if queued > pool.config.GlobalQueue {
		// Sort all accounts with queued transactions by heartbeat
		addresses := make(addresssByHeartbeat, 0, len(pool.queue))
		for addr := range pool.queue {
			if !pool.locals.contains(addr) { // don't drop locals
				addresses = append(addresses, addressByHeartbeat{addr, pool.beats[addr]})
			}
		}
		sort.Sort(addresses)
		// 从后往前，也就是心跳越新的就越会被删除。
		// Drop transactions until the total is below the limit or only locals remain
		for drop := queued - pool.config.GlobalQueue; drop > 0 && len(addresses) > 0; {
			addr := addresses[len(addresses)-1]
			list := pool.queue[addr.address]

			addresses = addresses[:len(addresses)-1]
			//如果它们小于溢出，则删除所有事务
			// Drop all transactions if they are less than the overflow
			if size := uint64(list.Len()); size <= drop {
				for _, tx := range list.Flatten() {
					pool.removeTx(tx.Hash())
				}
				drop -= size
				queuedRateLimitCounter.Inc(int64(size))
				continue
			}//否则只丢弃最后几笔交易
			// Otherwise drop only last few transactions
			txs := list.Flatten()
			for i := len(txs) - 1; i >= 0 && drop > 0; i-- {
				pool.removeTx(txs[i].Hash())
				drop--
				queuedRateLimitCounter.Inc(1)
			}
		}
	}
}
//demoteUnexecutables 从pending删除无效的或者是已经处理过的交易，其他的不可执行的交易会被移动到future queue中。
// demoteUnexecutables removes invalid and processed transactions from the pools
// executable/pending queue and any subsequent transactions that become unexecutable
// are moved back into the future queue.
func (pool *TxPool) demoteUnexecutables() {
	// Iterate over all accounts and demote any non-executable transactions
	for addr, list := range pool.pending {
		nonce := pool.currentState.GetNonce(addr)
		// 删除所有小于当前地址的nonce的交易，并从pool.all删除。
		// Drop all transactions that are deemed too old (low nonce)
		for _, tx := range list.Forward(nonce) {
			hash := tx.Hash()
			log.Trace("Removed old pending transaction", "hash", hash)
			delete(pool.all, hash)
			pool.priced.Removed()
		}// 删除所有的太昂贵的交易。 用户的balance可能不够用。或者是out of gas
		// Drop all transactions that are too costly (low balance or out of gas), and queue any invalids back for later
		drops, invalids := list.Filter(pool.currentState.GetBalance(addr), pool.currentMaxGas)
		for _, tx := range drops {
			hash := tx.Hash()
			log.Trace("Removed unpayable pending transaction", "hash", hash)
			delete(pool.all, hash)
			pool.priced.Removed()
			pendingNofundsCounter.Inc(1)
		}
		for _, tx := range invalids {
			hash := tx.Hash()
			log.Trace("Demoting pending transaction", "hash", hash)
			pool.enqueueTx(hash, tx)
		}
		// 如果存在一个空洞(nonce空洞)， 那么需要把所有的交易都放入future queue。
		// 这一步确实应该不可能发生，因为Filter已经把 invalids的都处理了。 应该不存在invalids的交易，也就是不存在空洞的。
		// If there's a gap in front, warn (should never happen) and postpone all transactions
		if list.Len() > 0 && list.txs.Get(nonce) == nil {
			for _, tx := range list.Cap(0) {
				hash := tx.Hash()
				log.Error("Demoting invalidated transaction", "hash", hash)
				pool.enqueueTx(hash, tx)
			}
		}//如果整个队列条目变空，则删除它。
		// Delete the entire queue entry if it became empty.
		if list.Empty() {
			delete(pool.pending, addr)
			delete(pool.beats, addr)
		}
	}
}
// addressByHeartbeat是一个标记有上一个活动时间戳的帐户地址。
// addressByHeartbeat is an account address tagged with its last activity timestamp.
type addressByHeartbeat struct {
	address   common.Address
	heartbeat time.Time
}

type addresssByHeartbeat []addressByHeartbeat

func (a addresssByHeartbeat) Len() int           { return len(a) }
func (a addresssByHeartbeat) Less(i, j int) bool { return a[i].heartbeat.Before(a[j].heartbeat) }
func (a addresssByHeartbeat) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
//accountSet 就是一个账号的集合和一个处理签名的对象.
// accountSet is simply a set of addresses to check for existence, and a signer
// capable of deriving addresses from transactions.
type accountSet struct {
	accounts map[common.Address]struct{}
	signer   types.Signer
}
// newAccountSet创建一个新的地址集，其中包含用于发件人派生的关联签名者。
// newAccountSet creates a new address set with an associated signer for sender
// derivations.
func newAccountSet(signer types.Signer) *accountSet {
	return &accountSet{
		accounts: make(map[common.Address]struct{}),
		signer:   signer,
	}
}
//包含检查集合中是否包含给定地址的检查。
// contains checks if a given address is contained within the set.
func (as *accountSet) contains(addr common.Address) bool {
	_, exist := as.accounts[addr]
	return exist
}

// containsTx checks if the sender of a given tx is within the set. If the sender
// cannot be derived, this method returns false.
// containsTx检查给定tx的发送者是否在集合内。 如果发件人无法被计算出，则此方法返回false。
func (as *accountSet) containsTx(tx *types.Transaction) bool {
	if addr, err := types.Sender(as.signer, tx); err == nil {
		return as.contains(addr)
	}
	return false
}
//添加在要跟踪的集合中插入新地址。
// add inserts a new address into the set to track.
func (as *accountSet) add(addr common.Address) {
	as.accounts[addr] = struct{}{}
}
