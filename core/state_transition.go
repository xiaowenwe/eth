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

package core

import (
	"errors"
	"math"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/params"
)

var (
	errInsufficientBalanceForGas = errors.New("insufficient balance to pay for gas")
)

/*
The State Transitioning Model
状态转换模型
A state transition is a change made when a transaction is applied to the current world state
状态转换 是指用当前的world state来执行交易，并改变当前的world state
The state transitioning model does all all the necessary work to work out a valid new state root.
状态转换做了所有所需的工作来产生一个新的有效的state root
1) Nonce handling  Nonce 处理
2) Pre pay gas     预先支付Gas
3) Create a new state object if the recipient is \0*32 如果接收人是空，那么创建一个新的state object
4) Value transfer  转账
== If contract creation ==
  4a) Attempt to run transaction data 尝试运行输入的数据
  4b) If valid, use result as code for the new state object 如果有效，那么用运行的结果作为新的state object的code
== end ==
5) Run Script section 运行脚本部分
6) Derive new state root 导出新的state root
*/
type StateTransition struct {
	gp         *GasPool   //用来追踪区块内部的Gas的使用情况
	msg        Message		// Message Call
	gas        uint64//gas表示即时可用Gas数量，初始值均为0
	gasPrice   *big.Int		// gas的价格
	initialGas uint64	// 最开始的gas初始值均为0
	value      *big.Int		// 转账的值
	data       []byte		// 输入数据
	state      vm.StateDB	// StateDB
	evm        *vm.EVM		// 虚拟机
}

// Message represents a message sent to a contract.
type Message interface {
	From() common.Address
	//FromFrontier() (common.Address, error)
	To() *common.Address

	GasPrice() *big.Int
	Gas() uint64
	Value() *big.Int

	Nonce() uint64
	CheckNonce() bool
	Data() []byte
}
//关于g0的计算，在黄皮书上由详细的介绍 和黄皮书有一定出入的部分在于if contractCreation && homestead
// {igas.SetUint64(params.TxGasContractCreation) 这是因为 Gtxcreate+Gtransaction = TxGasContractCreation
// IntrinsicGas computes the 'intrinsic gas' for a message with the given data.
func IntrinsicGas(data []byte, contractCreation, homestead bool) (uint64, error) {
	// Set the starting gas for the raw transaction
	var gas uint64
	if contractCreation && homestead {
		gas = params.TxGasContractCreation
	} else {
		gas = params.TxGas
	}
	//通过交易数据量来提取所需的气体
	// Bump the required gas by the amount of transactional data
	if len(data) > 0 {
		// Zero and non-zero bytes are priced differently
		//零和非零字节的价格不同
		var nz uint64
		for _, byt := range data {
			if byt != 0 {
				nz++
			}
		}
		// Make sure we don't exceed uint64 for all data combinations
		//确保所有数据组合都不超过uint64
		if (math.MaxUint64-gas)/params.TxDataNonZeroGas < nz {
			return 0, vm.ErrOutOfGas
		}
		gas += nz * params.TxDataNonZeroGas

		z := uint64(len(data)) - nz
		if (math.MaxUint64-gas)/params.TxDataZeroGas < z {
			return 0, vm.ErrOutOfGas
		}
		gas += z * params.TxDataZeroGas
	}
	return gas, nil
}
 // New State Transition initialises and returns a new state transition object.
// NewStateTransition初始化并返回一个新的状态转换对象。
// NewStateTransition initialises and returns a new state transition object.
func NewStateTransition(evm *vm.EVM, msg Message, gp *GasPool) *StateTransition {
	return &StateTransition{
		gp:       gp,
		evm:      evm,
		msg:      msg,
		gasPrice: msg.GasPrice(),
		value:    msg.Value(),
		data:     msg.Data(),
		state:    evm.StateDB,
	}
}

// ApplyMessage computes the new state by applying the given message
// against the old state within the environment.
// ApplyMessage 通过应用给定的Message 和状态来生成新的状态
// ApplyMessage returns the bytes returned by any EVM execution (if it took place),
// the gas used (which includes gas refunds) and an error if it failed. An error always
// indicates a core error meaning that the message would always fail for that particular
// state and would never be accepted within a block.
// ApplyMessage返回由任何EVM执行（如果发生）返回的字节，
// 使用的Gas（包括Gas退款），如果失败则返回错误。 一个错误总是表示一个核心错误，
// 意味着这个消息对于这个特定的状态将总是失败，并且永远不会在一个块中被接受。
func ApplyMessage(evm *vm.EVM, msg Message, gp *GasPool) ([]byte, uint64, bool, error) {
	return NewStateTransition(evm, msg, gp).TransitionDb()
}

func (st *StateTransition) from() vm.AccountRef {
	f := st.msg.From()
	if !st.state.Exist(f) {
		st.state.CreateAccount(f)
	}
	return vm.AccountRef(f)
}

func (st *StateTransition) to() vm.AccountRef {
	if st.msg == nil {
		return vm.AccountRef{}
	}
	to := st.msg.To()
	if to == nil {
		return vm.AccountRef{} // contract creation
	}

	reference := vm.AccountRef(*to)
	if !st.state.Exist(*to) {
		st.state.CreateAccount(*to)
	}
	return reference
}

func (st *StateTransition) useGas(amount uint64) error {
	if st.gas < amount {
		return vm.ErrOutOfGas
	}
	st.gas -= amount

	return nil
}
//buyGas， 实现Gas的预扣费， 首先就扣除你的GasLimit * GasPrice的钱。 然后根据计算完的状态在退还一部分。
func (st *StateTransition) buyGas() error {
	var (
		state  = st.state
		sender = st.from()
	)
	mgval := new(big.Int).Mul(new(big.Int).SetUint64(st.msg.Gas()), st.gasPrice)
	if state.GetBalance(sender.Address()).Cmp(mgval) < 0 {
		return errInsufficientBalanceForGas
	}
	if err := st.gp.SubGas(st.msg.Gas()); err != nil {// 从区块的gaspool里面减去， 因为区块是由GasLimit限制整个区块的Gas使用的。
		return err
	}
	st.gas += st.msg.Gas()

	st.initialGas = st.msg.Gas()

	state.SubBalance(sender.Address(), mgval)
	// 从账号里面减去 GasLimit * GasPrice
	return nil
}
//执行前的检查
func (st *StateTransition) preCheck() error {
	msg := st.msg
	sender := st.from()

	// Make sure this transaction's nonce is correct确保此事务的nonce是正确的
	if msg.CheckNonce() {
		nonce := st.state.GetNonce(sender.Address())
		// 当前本地的nonce 需要和 msg的Nonce一样 不然就是状态不同步了。
		if nonce < msg.Nonce() {
			return ErrNonceTooHigh
		} else if nonce > msg.Nonce() {
			return ErrNonceTooLow
		}
	}
	return st.buyGas()
}
// TransitionDb将通过应用当前消息并返回包含使用过的气体的结果来转换状态。 如果失败，它会返回一个错误。 错误表示共识问题。
// TransitionDb will transition the state by applying the current message and
// returning the result including the the used gas. It returns an error if it
// failed. An error indicates a consensus issue.
func (st *StateTransition) TransitionDb() (ret []byte, usedGas uint64, failed bool, err error) {
	if err = st.preCheck(); err != nil {//购买Gas。首先从交易的(转帐)转出方账户扣除一笔Ether，费用等于tx.data.GasLimit * tx.data.Price；
	// 同时 st.initialGas = st.gas = tx.data.GasLimit；然后(GasPool) gp -= st.gas。
		return
	}
	msg := st.msg
	sender := st.from() // err checked in preCheck在检查前检查错误

	homestead := st.evm.ChainConfig().IsHomestead(st.evm.BlockNumber)
	contractCreation := msg.To() == nil // 如果msg.To是nil 那么认为是一个合约创建
	// 计算最开始的Gas  g0
	// Pay intrinsic gas
	//计算tx的固有Gas消耗 - intrinsicGas。它分为两个部分，每一个tx预设的消耗量，这个消耗量还因tx是否含有(转帐)转入方地址而略有不同；
	// 以及针对tx.data.Payload的Gas消耗，Payload类型是[]byte，关于它的固有消耗依赖于[]byte中非0字节和0字节的长度。
	// 最终，st.gas -= intrinsicGas
	gas, err := IntrinsicGas(st.data, contractCreation, homestead)
	if err != nil {
		return nil, 0, false, err
	}
	if err = st.useGas(gas); err != nil {
		return nil, 0, false, err
	}

	var (
		evm = st.evm
		// vm错误不会影响一致性，因此不会分配给err，除非平衡错误不足。
		// vm errors do not effect consensus and are therefor
		// not assigned to err, except for insufficient balance
		// error.
		vmerr error
	)
	//EVM执行。如果交易的(转帐)转入方地址(tx.data.Recipient)为空，调用EVM的Create()函数；
	// 否则，调用Call()函数。无论哪个函数返回后，更新st.gas。
	if contractCreation { //如果是合约创建， 那么调用evm的Create方法
		ret, _, st.gas, vmerr = evm.Create(sender, st.data, st.gas, st.value)
	} else {
		// 如果是方法调用。那么首先设置sender的nonce。
		// Increment the nonce for the next transaction
		st.state.SetNonce(sender.Address(), st.state.GetNonce(sender.Address())+1)
		ret, st.gas, vmerr = evm.Call(sender, st.to().Address(), st.data, st.gas, st.value)
	}
	if vmerr != nil {
		log.Debug("VM returned with error", "err", vmerr)
		//唯一可能的共识错误是如果没有足够的余额来完成转移， 第一笔余额转账可能永远不会失败。
		// The only possible consensus-error would be if there wasn't
		// sufficient balance to make the transfer happen. The first
		// balance transfer may never fail.
		if vmerr == vm.ErrInsufficientBalance {
			return nil, 0, false, vmerr
		}
	}
	//new(big.Int).Set(st.gasUsed())  计算被使用的Gas数量
	//4计算本次执行交易的实际Gas消耗： requiredGas = st.initialGas - st.gas
	//偿退Gas。它包括两个部分：首先将剩余st.gas 折算成Ether，归还给交易的(转帐)转出方账户；然后，基于实际消耗量requiredGas，
	// 系统提供一定的补偿，数量为refundGas。refundGas 所折算的Ether会被立即加在(转帐)转出方账户上，
	// 同时st.gas += refundGas，gp += st.gas，即剩余的Gas加上系统补偿的Gas，被一起归并进GasPool，供之后的交易执行使用。
	st.refundGas()//计算Gas的退费 会增加到 st.gas上面。 所以矿工拿到的是退税后的
//6奖励所属区块的挖掘者：系统给所属区块的作者，亦即挖掘者账户，增加一笔金额，数额等于 st.data,Price * (st.initialGas - st.gas)。
// 注意，这里的st.gas在步骤5中被加上了refundGas, 所以这笔奖励金所对应的Gas，其数量小于该交易实际消耗量requiredGas。
	st.state.AddBalance(st.evm.Coinbase, new(big.Int).Mul(new(big.Int).SetUint64(st.gasUsed()), st.gasPrice))
	//st.state.AddBalance(st.evm.Coinbase, new(big.Int).Mul(st.gasUsed(), st.gasPrice)) // 给矿工增加收入。
	// requiredGas和gasUsed的区别一个是没有退税的， 一个是退税了的。
	// 看上面的调用 ApplyMessage直接丢弃了requiredGas, 说明返回的是退税了的。
	return ret, st.gasUsed(), vmerr != nil, err
}
//退税，退税是为了奖励大家运行一些能够减轻区块链负担的指令， 比如清空账户的storage. 或者是运行suicide命令来清空账号。
func (st *StateTransition) refundGas() {
	//退税的总金额不会超过用户Gas总使用的1/2。
	// Apply refund counter, capped to half of the used gas.
	refund := st.gasUsed() / 2
	if refund > st.state.GetRefund() {
		refund = st.state.GetRefund()
	}
	st.gas += refund
	//返回剩余气体的ETH，以原始速率交换。
	// Return ETH for remaining gas, exchanged at the original rate.
	sender := st.from()

	remaining := new(big.Int).Mul(new(big.Int).SetUint64(st.gas), st.gasPrice)
	// 用户还剩下的Gas还回去。
	st.state.AddBalance(sender.Address(), remaining)

	// Also return remaining gas to the block gas counter so it is
	// available for the next transaction.
	// 同时也把退税的钱还给gaspool给下个交易腾点Gas空间。
	st.gp.AddGas(st.gas)
}
// gasUsed返回状态转换所用的气体量。
// gasUsed returns the amount of gas used up by the state transition.
func (st *StateTransition) gasUsed() uint64 {
	return st.initialGas - st.gas
}
