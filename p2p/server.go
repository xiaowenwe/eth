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
//server对象主要完成的工作把之前介绍的所有组件组合在一起。 使用rlpx.go来处理加密链路。
// 使用discover来处理节点发现和查找。 使用dial来生成和连接需要连接的节点。 使用peer对象来处理每个连接。
//server启动了一个listenLoop来监听和接收新的连接。
//启动一个run的goroutine来调用dialstate生成新的dial任务并进行连接。 goroutine之间使用channel来进行通讯和配合。
// Package p2p implements the Ethereum p2p network protocols.
//实现了Ethereum p2p网络协议。
package p2p

import (
	"crypto/ecdsa"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/mclock"
	"github.com/ethereum/go-ethereum/event"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/p2p/discover"
	"github.com/ethereum/go-ethereum/p2p/discv5"
	"github.com/ethereum/go-ethereum/p2p/nat"
	"github.com/ethereum/go-ethereum/p2p/netutil"
)

//server是p2p的最主要的部分。集合了所有之前的组件。
const ( // 默认的连接超时时间
	defaultDialTimeout = 15 * time.Second

	// Connectivity defaults.//连接默认值。
	maxActiveDialTasks     = 16
	defaultMaxPendingPeers = 50
	defaultDialRatio       = 3
	//读取完整信息所允许的最长时间。
	//这实际上是连接闲置的时间量。
	// Maximum time allowed for reading a complete message.
	// This is effectively the amount of time a connection can be idle.
	frameReadTimeout = 30 * time.Second
	//写入完整消息所允许的最长时间。
	// Maximum amount of time allowed for writing a complete message.
	frameWriteTimeout = 20 * time.Second
)

var errServerStopped = errors.New("server stopped")

// Config holds Server options.//配置保存服务器选项。
type Config struct {
	//此字段必须设置为有效的secp256k1私钥。
	// This field must be set to a valid secp256k1 private key.
	PrivateKey *ecdsa.PrivateKey `toml:"-"`
	// MaxPeers是可以的最大对等数量
	// 连接的。 它必须大于零。
	// MaxPeers is the maximum number of peers that can be
	// connected. It must be greater than zero.
	MaxPeers int
	// MaxPendingPeers是握手阶段可以挂起的最大对等点数量，对入站和出站连接分别统计。
	//零默认为预设值。
	// MaxPendingPeers is the maximum number of peers that can be pending in the
	// handshake phase, counted separately for inbound and outbound connections.
	// Zero defaults to preset values.
	MaxPendingPeers int `toml:",omitempty"`
	// DialRatio控制入站到拨号连接的比率。
	//示例：DialRatio为2允许拨打1/2的连接。
	//将DialRatio设置为零默认为3。
	// DialRatio controls the ratio of inbound to dialed connections.
	// Example: a DialRatio of 2 allows 1/2 of connections to be dialed.
	// Setting DialRatio to zero defaults it to 3.
	DialRatio int `toml:",omitempty"`
	// NoDiscovery可用于禁用对等发现机制。
	//禁用对于协议调试（手动拓扑）非常有用。
	// NoDiscovery can be used to disable the peer discovery mechanism.
	// Disabling is useful for protocol debugging (manual topology).
	NoDiscovery bool
	// DiscoveryV5指定是否基于新的基于主题发现的V5发现
	//协议应该启动或不启动。
	// DiscoveryV5 specifies whether the the new topic-discovery based V5 discovery
	// protocol should be started or not.
	DiscoveryV5 bool `toml:",omitempty"`
	// Name设置此服务器的节点名称。
	//使用common.MakeName创建一个遵循现有约定的名称。
	// Name sets the node name of this server.
	// Use common.MakeName to create a name that follows existing conventions.
	Name string `toml:"-"`
	// BootstrapNodes用于建立与网络其余部分的连接
	// BootstrapNodes are used to establish connectivity
	// with the rest of the network.
	BootstrapNodes []*discover.Node
	//用于使用V5发现协议建立与网络其余部分的连接。
	// BootstrapNodesV5 are used to establish connectivity
	// with the rest of the network using the V5 discovery
	// protocol.
	BootstrapNodesV5 []*discv5.Node `toml:",omitempty"`
	//静态节点用作预配置的连接，在断开连接时始终保持并重新连接。
	// Static nodes are used as pre-configured connections which are always
	// maintained and re-connected on disconnects.
	StaticNodes []*discover.Node
	//可信节点用作预先配置的连接，始终允许连接，甚至超过对等限制。
	// Trusted nodes are used as pre-configured connections which are always
	// allowed to connect, even above the peer limit.
	TrustedNodes []*discover.Node
	//连接可以限制在某些IP网络中。// NetRestrict != 0 则只考虑与列表中包含的IP进行连接
	//如果将此选项设置为非零值，则仅考虑与包含在列表中的IP网络之一相匹配的主机。
	// Connectivity can be restricted to certain IP networks.
	// If this option is set to a non-nil value, only hosts which match one of the
	// IP networks contained in the list are considered.
	NetRestrict *netutil.Netlist `toml:",omitempty"`
	// NodeDatabase是包含网络中先前看到的活动节点的数据库的路径。
	// NodeDatabase is the path to the database containing the previously seen
	// live nodes in the network.
	NodeDatabase string `toml:",omitempty"`
	//协议应该包含服务器支持的协议。 为每个对等点启动匹配协议。
	// Protocols should contain the protocols supported
	// by the server. Matching protocols are launched for
	// each peer.
	Protocols []Protocol `toml:"-"`
	//如果将ListenAddr设置为非零地址，则服务器将侦听传入连接。
	//如果端口为零，操作系统将选择一个端口。 当服务器启动时，ListenAddr字段将更新为实际地址。
	// If ListenAddr is set to a non-nil address, the server
	// will listen for incoming connections.
	//
	// If the port is zero, the operating system will pick a port. The
	// ListenAddr field will be updated with the actual address when
	// the server is started.
	ListenAddr string
	//如果设置为非零值，则使用给定的NAT端口映射器使侦听端口可用于Internet。
	// If set to a non-nil value, the given NAT port mapper
	// is used to make the listening port available to the
	// Internet.
	NAT nat.Interface `toml:",omitempty"`
	//如果拨号程序设置为非零值，则使用给定的拨号程序拨打出站对等连接。
	// If Dialer is set to a non-nil value, the given Dialer
	// is used to dial outbound peer connections.
	Dialer NodeDialer `toml:"-"`
	//如果NoDial为真，服务器将不会拨打任何对等点。
	// If NoDial is true, the server will not dial any peers.
	NoDial bool `toml:",omitempty"`
	//如果设置了EnableMsgEvents，那么无论何时向对等体发送消息或从对等体接收消息，服务器都会发出PeerEvents
	// If EnableMsgEvents is set then the server will emit PeerEvents
	// whenever a message is sent to or received from a peer
	EnableMsgEvents bool
	// Logger是一个用于p2p.Server的自定义记录器。
	// Logger is a custom logger to use with the p2p.Server.
	Logger log.Logger `toml:",omitempty"`
}

//服务器管理所有对等连接。
// Server manages all peer connections.
type Server struct {
	//服务器运行时，Config字段不能被修改
	// Config fields may not be modified while the server is running.
	Config
	//挂钩进行测试。 这些是有用的，因为我们可以禁止整个协议栈。
	// Hooks for testing. These are useful because we can inhibit
	// the whole protocol stack.
	newTransport func(net.Conn) transport
	newPeerHook  func(*Peer)

	lock    sync.Mutex // protects running//运行保护
	running bool

	ntab         discoverTable
	listener     net.Listener
	ourHandshake *protoHandshake
	lastLookup   time.Time
	DiscV5       *discv5.Network
	//这些是Peers，PeerCount（而没有其他）
	// These are for Peers, PeerCount (and nothing else).
	peerOp chan peerOpFunc

	peerOpDone chan struct{}

	quit          chan struct{}
	addstatic     chan *discover.Node
	removestatic  chan *discover.Node
	posthandshake chan *conn
	addpeer       chan *conn
	delpeer       chan peerDrop
	loopWG        sync.WaitGroup // loop, listenLoop
	peerFeed      event.Feed
	log           log.Logger
}

type peerOpFunc func(map[discover.NodeID]*Peer)

type peerDrop struct {
	*Peer
	err       error
	requested bool // true if signaled by the peer如果由对等方发信号，则为true
}

type connFlag int

const (
	dynDialedConn connFlag = 1 << iota
	staticDialedConn
	inboundConn //入站连接
	trustedConn //信任连接
)

// conn用两次握手期间收集的信息包装网络连接。
// conn wraps a network connection with information gathered
// during the two handshakes.
type conn struct {
	fd net.Conn
	transport
	flags connFlag
	cont  chan error      // The run loop uses cont to signal errors to SetupConn.//运行循环使用cont向SetupConn发送错误信号。
	id    discover.NodeID // valid after the encryption handshake//在加密握手后有效
	caps  []Cap           // valid after the protocol handshake//在协议握手后有效
	name  string          // valid after the protocol handshake//在协议握手后有效
}

type transport interface {
	// The two handshakes.//两次握手。
	doEncHandshake(prv *ecdsa.PrivateKey, dialDest *discover.Node) (discover.NodeID, error)
	doProtoHandshake(our *protoHandshake) (*protoHandshake, error)
	// MsgReadWriter只能在加密握手完成后使用。 该代码使用conn.id通过在加密握手后将其设置为非零值来跟踪此情况。
	// The MsgReadWriter can only be used after the encryption
	// handshake has completed. The code uses conn.id to track this
	// by setting it to a non-nil value after the encryption handshake.
	MsgReadWriter
	//传输必须提供Close，因为我们在一些测试中使用MsgPipe。
	// 关闭实际的网络连接不会在这些测试中做任何事情，因为NsgPipe不使用它。
	// transports must provide Close because we use MsgPipe in some of
	// the tests. Closing the actual network connection doesn't do
	// anything in those tests because NsgPipe doesn't use it.
	close(err error)
}

func (c *conn) String() string {
	s := c.flags.String()
	if (c.id != discover.NodeID{}) {
		s += " " + c.id.String()
	}
	s += " " + c.fd.RemoteAddr().String()
	return s
}

func (f connFlag) String() string {
	s := ""
	if f&trustedConn != 0 {
		s += "-trusted"
	}
	if f&dynDialedConn != 0 {
		s += "-dyndial"
	}
	if f&staticDialedConn != 0 {
		s += "-staticdial"
	}
	if f&inboundConn != 0 {
		s += "-inbound"
	}
	if s != "" {
		s = s[1:]
	}
	return s
}

func (c *conn) is(f connFlag) bool {
	return c.flags&f != 0
}

// Peers returns all connected peers.
// Peers返回所有连接的对等体。
func (srv *Server) Peers() []*Peer {
	var ps []*Peer
	select {
	//注意：我们很想把这个函数放入一个变量中，但这在某些环境中似乎会导致一个奇怪的编译器错误。
	// Note: We'd love to put this function into a variable but
	// that seems to cause a weird compiler error in some
	// environments.
	case srv.peerOp <- func(peers map[discover.NodeID]*Peer) {
		for _, p := range peers {
			ps = append(ps, p)
		}
	}:
		<-srv.peerOpDone
	case <-srv.quit:
	}
	return ps
}

// PeerCount返回连接的对等体的数量。
// PeerCount returns the number of connected peers.
func (srv *Server) PeerCount() int {
	var count int
	select {
	case srv.peerOp <- func(ps map[discover.NodeID]*Peer) { count = len(ps) }:
		<-srv.peerOpDone
	case <-srv.quit:
	}
	return count
}

// AddPeer连接到给定的节点并保持连接，直到服务器关闭。 如果连接因任何原因失败，服务器将尝试重新连接对等体。
// AddPeer connects to the given node and maintains the connection until the
// server is shut down. If the connection fails for any reason, the server will
// attempt to reconnect the peer.
func (srv *Server) AddPeer(node *discover.Node) {
	select {
	case srv.addstatic <- node:
	case <-srv.quit:
	}
}

// RemovePeer与给定节点断开连接
// RemovePeer disconnects from the given node
func (srv *Server) RemovePeer(node *discover.Node) {
	select {
	case srv.removestatic <- node:
	case <-srv.quit:
	}
}

//订阅者订阅给定的频道
// SubscribePeers subscribes the given channel to peer events
func (srv *Server) SubscribeEvents(ch chan *PeerEvent) event.Subscription {
	return srv.peerFeed.Subscribe(ch)
}

//自身返回本地节点的端点信息。
// Self returns the local node's endpoint information.
func (srv *Server) Self() *discover.Node {
	srv.lock.Lock()
	defer srv.lock.Unlock()

	if !srv.running {
		return &discover.Node{IP: net.ParseIP("0.0.0.0")}
	}
	return srv.makeSelf(srv.listener, srv.ntab)
}

//makeSelf如果服务器没有运行，返回一个空节点,否则注入侦听器地址
func (srv *Server) makeSelf(listener net.Listener, ntab discoverTable) *discover.Node {
	//如果服务器没有运行，返回一个空节点。
	//如果节点正在运行但发现已关闭，请手动组装节点信息。
	// If the server's not running, return an empty node.
	// If the node is running but discovery is off, manually assemble the node infos.
	if ntab == nil {
		//禁止入站连接，使用零地址
		// Inbound connections disabled, use zero address.
		if listener == nil {
			return &discover.Node{IP: net.ParseIP("0.0.0.0"), ID: discover.PubkeyID(&srv.PrivateKey.PublicKey)}
		}
		//否则注入侦听器地址
		// Otherwise inject the listener address too
		addr := listener.Addr().(*net.TCPAddr)
		return &discover.Node{
			ID:  discover.PubkeyID(&srv.PrivateKey.PublicKey),
			IP:  addr.IP,
			TCP: uint16(addr.Port),
		}
	}
	//否则返回发现节点。
	// Otherwise return the discovery node.
	return ntab.Self()
}

//停止终止服务器和所有主动对等连接。
//阻塞直到所有活动连接都关闭。
// Stop terminates the server and all active peer connections.
// It blocks until all active connections have been closed.
func (srv *Server) Stop() {
	srv.lock.Lock()
	defer srv.lock.Unlock()
	if !srv.running {
		return
	}
	srv.running = false
	if srv.listener != nil {
		//这个解锁监听器接受
		// this unblocks listener Accept
		srv.listener.Close()
	}
	close(srv.quit)
	srv.loopWG.Wait()
}

// sharedUDPConn实现共享连接。 Write将消息发送到基础连接，而read将返回发现无法处理并由主监听器发送到未处理通道的消息。
// sharedUDPConn implements a shared connection. Write sends messages to the underlying connection while read returns
// messages that were found unprocessable and sent to the unhandled channel by the primary listener.
type sharedUDPConn struct {
	*net.UDPConn
	unhandled chan discover.ReadPacket
}

// ReadFromUDP实现discv5.conn
// ReadFromUDP implements discv5.conn
func (s *sharedUDPConn) ReadFromUDP(b []byte) (n int, addr *net.UDPAddr, err error) {
	packet, ok := <-s.unhandled
	if !ok {
		return 0, nil, fmt.Errorf("Connection was closed")
	}
	l := len(packet.Data)
	if l > len(b) {
		l = len(b)
	}
	copy(b[:l], packet.Data[:l])
	return l, packet.Addr, nil
}

//关闭实现discv5.conn
// Close implements discv5.conn
func (s *sharedUDPConn) Close() error {
	return nil
}

//并不存在一个newServer的方法。 初始化的工作放在Start()方法中。
//开始启动运行服务器。
//停止后服务器无法重新使用。
// Start starts running the server.
// Servers can not be re-used after stopping.
func (srv *Server) Start() (err error) {
	srv.lock.Lock()
	defer srv.lock.Unlock()
	if srv.running { //避免多次启动。 srv.lock为了避免多线程重复启动
		return errors.New("server already running")
	}
	srv.running = true
	srv.log = srv.Config.Logger
	if srv.log == nil {
		srv.log = log.New()
	}
	srv.log.Info("Starting P2P networking")

	// static fields
	if srv.PrivateKey == nil {
		return fmt.Errorf("Server.PrivateKey must be set to a non-nil key")
	}
	if srv.newTransport == nil { //这里注意的是Transport使用了newRLPX 使用了rlpx.go中的网络协议。
		srv.newTransport = newRLPX // 加密链路使用RLPX协议
	}
	if srv.Dialer == nil { //使用了TCLPDialer
		srv.Dialer = TCPDialer{&net.Dialer{Timeout: defaultDialTimeout}}
	}
	srv.quit = make(chan struct{})
	srv.addpeer = make(chan *conn)
	srv.delpeer = make(chan peerDrop)
	srv.posthandshake = make(chan *conn)
	srv.addstatic = make(chan *discover.Node)
	srv.removestatic = make(chan *discover.Node)
	srv.peerOp = make(chan peerOpFunc)
	srv.peerOpDone = make(chan struct{})

	var (
		conn      *net.UDPConn
		sconn     *sharedUDPConn
		realaddr  *net.UDPAddr
		unhandled chan discover.ReadPacket
	)

	if !srv.NoDiscovery || srv.DiscoveryV5 {
		addr, err := net.ResolveUDPAddr("udp", srv.ListenAddr) // 启动discover网络
		if err != nil {
			return err
		} // 开启UDP监听
		conn, err = net.ListenUDP("udp", addr)
		if err != nil {
			return err
		}
		realaddr = conn.LocalAddr().(*net.UDPAddr)
		if srv.NAT != nil {
			if !realaddr.IP.IsLoopback() {
				go nat.Map(srv.NAT, srv.quit, "udp", realaddr.Port, realaddr.Port, "ethereum discovery")
			}
			// TODO: react to external IP changes over time.
			if ext, err := srv.NAT.ExternalIP(); err == nil {
				realaddr = &net.UDPAddr{IP: ext, Port: realaddr.Port}
			}
		}
	}

	if !srv.NoDiscovery && srv.DiscoveryV5 {
		unhandled = make(chan discover.ReadPacket, 100)
		sconn = &sharedUDPConn{conn, unhandled}
	}

	// node table//启动discover网络。 开启UDP的监听。
	if !srv.NoDiscovery {
		cfg := discover.Config{
			PrivateKey:   srv.PrivateKey,
			AnnounceAddr: realaddr,
			NodeDBPath:   srv.NodeDatabase,
			NetRestrict:  srv.NetRestrict,
			Bootnodes:    srv.BootstrapNodes,
			Unhandled:    unhandled,
		}
		////设置最开始的启动节点。当找不到其他的节点的时候。 那么就连接这些启动节点。这些节点的信息是写死在配置文件里面的。
		ntab, err := discover.ListenUDP(conn, cfg)
		if err != nil {
			return err
		}
		srv.ntab = ntab
	}

	if srv.DiscoveryV5 { ////这是新的节点发现协议。 暂时还没有使用。  这里暂时没有分析。
		var (
			ntab *discv5.Network
			err  error
		)
		if sconn != nil {
			ntab, err = discv5.ListenUDP(srv.PrivateKey, sconn, realaddr, "", srv.NetRestrict) //srv.NodeDatabase)
		} else {
			ntab, err = discv5.ListenUDP(srv.PrivateKey, conn, realaddr, "", srv.NetRestrict) //srv.NodeDatabase)
		}
		if err != nil {
			return err
		}
		if err := ntab.SetFallbackNodes(srv.BootstrapNodesV5); err != nil {
			return err
		}
		srv.DiscV5 = ntab
	}

	dynPeers := srv.maxDialedConns()
	//创建dialerstate。// 新建dialerstate用来处理与节点的连接
	dialer := newDialState(srv.StaticNodes, srv.BootstrapNodes, srv.ntab, dynPeers, srv.NetRestrict)

	// handshake
	//我们自己的协议的handShake
	srv.ourHandshake = &protoHandshake{Version: baseProtocolVersion, Name: srv.Name, ID: discover.PubkeyID(&srv.PrivateKey.PublicKey)}
	for _, p := range srv.Protocols { //增加所有的协议的Caps
		srv.ourHandshake.Caps = append(srv.ourHandshake.Caps, p.cap())
	}
	// listen/dial
	if srv.ListenAddr != "" {
		if err := srv.startListening(); err != nil { //开始监听TCP端口
			return err
		}
	}
	if srv.NoDial && srv.ListenAddr == "" {
		srv.log.Warn("P2P server will be useless, neither dialing nor listening")
	}

	srv.loopWG.Add(1)
	//启动goroutine 来处理程序
	go srv.run(dialer)
	srv.running = true
	return nil
}

//启动监听。 可以看到是TCP协议。 这里的监听端口和UDP的端口是一样的。 默认都是30303
func (srv *Server) startListening() error {
	// Launch the TCP listener.//启动TCP侦听器。
	listener, err := net.Listen("tcp", srv.ListenAddr)
	if err != nil {
		return err
	}
	laddr := listener.Addr().(*net.TCPAddr)
	srv.ListenAddr = laddr.String()
	srv.listener = listener
	srv.loopWG.Add(1)
	go srv.listenLoop()
	//如果配置了NAT，映射TCP侦听端口。
	// Map the TCP listening port if NAT is configured.
	if !laddr.IP.IsLoopback() && srv.NAT != nil {
		srv.loopWG.Add(1)
		go func() {
			nat.Map(srv.NAT, srv.quit, "tcp", laddr.Port, laddr.Port, "ethereum p2p")
			srv.loopWG.Done()
		}()
	}
	return nil
}

type dialer interface {
	newTasks(running int, peers map[discover.NodeID]*Peer, now time.Time) []task
	taskDone(task, time.Time)
	addStatic(*discover.Node)
	removeStatic(*discover.Node)
}

//上面说到的流程是listenLoop的流程，listenLoop主要是用来接收外部主动连接者的。
// 还有部分情况是节点需要主动发起连接来连接外部节点的流程。
// 以及处理刚才上面的checkpoint队列信息的流程。这部分代码都在server.run这个goroutine里面。
func (srv *Server) run(dialstate dialer) {
	defer srv.loopWG.Done()
	var (
		peers        = make(map[discover.NodeID]*Peer)
		inboundCount = 0
		trusted      = make(map[discover.NodeID]bool, len(srv.TrustedNodes))
		taskdone     = make(chan task, maxActiveDialTasks)
		runningTasks []task
		queuedTasks  []task // tasks that can't run yet无法运行的任务
	)
	//将可信节点放入地图中以加速检查。
	//受信任的对等体在启动时加载，并且在服务器运行时不能修改。
	// Put trusted nodes into a map to speed up checks.
	// Trusted peers are loaded on startup and cannot be
	// modified while the server is running.
	// 被信任的节点又这样一个特性， 如果连接太多，那么其他节点会被拒绝掉。但是被信任的节点会被接收。
	for _, n := range srv.TrustedNodes {
		trusted[n.ID] = true
	}

	// removes t from runningTasks
	// 定义了一个函数，用来从runningTasks队列删除某个Task
	delTask := func(t task) {
		for i := range runningTasks {
			if runningTasks[i] == t {
				runningTasks = append(runningTasks[:i], runningTasks[i+1:]...)
				break
			}
		}
	}
	// 同时开始连接的节点数量是16个。 遍历 runningTasks队列，并启动这些任务。
	// starts until max number of active tasks is satisfied
	startTasks := func(ts []task) (rest []task) {
		i := 0
		for ; len(runningTasks) < maxActiveDialTasks && i < len(ts); i++ {
			t := ts[i]
			srv.log.Trace("New dial task", "task", t)
			go func() { t.Do(srv); taskdone <- t }()
			runningTasks = append(runningTasks, t)
		}
		return ts[i:]
	}
	scheduleTasks := func() {
		// Start from queue first.
		// 首先调用startTasks启动一部分，把剩下的返回给queuedTasks.
		queuedTasks = append(queuedTasks[:0], startTasks(queuedTasks)...)
		// Query dialer for new tasks and start as many as possible now.
		// 调用newTasks来生成任务，并尝试用startTasks启动。并把暂时无法启动的放入queuedTasks队列
		if len(runningTasks) < maxActiveDialTasks {
			nt := dialstate.newTasks(len(runningTasks)+len(queuedTasks), peers, time.Now())
			queuedTasks = append(queuedTasks, startTasks(nt)...)
		}
	}

running:
	for {
		//调用 dialstate.newTasks来生成新任务。 并调用startTasks启动新任务。
		//如果 dialTask已经全部启动，那么会生成一个睡眠超时任务。
		scheduleTasks()

		select {
		case <-srv.quit:
			//服务器已停止。 运行清理逻辑。
			// The server was stopped. Run the cleanup logic.
			break running
		case n := <-srv.addstatic:
			// AddPeer使用此通道添加到短暂的静态对等列表。 将它添加到拨号程序，它将保持连接的节点。
			// This channel is used by AddPeer to add to the
			// ephemeral static peer list. Add it to the dialer,
			// it will keep the node connected.
			srv.log.Debug("Adding static node", "node", n)
			dialstate.addStatic(n)
		case n := <-srv.removestatic:
			//此通道由RemovePeer用来向对等方发送断开连接请求，并开始停止保持连接节点
			// This channel is used by RemovePeer to send a
			// disconnect request to a peer and begin the
			// stop keeping the node connected
			srv.log.Debug("Removing static node", "node", n)
			dialstate.removeStatic(n)
			if p, ok := peers[n.ID]; ok {
				p.Disconnect(DiscRequested)
			}
		case op := <-srv.peerOp:
			// This channel is used by Peers and PeerCount.
			//此对话由Peers和PeerCount使用。
			op(peers)
			srv.peerOpDone <- struct{}{}
		case t := <-taskdone:
			//任务完成。 告诉dialstate关于它的事	可以更新其状态并将其从活动任务列表中删除。
			// A task got done. Tell dialstate about it so it
			// can update its state and remove it from the active
			// tasks list.
			srv.log.Trace("Dial task done", "task", t)
			dialstate.taskDone(t, time.Now())
			delTask(t)
		case c := <-srv.posthandshake:
			// A connection has passed the encryption handshake so
			// the remote identity is known (but hasn't been verified yet).
			//连接已通过加密握手，因此远程身份已知（但尚未验证）。
			// 记得之前调用checkpoint方法，会把连接发送给这个channel。
			if trusted[c.id] {
				//在检查MaxPeers之前，确保已设置可信标志。
				// Ensure that the trusted flag is set before checking against MaxPeers.
				c.flags |= trustedConn
			}
			// TODO: track in-progress inbound node IDs (pre-Peer) to avoid dialing them.
			select {
			case c.cont <- srv.encHandshakeChecks(peers, inboundCount, c):
			case <-srv.quit:
				break running
			}
		case c := <-srv.addpeer:
			//此时连接已经过了协议握手。
			//它的功能是已知的，并且远程身份验证。
			// At this point the connection is past the protocol handshake.
			// Its capabilities are known and the remote identity is verified.
			// 两次握手之后会调用checkpoint把连接发送到addpeer这个channel。
			// 然后通过newPeer创建了Peer对象。
			// 启动一个goroutine 启动peer对象。 调用了peer.run方法
			err := srv.protoHandshakeChecks(peers, inboundCount, c)
			if err == nil {
				//握手完成并通过所有检查。
				// The handshakes are done and it passed all checks.
				p := newPeer(c, srv.Protocols)
				// If message events are enabled, pass the peerFeed
				// to the peer
				//如果启用了消息事件，则将peerFeed传递给对等体
				if srv.EnableMsgEvents {
					p.events = &srv.peerFeed
				}
				name := truncateName(c.name)
				srv.log.Debug("Adding p2p peer", "name", name, "addr", c.fd.RemoteAddr(), "peers", len(peers)+1)
				go srv.runPeer(p)
				peers[c.id] = p
				if p.Inbound() {
					inboundCount++
				}
			}
			//拨号程序逻辑依赖于在添加或丢弃对等点后拨号任务完成的假设。 最后解除封锁任务。
			// The dialer logic relies on the assumption that
			// dial tasks complete after the peer has been added or
			// discarded. Unblock the task last.
			select {
			case c.cont <- err:
			case <-srv.quit:
				break running
			}
		case pd := <-srv.delpeer:
			//对等方断开连接。
			// A peer disconnected.
			d := common.PrettyDuration(mclock.Now() - pd.created)
			pd.log.Debug("Removing p2p peer", "duration", d, "peers", len(peers)-1, "req", pd.requested, "err", pd.err)
			delete(peers, pd.ID())
			if pd.Inbound() {
				inboundCount--
			}
		}
	}

	srv.log.Trace("P2P networking is spinning down")
	//终止发现。 如果有正在运行的查询，它将很快终止。
	// Terminate discovery. If there is a running lookup it will terminate soon.
	if srv.ntab != nil {
		srv.ntab.Close()
	}
	if srv.DiscV5 != nil {
		srv.DiscV5.Close()
	}
	//断开所有同伴。
	// Disconnect all peers.
	for _, p := range peers {
		p.Disconnect(DiscQuitting)
	}
	//等待同伴关闭。 待处理的连接和任务不在这里处理，并且会很快终止，因为srv.quit已关闭。
	// Wait for peers to shut down. Pending connections and tasks are
	// not handled here and will terminate soon-ish because srv.quit
	// is closed.
	for len(peers) > 0 {
		p := <-srv.delpeer
		p.log.Trace("<-delpeer (spindown)", "remainingTasks", len(runningTasks))
		delete(peers, p.ID())
	}
}

//协议握手检查
func (srv *Server) protoHandshakeChecks(peers map[discover.NodeID]*Peer, inboundCount int, c *conn) error {
	// Drop connections with no matching protocols.
	//删除没有匹配协议的连接。
	if len(srv.Protocols) > 0 && countMatchingProtocols(srv.Protocols, c.caps) == 0 {
		return DiscUselessPeer
	}
	//重复加密握手检查，因为对等设置可能在握手之间发生了变化。
	// Repeat the encryption handshake checks because the
	// peer set might have changed between the handshakes.
	return srv.encHandshakeChecks(peers, inboundCount, c)
}

//enc握手检查
func (srv *Server) encHandshakeChecks(peers map[discover.NodeID]*Peer, inboundCount int, c *conn) error {
	switch {
	case !c.is(trustedConn|staticDialedConn) && len(peers) >= srv.MaxPeers:
		return DiscTooManyPeers
	case !c.is(trustedConn) && c.is(inboundConn) && inboundCount >= srv.maxInboundConns():
		return DiscTooManyPeers
	case peers[c.id] != nil:
		return DiscAlreadyConnected
	case c.id == srv.Self().ID:
		return DiscSelf
	default:
		return nil
	}
}

func (srv *Server) maxInboundConns() int {
	return srv.MaxPeers - srv.maxDialedConns()
}

//最大拨号数量
func (srv *Server) maxDialedConns() int {
	if srv.NoDiscovery || srv.NoDial {
		return 0
	}
	r := srv.DialRatio
	if r == 0 {
		r = defaultDialRatio
	}
	return srv.MaxPeers / r
}

type tempError interface {
	Temporary() bool
}

//listenLoop()。 这是一个死循环的goroutine。 会监听端口并接收外部的请求。
// listenLoop runs in its own goroutine and accepts
// inbound connections.
func (srv *Server) listenLoop() {
	defer srv.loopWG.Done()
	srv.log.Info("RLPx listener up", "self", srv.makeSelf(srv.listener, srv.ntab))
	// 连接处理数
	tokens := defaultMaxPendingPeers
	if srv.MaxPendingPeers > 0 {
		tokens = srv.MaxPendingPeers
	}
	//创建maxAcceptConns个槽位。 我们只同时处理这么多连接。 多了也不要。
	slots := make(chan struct{}, tokens)
	for i := 0; i < tokens; i++ {
		slots <- struct{}{}
	}

	for {
		//在接受之前等待握手插槽。
		// Wait for a handshake slot before accepting.
		<-slots

		var (
			fd  net.Conn
			err error
		)
		for {
			fd, err = srv.listener.Accept()
			if tempErr, ok := err.(tempError); ok && tempErr.Temporary() {
				srv.log.Debug("Temporary read error", "err", err)
				continue
			} else if err != nil {
				srv.log.Debug("Read error", "err", err)
				return
			}
			break
		}

		// Reject connections that do not match NetRestrict.
		// 白名单。 如果不在白名单里面。那么关闭连接。
		if srv.NetRestrict != nil {
			if tcp, ok := fd.RemoteAddr().(*net.TCPAddr); ok && !srv.NetRestrict.Contains(tcp.IP) {
				srv.log.Debug("Rejected conn (not whitelisted in NetRestrict)", "addr", fd.RemoteAddr())
				fd.Close()
				slots <- struct{}{}
				continue
			}
		}

		fd = newMeteredConn(fd, true)
		srv.log.Trace("Accepted connection", "addr", fd.RemoteAddr())
		go func() {
			//看来只要连接建立完成之后。 槽位就会归还。 SetupConn这个函数我们记得再dialTask.Do里面也有调用， 这个函数主要是执行连接的几次握手。
			srv.SetupConn(fd, inboundConn, nil)
			slots <- struct{}{}
		}()
	}
}

//SetupConn,这个函数执行握手协议，并尝试把连接创建位一个peer对象。
// SetupConn runs the handshakes and attempts to add the connection
// as a peer. It returns when the connection has been added as a peer
// or the handshakes have failed.
// SetupConn运行握手并尝试将连接添加为对等体。 它在连接添加为对等体或握手失败时返回
func (srv *Server) SetupConn(fd net.Conn, flags connFlag, dialDest *discover.Node) error {
	self := srv.Self()
	if self == nil {
		return errors.New("shutdown")
	}
	//创建了一个conn对象。 newTransport指针实际上指向的newRLPx方法。 实际上是把fd用rlpx协议包装了一下。
	c := &conn{fd: fd, transport: srv.newTransport(fd), flags: flags, cont: make(chan error)}
	err := srv.setupConn(c, flags, dialDest)
	if err != nil {
		c.close(err)
		srv.log.Trace("Setting up connection failed", "id", c.id, "err", err)
	}
	return err
}

func (srv *Server) setupConn(c *conn, flags connFlag, dialDest *discover.Node) error {
	// Prevent leftover pending conns from entering the handshake.
	//防止剩下的conns进入握手。
	srv.lock.Lock()
	running := srv.running
	srv.lock.Unlock()
	if !running {
		return errServerStopped
	}
	//运行加密握手。
	// Run the encryption handshake.
	var err error
	//这里实际上执行的是rlpx.go里面的doEncHandshake.因为transport是conn的一个匿名字段。 匿名字段的方法会直接作为conn的一个方法个方法。
	if c.id, err = c.doEncHandshake(srv.PrivateKey, dialDest); err != nil {
		srv.log.Trace("Failed RLPx handshake", "addr", c.fd.RemoteAddr(), "conn", c.flags, "err", err)
		return err
	}
	clog := srv.log.New("id", c.id, "addr", c.fd.RemoteAddr(), "conn", c.flags)
	//对于拨号连接，请检查远程公钥是否匹配。
	// For dialed connections, check that the remote public key matches.
	if dialDest != nil && c.id != dialDest.ID { // 如果连接握手的ID和对应的ID不匹配
		clog.Trace("Dialed identity mismatch", "want", c, dialDest.ID)
		return DiscUnexpectedIdentity
	}
	// 这个checkpoint其实就是把第一个参数发送给第二个参数指定的队列。然后从c.cout接收返回信息。 是一个同步的方法。
	//至于这里，后续的操作只是检查了一下连接是否合法就返回了。
	err = srv.checkpoint(c, srv.posthandshake) // 将c发送给srv.posthandshake
	if err != nil {
		clog.Trace("Rejected peer before protocol handshake", "err", err)
		return err
	}
	//运行协议握手
	// Run the protocol handshake
	phs, err := c.doProtoHandshake(srv.ourHandshake)
	if err != nil {
		clog.Trace("Failed proto handshake", "err", err)
		return err
	}
	if phs.ID != c.id {
		clog.Trace("Wrong devp2p handshake identity", "err", phs.ID)
		return DiscUnexpectedIdentity
	}
	c.caps, c.name = phs.Caps, phs.Name
	// 这里两次握手都已经完成了。 把c发送给addpeer队列。 后台处理这个队列的时候，会处理这个连接
	err = srv.checkpoint(c, srv.addpeer) // 将c发送给addpeer队列
	if err != nil {
		clog.Trace("Rejected peer", "err", err)
		return err
	}
	//如果检查成功完成，runPeer现在已经通过运行启动。
	// If the checks completed successfully, runPeer has now been
	// launched by run.
	clog.Trace("connection set up", "inbound", dialDest == nil)
	return nil
}

func truncateName(s string) string {
	if len(s) > 20 {
		return s[:20] + "..."
	}
	return s
}

//检查点发送conn运行，执行阶段后的握手检查（posthandshake，addpeer）
// checkpoint sends the conn to run, which performs the
// post-handshake checks for the stage (posthandshake, addpeer).
func (srv *Server) checkpoint(c *conn, stage chan<- *conn) error {
	select {
	case stage <- c:
	case <-srv.quit:
		return errServerStopped
	}
	select {
	case err := <-c.cont:
		return err
	case <-srv.quit:
		return errServerStopped
	}
}

// runPeer在每个对等体的自己的goroutine中运行。
//等待对等逻辑返回并删除对等点。
// runPeer runs in its own goroutine for each peer.
// it waits until the Peer logic returns and removes
// the peer.
func (srv *Server) runPeer(p *Peer) {
	if srv.newPeerHook != nil {
		srv.newPeerHook(p)
	}

	// broadcast peer add
	srv.peerFeed.Send(&PeerEvent{
		Type: PeerEventTypeAdd,
		Peer: p.ID(),
	})
	//运行协议，阻塞
	// run the protocol
	remoteRequested, err := p.run()
	//广播同伴下降
	// broadcast peer drop
	srv.peerFeed.Send(&PeerEvent{
		Type:  PeerEventTypeDrop,
		Peer:  p.ID(),
		Error: err.Error(),
	})
	//注意：运行等待现有对等体在返回之前在srv.delpeer上发送，所以此发送不应在srv.quit上选择。
	// Note: run waits for existing peers to be sent on srv.delpeer
	// before returning, so this send should not select on srv.quit.
	srv.delpeer <- peerDrop{p, err, remoteRequested}
}

// NodeInfo表示主机已知信息的简短摘要。
// NodeInfo represents a short summary of the information known about the host.
type NodeInfo struct {
	ID    string `json:"id"`    // Unique node identifier (also the encryption key)唯一的节点标识符（也是加密密钥）
	Name  string `json:"name"`  // Name of the node, including client type, version, OS, custom data //节点的名称，包括客户端类型，版本，操作系统，自定义数据
	Enode string `json:"enode"` // Enode URL for adding this peer from remote peers// Enode URL，用于从远程对端添加此对等端
	IP    string `json:"ip"`    // IP address of the node节点的IP地址
	Ports struct {
		Discovery int `json:"discovery"` // UDP listening port for discovery protocol用于发现协议的UDP侦听端口
		Listener  int `json:"listener"`  // TCP listening port for RLPx//RLPx的TCP侦听端口
	} `json:"ports"`
	ListenAddr string                 `json:"listenAddr"`
	Protocols  map[string]interface{} `json:"protocols"`
}

// NodeInfo收集并返回有关主机的已知元数据集合。
// NodeInfo gathers and returns a collection of metadata known about the host.
func (srv *Server) NodeInfo() *NodeInfo {
	node := srv.Self()
	//收集并组装通用节点信息
	// Gather and assemble the generic node infos
	info := &NodeInfo{
		Name:       srv.Name,
		Enode:      node.String(),
		ID:         node.ID.String(),
		IP:         node.IP.String(),
		ListenAddr: srv.ListenAddr,
		Protocols:  make(map[string]interface{}),
	}
	info.Ports.Discovery = int(node.UDP)
	info.Ports.Listener = int(node.TCP)
	//收集所有正在运行的协议信息（每种协议类型只有一次）
	// Gather all the running protocol infos (only once per protocol type)
	for _, proto := range srv.Protocols {
		if _, ok := info.Protocols[proto.Name]; !ok {
			nodeInfo := interface{}("unknown")
			if query := proto.NodeInfo; query != nil {
				nodeInfo = proto.NodeInfo()
			}
			info.Protocols[proto.Name] = nodeInfo
		}
	}
	return info
}

// PeersInfo返回描述连接对等体的元数据对象数组。
// PeersInfo returns an array of metadata objects describing connected peers.
func (srv *Server) PeersInfo() []*PeerInfo {
	// Gather all the generic and sub-protocol specific infos
	//收集所有通用和子协议特定的信息
	infos := make([]*PeerInfo, 0, srv.PeerCount())
	for _, peer := range srv.Peers() {
		if peer != nil {
			infos = append(infos, peer.Info())
		}
	}
	//按照节点标识符按字母顺序排列结果数组
	// Sort the result array alphabetically by node identifier
	for i := 0; i < len(infos); i++ {
		for j := i + 1; j < len(infos); j++ {
			if infos[i].ID > infos[j].ID {
				infos[i], infos[j] = infos[j], infos[i]
			}
		}
	}
	return infos
}
