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

// Package consensus implements different Ethereum consensus engines.
package consensus

import (
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/rpc"
	"math/big"
)

// ChainReader defines a small collection of methods needed to access the local
// blockchain during header and/or uncle verification.
// ChainReader定义了在标头和/或叔叔验证期间访问本地区块链所需的一小组方法。
type ChainReader interface {
	// Config retrieves the blockchain's chain configuration.
	//置检索区块链的链配置配。
	Config() *params.ChainConfig

	// CurrentHeader retrieves the current header from the local chain.
	// CurrentHeader从本地链中检索当前标题。
	CurrentHeader() *types.Header

	// GetHeader retrieves a block header from the database by hash and number.
	// GetHeader通过散列和数字从数据库中检索块标题。
	GetHeader(hash common.Hash, number uint64) *types.Header

	// GetHeaderByNumber retrieves a block header from the database by number.
	// GetHeaderByNumber按数字从数据库中检索块标题。
	GetHeaderByNumber(number uint64) *types.Header

	// GetHeaderByHash retrieves a block header from the database by its hash.
	// GetHeaderByHash通过它的散列从数据库中检索块标题。
	GetHeaderByHash(hash common.Hash) *types.Header

	// GetBlock retrieves a block from the database by hash and number.
	// GetBlock通过散列和数字从数据库中检索一个块。
	GetBlock(hash common.Hash, number uint64) *types.Block
}

// Engine is an algorithm agnostic consensus engine.
//引擎是一种算法不可知的共识引擎。
type Engine interface {
	// Author retrieves the Ethereum address of the account that minted the given
	// block, which may be different from the header's coinbase if a consensus
	// engine is based on signatures.
	//作者检索铸造给定块的帐户的以太坊地址，如果共识引擎基于签名，则该地址可能与标头的coinbase不同。
	Author(header *types.Header) (common.Address, error)

	// VerifyHeader checks whether a header conforms to the consensus rules of a
	// given engine. Verifying the seal may be done optionally here, or explicitly
	// via the VerifySeal method.
	//验证头检查头是否符合给定引擎的共识规则。 在此可以选择验证密封，也可以通过Verify Real方法明确地进行验证。
	VerifyHeader(chain ChainReader, header *types.Header, seal bool) error

	// VerifyHeaders is similar to VerifyHeader, but verifies a batch of headers
	// concurrently. The method returns a quit channel to abort the operations and
	// a results channel to retrieve the async verifications (the order is that of
	// the input slice).
	// VerifyHeaders与VerifyHeader类似，但同时验证一批标题。 该方法返回一个退出通道以中止操作，
	// 并返回一个结果通道以检索异步验证（顺序是输入片的顺序）。
	VerifyHeaders(chain ChainReader, headers []*types.Header, seals []bool) (chan<- struct{}, <-chan error)

	// VerifyUncles verifies that the given block's uncles conform to the consensus
	// rules of a given engine.
	// VerifyUncles验证给定块的叔叔是否符合给定引擎的共识规则。
	VerifyUncles(chain ChainReader, block *types.Block) error

	// VerifySeal checks whether the crypto seal on a header is valid according to
	// the consensus rules of the given engine.
	// VerifySeal根据给定引擎的共识规则检查头部上的加密封印是否有效。
	//VerifySeal()函数基于跟Seal()完全一样的算法原理，通过验证区块的某些属性(Header.Nonce，Header.MixDigest等)是否正确，来确定该区块是否已经经过Seal操作
	VerifySeal(chain ChainReader, header *types.Header) error

	// Prepare initializes the consensus fields of a block header according to the
	// rules of a particular engine. The changes are executed inline.
	//准备根据特定引擎的规则初始化块头的共识字段。 这些更改以内联方式执行。
	//Prepare()函数往往在Header创建时调用，用来对Header.Difficulty等属性赋值
	Prepare(chain ChainReader, header *types.Header) error

	// Finalize runs any post-transaction state modifications (e.g. block rewards)
	// and assembles the final block.
	// Note: The block header and state database might be updated to reflect any
	// consensus rules that happen at finalization (e.g. block rewards).
	// Finalize运行任何事务后状态修改（例如块奖励）并组装最终块。
	//注意：块头和状态数据库可能会更新以反映任何
	//最终确定时发生的共识规则（例如，阻止奖励）。
	//Finalize()会最终生成Root，TxHash，UncleHash，ReceiptHash等成员。
	Finalize(chain ChainReader, header *types.Header, state *state.StateDB, txs []*types.Transaction,
		uncles []*types.Header, receipts []*types.Receipt) (*types.Block, error)

	// Seal generates a new block for the given input block with the local miner's
	// seal place on top.// Seal为当前矿工密封位置顶部的给定输入块生成一个新块。
	//Seal()函数可对一个调用过Finalize()的区块进行授权或封印，并将封印过程产生的一些值赋予区块中剩余尚未赋值的成员
	// (Header.Nonce, Header.MixDigest)。Seal()成功时返回的区块全部成员齐整，可视为一个正常区块，可被广播到整个网络中，也可以被插入区块链等。
	// 所以，对于挖掘一个新区块来说，所有相关代码里Engine.Seal()是其中最重要，也是最复杂的一步。
	Seal(chain ChainReader, block *types.Block, stop <-chan struct{}) (*types.Block, error)

	// CalcDifficulty is the difficulty adjustment algorithm. It returns the difficulty
	// that a new block should have.是难度调整算法。 它返回了新块应该具有的难度。
	CalcDifficulty(chain ChainReader, time uint64, parent *types.Header) *big.Int

	// APIs returns the RPC APIs this consensus engine provides.
	// API返回这个共识引擎提供的RPC API。
	APIs(chain ChainReader) []rpc.API
}

// PoW is a consensus engine based on proof-of-work.PoW是基于工作证明的共识引擎。
type PoW interface {
	Engine

	// Hashrate returns the current mining hashrate of a PoW consensus engine.
	//Hashrate返回PoW共识引擎的当前挖掘哈希率。
	Hashrate() float64
}
