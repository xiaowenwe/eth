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

package node

import (
	"reflect"

	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/ethdb"
	"github.com/ethereum/go-ethereum/event"
	"github.com/ethereum/go-ethereum/p2p"
	"github.com/ethereum/go-ethereum/rpc"
)

// ServiceContext是继承自服务独立选项的集合
//协议栈，传递给所有构造函数以供选择使用;
//以及在服务环境中操作的实用方法。
// ServiceContext is a collection of service independent options inherited from
// the protocol stack, that is passed to all constructors to be optionally used;
// as well as utility methods to operate on the service environment.
type ServiceContext struct {
	config         *Config
	services       map[reflect.Type]Service // Index of the already constructed services//已经构建的服务的索引
	EventMux       *event.TypeMux           // Event multiplexer used for decoupled notifications//用于解耦通知的事件多路复用器
	AccountManager *accounts.Manager        // Account manager created by the node.//由节点创建的帐户管理器。
}

// OpenDatabase从节点的数据目录中打开一个具有给定名称的现有数据库（或者创建一个，如果前面没有找到的话）。 如果节点是短暂节点，则返回内存数据库。
// OpenDatabase opens an existing database with the given name (or creates one
// if no previous can be found) from within the node's data directory. If the
// node is an ephemeral one, a memory database is returned.
func (ctx *ServiceContext) OpenDatabase(name string, cache int, handles int) (ethdb.Database, error) {
	if ctx.config.DataDir == "" {
		return ethdb.NewMemDatabase()
	}
	db, err := ethdb.NewLDBDatabase(ctx.config.resolvePath(name), cache, handles)
	if err != nil {
		return nil, err
	}
	return db, nil
}

//如果用户路径是相对的并且用户实际使用持久性存储，则ResolvePath将用户路径解析到数据目录中。
// 它将为emphemeral存储返回一个空字符串，并为绝对路径输入用户自己的输入。
// ResolvePath resolves a user path into the data directory if that was relative
// and if the user actually uses persistent storage. It will return an empty string
// for emphemeral storage and the user's own input for absolute paths.
func (ctx *ServiceContext) ResolvePath(path string) string {
	return ctx.config.resolvePath(path)
}

// Service retrieves a currently running service registered of a specific type.
func (ctx *ServiceContext) Service(service interface{}) error {
	element := reflect.ValueOf(service).Elem()
	if running, ok := ctx.services[element.Type()]; ok {
		element.Set(reflect.ValueOf(running))
		return nil
	}
	return ErrServiceUnknown
}

// ServiceConstructor是构造函数所需的函数签名
//注册服务实例。
// ServiceConstructor is the function signature of the constructors needed to be
// registered for service instantiation.
type ServiceConstructor func(ctx *ServiceContext) (Service, error)

//服务是可以注册到节点的单独协议。
//注意：
//将服务生命周期管理委托给节点。 该服务被允许
//创建时自行初始化，但没有任何goroutines应该在创建之外旋转
//启动方法。
//•重新启动逻辑不是必需的，因为每次启动服务时，节点都会创建一个新的实例。
// Service is an individual protocol that can be registered into a node.
//
// Notes:
//
// • Service life-cycle management is delegated to the node. The service is allowed to
// initialize itself upon creation, but no goroutines should be spun up outside of the
// Start method.
//
// • Restart logic is not required as the node will create a fresh instance
// every time a service is started.
type Service interface {
	//协议检索服务希望启动的P2P协议。
	// Protocols retrieves the P2P protocols the service wishes to start.
	Protocols() []p2p.Protocol
	// API检索服务提供的RPC描述符列表
	// APIs retrieves the list of RPC descriptors the service provides
	APIs() []rpc.API
	//在所有服务已经构建完成之后调用Start，并且初始化网络层以产生服务所需的任何goroutines。
	// Start is called after all services have been constructed and the networking
	// layer was also initialized to spawn any goroutines required by the service.
	Start(server *p2p.Server) error
	//停止终止属于该服务的所有goroutines，阻塞直到它们全部终止。
	// Stop terminates all goroutines belonging to the service, blocking until they
	// are all terminated.
	Stop() error
}
