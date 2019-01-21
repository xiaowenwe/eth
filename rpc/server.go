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
	"context"
	"fmt"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/ethereum/go-ethereum/log"
	"gopkg.in/fatih/set.v0"
)

const MetadataApi = "rpc"

// CodecOption指定此编解码器支持的消息类型
// CodecOption specifies which type of messages this codec supports
type CodecOption int

const (
	//OptionMethodInvocation表示编解码器支持RPC方法调用
	// OptionMethodInvocation is an indication that the codec supports RPC method calls
	OptionMethodInvocation CodecOption = 1 << iota
	// OptionSubscriptions表示编解码器支持RPC通知
	// OptionSubscriptions is an indication that the codec suports RPC notifications
	OptionSubscriptions = 1 << iota // support pub sub
)

// NewServer将创建一个没有注册处理程序的新服务器实例。
// NewServer will create a new server instance with no registered handlers.
func NewServer() *Server {
	server := &Server{ //server先实例化
		services: make(serviceRegistry), //type 包括服务名，回调，类型，输入参数类型，方法的接收者
		codecs:   set.New(),             //用来存储所有的编码解码器，其实就是所有的连接。
		run:      1,                     //1,运行；非1不运行
	}
	//注册一个默认服务，它将提供有关RPC服务的元信息，例如它提供的服务和方法。
	// register a default service which will provide meta information about the RPC service such as the services and
	// methods it offers.
	rpcService := &RPCService{server}
	server.RegisterName(MetadataApi, rpcService)

	return server
}

// RPCService提供有关服务器的元信息。
//例如 提供有关加载模块的信息。
// RPCService gives meta information about the server.
// e.g. gives information about the loaded modules.
type RPCService struct { //这个是RPCService的定义结构
	server *Server //可以看出，该server是指针引用进来的
}

// Modules返回RPC服务列表及其版本号
// Modules returns the list of RPC services with their version number
func (s *RPCService) Modules() map[string]string {
	modules := make(map[string]string)
	for name := range s.server.services {
		modules[name] = "1.0"
	}
	return modules //其实只是返回每个service的名称和其版本号。。。
}

// RegisterName将为给定名称下的给定rcvr类型创建一个服务。 如果给定的rcvr上没有方法匹配条件是RPC方法或预订，则返回错误。
// 否则，会创建一个新服务并将其添加到此服务器实例所服务的服务集合中。
// RegisterName will create a service for the given rcvr type under the given name. When no methods on the given rcvr
// match the criteria to be either a RPC method or a subscription an error is returned. Otherwise a new service is
// created and added to the service collection this server instance serves.
func (s *Server) RegisterName(name string, rcvr interface{}) error {
	if s.services == nil {
		s.services = make(serviceRegistry) //其实只是返回每个service的名称和其版本号。。。
	}

	svc := new(service)
	svc.typ = reflect.TypeOf(rcvr)
	rcvrVal := reflect.ValueOf(rcvr)

	if name == "" {
		return fmt.Errorf("no service name for type %s", svc.typ.String())
	}
	if !isExported(reflect.Indirect(rcvrVal).Type().Name()) { //判断是否是公共服务
		return fmt.Errorf("%s is not exported", reflect.Indirect(rcvrVal).Type().Name())
	}

	methods, subscriptions := suitableCallbacks(rcvrVal, svc.typ) //返回方法和订阅的反射结构体包括输入方法参数和输出参数和错误
	//已经是给定sname下的先前服务注册，合并方法/订阅// 若services中已经有了该service，则直接合并方法和订阅
	// already a previous service register under given sname, merge methods/subscriptions
	if regsvc, present := s.services[name]; present {
		if len(methods) == 0 && len(subscriptions) == 0 {
			return fmt.Errorf("Service %T doesn't have any suitable methods/subscriptions to expose", rcvr)
		}
		for _, m := range methods {
			regsvc.callbacks[formatName(m.method.Name)] = m //添加方法到服务
		}
		for _, s := range subscriptions {
			regsvc.subscriptions[formatName(s.method.Name)] = s //添加订阅到服务
		}
		return nil
	}

	svc.name = name
	svc.callbacks, svc.subscriptions = methods, subscriptions

	if len(svc.callbacks) == 0 && len(svc.subscriptions) == 0 {
		return fmt.Errorf("Service %T doesn't have any suitable methods/subscriptions to expose", rcvr)
	}
	//把svc添加到services的map里   //根据名称存入服务
	s.services[svc.name] = svc
	return nil
}

/*当server启动 s.run的值就为1，直到server stop。
将codec add进s.codecs，codecs是一个set。
处理完请求数据，返回时需要从s.codecs remove 这个codec
对s.codecs的add 和 remove需要添加互斥锁，保证s.codecs的线程安全。*/
// serveRequest will reads requests from the codec, calls the RPC callback and
// writes the response to the given codec.
//
// If singleShot is true it will process a single request, otherwise it will handle
// requests until the codec returns an error when reading a request (in most cases
// an EOF). It executes requests in parallel when singleShot is false.
// serveRequest将从编解码器读取请求，调用RPC回调和
//将响应写入给定的编解码器。
//如果singleShot为true，它将处理单个请求，否则它将处理请求，直到编解码器在读取请求时返回错误（在大多数情况下为EOF）。 当singleShot为false时，它并行执行请求。
//c.e.Encode(res)会调用enc.w.Write(b)，这个w就是func (srv *Server) ServeHTTP(w http.ResponseWriter,
// r *http.Request)方法传入的http.ResponseWriter。借用这个writer来实现server和client的通信。
func (s *Server) serveRequest(codec ServerCodec, singleShot bool, options CodecOption) error {
	var pend sync.WaitGroup
	//结束时候的调用
	defer func() {
		if err := recover(); err != nil {
			const size = 64 << 10
			buf := make([]byte, size)
			buf = buf[:runtime.Stack(buf, false)] //// Stack将调用goroutine的堆栈跟踪格式化为buf，并返回写入buf的字节数。 如果全部为真，则在当前goroutine的跟踪之后，Stack格式化所有其他goroutine的跟踪到buf。
			log.Error(string(buf))
		}
		s.codecsMu.Lock()
		s.codecs.Remove(codec)
		s.codecsMu.Unlock()
	}()
	//context.Background() 返回一个空的Context，这个空的Context一般用于整个Context树的根节点。
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	//如果编解码器支持通知，则包含回调函数可用于向客户端发送通知的通知程序。
	// 这对编解码器/连接是很危险的。 如果连接关闭，通知程序将停止并取消所有活动的订阅
	// if the codec supports notification include a notifier that callbacks can use
	// to send notification to clients. It is thight to the codec/connection. If the
	// connection is closed the notifier will stop and cancels all active subscriptions.
	if options&OptionSubscriptions == OptionSubscriptions {
		ctx = context.WithValue(ctx, notifierKey{}, newNotifier(codec))
	}
	s.codecsMu.Lock()
	//接受一个*int32类型的指针值,并会返回该指针值指向的那个值
	if atomic.LoadInt32(&s.run) != 1 { // server stopped
		s.codecsMu.Unlock()
		return &shutdownError{}
	}
	s.codecs.Add(codec) //把请求加入集合
	s.codecsMu.Unlock()
	//测试服务器是否被命令停止
	// test if the server is ordered to stop
	for atomic.LoadInt32(&s.run) == 1 { //确保当前server没有停止
		reqs, batch, err := s.readRequest(codec) //处理请求的codec数据。
		if err != nil {
			// If a parsing error occurred, send an error如果发生解析错误，请发送错误
			if err.Error() != "EOF" {
				log.Debug(fmt.Sprintf("read error %v\n", err))
				codec.Write(codec.CreateErrorResponse(nil, err))
			}
			//错误或流结束，等待请求并拆除
			// Error or end of stream, wait for requests and tear down
			pend.Wait()
			return nil
		}
		//检查服务器是否被命令关闭并返回一个错误，告诉客户端他的请求失败。
		// check if server is ordered to shutdown and return an error
		// telling the client that his request failed.
		if atomic.LoadInt32(&s.run) != 1 {
			err = &shutdownError{}
			if batch {
				resps := make([]interface{}, len(reqs))
				for i, r := range reqs {
					resps[i] = codec.CreateErrorResponse(&r.id, err)
				}
				codec.Write(resps)
			} else {
				codec.Write(codec.CreateErrorResponse(&reqs[0].id, err))
			}
			return nil
		}
		//如果单个请求正在执行，请立即运行并返回
		// If a single shot request is executing, run and return immediately
		if singleShot { //非并发
			if batch {
				s.execBatch(ctx, codec, reqs) //批处理请求
			} else {
				s.exec(ctx, codec, reqs[0])
			}
			return nil
		}
		//对于多镜头连接，启动一个goroutine来服务并循环回去
		// For multi-shot connections, start a goroutine to serve and loop back
		pend.Add(1) //添加一个阻塞任务

		go func(reqs []*serverRequest, batch bool) {
			defer pend.Done()
			if batch {
				s.execBatch(ctx, codec, reqs) //这个批处理请求
			} else {
				s.exec(ctx, codec, reqs[0]) //处理单个请求
			}
		}(reqs, batch)
	}
	return nil
}

// ServeCodec从编解码器读取传入请求，调用相应的回调并使用给定的编解码器将响应写回。
// 它会阻止，直到编解码器关闭或服务器停止。 无论哪种情况，编解码器都会关闭。订阅消息，不会自动关闭
// ServeCodec reads incoming requests from codec, calls the appropriate callback and writes the
// response back using the given codec. It will block until the codec is closed or the server is
// stopped. In either case the codec is closed.
func (s *Server) ServeCodec(codec ServerCodec, options CodecOption) {
	defer codec.Close()
	s.serveRequest(codec, false, options)
}

//这个方法和上面的那个方法刚好相反，是同步处理请求的，等处理结束后，整个过程才会结束。此结束不提供codec.Close()方法，不用想也该明白，同步结束了，后面该干嘛就干嘛。
// ServeSingleRequest从给定的编解码器中读取和处理单个RPC请求。 除非发生不可恢复的错误，否则它不会关闭编解码器。 注意，此方法将在处理单个请求后返回！
// ServeSingleRequest reads and processes a single RPC request from the given codec. It will not
// close the codec unless a non-recoverable error has occurred. Note, this method will return after
// a single request has been processed!
/*参数codec中存储的是客户端发来的请求，经过处理后，会将响应结果写入codec中并返回给客户端。
该方法处理完codec中的内容后，会调用codec.Close()接口方法，处理请求结束时候的一些操作。
注意，看s.serveRequest(codec, false, options)，里面的false表示该方法是并发处理请求的*/
func (s *Server) ServeSingleRequest(codec ServerCodec, options CodecOption) {
	s.serveRequest(codec, true, options)
}

// Stop将停止读取新请求，等待stopPendingRequestTimeout允许挂起的请求完成，
//关闭所有将取消待处理请求/订阅的编解码器。
// Stop will stop reading new requests, wait for stopPendingRequestTimeout to allow pending requests to finish,
// close all codecs which will cancel pending requests/subscriptions.
func (s *Server) Stop() {
	if atomic.CompareAndSwapInt32(&s.run, 1, 0) {
		log.Debug("RPC Server shutdown initiatied")
		s.codecsMu.Lock()
		defer s.codecsMu.Unlock()
		s.codecs.Each(func(c interface{}) bool {
			c.(ServerCodec).Close()
			return true
		})
	}
}

// createSubscription将调用订阅回调并返回订阅ID或错误。
// createSubscription will call the subscription callback and returns the subscription id or error.
func (s *Server) createSubscription(ctx context.Context, c ServerCodec, req *serverRequest) (ID, error) {
	//订阅将第一个参数作为可选参数后面的上下文
	// subscription have as first argument the context following optional arguments
	args := []reflect.Value{req.callb.rcvr, reflect.ValueOf(ctx)}
	args = append(args, req.args...)
	reply := req.callb.method.Func.Call(args) //调用方法

	if !reply[1].IsNil() { // subscription creation failed
		return "", reply[1].Interface().(error)
	}

	return reply[0].Interface().(*Subscription).ID, nil
}

//跳过对订阅和取消订阅的请求处理。
//reply := req.callb.method.Func.Call(arguments) 执行了RPC方法并返回结果reply。
//codec.CreateResponse(req.id, reply[0].Interface())是rpc.json.go对返回结果的封装。
//回到exec(ctx context.Context, codec ServerCodec, req *serverRequest)方法。codec.Write(response)对返回结果json序列化。
//如果请求方法是订阅执行有回调callback()。
// handle executes a request and returns the response from the callback.
func (s *Server) handle(ctx context.Context, codec ServerCodec, req *serverRequest) (interface{}, func()) {
	if req.err != nil {
		return codec.CreateErrorResponse(&req.id, req.err), nil
	}

	if req.isUnsubscribe { // cancel subscription, first param must be the subscription id//取消订阅，第一个参数必须是订阅ID
		if len(req.args) >= 1 && req.args[0].Kind() == reflect.String {
			notifier, supported := NotifierFromContext(ctx) //读取ctx中的通知
			if !supported {                                 // interface doesn't support subscriptions (e.g. http)
				return codec.CreateErrorResponse(&req.id, &callbackError{ErrNotificationsUnsupported.Error()}), nil
			}

			subid := ID(req.args[0].String())
			if err := notifier.unsubscribe(subid); err != nil {
				return codec.CreateErrorResponse(&req.id, &callbackError{err.Error()}), nil
			}

			return codec.CreateResponse(req.id, true), nil
		}
		return codec.CreateErrorResponse(&req.id, &invalidParamsError{"Expected subscription id as first argument"}), nil
	}
	//如果是订阅消息。 那么创建订阅。并激活订阅
	if req.callb.isSubscribe {
		subid, err := s.createSubscription(ctx, codec, req)
		if err != nil {
			return codec.CreateErrorResponse(&req.id, &callbackError{err.Error()}), nil
		}

		// active the subscription after the sub id was successfully sent to the client//在子ID成功发送到客户端后激活订阅
		activateSub := func() {
			notifier, _ := NotifierFromContext(ctx)
			notifier.activate(subid, req.svcname)
		}

		return codec.CreateResponse(req.id, subid), activateSub
	}
	//常规RPC调用，准备参数
	// regular RPC call, prepare arguments
	if len(req.args) != len(req.callb.argTypes) {
		rpcErr := &invalidParamsError{fmt.Sprintf("%s%s%s expects %d parameters, got %d",
			req.svcname, serviceMethodSeparator, req.callb.method.Name,
			len(req.callb.argTypes), len(req.args))}
		return codec.CreateErrorResponse(&req.id, rpcErr), nil
	}

	arguments := []reflect.Value{req.callb.rcvr}
	if req.callb.hasCtx {
		arguments = append(arguments, reflect.ValueOf(ctx))
	}
	if len(req.args) > 0 {
		arguments = append(arguments, req.args...)
	}

	// execute RPC method and return result/执行RPC方法并返回结果
	reply := req.callb.method.Func.Call(arguments)
	if len(reply) == 0 {
		return codec.CreateResponse(req.id, nil), nil
	}

	if req.callb.errPos >= 0 { // test if method returned an error//测试方法是否返回错误
		if !reply[req.callb.errPos].IsNil() {
			e := reply[req.callb.errPos].Interface().(error)
			res := codec.CreateErrorResponse(&req.id, &callbackError{e.Error()})
			return res, nil
		}
	}
	return codec.CreateResponse(req.id, reply[0].Interface()), nil
}

// exec executes the given request and writes the result back using the codec.// exec执行给定的请求并使用编解码器将结果写回。
func (s *Server) exec(ctx context.Context, codec ServerCodec, req *serverRequest) {
	var response interface{}
	var callback func()
	if req.err != nil {
		response = codec.CreateErrorResponse(&req.id, req.err)
	} else {
		response, callback = s.handle(ctx, codec, req)
	}

	if err := codec.Write(response); err != nil {
		log.Error(fmt.Sprintf("%v\n", err))
		codec.Close()
	}
	//当请求是订阅者请求时，这允许激活这些订阅
	// when request was a subscribe request this allows these subscriptions to be actived
	if callback != nil {
		callback()
	}
}

// execBatch执行给定的请求，并使用编解码器将结果写回。
//它只会在处理完最后一个请求时写回响应。
// execBatch executes the given requests and writes the result back using the codec.
// It will only write the response back when the last request is processed.
func (s *Server) execBatch(ctx context.Context, codec ServerCodec, requests []*serverRequest) {
	responses := make([]interface{}, len(requests))
	var callbacks []func()
	for i, req := range requests {
		if req.err != nil {
			responses[i] = codec.CreateErrorResponse(&req.id, req.err)
		} else {
			var callback func()
			if responses[i], callback = s.handle(ctx, codec, req); callback != nil {
				callbacks = append(callbacks, callback)
			}
		}
	}

	if err := codec.Write(responses); err != nil {
		log.Error(fmt.Sprintf("%v\n", err))
		codec.Close()
	}
	//当请求包含一个或多个订阅请求时，这允许激活这些订阅
	// when request holds one of more subscribe requests this allows these subscriptions to be activated
	for _, c := range callbacks {
		c()
	}
}

// readRequest从编解码器请求下一个（批处理）请求。 它将返回请求集合，指示请求是否是批处理，无效请求标识符以及无法读取/解析请求时的错误。
// readRequest requests the next (batch) request from the codec. It will return the collection
// of requests, an indication if the request was a batch, the invalid request identifier and an
// error when the request could not be read/parsed.
//该方法解析并读取有效的客户端请求，会区分哪些是方法，哪些是订阅，根据不同状况，将这些信息都组装到requests[]中；
func (s *Server) readRequest(codec ServerCodec) ([]*serverRequest, bool, Error) {
	reqs, batch, err := codec.ReadRequestHeaders() //codec.ReadRequestHeaders()解析了请求数据
	if err != nil {
		return nil, batch, err
	}

	requests := make([]*serverRequest, len(reqs))

	// verify requests//验证请求
	for i, r := range reqs {
		var ok bool
		var svc *service

		if r.err != nil {
			requests[i] = &serverRequest{id: r.id, err: r.err}
			continue
		}

		if r.isPubSub && strings.HasSuffix(r.method, unsubscribeMethodSuffix) { //公共方法且是取消订阅
			requests[i] = &serverRequest{id: r.id, isUnsubscribe: true}
			argTypes := []reflect.Type{reflect.TypeOf("")}                                // expect subscription id as first arg期望订阅ID为第一个arg
			if args, err := codec.ParseRequestArguments(argTypes, r.params); err == nil { //根据注册的服务参数类型解析请求方法的参数值
				requests[i].args = args
			} else {
				requests[i].err = &invalidParamsError{err.Error()}
			}
			continue
		}

		if svc, ok = s.services[r.service]; !ok { // rpc method isn't availablerpc方法不可用，注册过的服务中找到rpc调用服务
			requests[i] = &serverRequest{id: r.id, err: &methodNotFoundError{r.service, r.method}}
			continue
		}

		if r.isPubSub { // eth_subscribe, r.method contains the subscription method name// eth_subscribe，r.method包含订阅方法名称
			if callb, ok := svc.subscriptions[r.method]; ok { //是否是订阅服务
				requests[i] = &serverRequest{id: r.id, svcname: svc.name, callb: callb}
				if r.params != nil && len(callb.argTypes) > 0 {
					argTypes := []reflect.Type{reflect.TypeOf("")}
					argTypes = append(argTypes, callb.argTypes...)
					if args, err := codec.ParseRequestArguments(argTypes, r.params); err == nil { //根据注册的服务参数类型解析请求方法的参数值
						requests[i].args = args[1:] // first one is service.method name which isn't an actual argument第一个是service.method名称，它不是实际的参数
					} else {
						requests[i].err = &invalidParamsError{err.Error()}
					}
				}
			} else {
				requests[i] = &serverRequest{id: r.id, err: &methodNotFoundError{r.service, r.method}}
			}
			continue
		}

		if callb, ok := svc.callbacks[r.method]; ok { // lookup RPC method查找RPC方法
			requests[i] = &serverRequest{id: r.id, svcname: svc.name, callb: callb}
			if r.params != nil && len(callb.argTypes) > 0 {
				if args, err := codec.ParseRequestArguments(callb.argTypes, r.params); err == nil { //根据注册的服务参数类型解析请求方法的参数值
					requests[i].args = args
				} else {
					requests[i].err = &invalidParamsError{err.Error()}
				}
			}
			continue
		}

		requests[i] = &serverRequest{id: r.id, err: &methodNotFoundError{r.service, r.method}}
	}

	return requests, batch, nil
}
