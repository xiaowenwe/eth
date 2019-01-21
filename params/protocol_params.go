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

package params

import "math/big"

var (
	TargetGasLimit uint64 = GenesisGasLimit // The artificial target人工目标
)

const (
	GasLimitBoundDivisor uint64 = 1024    // The bound divisor of the gas limit, used in update calculations.气体极限的界因子，用于更新计算。
	MinGasLimit          uint64 = 5000    // Minimum the gas limit may ever be.气体极限可能是最小的。
	GenesisGasLimit      uint64 = 4712388 // Gas limit of the Genesis block.创世纪地块的气体极限。

	MaximumExtraDataSize  uint64 = 32    // Maximum size extra data may be after Genesis.最大尺寸的额外数据可能在创世纪之后
	ExpByteGas            uint64 = 10    // Times ceil(log256(exponent)) for the EXP instruction.//exp指令的倍(log 256(指数)。
	SloadGas              uint64 = 50    // Multiplied by the number of 32-byte words that are copied (round up) for any *COPY operation and added.乘以对任何*复制操作和添加的复制(整)的32字节字数。
	CallValueTransferGas  uint64 = 9000  // Paid for CALL when the value transfer is non-zero.当价值转移为非零时，为呼叫付费。
	CallNewAccountGas     uint64 = 25000 // Paid for CALL when the destination address didn't exist prior.当目标地址之前不存在时，已付费。
	TxGas                 uint64 = 21000 // Per transaction not creating a contract. NOTE: Not payable on data of calls between transactions.没有创建合同的交易。注：交易之间的通话数据不支付。
	TxGasContractCreation uint64 = 53000 // Per transaction that creates a contract. NOTE: Not payable on data of calls between transactions.//每个创建契约的事务。注：交易之间的通话数据不支付。
	TxDataZeroGas         uint64 = 4     // Per byte of data attached to a transaction that equals zero. NOTE: Not payable on data of calls between transactions.附加到等于零的事务的数据的每字节。注：交易之间的通话数据不支付。
	QuadCoeffDiv          uint64 = 512   // Divisor for the quadratic particle of the memory cost equation.内存成本方程的二次粒子除数。
	SstoreSetGas          uint64 = 20000 // Once per SLOAD operation.每次SLOAD操作一次。
	LogDataGas            uint64 = 8     // Per byte in a LOG* operation's data.日志*操作的数据中的每一个字节。
	CallStipend           uint64 = 2300  // Free gas given at beginning of call.呼叫开始时提供的免费汽油。

	Sha3Gas          uint64 = 30    // Once per SHA3 operation.每次SHA 3操作一次。
	Sha3WordGas      uint64 = 6     // Once per word of the SHA3 operation's data.每字一次的SHA 3操作的数据。
	SstoreResetGas   uint64 = 5000  // Once per SSTORE operation if the zeroness changes from zero.如果零值从零变为零，则每次SSTORE操作一次。
	SstoreClearGas   uint64 = 5000  // Once per SSTORE operation if the zeroness doesn't change.如果零值不变，则每次SSTORE操作一次。
	SstoreRefundGas  uint64 = 15000 // Once per SSTORE operation if the zeroness changes to zero.如果零值变为零，则每次SSTORE操作一次
	JumpdestGas      uint64 = 1     // Refunded gas, once per SSTORE operation if the zeroness changes to zero.如果SARNONESS改变为零，则退回气体，每StAFT操作一次。
	EpochDuration    uint64 = 30000 // Duration between proof-of-work epochs.工作证明时期之间的持续时间。
	CallGas          uint64 = 40    // Once per CALL operation & message call transaction.每个呼叫操作和消息呼叫事务处理一次。
	CreateDataGas    uint64 = 200   //
	CallCreateDepth  uint64 = 1024  // Maximum depth of call/create stack.调用/创建堆栈的最大深度。
	ExpGas           uint64 = 10    // Once per EXP instruction每次执行指令一次
	LogGas           uint64 = 375   // Per LOG* operation.每个日志*操作。
	CopyGas          uint64 = 3     //
	StackLimit       uint64 = 1024  // Maximum size of VM stack allowed.允许VM堆栈的最大大小。
	TierStepGas      uint64 = 0     // Once per operation, for a selection of them.每次操作一次，选择一次。
	LogTopicGas      uint64 = 375   // Multiplied by the * of the LOG*, per LOG transaction. e.g. LOG0 incurs 0 * c_txLogTopicGas, LOG4 incurs 4 * c_txLogTopicGas.乘以日志*的*，每个日志事务。例如，LOG 0产生0*c_txLogTopicgas，LOG 4产生4*c_txLogTopicgas。
	CreateGas        uint64 = 32000 // Once per CREATE operation & contract-creation transaction.每个创建操作和合同创建交易一次。
	SuicideRefundGas uint64 = 24000 // Refunded following a suicide operation.自杀手术后退款。
	MemoryGas        uint64 = 3     // Times the address of the (highest referenced byte in memory + 1). NOTE: referencing happens on read, write and in instructions such as RETURN and CALL.乘以(内存1中引用的最高字节)的地址。注意：引用发生在读、写和指令中，例如返回和调用。
	TxDataNonZeroGas uint64 = 68    // Per byte of data attached to a transaction that is not equal to zero. NOTE: Not payable on data of calls between transactions.附加到不等于零的交易的数据的每字节。注：交易之间的通话数据不支付。

	MaxCodeSize = 24576 // Maximum bytecode to permit for a contract允许签订合同的最大字节码

	// Precompiled contract gas prices预编合同燃气价格

	EcrecoverGas            uint64 = 3000   // Elliptic curve sender recovery gas price椭圆曲线发送器回收煤气价格
	Sha256BaseGas           uint64 = 60     // Base price for a SHA256 operation//SHA 256操作的基本价格
	Sha256PerWordGas        uint64 = 12     // Per-word price for a SHA256 operation//SHA 256操作的单字价格
	Ripemd160BaseGas        uint64 = 600    // Base price for a RIPEMD160 operation//RIPEMD 160操作的基本价格
	Ripemd160PerWordGas     uint64 = 120    // Per-word price for a RIPEMD160 operationRIPEMD 160操作的单字价格
	IdentityBaseGas         uint64 = 15     // Base price for a data copy operation数据复制操作的基本价格
	IdentityPerWordGas      uint64 = 3      // Per-work price for a data copy operation数据复制操作的每个工作价格
	ModExpQuadCoeffDiv      uint64 = 20     // Divisor for the quadratic particle of the big int modular exponentiation大整数模指数二次粒子的除数
	Bn256AddGas             uint64 = 500    // Gas needed for an elliptic curve addition椭圆曲线加法所需的气体
	Bn256ScalarMulGas       uint64 = 40000  // Gas needed for an elliptic curve scalar multiplication椭圆曲线标量乘法所需的气体
	Bn256PairingBaseGas     uint64 = 100000 // Base price for an elliptic curve pairing check椭圆曲线配对检验的基本价格
	Bn256PairingPerPointGas uint64 = 80000  // Per-point price for an elliptic curve pairing check椭圆曲线配对检验的每点价格
)

var (
	DifficultyBoundDivisor = big.NewInt(2048)   // The bound divisor of the difficulty, used in the update calculations.困难的界因子，用于更新计算。
	GenesisDifficulty      = big.NewInt(131072) // Difficulty of the Genesis block.创世纪地块的困难。
	MinimumDifficulty      = big.NewInt(131072) // The minimum that the difficulty may ever be.难度可能是最小的。
	DurationLimit          = big.NewInt(13)     // The decision boundary on the blocktime duration used to determine whether difficulty should go up or not.阻塞时间的判定边界，用于确定困难是否应该上升。
)
