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
//journal代表了操作日志， 并针对各种操作的日志提供了对应的回滚功能。 可以基于这个日志来做一些事务类型的操作。
//类型定义，定义了journalEntry这个接口，提供了undo的功能。 journal 就是journalEntry的列表。
package state

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common"
)

type journalEntry interface {
	undo(*StateDB)
}

type journal []journalEntry

type (
	// Changes to the account trie.
	createObjectChange struct { //创建对象的日志。 undo方法就是从StateDB中删除创建的对象。
		account *common.Address
	}
	resetObjectChange struct { // 对于stateObject的修改， undo方法就是把值改为原来的对象。
		prev *stateObject
	}
	suicideChange struct { // 自杀的更改。自杀应该是删除账号，但是如果没有commit的化，对象还没有从stateDB删除。
		account     *common.Address
		prev        bool // whether account had already suicided
		prevbalance *big.Int
	}

	// Changes to individual accounts.
	balanceChange struct {
		account *common.Address
		prev    *big.Int
	}
	nonceChange struct {
		account *common.Address
		prev    uint64
	}
	storageChange struct {
		account       *common.Address
		key, prevalue common.Hash
	}
	codeChange struct {
		account            *common.Address
		prevcode, prevhash []byte
	}
	// 我理解是DAO事件的退款处理
	// Changes to other state values.
	refundChange struct {
		prev uint64
	}
	// 增加了日志的修改
	addLogChange struct {
		txhash common.Hash
	}
	// 这个是增加 VM看到的 SHA3的 原始byte[], 增加SHA3 hash -> byte[] 的对应关系
	addPreimageChange struct {
		hash common.Hash
	}
	touchChange struct {
		account   *common.Address
		prev      bool
		prevDirty bool
	}
)

func (ch createObjectChange) undo(s *StateDB) {
	delete(s.stateObjects, *ch.account)
	delete(s.stateObjectsDirty, *ch.account)
}

func (ch resetObjectChange) undo(s *StateDB) {
	s.setStateObject(ch.prev)
}

func (ch suicideChange) undo(s *StateDB) {
	obj := s.getStateObject(*ch.account)
	if obj != nil {
		obj.suicided = ch.prev
		obj.setBalance(ch.prevbalance)
	}
}

var ripemd = common.HexToAddress("0000000000000000000000000000000000000003")

func (ch touchChange) undo(s *StateDB) {
	if !ch.prev && *ch.account != ripemd {
		s.getStateObject(*ch.account).touched = ch.prev
		if !ch.prevDirty {
			delete(s.stateObjectsDirty, *ch.account)
		}
	}
}

func (ch balanceChange) undo(s *StateDB) {
	s.getStateObject(*ch.account).setBalance(ch.prev)
}

func (ch nonceChange) undo(s *StateDB) {
	s.getStateObject(*ch.account).setNonce(ch.prev)
}

func (ch codeChange) undo(s *StateDB) {
	s.getStateObject(*ch.account).setCode(common.BytesToHash(ch.prevhash), ch.prevcode)
}

func (ch storageChange) undo(s *StateDB) {
	s.getStateObject(*ch.account).setState(ch.key, ch.prevalue)
}

func (ch refundChange) undo(s *StateDB) {
	s.refund = ch.prev
}

func (ch addLogChange) undo(s *StateDB) {
	logs := s.logs[ch.txhash]
	if len(logs) == 1 {
		delete(s.logs, ch.txhash)
	} else {
		s.logs[ch.txhash] = logs[:len(logs)-1]
	}
	s.logSize--
}

func (ch addPreimageChange) undo(s *StateDB) {
	delete(s.preimages, ch.hash)
}
