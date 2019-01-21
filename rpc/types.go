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

package rpc

import (
	"fmt"
	"math"
	"reflect"
	"strings"
	"sync"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"gopkg.in/fatih/set.v0"
)

// API描述了通过RPC接口提供的一组方法
// API describes the set of methods offered over the RPC interface
type API struct {
	Namespace string      // namespace under which the rpc methods of Service are exposed
	Version   string      // api version for DApp's
	Service   interface{} // receiver instance which holds the methods持有方法的接收器实例
	Public    bool        // indication if the methods must be considered safe for public use
}

func (a *API) String() string {
	return fmt.Sprintf(`--------- %v %v %v %v `, a.Namespace, a.Version, a.Service, a.Public)
}

//回调是在服务器中注册的方法回调
// callback is a method callback which was registered in the server
type callback struct {
	rcvr        reflect.Value  // receiver of method//方法的接收者，注册的服务如Service:   NewPublicAdminAPI(n),
	method      reflect.Method // callback
	argTypes    []reflect.Type // input argument types//输入参数类型
	hasCtx      bool           // method's first argument is a context (not included in argTypes)方法的第一个参数是一个上下文（不包含在argTypes中）
	errPos      int            // err return idx, of -1 when method cannot return errorerr返回idx，当方法不能返回错误时为-1
	isSubscribe bool           // indication if the callback is a subscription指示回调是否为订阅
}

//服务代表一个注册对象，注册的有哪些服务
// service represents a registered object
type service struct {
	name          string        // name for service服务名称如eth，admin
	typ           reflect.Type  // receiver type接收器类型
	callbacks     callbacks     // registered handlers注册处理程序
	subscriptions subscriptions // available subscriptions/notifications可用订阅/通知
}

// serverRequest是传入请求，具体调用参数
// serverRequest is an incoming request
type serverRequest struct {
	id            interface{}
	svcname       string //服务名，如eth，admin
	callb         *callback
	args          []reflect.Value
	isUnsubscribe bool
	err           Error
}

type serviceRegistry map[string]*service // collection of services服务的集合
type callbacks map[string]*callback      // collection of RPC callbacksRPC回调的集合
type subscriptions map[string]*callback  // collection of subscription callbacks订阅回调的集合
//服务器代表一个RPC服务器
// Server represents a RPC server
type Server struct {
	services serviceRegistry //用来存储service

	run      int32      //用来控制server运行还是停止，1为运行，非1为停止
	codecsMu sync.Mutex //用来保护多线程访问codecs的锁
	codecs   *set.Set   //用来存储所有的编码解码器，其实就是所有的连接。
}

// rpcRequest表示原始的传入RPC请求/json.go解析的rpc请求
// rpcRequest represents a raw incoming RPC request
type rpcRequest struct {
	service  string //服务名，如eth，admin
	method   string //方法
	id       interface{}
	isPubSub bool
	params   interface{} //调用参数
	err      Error       // invalid batch element
}

// Error wraps RPC errors, which contain an error code in addition to the message.
type Error interface {
	Error() string  // returns the message
	ErrorCode() int // returns the code
}

// ServerCodec实现读取，解析和编写RPC会话服务器端的RPC消息。
// 由于编解码器可以同时在多个go-routine中调用，因此实现必须安全地执行。
// ServerCodec implements reading, parsing and writing RPC messages for the server side of
// a RPC session. Implementations must be go-routine safe since the codec can be called in
// multiple go-routines concurrently.
type ServerCodec interface {
	// Read next request//读取下一个请求
	ReadRequestHeaders() ([]rpcRequest, bool, Error)
	// Parse request argument to the given types解析给定类型的请求参数
	ParseRequestArguments(argTypes []reflect.Type, params interface{}) ([]reflect.Value, Error)
	// Assemble success response, expects response id and payload组装成功响应，期望响应ID和有效负载
	CreateResponse(id interface{}, reply interface{}) interface{}
	// Assemble error response, expects response id and error组装错误响应，期望响应ID和错误
	CreateErrorResponse(id interface{}, err Error) interface{}
	// Assemble error response with extra information about the error through info通过信息汇总错误响应以及有关错误的额外信息
	CreateErrorResponseWithInfo(id interface{}, err Error, info interface{}) interface{}
	// Create notification response创建通知响应
	CreateNotification(id, namespace string, event interface{}) interface{}
	// Write msg to client.写信息给客户端。
	Write(msg interface{}) error
	// Close underlying data stream关闭底层数据流
	Close()
	// Closed when underlying connection is closed底层连接关闭时关闭
	Closed() <-chan interface{}
}

type BlockNumber int64

const (
	PendingBlockNumber  = BlockNumber(-2)
	LatestBlockNumber   = BlockNumber(-1)
	EarliestBlockNumber = BlockNumber(0)
)

// UnmarshalJSON将给定的JSON片段解析为BlockNumber。 它支持：
// - “latest”，“early”或“pending”作为字符串参数
// - 块号
//返回错误：
//当给定参数不是已知字符串时出现无效块号错误
//当给定的块号太小或太大时超出范围错误
// UnmarshalJSON parses the given JSON fragment into a BlockNumber. It supports:
// - "latest", "earliest" or "pending" as string arguments
// - the block number
// Returned errors:
// - an invalid block number error when the given argument isn't a known strings
// - an out of range error when the given block number is either too little or too large
func (bn *BlockNumber) UnmarshalJSON(data []byte) error {
	input := strings.TrimSpace(string(data))
	if len(input) >= 2 && input[0] == '"' && input[len(input)-1] == '"' {
		input = input[1 : len(input)-1]
	}

	switch input {
	case "earliest":
		*bn = EarliestBlockNumber
		return nil
	case "latest":
		*bn = LatestBlockNumber
		return nil
	case "pending":
		*bn = PendingBlockNumber
		return nil
	}

	blckNum, err := hexutil.DecodeUint64(input)
	if err != nil {
		return err
	}
	if blckNum > math.MaxInt64 {
		return fmt.Errorf("Blocknumber too high")
	}

	*bn = BlockNumber(blckNum)
	return nil
}

func (bn BlockNumber) Int64() int64 {
	return (int64)(bn)
}
