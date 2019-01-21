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

package core

import (
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus"
	"github.com/ethereum/go-ethereum/consensus/misc"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/params"
)
//执行tx的入口函数是StateProcessor的Process()函数
// StateProcessor is a basic Processor, which takes care of transitioning
// state from one point to another.
//StateTransition是用来处理一个一个的交易的。那么StateProcessor就是用来处理区块级别的交易的。
// StateProcessor implements Processor.
type StateProcessor struct {
	config *params.ChainConfig // Chain configuration options
	bc     *BlockChain         // Canonical block chain
	engine consensus.Engine    // Consensus engine used for block rewards
}
// NewStateProcessor初始化一个新的StateProcessor。
// NewStateProcessor initialises a new StateProcessor.
func NewStateProcessor(config *params.ChainConfig, bc *BlockChain, engine consensus.Engine) *StateProcessor {
	return &StateProcessor{
		config: config,
		bc:     bc,
		engine: engine,
	}
}
//Process，这个方法会被blockchain调用。
// Process processes the state changes according to the Ethereum rules by running
// the transaction messages using the statedb and applying any rewards to both
// the processor (coinbase) and any included uncles.
// Process 根据以太坊规则运行交易信息来对statedb进行状态改变，以及奖励挖矿者或者是其他的叔父节点。
// Process returns the receipts and logs accumulated during the process and
// returns the amount of gas that was used in the process. If any of the
// transactions failed to execute due to insufficient gas it will return an error.
// Process返回执行过程中累计的收据和日志，并返回过程中使用的Gas。 如果由于Gas不足而导致任何交易执行失败，将返回错误。
func (p *StateProcessor) Process(block *types.Block, statedb *state.StateDB, cfg vm.Config) (types.Receipts, []*types.Log, uint64, error) {
	var (
		receipts types.Receipts
		usedGas  = new(uint64)
		header   = block.Header()
		allLogs  []*types.Log
		//GasPool对象是在一个Block执行开始时创建，并在该Block内所有tx的执行过程中共享，对于一个tx的执行可视为“全局”存储对象；
		gp       = new(GasPool).AddGas(block.GasLimit())//GasPool 类型其实就是big.Int。在一个Block的处理过程(即其所有tx的执行过程)中，GasPool 的值能够告诉你，剩下还有多少Gas可以使用。
		// 在每一个tx执行过程中，Ethereum 还设计了偿退(refund)环节，所偿退的Gas数量也会加到这个GasPool里。
	)
	// Mutate the the block and state according to any hard-fork specs
	// DAO 事件的硬分叉处理
	if p.config.DAOForkSupport && p.config.DAOForkBlock != nil && p.config.DAOForkBlock.Cmp(block.Number()) == 0 {
		misc.ApplyDAOHardFork(statedb)
	}
	//Process()函数的核心是一个for循环，它将Block里的所有tx逐个遍历执行。
	// 具体的执行函数叫ApplyTransaction()，它每次执行tx, 会返回一个收据(Receipt)对象。
	// Iterate over and process the individual transactions迭代并处理个人交易
	for i, tx := range block.Transactions() {
		statedb.Prepare(tx.Hash(), block.Hash(), i)//设置transaction hash 和block hash当前的交易的index
		receipt, _, err := ApplyTransaction(p.config, p.bc, nil, gp, statedb, header, tx, usedGas, cfg)
		if err != nil {
			return nil, nil, 0, err
		}
		receipts = append(receipts, receipt)
		allLogs = append(allLogs, receipt.Logs...)
	}//完成块，应用任何引擎特定的额外特性（例如块奖励）
	// Finalize the block, applying any consensus engine specific extras (e.g. block rewards)
	p.engine.Finalize(p.bc, header, statedb, block.Transactions(), block.Uncles(), receipts)
	// 返回收据 日志 总的Gas使用量和nil
	return receipts, allLogs, *usedGas, nil
}
//ApplyTransaction()首先根据输入参数分别封装出一个Message对象和一个EVM对象，然后加上一个传入的GasPool类型变量，由TransitionDb()函数完成tx的执行，
// 待TransitionDb()返回之后，创建一个收据Receipt对象，最后返回该Recetip对象，以及整个tx执行过程所消耗Gas数量。
// ApplyTransaction attempts to apply a transaction to the given state database
// and uses the input parameters for its environment. It returns the receipt
// for the transaction, gas used and an error if the transaction failed,
// indicating the block was invalid.
//ApplyTransaction尝试将交易应用于给定的状态数据库，并使用其环境的输入参数。
//它返回交易的收据，使用的Gas和错误，如果交易失败，表明块是无效的。

func ApplyTransaction(config *params.ChainConfig, bc *BlockChain, author *common.Address, gp *GasPool, statedb *state.StateDB, header *types.Header, tx *types.Transaction, usedGas *uint64, cfg vm.Config) (*types.Receipt, uint64, error) {
	// 把交易转换成Message
	// 这里如何验证消息确实是Sender发送的。 TODO
	// 1Message由此次待执行的tx对象转化而来，并携带了解析出的tx的(转帐)转出方地址，属于待处理的数据对象
	msg, err := tx.AsMessage(types.MakeSigner(config, header.Number))
	if err != nil {
		return nil, 0, err
	}
	// Create a new context to be used in the EVM environment
	// 每一个交易都创建了新的虚拟机环境。
	context := NewEVMContext(msg, header, bc, author)
	//创建一个新的环境，它包含有关事务和调用机制的所有相关信息。
	// Create a new environment which holds all relevant information
	// about the transaction and calling mechanisms.
	//EVM 作为Ethereum世界里的虚拟机(Virtual Machine)，作为此次tx的实际执行者，完成转帐和合约(Contract)的相关操作。
	vmenv := vm.NewEVM(context, statedb, config, cfg)
	//将交易应用到当前状态（包含在env中）
	// Apply the transaction to the current state (included in the env)
	_, gas, failed, err := ApplyMessage(vmenv, msg, gp)
	if err != nil {
		return nil, 0, err
	}
	// Update the state with pending changes
	// 求得中间状态
	var root []byte
	if config.IsByzantium(header.Number) {
		statedb.Finalise(true)
	} else {
		root = statedb.IntermediateRoot(config.IsEIP158(header.Number)).Bytes()
	}
	*usedGas += gas

	// Create a new receipt for the transaction, storing the intermediate root and gas used by the tx
	// based on the eip phase, we're passing wether the root touch-delete accounts.
	// 创建一个收据, 用来存储中间状态的root, 以及交易使用的gas
	receipt := types.NewReceipt(root, failed, *usedGas)
	receipt.TxHash = tx.Hash()
	receipt.GasUsed = gas
	// if the transaction created a contract, store the creation address in the receipt.
	// 如果是创建合约的交易.那么我们把创建地址存储到收据里面.
	if msg.To() == nil {
	receipt.ContractAddress = crypto.CreateAddress(vmenv.Context.Origin, tx.Nonce())
}
	// Set the receipt logs and create a bloom for filtering
	// 拿到所有的日志并创建日志的布隆过滤器.
	receipt.Logs = statedb.GetLogs(tx.Hash())
	receipt.Bloom = types.CreateBloom(types.Receipts{receipt})

	return receipt, gas, err
}
