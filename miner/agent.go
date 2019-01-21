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
	"sync"

	"sync/atomic"

	"github.com/ethereum/go-ethereum/consensus"
	"github.com/ethereum/go-ethereum/log"
)

//agent 是具体执行挖矿的对象。 它执行的流程就是，接受计算好了的区块头， 计算mixhash和nonce， 把挖矿好的区块头返回。
//构造CpuAgent, 一般情况下不会使用CPU来进行挖矿，一般来说挖矿都是使用的专门的GPU进行挖矿， GPU挖矿的代码不会在这里体现。
type CpuAgent struct {
	mu sync.Mutex

	workCh        chan *Work    //接受挖矿任务的通道
	stop          chan struct{} // 结构体通道对象
	quitCurrentOp chan struct{}
	returnCh      chan<- *Result // 挖矿完成后的返回channel

	chain  consensus.ChainReader // 获取区块链的信息
	engine consensus.Engine      // 一致性引擎，这里指的是Pow引擎

	isMining int32 // isMining indicates whether the agent is currently mining是“挖掘”指示代理当前是否正在进行挖掘。
}

func NewCpuAgent(chain consensus.ChainReader, engine consensus.Engine) *CpuAgent {
	miner := &CpuAgent{
		chain:  chain,
		engine: engine,
		stop:   make(chan struct{}, 1),
		workCh: make(chan *Work, 1),
	}
	return miner
}

//设置返回值channel和得到Work的channel， 方便外界传值和得到返回信息。
func (self *CpuAgent) Work() chan<- *Work            { return self.workCh }
func (self *CpuAgent) SetReturnCh(ch chan<- *Result) { self.returnCh = ch }

func (self *CpuAgent) Stop() {
	if !atomic.CompareAndSwapInt32(&self.isMining, 1, 0) {
		return // agent already stopped代理已经停止
	}
	self.stop <- struct{}{}
done:
	// Empty work channel
	for {
		select {
		case <-self.workCh:
		default:
			break done
		}
	}
}

//启动和消息循环，如果已经启动挖矿，那么直接退出，
// 否则启动update 这个goroutine update 从workCh接受任务，进行挖矿，或者是接受退出信息，退出。
func (self *CpuAgent) Start() {
	if !atomic.CompareAndSwapInt32(&self.isMining, 0, 1) {
		return // agent already started
	}
	go self.update()
}

//接收chan调用挖矿
func (self *CpuAgent) update() {
out:
	for {
		select {
		case work := <-self.workCh:
			self.mu.Lock()
			if self.quitCurrentOp != nil {
				close(self.quitCurrentOp)
			}
			self.quitCurrentOp = make(chan struct{})
			go self.mine(work, self.quitCurrentOp)
			self.mu.Unlock()
		case <-self.stop: // 跳出for循环，for循环不断监听self信号，当监测到self停止时，则调用关闭操作代码，并直接挑出循环监听，函数退出。
			self.mu.Lock()
			if self.quitCurrentOp != nil {
				close(self.quitCurrentOp)
				self.quitCurrentOp = nil
			}
			self.mu.Unlock()
			break out
		}
	}
}

//mine, 挖矿，调用一致性引擎进行挖矿， 如果挖矿成功，把消息发送到returnCh上面。
func (self *CpuAgent) mine(work *Work, stop <-chan struct{}) {
	if result, err := self.engine.Seal(self.chain, work.Block, stop); result != nil {
		log.Info("Successfully sealed new block", "number", result.Number(), "hash", result.Hash())
		self.returnCh <- &Result{work, result}
	} else {
		if err != nil {
			log.Warn("Block sealing failed", "err", err)
		}
		self.returnCh <- nil
	}
}

// 这个函数返回当前的HashRate。
func (self *CpuAgent) GetHashRate() int64 {
	if pow, ok := self.engine.(consensus.PoW); ok {
		return int64(pow.Hashrate())
	}
	return 0
}
