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

// Package miner implements Ethereum block creation and mining.
package miner

import (
	"fmt"
	"sync/atomic"

	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/eth/downloader"
	"github.com/ethereum/go-ethereum/ethdb"
	"github.com/ethereum/go-ethereum/event"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/params"
)

//miner用来对worker进行管理， 订阅外部事件，控制worker的启动和停止。
// Backend wraps all methods required for mining.
type Backend interface {
	AccountManager() *accounts.Manager //账户管理
	BlockChain() *core.BlockChain
	TxPool() *core.TxPool
	ChainDb() ethdb.Database
}

// Miner creates blocks and searches for proof-of-work values.矿工创建块和搜索工作量证明值。
type Miner struct {
	mux *event.TypeMux // 事件锁，已被feed.mu.lock替代

	worker *worker // 干活的人

	coinbase common.Address   // 结点地址
	mining   int32            // 代表挖矿进行中的状态
	eth      Backend          // Backend对象，Backend是一个自定义接口封装了所有挖矿所需方法。
	engine   consensus.Engine // 共识引擎

	canStart    int32 // can start indicates whether we can start the mining operation可以启动指示是否可以启动采矿操作。
	shouldStart int32 // should start indicates whether we should start after sync应该开始指示我们是否应该在同步之后开始

}

//构造, 创建了一个CPU agent 启动了miner的update goroutine
func New(eth Backend, config *params.ChainConfig, mux *event.TypeMux, engine consensus.Engine) *Miner {
	miner := &Miner{
		eth:      eth,
		mux:      mux,
		engine:   engine,
		worker:   newWorker(config, engine, common.Address{}, eth, mux),
		canStart: 1,
	}
	miner.Register(NewCpuAgent(eth.BlockChain(), engine))
	go miner.update()

	return miner
}

//update订阅了downloader的事件， 注意这个goroutine是一个一次性的循环，
// 只要接收到一次downloader的downloader.DoneEvent或者 downloader.FailedEvent事件，
// 就会设置canStart为1. 并退出循环， 这是为了避免黑客恶意的 DOS攻击，让你不断的处于异常状态
// update keeps track of the downloader events. Please be aware that this is a one shot type of update loop.
// It's entered once and as soon as `Done` or `Failed` has been broadcasted the events are unregistered and
// the loop is exited. This to prevent a major security vuln where external parties can DOS you with blocks
// and halt your mining operation for as long as the DOS continues.
func (self *Miner) update() {
	// 注册下载开始事件，下载结束事件，下载失败事件。
	events := self.mux.Subscribe(downloader.StartEvent{}, downloader.DoneEvent{}, downloader.FailedEvent{})
out:
	for ev := range events.Chan() {
		switch ev.Data.(type) {
		case downloader.StartEvent:
			atomic.StoreInt32(&self.canStart, 0)
			if self.Mining() { // 开始下载对应Miner操作Mining
				self.Stop()
				atomic.StoreInt32(&self.shouldStart, 1)
				log.Info("Mining aborted due to sync")
			}
		case downloader.DoneEvent, downloader.FailedEvent: // 下载完成和失败都走相同的分支
			shouldStart := atomic.LoadInt32(&self.shouldStart) == 1

			atomic.StoreInt32(&self.canStart, 1)
			atomic.StoreInt32(&self.shouldStart, 0)
			if shouldStart {
				self.Start(self.coinbase)
			}
			// unsubscribe. we're only interested in this event once
			// 处理完以后要取消订阅
			events.Unsubscribe()
			// stop immediately and ignore all further pending events
			//立即停止并忽略所有进一步的未决事件
			break out
		}
	}
}

//开始挖矿,它是属于Miner指针实例的方法，首字母大写代表可以被外部所访问，传入一个地址。
func (self *Miner) Start(coinbase common.Address) {
	atomic.StoreInt32(&self.shouldStart, 1) //shouldStart 是是否应该启动
	self.SetEtherbase(coinbase)

	if atomic.LoadInt32(&self.canStart) == 0 { //canStart是否能够启动，
		log.Info("Network syncing, will start miner afterwards")
		return
	}
	atomic.StoreInt32(&self.mining, 1)

	log.Info("Starting mining operation")
	self.worker.start()         // 启动worker 开始挖矿
	self.worker.commitNewWork() //提交新的挖矿任务。
}

//停止挖矿
func (self *Miner) Stop() {
	self.worker.stop()
	atomic.StoreInt32(&self.mining, 0)
	atomic.StoreInt32(&self.shouldStart, 0)
}

//注册agent
func (self *Miner) Register(agent Agent) {
	if self.Mining() {
		agent.Start()
	}
	self.worker.register(agent)
}

func (self *Miner) Unregister(agent Agent) {
	self.worker.unregister(agent)
}

// 如果miner的mining属性大于1即返回ture，说明正在挖矿中。
func (self *Miner) Mining() bool {
	return atomic.LoadInt32(&self.mining) > 0
}

func (self *Miner) HashRate() (tot int64) {
	if pow, ok := self.engine.(consensus.PoW); ok {
		tot += int64(pow.Hashrate())
	}
	// do we care this might race? is it worth we're rewriting some
	// aspects of the worker/locking up agents so we can get an accurate
	// hashrate?
	for agent := range self.worker.agents {
		if _, ok := agent.(*CpuAgent); !ok {
			tot += agent.GetHashRate()
		}
	}
	return
}

//设置额外数据
func (self *Miner) SetExtra(extra []byte) error {
	if uint64(len(extra)) > params.MaximumExtraDataSize {
		return fmt.Errorf("Extra exceeds max length. %d > %v", len(extra), params.MaximumExtraDataSize)
	}
	self.worker.setExtra(extra)
	return nil
}

//挂起返回当前挂起的块和关联状态。
// Pending returns the currently pending block and associated state.
func (self *Miner) Pending() (*types.Block, *state.StateDB) {
	return self.worker.pending()
}

// PendingBlock returns the currently pending block.
//
// Note, to access both the pending block and the pending state
// simultaneously, please use Pending(), as the pending state can
// change between multiple method calls
// PendingBlock返回当前挂起的块。
//
//注意，要访问待处理块和待处理状态
//同时，请使用Pending（）作为挂起状态
//在多个方法调用之间切换
func (self *Miner) PendingBlock() *types.Block {
	return self.worker.pendingBlock()
}

//设置挖矿者地址
func (self *Miner) SetEtherbase(addr common.Address) {
	self.coinbase = addr
	self.worker.setEtherbase(addr)
}
