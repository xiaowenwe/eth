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

package p2p

import (
	"fmt"

	"github.com/ethereum/go-ethereum/p2p/discover"
)

//协议代表一个P2P子协议实现。
// Protocol represents a P2P subprotocol implementation.
type Protocol struct {
	//名称应该包含官方协议名称，通常是三个字母的单词。
	// Name should contain the official protocol name,
	// often a three-letter word.
	Name string
	//版本应该包含协议的版本号。
	// Version should contain the version number of the protocol.
	Version uint
	//长度应该包含协议使用的消息码的数量。
	// Length should contain the number of message codes used
	// by the protocol.
	Length uint64
	//当协议与对等端进行协商时，会在新的groutine中调用Run。 它应该读取和写入来自rw的消息。 每条消息的有效负载必须完全消耗。
	//当启动返回时，对等连接关闭。 它应该返回遇到的任何协议级错误（例如I / O错误）
	// Run is called in a new groutine when the protocol has been
	// negotiated with a peer. It should read and write messages from
	// rw. The Payload for each message must be fully consumed.
	//
	// The peer connection is closed when Start returns. It should return
	// any protocol-level error (such as an I/O error) that is
	// encountered.
	Run func(peer *Peer, rw MsgReadWriter) error
	// NodeInfo是一个可选的帮助方法来检索关于主机节点的特定于协议的元数据。
	// NodeInfo is an optional helper method to retrieve protocol specific metadata
	// about the host node.
	NodeInfo func() interface{}
	// PeerInfo是一个可选的帮助方法，用于检索关于网络中某个对等体的协议特定元数据。
	// 如果设置了信息检索功能，但返回nil，则认为协议握手仍在运行。
	// PeerInfo is an optional helper method to retrieve protocol specific metadata
	// about a certain peer in the network. If an info retrieval function is set,
	// but returns nil, it is assumed that the protocol handshake is still running.
	PeerInfo func(id discover.NodeID) interface{}
}

func (p Protocol) cap() Cap {
	return Cap{p.Name, p.Version}
}

// Cap是对等能力的结构。
// Cap is the structure of a peer capability.
type Cap struct {
	Name    string
	Version uint
}

func (cap Cap) RlpData() interface{} {
	return []interface{}{cap.Name, cap.Version}
}

func (cap Cap) String() string {
	return fmt.Sprintf("%s/%d", cap.Name, cap.Version)
}

type capsByNameAndVersion []Cap

func (cs capsByNameAndVersion) Len() int      { return len(cs) }
func (cs capsByNameAndVersion) Swap(i, j int) { cs[i], cs[j] = cs[j], cs[i] }
func (cs capsByNameAndVersion) Less(i, j int) bool {
	return cs[i].Name < cs[j].Name || (cs[i].Name == cs[j].Name && cs[i].Version < cs[j].Version)
}
