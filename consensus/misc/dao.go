// Copyright 2016 The go-ethereum Authors
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

package misc

import (
	"bytes"
	"errors"
	"math/big"

	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/params"
)

var (
	// ErrBadProDAOExtra is returned if a header doens't support the DAO fork on a
	// pro-fork client.
	// ErrBadProDAOExtra返回，如果一个头不支持DAO分叉
	// pro-fork客户端。
	ErrBadProDAOExtra = errors.New("bad DAO pro-fork extra-data")
	//如果标题确实支持DAO fork，则返回ErrBadNoDAOExtra叉客户端。
	// ErrBadNoDAOExtra is returned if a header does support the DAO fork on a no-
	// fork client.
	ErrBadNoDAOExtra = errors.New("bad DAO no-fork extra-data")
)

// VerifyDAOHeaderExtraData validates the extra-data field of a block header to
// ensure it conforms to DAO hard-fork rules.
//
// DAO hard-fork extension to the header validity:
//   a) if the node is no-fork, do not accept blocks in the [fork, fork+10) range
//      with the fork specific extra-data set
//   b) if the node is pro-fork, require blocks in the specific range to have the
//      unique extra-data set.
// VerifyDAOHeaderExtraData验证块头的额外数据字段
//确保它符合DAO硬叉规则。
//
// DAO硬分叉扩展到头部有效性：
// a）如果节点不是fork，不要接受[fork，fork + 10）范围内的块
//使用fork特定的额外数据集
// b）如果该节点是pro-fork，则需要在特定范围内的块具有该分支
//独特的额外数据集。
func VerifyDAOHeaderExtraData(config *params.ChainConfig, header *types.Header) error {
	//如果节点不关心DAO fork，则进行短路验证
	// Short circuit validation if the node doesn't care about the DAO fork
	if config.DAOForkBlock == nil {
		return nil
	} //确保块在fork的修改后的额外数据范围内
	// Make sure the block is within the fork's modified extra-data range
	limit := new(big.Int).Add(config.DAOForkBlock, params.DAOForkExtraRange)
	if header.Number.Cmp(config.DAOForkBlock) < 0 || header.Number.Cmp(limit) >= 0 {
		return nil
	} //根据我们是否支持或反对fork，验证额外数据内容
	// Depending on whether we support or oppose the fork, validate the extra-data contents
	if config.DAOForkSupport {
		if !bytes.Equal(header.Extra, params.DAOForkBlockExtra) {
			return ErrBadProDAOExtra
		}
	} else {
		if bytes.Equal(header.Extra, params.DAOForkBlockExtra) {
			return ErrBadNoDAOExtra
		}
	} //一切正常，标题具有我们期望的相同额外数据
	// All ok, header has the same extra-data we expect
	return nil
}

///ApplyDAOHardFork根据DAO硬叉/规则修改状态数据库，将一组DAO帐户的所有余额转移到单个退款/合同。
// ApplyDAOHardFork modifies the state database according to the DAO hard-fork
// rules, transferring all balances of a set of DAO accounts to a single refund
// contract.
func ApplyDAOHardFork(statedb *state.StateDB) {
	// Retrieve the contract to refund balances into/检索将余额退还给
	if !statedb.Exist(params.DAORefundContract) {
		statedb.CreateAccount(params.DAORefundContract)
	}
	///将每个DAO帐户和额外余额帐户资金移到退款合同中。
	// Move every DAO account and extra-balance account funds into the refund contract
	for _, addr := range params.DAODrainList() {
		statedb.AddBalance(params.DAORefundContract, statedb.GetBalance(addr))
		statedb.SetBalance(addr, new(big.Int))
	}
}
