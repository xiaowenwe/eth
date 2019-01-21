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

package vm

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common"
)

//contract 代表了以太坊 state database里面的一个合约。包含了合约代码，调用参数。
// ContractRef is a reference to the contract's backing object
// ContractRef是对合同支持对象的引用
type ContractRef interface {
	Address() common.Address
}

// AccountRef实现ContractRef。
// EVM初始化期间使用帐户引用，主要用于获取地址。 删除这个对象证明是困难的，因为缓存的跳转目的地
//是从作为ContractRef的父合同（即调用者）获取的。
// AccountRef implements ContractRef.
//
// Account references are used during EVM initialisation and
// it's primary use is to fetch addresses. Removing this object
// proves difficult because of the cached jump destinations which
// are fetched from the parent contract (i.e. the caller), which
// is a ContractRef.
type AccountRef common.Address

// Address将AccountRef转换为地址
// Address casts AccountRef to a Address
func (ar AccountRef) Address() common.Address { return (common.Address)(ar) }

//合约代表状态数据库中的以太坊合约。 它包含合同代码，调用参数。 合同实施ContractRef
// Contract represents an ethereum contract in the state database. It contains
// the the contract code, calling arguments. Contract implements ContractRef
type Contract struct {
	// CallerAddress是初始化此合同的调用者的结果。 但是，当“调用方法”被委托时，该值需要初始化为调用者的调用者的值。
	// CallerAddress is the result of the caller which initialised this
	// contract. However when the "call method" is delegated this value
	// needs to be initialised to that of the caller's caller.
	// CallerAddress是初始化这个合约的人。 如果是delegate，这个值被设置为调用者的调用者。
	CallerAddress common.Address
	caller        ContractRef //caller是转帐转出方地址(账户)
	self          ContractRef //self是转入方地址

	jumpdests destinations // result of JUMPDEST analysis. JUMPDEST指令的分析

	Code     []byte          //代码，指令数组，其中每一个byte都对应于一个预定义的虚拟机指令
	CodeHash common.Hash     //代码的HASH，Code的RLP哈希值
	CodeAddr *common.Address //代码地址
	Input    []byte          // 入参，数据数组，是指令所操作的数据集合

	Gas   uint64 // 合约还有多少Gas
	value *big.Int

	Args []byte //是参数

	DelegateCall bool
}

// NewContract返回执行EVM的新合约环境將剩餘的gas给合约，转账金额告诉合约
// NewContract returns a new contract environment for the execution of EVM.
func NewContract(caller ContractRef, object ContractRef, value *big.Int, gas uint64) *Contract {
	c := &Contract{CallerAddress: caller.Address(), caller: caller, self: object, Args: nil}

	if parent, ok := caller.(*Contract); ok {
		// 如果 caller 是一个合约，说明是合约调用了我们。 jumpdests设置为caller的jumpdests
		// Reuse JUMPDEST analysis from parent context if available.
		c.jumpdests = parent.jumpdests
	} else {
		c.jumpdests = make(destinations)
	}
	//气体应该是指针，因此可以通过运行安全地减少
	//此指针将关闭状态转换
	// Gas should be a pointer so it can safely be reduced through the run
	// This pointer will be off the state transition
	c.Gas = gas
	// ensures a value is set
	c.value = value

	return c
}

//AsDelegate将合约设置为委托调用并返回当前合同（用于链式调用）
// AsDelegate sets the contract to be a delegate call and returns the current
// contract (for chaining calls)
func (c *Contract) AsDelegate() *Contract {
	c.DelegateCall = true
	// NOTE: caller must, at all times be a contract. It should never happen
	// that caller is something other than a Contract.
	parent := c.caller.(*Contract)
	c.CallerAddress = parent.CallerAddress
	c.value = parent.value

	return c
}

//GetOp 用来获取下一跳指令
// GetOp returns the n'th element in the contract's byte array
func (c *Contract) GetOp(n uint64) OpCode {
	return OpCode(c.GetByte(n))
}

// GetByte returns the n'th byte in the contract's byte array
func (c *Contract) GetByte(n uint64) byte {
	if n < uint64(len(c.Code)) {
		return c.Code[n]
	}

	return 0
}

///呼叫者返回合同的呼叫者。
//调用方在合同为委托时将递归地调用调用方。
//呼叫，包括呼叫者的呼叫者。

// Caller returns the caller of the contract.
//
// Caller will recursively call caller when the contract is a delegate
// call, including that of caller's caller.
func (c *Contract) Caller() common.Address {
	return c.CallerAddress
}

//UseGas使用Gas。
// UseGas attempts the use gas and subtracts it and returns true on success
func (c *Contract) UseGas(gas uint64) (ok bool) {
	if c.Gas < gas {
		return false
	}
	c.Gas -= gas
	return true
}

//当Contract对象作为一个ContractRef接口出现时，它返回的地址就是它的self地址
// Address returns the contracts address
func (c *Contract) Address() common.Address {
	return c.self.Address()
}

// Value returns the contracts value (sent to it from it's caller)
func (c *Contract) Value() *big.Int {
	return c.value
}

//SetCode ，SetCallCode 设置代码。
// SetCode sets the code to the contract
func (self *Contract) SetCode(hash common.Hash, code []byte) {
	self.Code = code
	self.CodeHash = hash
}

// SetCallCode sets the code of the contract and address of the backing data
// object
func (self *Contract) SetCallCode(addr *common.Address, hash common.Hash, code []byte) {
	self.Code = code
	self.CodeHash = hash
	self.CodeAddr = addr
}
