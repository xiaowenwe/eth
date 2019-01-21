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

package rpc

import (
	"bytes"
	"container/list"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ethereum/go-ethereum/log"
)

var (
	ErrClientQuit                = errors.New("client is closed")
	ErrNoResult                  = errors.New("no result in JSON-RPC response")
	ErrSubscriptionQueueOverflow = errors.New("subscription queue overflow")
)

const (
	// Timeouts
	tcpKeepAliveInterval = 30 * time.Second
	defaultDialTimeout   = 10 * time.Second // used when dialing if the context has no deadline如果上下文没有截止日期，则在拨号时使用
	defaultWriteTimeout  = 10 * time.Second // used for calls if the context has no deadline如果上下文没有截止日期，则用于调用
	subscribeTimeout     = 5 * time.Second  // overall timeout eth_subscribe, rpc_modules calls整体超时eth_subscribe，rpc_modules调用
)

const (
	//当订阅者无法跟上时，将删除订阅。
	//这可以通过提供具有足够大小的缓冲区的通道来解决，
	//但这可能很不方便，很难在文档中解释。 另一个问题 缓冲通道是缓冲区是静态的，即使它可能在大多数时间都不需要。
	//这里采用的方法是维护每个订阅链表缓冲区 按需收缩。 如果缓冲区达到以下大小，则会删除订阅。
	// Subscriptions are removed when the subscriber cannot keep up.
	//
	// This can be worked around by supplying a channel with sufficiently sized buffer,
	// but this can be inconvenient and hard to explain in the docs. Another issue with
	// buffered channels is that the buffer is static even though it might not be needed
	// most of the time.
	//
	// The approach taken here is to maintain a per-subscription linked list buffer
	// shrinks on demand. If the buffer reaches the size below, the subscription is
	// dropped.
	maxClientSubscriptionBuffer = 8000
)

// BatchElem是批处理请求中的元素。
// BatchElem is an element in a batch request.
type BatchElem struct {
	Method string
	Args   []interface{}
	//结果已解组到此字段中。 必须将结果设置为所需类型的非零指针值，否则将丢弃响应。
	// The result is unmarshaled into this field. Result must be set to a
	// non-nil pointer value of the desired type, otherwise the response will be
	// discarded.
	Result interface{}
	//如果服务器为此请求返回错误，或者如果解组为Result失败，则会设置错误。 它没有设置I / O错误。
	// Error is set if the server returns an error for this request, or if
	// unmarshaling into Result fails. It is not set for I/O errors.
	Error error
}

//此类型的值可以是JSON-RPC请求，通知，成功响应或错误响应。 它取决于哪个领域。
// A value of this type can a JSON-RPC request, notification, successful response or
// error response. Which one it is depends on the fields.
type jsonrpcMessage struct {
	Version string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Error   *jsonError      `json:"error,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
}

func (msg *jsonrpcMessage) isNotification() bool {
	return msg.ID == nil && msg.Method != ""
}

func (msg *jsonrpcMessage) isResponse() bool {
	return msg.hasValidID() && msg.Method == "" && len(msg.Params) == 0
}

func (msg *jsonrpcMessage) hasValidID() bool {
	return len(msg.ID) > 0 && msg.ID[0] != '{' && msg.ID[0] != '['
}

func (msg *jsonrpcMessage) String() string {
	b, _ := json.Marshal(msg)
	return string(b)
}

///Client表示到RPC服务器的连接。
// Client represents a connection to an RPC server.
type Client struct {
	idCounter   uint32
	connectFunc func(ctx context.Context) (net.Conn, error)
	isHTTP      bool
	//writeConn仅能安全地访问外部调度，并且/写锁保持不变。写锁是通过发送/requestOp获得的，通过发送sendDone释放的。
	// writeConn is only safe to access outside dispatch, with the
	// write lock held. The write lock is taken by sending on
	// requestOp and released by sending on sendDone.
	writeConn net.Conn

	// for dispatch
	close       chan struct{}
	didQuit     chan struct{}                  // closed when client quits客户退出时关闭
	reconnected chan net.Conn                  // where write/reconnect sends the new connection写入/重新连接发送新连接的地方
	readErr     chan error                     // errors from read读错误
	readResp    chan []*jsonrpcMessage         // valid messages from read读取的有效消息
	requestOp   chan *requestOp                // for registering response IDs用于注册响应ID
	sendDone    chan error                     // signals write completion, releases write lock信号写入完成，释放写锁
	respWait    map[string]*requestOp          // active requests主动请求
	subs        map[string]*ClientSubscription // active subscriptions活动订阅
}

//请求参数
type requestOp struct {
	ids  []json.RawMessage
	err  error
	resp chan *jsonrpcMessage // receives up to len(ids) responses接收最多len（ids）响应
	sub  *ClientSubscription  // only set for EthSubscribe requests仅为EthSubscribe请求设置
}

//获取rpc服务响应结果
func (op *requestOp) wait(ctx context.Context) (*jsonrpcMessage, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case resp := <-op.resp:
		return resp, op.err
	}
}

// Dial为给定的URL创建一个新客户端。
//当前支持的URL方案是“http”，“https”，“ws”和“wss”。 如果rawurl是
//没有URL方案的文件名，使用UNIX建立本地套接字连接
// Windows上支持的平台和命名管道上的域套接字。 如果你想
//配置传输选项，改为使用DialHTTP，DialWebsocket或DialIPC。
//对于websocket连接，原点设置为本地主机名。
//如果连接丢失，客户端将自动重新连接。
// Dial creates a new client for the given URL.
//
// The currently supported URL schemes are "http", "https", "ws" and "wss". If rawurl is a
// file name with no URL scheme, a local socket connection is established using UNIX
// domain sockets on supported platforms and named pipes on Windows. If you want to
// configure transport options, use DialHTTP, DialWebsocket or DialIPC instead.
//
// For websocket connections, the origin is set to the local host name.
//
// The client reconnects automatically if the connection is lost.
func Dial(rawurl string) (*Client, error) {
	return DialContext(context.Background(), rawurl)
}

// DialContext创建一个新的RPC客户端，就像Dial一样。
//上下文用于取消或超时初始连接建立。 它不会影响与客户端的后续交互。
// DialContext creates a new RPC client, just like Dial.
//
// The context is used to cancel or time out the initial connection establishment. It does
// not affect subsequent interactions with the client.
func DialContext(ctx context.Context, rawurl string) (*Client, error) {
	u, err := url.Parse(rawurl)
	if err != nil {
		return nil, err
	}
	switch u.Scheme {
	case "http", "https":
		return DialHTTP(rawurl)
	case "ws", "wss":
		return DialWebsocket(ctx, rawurl, "")
	case "":
		return DialIPC(ctx, rawurl)
	default:
		return nil, fmt.Errorf("no known transport for URL scheme %q", u.Scheme)
	}
}

//创建rpc客户端
func newClient(initctx context.Context, connectFunc func(context.Context) (net.Conn, error)) (*Client, error) {
	conn, err := connectFunc(initctx)
	if err != nil {
		return nil, err
	}
	_, isHTTP := conn.(*httpConn)

	c := &Client{
		writeConn:   conn,
		isHTTP:      isHTTP,
		connectFunc: connectFunc,
		close:       make(chan struct{}),
		didQuit:     make(chan struct{}),
		reconnected: make(chan net.Conn),
		readErr:     make(chan error),
		readResp:    make(chan []*jsonrpcMessage),
		requestOp:   make(chan *requestOp),
		sendDone:    make(chan error, 1),
		respWait:    make(map[string]*requestOp),
		subs:        make(map[string]*ClientSubscription),
	}
	if !isHTTP {
		go c.dispatch(conn)
	}
	return c, nil
}

func (c *Client) nextID() json.RawMessage {
	id := atomic.AddUint32(&c.idCounter, 1)
	return []byte(strconv.FormatUint(uint64(id), 10))
}

// SupportedModules调用rpc_modules方法，检索列表
//服务器上可用的API。
// SupportedModules calls the rpc_modules method, retrieving the list of
// APIs that are available on the server.
func (c *Client) SupportedModules() (map[string]string, error) {
	var result map[string]string
	ctx, cancel := context.WithTimeout(context.Background(), subscribeTimeout)
	defer cancel()
	err := c.CallContext(ctx, &result, "rpc_modules")
	return result, err
}

//关闭关闭客户端，中止任何正在进行的请求。
// Close closes the client, aborting any in-flight requests.
func (c *Client) Close() {
	if c.isHTTP {
		return
	}
	select {
	case c.close <- struct{}{}:
		<-c.didQuit
	case <-c.didQuit:
	}
}

//Call使用给定的参数执行JSON-RPC调用，如果没有发生错误，则将其解封为/结果。/结果必须是指针，以便包json可以解组到其中。您/也可以传递零，在这种情况下，结果将被忽略。
// Call performs a JSON-RPC call with the given arguments and unmarshals into
// result if no error occurred.
//
// The result must be a pointer so that package json can unmarshal into it. You
// can also pass nil, in which case the result is ignored.
func (c *Client) Call(result interface{}, method string, args ...interface{}) error {
	ctx := context.Background()
	return c.CallContext(ctx, result, method, args...)
}

// CallContext使用给定的参数执行JSON-RPC调用。 如果是上下文
//在调用成功返回之前取消，CallContext立即返回。
//结果必须是一个指针，以便包json可以解组。 您
//也可以传递nil，在这种情况下会忽略结果。
// CallContext performs a JSON-RPC call with the given arguments. If the context is
// canceled before the call has successfully returned, CallContext returns immediately.
//
// The result must be a pointer so that package json can unmarshal into it. You
// can also pass nil, in which case the result is ignored.
func (c *Client) CallContext(ctx context.Context, result interface{}, method string, args ...interface{}) error {
	msg, err := c.newMessage(method, args...)
	if err != nil { // 结果处理
		return err
	}
	// requestOp又一个结构体，封装响应参数的，包括原始请求消息，响应信息jsonrpcMessage，jsonrpcMessage也是一个结构体，
	// 封装了响应消息标准内容结构，包括版本，ID，方法，参数，错误，返回值，其中RawMessage在go源码位置json/stream.go又是一个自定义类型，属于go本身封装好的，类型是字节数组[]byte，也有自己的各种功能的方法。
	op := &requestOp{ids: []json.RawMessage{msg.ID}, resp: make(chan *jsonrpcMessage, 1)}
	// 通过rpc不同的渠道发送响应消息：这些渠道在上面命令部分已经介绍过，有HTTP，WebService等。
	if c.isHTTP {
		err = c.sendHTTP(ctx, op, msg)
	} else {
		err = c.send(ctx, op, msg)
	}
	if err != nil {
		return err
	}
	//对wait方法的研究
	// 对wait方法返回结果的处理
	// dispatch has accepted the request and will close the channel it when it quits.调度已接受该请求，并在退出时将其关闭。
	switch resp, err := op.wait(ctx); {
	case err != nil:
		return err
	case resp.Error != nil:
		return resp.Error
	case len(resp.Result) == 0:
		return ErrNoResult
	default:
		return json.Unmarshal(resp.Result, &result)
	}
}

// BatchCall将所有给定的请求作为单个批处理发送，并等待服务器
//为所有人返回一个回复。
//与Call相反，BatchCall仅返回I / O错误。 任何特定的错误
//通过相应BatchElem的Error字段报告请求。
//请注意，批处理调用可能无法在服务器端以原子方式执行。
// BatchCall sends all given requests as a single batch and waits for the server
// to return a response for all of them.
//
// In contrast to Call, BatchCall only returns I/O errors. Any error specific to
// a request is reported through the Error field of the corresponding BatchElem.
//
// Note that batch calls may not be executed atomically on the server side.
func (c *Client) BatchCall(b []BatchElem) error {
	ctx := context.Background()
	return c.BatchCallContext(ctx, b)
}

// BatchCall将所有给定的请求作为单个批处理发送，并等待服务器
//为所有人返回一个回复。 等待时间受限于
//上下文的截止日期。
//与CallContext相反，BatchCallContext仅返回已发生的错误
//发送请求时 通过以下方式报告特定于请求的任何错误
//相应BatchElem的错误字段。
//请注意，批处理调用可能无法在服务器端以原子方式执行。
// BatchCall sends all given requests as a single batch and waits for the server
// to return a response for all of them. The wait duration is bounded by the
// context's deadline.
//
// In contrast to CallContext, BatchCallContext only returns errors that have occurred
// while sending the request. Any error specific to a request is reported through the
// Error field of the corresponding BatchElem.
//
// Note that batch calls may not be executed atomically on the server side.
func (c *Client) BatchCallContext(ctx context.Context, b []BatchElem) error {
	msgs := make([]*jsonrpcMessage, len(b))
	op := &requestOp{
		ids:  make([]json.RawMessage, len(b)),
		resp: make(chan *jsonrpcMessage, len(b)),
	}
	for i, elem := range b {
		msg, err := c.newMessage(elem.Method, elem.Args...)
		if err != nil {
			return err
		}
		msgs[i] = msg
		op.ids[i] = msg.ID
	}

	var err error
	if c.isHTTP {
		err = c.sendBatchHTTP(ctx, op, msgs)
	} else {
		err = c.send(ctx, op, msgs)
	}

	// Wait for all responses to come back.
	for n := 0; n < len(b) && err == nil; n++ {
		var resp *jsonrpcMessage
		resp, err = op.wait(ctx)
		if err != nil {
			break
		}
		//找到与此响应对应的元素。
		//保证元素存在，因为调度
		//仅向我们的频道发送有效的ID。
		// Find the element corresponding to this response.
		// The element is guaranteed to be present because dispatch
		// only sends valid IDs to our channel.
		var elem *BatchElem
		for i := range msgs {
			if bytes.Equal(msgs[i].ID, resp.ID) {
				elem = &b[i]
				break
			}
		}
		if resp.Error != nil {
			elem.Error = resp.Error
			continue
		}
		if len(resp.Result) == 0 {
			elem.Error = ErrNoResult
			continue
		}
		elem.Error = json.Unmarshal(resp.Result, elem.Result)
	}
	return err
}

// Eth Subscribe在“eth”命名空间下注册订阅。
// EthSubscribe registers a subscripion under the "eth" namespace.
func (c *Client) EthSubscribe(ctx context.Context, channel interface{}, args ...interface{}) (*ClientSubscription, error) {
	return c.Subscribe(ctx, "eth", channel, args...)
}

// Shh Subscribe在“shh”命名空间下注册订阅。
// ShhSubscribe registers a subscripion under the "shh" namespace.
func (c *Client) ShhSubscribe(ctx context.Context, channel interface{}, args ...interface{}) (*ClientSubscription, error) {
	return c.Subscribe(ctx, "shh", channel, args...)
}

//订阅使用给定的参数调用“<namespace> _subscribe”方法，
//注册订阅 订阅的服务器通知是
//发送到指定频道。 通道的元素类型必须匹配
//订阅返回的预期内容类型。
// context参数取消设置订阅的RPC请求，但在Subscribe返回后对订阅没有影响。
//最终将放弃缓慢的订阅者。 客户端最多可缓冲8000个通知
//在考虑用户死亡之前 订阅Err频道将收到
// ErrSubscriptionQueueOverflow。 在通道上使用足够大的缓冲区或确保
//通道通常至少有一个阅读器来防止出现此问题。
// Subscribe calls the "<namespace>_subscribe" method with the given arguments,
// registering a subscription. Server notifications for the subscription are
// sent to the given channel. The element type of the channel must match the
// expected type of content returned by the subscription.
//
// The context argument cancels the RPC request that sets up the subscription but has no
// effect on the subscription after Subscribe has returned.
//
// Slow subscribers will be dropped eventually. Client buffers up to 8000 notifications
// before considering the subscriber dead. The subscription Err channel will receive
// ErrSubscriptionQueueOverflow. Use a sufficiently large buffer on the channel or ensure
// that the channel usually has at least one reader to prevent this issue.
func (c *Client) Subscribe(ctx context.Context, namespace string, channel interface{}, args ...interface{}) (*ClientSubscription, error) {
	// Check type of channel first.
	chanVal := reflect.ValueOf(channel)
	if chanVal.Kind() != reflect.Chan || chanVal.Type().ChanDir()&reflect.SendDir == 0 {
		panic("first argument to Subscribe must be a writable channel")
	}
	if chanVal.IsNil() {
		panic("channel given to Subscribe must not be nil")
	}
	if c.isHTTP { //http不支持订阅
		return nil, ErrNotificationsUnsupported
	}

	msg, err := c.newMessage(namespace+subscribeMethodSuffix, args...)
	if err != nil {
		return nil, err
	}
	op := &requestOp{
		ids:  []json.RawMessage{msg.ID},
		resp: make(chan *jsonrpcMessage),
		sub:  newClientSubscription(c, namespace, chanVal),
	}
	//发送订阅请求。
	//响应的到达和有效性在sub.quit上发出信号。
	// Send the subscription request.
	// The arrival and validity of the response is signaled on sub.quit.
	if err := c.send(ctx, op, msg); err != nil {
		return nil, err
	}
	if _, err := op.wait(ctx); err != nil {
		return nil, err
	}
	return op.sub, nil
}

//封装请求消息
func (c *Client) newMessage(method string, paramsIn ...interface{}) (*jsonrpcMessage, error) {
	params, err := json.Marshal(paramsIn)
	if err != nil {
		return nil, err
	}
	return &jsonrpcMessage{Version: "2.0", ID: c.nextID(), Method: method, Params: params}, nil
}

//这时候请求被select阻塞住，直到c.requestOp receive到op，或者receive 到 ctx.Done()，或receive到 c.didQuit。c.requestOp拿到op
//，调用write方法把请求的内容写到conn通道去。然后发送给sendDone chan，client的dispactch方法会收到这个结果。
// send registers op with the dispatch loop, then sends msg on the connection.
// if sending fails, op is deregistered.
func (c *Client) send(ctx context.Context, op *requestOp, msg interface{}) error {
	select {
	case c.requestOp <- op:
		log.Trace("", "msg", log.Lazy{Fn: func() string {
			return fmt.Sprint("sending ", msg)
		}})
		err := c.write(ctx, msg)
		c.sendDone <- err
		return err
	case <-ctx.Done():
		//如果客户端过载或无法跟上，可能会发生这种情况
		//订阅通知。
		// This can happen if the client is overloaded or unable to keep up with
		// subscription notifications.
		return ctx.Err()
	case <-c.didQuit:
		return ErrClientQuit
	}
}

func (c *Client) write(ctx context.Context, msg interface{}) error {
	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(defaultWriteTimeout)
	}
	////上一次写入失败。 尝试建立新连接。
	// The previous write failed. Try to establish a new connection.
	if c.writeConn == nil {
		if err := c.reconnect(ctx); err != nil {
			return err
		}
	}
	c.writeConn.SetWriteDeadline(deadline)
	err := json.NewEncoder(c.writeConn).Encode(msg)
	if err != nil {
		c.writeConn = nil
	}
	return err
}

func (c *Client) reconnect(ctx context.Context) error {
	newconn, err := c.connectFunc(ctx)
	if err != nil {
		log.Trace(fmt.Sprintf("reconnect failed: %v", err))
		return err
	}
	select {
	case c.reconnected <- newconn:
		c.writeConn = newconn
		return nil
	case <-c.didQuit:
		newconn.Close()
		return ErrClientQuit
	}
}

// dispatch是客户端的主循环。
//它将读取消息发送到对Call和BatchCall的等待调用以及对已注册订阅的订阅通知。
// dispatch is the main loop of the client.
// It sends read messages to waiting calls to Call and BatchCall
// and subscription notifications to registered subscriptions.
func (c *Client) dispatch(conn net.Conn) {
	// Spawn the initial read loop.
	go c.read(conn) //读server通过conn返回的数据

	var (
		lastOp        *requestOp    // tracks last send operation//跟踪上次发送操作
		requestOpLock = c.requestOp // nil while the send lock is held发送锁定时保持为零
		reading       = true        // if true, a read loop is running如果为true，则表示正在运行读循环
	)
	defer close(c.didQuit)
	defer func() {
		c.closeRequestOps(ErrClientQuit)
		conn.Close()
		if reading {
			// Empty read channels until read is dead.
			for {
				select {
				case <-c.readResp:
				case <-c.readErr:
					return
				}
			}
		}
	}()

	for {
		select {
		case <-c.close:
			return
		/*	然后把server返回数据send 到c.readResp chan。
			dispatch的 select  case batch := <-c.readResp: receive到c.readResp。如果这个请求的是通知
			，走通知的响应，否则走c.handleResponse(msg)*/
		// Read path.
		case batch := <-c.readResp: //读取到一个回应。调用相应的方法处理
			for _, msg := range batch {
				switch {
				case msg.isNotification():
					log.Trace("", "msg", log.Lazy{Fn: func() string {
						return fmt.Sprint("<-readResp: notification ", msg)
					}})
					c.handleNotification(msg)
				case msg.isResponse():
					log.Trace("", "msg", log.Lazy{Fn: func() string {
						return fmt.Sprint("<-readResp: response ", msg)
					}})
					c.handleResponse(msg)
				default:
					log.Debug("", "msg", log.Lazy{Fn: func() string {
						return fmt.Sprint("<-readResp: dropping weird message", msg)
					}})
					// TODO: maybe close
				}
			}

		case err := <-c.readErr: //接收到读取失败信息，这个是read线程传递过来的。
			log.Debug(fmt.Sprintf("<-readErr: %v", err))
			c.closeRequestOps(err)
			conn.Close()
			reading = false

		case newconn := <-c.reconnected: //接收到一个重连接信息
			log.Debug(fmt.Sprintf("<-reconnected: (reading=%t) %v", reading, conn.RemoteAddr()))
			if reading { //等待之前的连接读取完成。
				// Wait for the previous read loop to exit. This is a rare case.
				conn.Close()
				<-c.readErr
			} //开启阅读的goroutine
			go c.read(newconn)
			reading = true
			conn = newconn

		// Send path.
		case op := <-requestOpLock:
			//接收到一个requestOp消息，那么设置requestOpLock为空，
			//这个时候如果有其他人也希望发送op到requestOp，会因为没有人处理而阻塞。
			// Stop listening for further send ops until the current one is done.
			requestOpLock = nil
			lastOp = op
			for _, id := range op.ids {
				c.respWait[string(id)] = op
			}

		case err := <-c.sendDone: //当op的请求信息已经发送到网络上。会发送信息到sendDone。如果发送过程出错，那么err !=nil。
			if err != nil {
				//删除上次发送的响应处理程序。 我们在这里删除它们
				//因为错误已在Call或BatchCall中处理。 当。。。的时候
				//读取循环下降，它将发出所有其他当前操作的信号。
				// Remove response handlers for the last send. We remove those here
				// because the error is already handled in Call or BatchCall. When the
				// read loop goes down, it will signal all other current operations.
				//把所有的id从等待队列删除。
				for _, id := range lastOp.ids {
					delete(c.respWait, string(id))
				}
			}
			// Listen for send ops again.//重新开始处理requestOp的消息。
			requestOpLock = c.requestOp
			lastOp = nil
		}
	}
}

// closeRequestOps取消阻止待处理的发送操作和活动订阅。
// closeRequestOps unblocks pending send ops and active subscriptions.
func (c *Client) closeRequestOps(err error) {
	didClose := make(map[*requestOp]bool)

	for id, op := range c.respWait {
		// Remove the op so that later calls will not close op.resp again.
		delete(c.respWait, id)

		if !didClose[op] {
			op.err = err
			close(op.resp)
			didClose[op] = true
		}
	}
	for id, sub := range c.subs {
		delete(c.subs, id)
		sub.quitWithError(err, false)
	}
}

//处理通知
func (c *Client) handleNotification(msg *jsonrpcMessage) {
	if !strings.HasSuffix(msg.Method, notificationMethodSuffix) {
		log.Debug(fmt.Sprint("dropping non-subscription message: ", msg))
		return
	}
	var subResult struct {
		ID     string          `json:"subscription"`
		Result json.RawMessage `json:"result"`
	}
	if err := json.Unmarshal(msg.Params, &subResult); err != nil {
		log.Debug(fmt.Sprint("dropping invalid subscription message: ", msg))
		return
	}
	if c.subs[subResult.ID] != nil {
		c.subs[subResult.ID].deliver(subResult.Result)
	}
}

//处理响应
//这时候把返回数据send给op.resp <- msg。 后续处理和http RPC的处理一致，走到CallContext方法的 resp, err := op.wait(ctx)。
func (c *Client) handleResponse(msg *jsonrpcMessage) {
	op := c.respWait[string(msg.ID)]
	if op == nil {
		log.Debug(fmt.Sprintf("unsolicited response %v", msg))
		return
	}
	delete(c.respWait, string(msg.ID))
	//对于正常响应，只需将回复转发给Call / BatchCall。
	// For normal responses, just forward the reply to Call/BatchCall.
	if op.sub == nil {
		op.resp <- msg
		return
	}
	//对于订阅响应，如果服务器启动订阅
	//表示成功。 在任何一种情况下，EthSubscribe都会被解锁
	// op.resp频道
	// For subscription responses, start the subscription if the server
	// indicates success. EthSubscribe gets unblocked in either case through
	// the op.resp channel.
	defer close(op.resp)
	if msg.Error != nil {
		op.err = msg.Error
		return
	}
	if op.err = json.Unmarshal(msg.Result, &op.sub.subid); op.err == nil {
		go op.sub.start()
		c.subs[op.sub.subid] = op.sub
	}
}

// Reading happens on a dedicated goroutine.
//读server通过conn返回的数据
func (c *Client) read(conn net.Conn) error {
	var (
		buf json.RawMessage
		dec = json.NewDecoder(conn)
	)
	readMessage := func() (rs []*jsonrpcMessage, err error) {
		buf = buf[:0]
		if err = dec.Decode(&buf); err != nil {
			return nil, err
		}
		if isBatch(buf) {
			err = json.Unmarshal(buf, &rs)
		} else {
			rs = make([]*jsonrpcMessage, 1)
			err = json.Unmarshal(buf, &rs[0])
		}
		return rs, err
	}

	for {
		resp, err := readMessage()
		if err != nil {
			c.readErr <- err
			return err
		}
		c.readResp <- resp
	}
}

// Subscriptions.
// ClientSubscription表示通过EthSubscribe建立的订阅。
// A ClientSubscription represents a subscription established through EthSubscribe.
type ClientSubscription struct {
	client    *Client
	etype     reflect.Type
	channel   reflect.Value
	namespace string
	subid     string
	in        chan json.RawMessage

	quitOnce sync.Once     // ensures quit is closed once确保戒烟一次关闭
	quit     chan struct{} // quit is closed when the subscription exits订阅退出时退出
	errOnce  sync.Once     // ensures err is closed once确保错误关闭一次
	err      chan error
}

func newClientSubscription(c *Client, namespace string, channel reflect.Value) *ClientSubscription {
	sub := &ClientSubscription{
		client:    c,
		namespace: namespace,
		etype:     channel.Type().Elem(),
		channel:   channel,
		quit:      make(chan struct{}),
		err:       make(chan error, 1),
		in:        make(chan json.RawMessage),
	}
	return sub
}

// Err返回订阅错误频道。 Err的预期用途是安排
//意外关闭客户端连接时重新订阅
//错误通道在订阅结束时收到一个值
//错误 如果已调用Close，则收到的错误为nil
//在底层客户端上，没有发生其他错误。
//在订阅上调用Unsubscribe时，将关闭错误通道。
// Err returns the subscription error channel. The intended use of Err is to schedule
// resubscription when the client connection is closed unexpectedly.
//
// The error channel receives a value when the subscription has ended due
// to an error. The received error is nil if Close has been called
// on the underlying client and no other error has occurred.
//
// The error channel is closed when Unsubscribe is called on the subscription.
func (sub *ClientSubscription) Err() <-chan error {
	return sub.err
}

//取消订阅取消订阅通知并关闭错误频道。
//可以安全地多次调用它。
// Unsubscribe unsubscribes the notification and closes the error channel.
// It can safely be called more than once.
func (sub *ClientSubscription) Unsubscribe() {
	sub.quitWithError(nil, true)
	sub.errOnce.Do(func() { close(sub.err) })
}

func (sub *ClientSubscription) quitWithError(err error, unsubscribeServer bool) {
	sub.quitOnce.Do(func() {
		//调度循环将无法执行取消订阅调用
		//如果在交付时被阻止 首先关闭sub.quit因为它
		//取消阻止交付
		// The dispatch loop won't be able to execute the unsubscribe call
		// if it is blocked on deliver. Close sub.quit first because it
		// unblocks deliver.
		close(sub.quit)
		if unsubscribeServer {
			sub.requestUnsubscribe()
		}
		if err != nil {
			if err == ErrClientQuit {
				err = nil // Adhere to subscription semantics.
			}
			sub.err <- err
		}
	})
}

func (sub *ClientSubscription) deliver(result json.RawMessage) (ok bool) {
	select {
	case sub.in <- result:
		return true
	case <-sub.quit:
		return false
	}
}

func (sub *ClientSubscription) start() {
	sub.quitWithError(sub.forward())
}

func (sub *ClientSubscription) forward() (err error, unsubscribeServer bool) {
	cases := []reflect.SelectCase{
		{Dir: reflect.SelectRecv, Chan: reflect.ValueOf(sub.quit)},
		{Dir: reflect.SelectRecv, Chan: reflect.ValueOf(sub.in)},
		{Dir: reflect.SelectSend, Chan: sub.channel},
	}
	buffer := list.New()
	defer buffer.Init()
	for {
		var chosen int
		var recv reflect.Value
		if buffer.Len() == 0 {
			// Idle, omit send case.
			chosen, recv, _ = reflect.Select(cases[:2])
		} else {
			// Non-empty buffer, send the first queued item.
			cases[2].Send = reflect.ValueOf(buffer.Front().Value)
			chosen, recv, _ = reflect.Select(cases)
		}

		switch chosen {
		case 0: // <-sub.quit
			return nil, false
		case 1: // <-sub.in
			val, err := sub.unmarshal(recv.Interface().(json.RawMessage))
			if err != nil {
				return err, true
			}
			if buffer.Len() == maxClientSubscriptionBuffer {
				return ErrSubscriptionQueueOverflow, true
			}
			buffer.PushBack(val)
		case 2: // sub.channel<-
			cases[2].Send = reflect.Value{} // Don't hold onto the value.
			buffer.Remove(buffer.Front())
		}
	}
}

func (sub *ClientSubscription) unmarshal(result json.RawMessage) (interface{}, error) {
	val := reflect.New(sub.etype)
	err := json.Unmarshal(result, val.Interface())
	return val.Elem().Interface(), err
}

func (sub *ClientSubscription) requestUnsubscribe() error {
	var result interface{}
	return sub.client.Call(&result, sub.namespace+unsubscribeMethodSuffix, sub.subid)
}
