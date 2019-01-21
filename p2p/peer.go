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
	"io"
	"net"
	"sort"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common/mclock"
	"github.com/ethereum/go-ethereum/event"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/p2p/discover"
	"github.com/ethereum/go-ethereum/rlp"
)

//在p2p代码里面。 peer代表了一条创建好的网络链路。
// 在一条链路上可能运行着多个协议。比如以太坊的协议(eth)。 Swarm的协议。
// 或者是Whisper的协议。
const (
	baseProtocolVersion    = 5
	baseProtocolLength     = uint64(16)
	baseProtocolMaxMsgSize = 2 * 1024

	snappyProtocolVersion = 5

	pingInterval = 15 * time.Second
)

const (
	// devp2p message codes
	handshakeMsg = 0x00
	discMsg      = 0x01
	pingMsg      = 0x02
	pongMsg      = 0x03
	getPeersMsg  = 0x04
	peersMsg     = 0x05
)

// protoHandshake是协议握手的RLP结构。
// protoHandshake is the RLP structure of the protocol handshake.
type protoHandshake struct {
	Version    uint64
	Name       string
	Caps       []Cap
	ListenPort uint64
	ID         discover.NodeID
	//忽略其他字段（为了兼容性）。
	// Ignore additional fields (for forward compatibility).
	Rest []rlp.RawValue `rlp:"tail"`
}

// PeerEventType是由p2p.Server发出的对等事件的类型
// PeerEventType is the type of peer events emitted by a p2p.Server
type PeerEventType string

const (
	// PeerEventTypeAdd是将对等体添加到p2p.Server时发出的事件类型
	// PeerEventTypeAdd is the type of event emitted when a peer is added
	// to a p2p.Server
	PeerEventTypeAdd PeerEventType = "add"
	// PeerEventTypeDrop是从p2p.Server中删除对等点时发出的事件类型
	// PeerEventTypeDrop is the type of event emitted when a peer is
	// dropped from a p2p.Server
	PeerEventTypeDrop PeerEventType = "drop"
	// PeerEventTypeMsgSend是消息成功发送给对等体时发出的事件类型
	// PeerEventTypeMsgSend is the type of event emitted when a
	// message is successfully sent to a peer
	PeerEventTypeMsgSend PeerEventType = "msgsend"
	// PeerEventTypeMsgRecv是从对等端收到消息时发出的事件类型
	// PeerEventTypeMsgRecv is the type of event emitted when a
	// message is received from a peer
	PeerEventTypeMsgRecv PeerEventType = "msgrecv"
)

// PeerEvent是当从p2p.Server添加或删除对等点时发出的事件，或者在对等连接上发送或接收消息时发出的事件
// PeerEvent is an event emitted when peers are either added or dropped from
// a p2p.Server or when a message is sent or received on a peer connection
type PeerEvent struct {
	Type     PeerEventType   `json:"type"`
	Peer     discover.NodeID `json:"peer"`
	Error    string          `json:"error,omitempty"`
	Protocol string          `json:"protocol,omitempty"`
	MsgCode  *uint64         `json:"msg_code,omitempty"`
	MsgSize  *uint32         `json:"msg_size,omitempty"`
}

// Peer代表连接的远程节点。
// Peer represents a connected remote node.
type Peer struct {
	rw      *conn
	running map[string]*protoRW //运行的协议
	log     log.Logger
	created mclock.AbsTime //精确时间

	wg       sync.WaitGroup
	protoErr chan error
	closed   chan struct{}
	disc     chan DiscReason //int
	//事件收到消息发送/接收事件（如果设置）
	// events receives message send / receive events if set
	events *event.Feed
}

//peer的创建，根据匹配找到当前Peer支持的protomap
// NewPeer returns a peer for testing purposes.
//NewPeer返回一个对等体用于测试目的。
func NewPeer(id discover.NodeID, name string, caps []Cap) *Peer {
	pipe, _ := net.Pipe()
	conn := &conn{fd: pipe, transport: nil, id: id, caps: caps, name: name}
	peer := newPeer(conn, nil)
	close(peer.closed) // ensures Disconnect doesn't block确保断开不会阻止
	return peer
}

// ID返回节点的公钥。
// ID returns the node's public key.
func (p *Peer) ID() discover.NodeID {
	return p.rw.id
}

// Name返回远程节点通告的节点名称。
// Name returns the node name that the remote node advertised.
func (p *Peer) Name() string {
	return p.rw.name
}

// Caps返回远程节点的功能（支持的子协议）。
// Caps returns the capabilities (supported subprotocols) of the remote peer.
func (p *Peer) Caps() []Cap {
	// TODO: maybe return copy
	return p.rw.caps
}

// RemoteAddr返回网络连接的远程地址。
// RemoteAddr returns the remote address of the network connection.
func (p *Peer) RemoteAddr() net.Addr {
	return p.rw.fd.RemoteAddr()
}

// LocalAddr返回网络连接的本地地址。
// LocalAddr returns the local address of the network connection.
func (p *Peer) LocalAddr() net.Addr {
	return p.rw.fd.LocalAddr()
}

//断开连接将以给定的原因终止对等连接。
//它立即返回并且不会等到连接关闭。
// Disconnect terminates the peer connection with the given reason.
// It returns immediately and does not wait until the connection is closed.
func (p *Peer) Disconnect(reason DiscReason) {
	select {
	case p.disc <- reason:
	case <-p.closed:
	}
}

// String实现fmt.Stringer。
// String implements fmt.Stringer.
func (p *Peer) String() string {
	return fmt.Sprintf("Peer %x %v", p.rw.id[:8], p.RemoteAddr())
}

//如果对等端是入站连接，则入站返回true
// Inbound returns true if the peer is an inbound connection
func (p *Peer) Inbound() bool {
	return p.rw.flags&inboundConn != 0
}

//和peer握手
func newPeer(conn *conn, protocols []Protocol) *Peer {
	protomap := matchProtocols(protocols, conn.caps, conn)
	p := &Peer{
		rw:       conn,
		running:  protomap,
		created:  mclock.Now(),
		disc:     make(chan DiscReason),             //uint
		protoErr: make(chan error, len(protomap)+1), // protocols + pingLoop
		closed:   make(chan struct{}),
		log:      log.New("id", conn.id, "conn", conn.flags),
	}
	return p
}

func (p *Peer) Log() log.Logger {
	return p.log
}

//peer的启动， 启动了两个goroutine线程。 一个是读取。一个是执行ping操作。
func (p *Peer) run() (remoteRequested bool, err error) {
	var (
		writeStart = make(chan struct{}, 1)
		writeErr   = make(chan error, 1)
		readErr    = make(chan error, 1)
		reason     DiscReason // sent to the peer//uint
	)
	p.wg.Add(2) //添加两个阻塞
	go p.readLoop(readErr)
	go p.pingLoop()
	//启动所有协议处理程序。
	// Start all protocol handlers.
	writeStart <- struct{}{}
	//启动所有的协议。
	p.startProtocols(writeStart, writeErr)
	//等待错误或断开连接。
	// Wait for an error or disconnect.
loop:
	for {
		select {
		case err = <-writeErr:
			// A write finished. Allow the next write to start if
			// there was no error.
			//写完了。 如果没有错误，允许下一次写入。
			if err != nil {
				reason = DiscNetworkError
				break loop
			}
			writeStart <- struct{}{}
		case err = <-readErr:
			if r, ok := err.(DiscReason); ok {
				remoteRequested = true
				reason = r
			} else {
				reason = DiscNetworkError
			}
			break loop
		case err = <-p.protoErr:
			reason = discReasonForError(err)
			break loop
		case err = <-p.disc:
			break loop
		}
	}

	close(p.closed)
	p.rw.close(reason)
	p.wg.Wait()
	return remoteRequested, err
}

//在看看pingLoop。这个方法很简单。就是定时的发送pingMsg消息到对端。
func (p *Peer) pingLoop() {
	ping := time.NewTimer(pingInterval)
	defer p.wg.Done()
	defer ping.Stop()
	for {
		select {
		case <-ping.C:
			if err := SendItems(p.rw, pingMsg); err != nil {
				p.protoErr <- err
				return
			}
			ping.Reset(pingInterval)
		case <-p.closed:
			return
		}
	}
}

//回过头来再看看readLoop方法。 这个方法也是一个死循环。 调用p.rw来读取一个Msg(这个rw实际是之前提到的frameRLPx的对象，也就是分帧之后的对象。
// 然后根据Msg的类型进行对应的处理，如果Msg的类型是内部运行的协议的类型。那么发送到对应协议的proto.in队列上面。
func (p *Peer) readLoop(errc chan<- error) {
	defer p.wg.Done()
	for {
		msg, err := p.rw.ReadMsg()
		if err != nil {
			errc <- err
			return
		}
		msg.ReceivedAt = time.Now()
		if err = p.handle(msg); err != nil {
			errc <- err
			return
		}
	}
}

//处理msg
func (p *Peer) handle(msg Msg) error {

	switch {
	case msg.Code == pingMsg:
		msg.Discard()
		go SendItems(p.rw, pongMsg)
	case msg.Code == discMsg:
		var reason [1]DiscReason
		//这是最后一条消息。 我们不需要丢弃或检查错误，因为连接将在其后关闭。
		// This is the last message. We don't need to discard or
		// check errors because, the connection will be closed after it.
		rlp.Decode(msg.Payload, &reason)
		return reason[0]
	case msg.Code < baseProtocolLength:
		//忽略其他基本协议消息
		// ignore other base protocol messages
		return msg.Discard()
	default:
		//这是一个子协议消息
		// it's a subprotocol message
		proto, err := p.getProto(msg.Code)
		if err != nil {
			return fmt.Errorf("msg code out of range: %v", msg.Code)
		}
		select {
		case proto.in <- msg:
			return nil
		case <-p.closed:
			return io.EOF
		}
	}
	return nil
}

//总共协议数量
func countMatchingProtocols(protocols []Protocol, caps []Cap) int {
	n := 0
	for _, cap := range caps {
		for _, proto := range protocols {
			if proto.Name == cap.Name && proto.Version == cap.Version {
				n++
			}
		}
	}
	return n
}

// matchProtocols为匹配命名的子协议创建结构。
// matchProtocols creates structures for matching named subprotocols.
func matchProtocols(protocols []Protocol, caps []Cap, rw MsgReadWriter) map[string]*protoRW {
	sort.Sort(capsByNameAndVersion(caps))
	offset := baseProtocolLength
	result := make(map[string]*protoRW)

outer:
	for _, cap := range caps {
		for _, proto := range protocols {
			if proto.Name == cap.Name && proto.Version == cap.Version {
				// If an old protocol version matched, revert it
				//如果旧的协议版本匹配，请将其恢复
				if old := result[cap.Name]; old != nil {
					offset -= old.Length
				}
				//分配新的匹配
				// Assign the new match
				result[cap.Name] = &protoRW{Protocol: proto, offset: offset, in: make(chan Msg), w: rw}
				offset += proto.Length

				continue outer
			}
		}
	}
	return result
}

//startProtocols方法，这个方法遍历所有的协议。
func (p *Peer) startProtocols(writeStart <-chan struct{}, writeErr chan<- error) {
	p.wg.Add(len(p.running))
	for _, proto := range p.running {
		proto := proto
		proto.closed = p.closed
		proto.wstart = writeStart
		proto.werr = writeErr
		var rw MsgReadWriter = proto
		if p.events != nil {
			rw = newMsgEventer(rw, p.events, p.ID(), proto.Name)
		}
		p.log.Trace(fmt.Sprintf("Starting protocol %s/%d", proto.Name, proto.Version))
		// 等于这里为每一个协议都开启了一个goroutine。 调用其Run方法。
		go func() {
			// proto.Run(p, rw)这个方法应该是一个死循环。 如果返回就说明遇到了错误。
			err := proto.Run(p, rw)
			if err == nil {
				p.log.Trace(fmt.Sprintf("Protocol %s/%d returned", proto.Name, proto.Version))
				err = errProtocolReturned
			} else if err != io.EOF {
				p.log.Trace(fmt.Sprintf("Protocol %s/%d failed", proto.Name, proto.Version), "err", err)
			}
			p.protoErr <- err
			p.wg.Done()
		}()
	}
}

// getProto finds the protocol responsible for handling
// the given message code.
// getProto找到负责处理给定消息代码的协议。
func (p *Peer) getProto(code uint64) (*protoRW, error) {
	for _, proto := range p.running {
		if code >= proto.offset && code < proto.offset+proto.Length {
			return proto, nil
		}
	}
	return nil, newPeerError(errInvalidMsgCode, "%d", code)
}

type protoRW struct {
	Protocol
	in     chan Msg        // receices read messages//接收读取消息
	closed <-chan struct{} // receives when peer is shutting down//在对等关闭时收到
	wstart <-chan struct{} // receives when write may start//在写入开始时收到
	werr   chan<- error    // for write results//用于写入结果
	offset uint64
	w      MsgWriter
}

//最后再看看protoRW的read和write方法。 可以看到读取和写入都是阻塞式的。
func (rw *protoRW) WriteMsg(msg Msg) (err error) {
	if msg.Code >= rw.Length {
		return newPeerError(errInvalidMsgCode, "not handled")
	}
	msg.Code += rw.offset
	select {
	case <-rw.wstart: ////等到可以写入的受在执行写入。 这难道是为了多线程控制么。
		err = rw.w.WriteMsg(msg)
		//将状态报告给Peer.run。 如果错误不为零，它将启动关闭，否则将解除下一个写入的阻止。
		// 调用协议代码也应该退出以防错误，但我们不想依赖它。
		// Report write status back to Peer.run. It will initiate
		// shutdown if the error is non-nil and unblock the next write
		// otherwise. The calling protocol code should exit for errors
		// as well but we don't want to rely on that.
		rw.werr <- err
	case <-rw.closed:
		err = fmt.Errorf("shutting down")
	}
	return err
}

func (rw *protoRW) ReadMsg() (Msg, error) {
	select {
	case msg := <-rw.in:
		msg.Code -= rw.offset
		return msg, nil
	case <-rw.closed:
		return Msg{}, io.EOF
	}
}

// PeerInfo表示已知关于连接对等体的信息的简短摘要。
// 独立于子协议的字段在此处被包含和初始化，其中协议特定被委派给所有连接的子协议。
// PeerInfo represents a short summary of the information known about a connected
// peer. Sub-protocol independent fields are contained and initialized here, with
// protocol specifics delegated to all connected sub-protocols.
type PeerInfo struct {
	ID      string   `json:"id"`   // Unique node identifier (also the encryption key)//唯一的节点标识符（也是加密密钥）
	Name    string   `json:"name"` // Name of the node, including client type, version, OS, custom data//节点的名称，包括客户端类型，版本，操作系统，自定义数据
	Caps    []string `json:"caps"` // Sum-protocols advertised by this particular peer//通过此特定对等方广告的协议
	Network struct {
		LocalAddress  string `json:"localAddress"`  // Local endpoint of the TCP data connection// TCP数据连接的本地端点
		RemoteAddress string `json:"remoteAddress"` // Remote endpoint of the TCP data connection// TCP数据连接的远程端点
		Inbound       bool   `json:"inbound"`
		Trusted       bool   `json:"trusted"`
		Static        bool   `json:"static"`
	} `json:"network"`
	Protocols map[string]interface{} `json:"protocols"` // Sub-protocol specific metadata fields//子协议特定的元数据字段
}

// Info收集并返回已知关于对等体的元数据集合
// Info gathers and returns a collection of metadata known about a peer.
func (p *Peer) Info() *PeerInfo {
	//收集协议功能
	// Gather the protocol capabilities
	var caps []string
	for _, cap := range p.Caps() {
		caps = append(caps, cap.String())
	}
	//组装通用对等元数据
	// Assemble the generic peer metadata
	info := &PeerInfo{
		ID:        p.ID().String(),
		Name:      p.Name(),
		Caps:      caps,
		Protocols: make(map[string]interface{}),
	}
	info.Network.LocalAddress = p.LocalAddr().String()
	info.Network.RemoteAddress = p.RemoteAddr().String()
	info.Network.Inbound = p.rw.is(inboundConn)
	info.Network.Trusted = p.rw.is(trustedConn)
	info.Network.Static = p.rw.is(staticDialedConn)
	//收集所有正在运行的协议信息
	// Gather all the running protocol infos
	for _, proto := range p.running {
		protoInfo := interface{}("unknown")
		if query := proto.Protocol.PeerInfo; query != nil {
			if metadata := query(p.ID()); metadata != nil {
				protoInfo = metadata
			} else {
				protoInfo = "handshake"
			}
		}
		info.Protocols[proto.Name] = protoInfo
	}
	return info
}
