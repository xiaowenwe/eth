// Copyright 2017 The go-ethereum Authors
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

package accounts

import (
	"reflect"
	"sort"
	"sync"

	"github.com/ethereum/go-ethereum/event"
)

//Manager是一个包含所有东西的账户管理工具。 可以和所有的Backends来通信来签署交易
// Manager is an overarching account manager that can communicate with various
// backends for signing transactions.
type Manager struct {
	// 所有已经注册的Backend
	backends map[reflect.Type][]Backend // Index of backends currently registered
	// 所有Backend的更新订阅器
	updaters []event.Subscription
	// backend更新的订阅槽
	// Wallet update subscriptions for all backends
	updates chan WalletEvent // Subscription sink for backend wallet changes
	// 所有已经注册的Backends的钱包的缓存
	wallets []Wallet // Cache of all wallets from all registered backends
	// 钱包到达和离开的通知
	feed event.Feed // Wallet feed notifying of arrivals/departures
	// 退出队列
	quit chan chan error
	lock sync.RWMutex
}

// NewManager创建一个通用帐户管理器，通过各种支持的后端签署事务。
// NewManager creates a generic account manager to sign transaction via various
// supported backends.
//创建Manager
func NewManager(backends ...Backend) *Manager {
	// Retrieve the initial list of wallets from the backends and sort by URL//从后端检索钱包的初始列表并按URL排序
	var wallets []Wallet
	for _, backend := range backends {
		wallets = merge(wallets, backend.Wallets()...)
	}
	//订阅所有后端的钱包通知
	// Subscribe to wallet notifications from all backends
	//成员变量updates就是该Manager本身得到所订阅事件的通道
	updates := make(chan WalletEvent, 4*len(backends))

	subs := make([]event.Subscription, len(backends))
	for i, backend := range backends {
		subs[i] = backend.Subscribe(updates)
	}
	// Assemble the account manager and return//组装客户经理并返回
	am := &Manager{
		backends: make(map[reflect.Type][]Backend),
		updaters: subs,
		updates:  updates,
		wallets:  wallets,
		quit:     make(chan chan error),
	}
	for _, backend := range backends {
		kind := reflect.TypeOf(backend)
		am.backends[kind] = append(am.backends[kind], backend)
	}
	go am.update()

	return am
}

//关闭会终止客户经理的内部通知流程。
// Close terminates the account manager's internal notification processes.
func (am *Manager) Close() error {
	errc := make(chan error)
	am.quit <- errc
	return <-errc
}

//update方法。 是一个goroutine。会监听所有backend触发的更新信息。 然后转发给feed.
// update is the wallet event loop listening for notifications from the backends
// and updating the cache of wallets.
func (am *Manager) update() {
	// Close all subscriptions when the manager terminates//在管理器终止时关闭所有订阅
	defer func() {
		am.lock.Lock()
		for _, sub := range am.updaters {
			sub.Unsubscribe()
		}
		am.updaters = nil
		am.lock.Unlock()
	}()

	// Loop until termination//循环直到终止
	for {
		select {
		case event := <-am.updates:
			// Wallet event arrived, update local cache// Wallet事件到达，更新本地缓存
			am.lock.Lock()
			switch event.Kind {
			case WalletArrived:
				am.wallets = merge(am.wallets, event.Wallet)
			case WalletDropped:
				am.wallets = drop(am.wallets, event.Wallet)
			}
			am.lock.Unlock()

			// Notify any listeners of the event//通知事件的所有听众
			am.feed.Send(event)

		case errc := <-am.quit:
			// Manager terminating, return/经理终止，退货
			errc <- nil
			return
		}
	}
}

//返回backend
// Backends retrieves the backend(s) with the given type from the account manager.//后端从客户经理检索具有给定类型的后端。
func (am *Manager) Backends(kind reflect.Type) []Backend {
	return am.backends[kind]
}

// Wallets返回在此客户经理下注册的所有签名者帐户。
// Wallets returns all signer accounts registered under this account manager.
func (am *Manager) Wallets() []Wallet {
	am.lock.RLock()
	defer am.lock.RUnlock()

	cpy := make([]Wallet, len(am.wallets))
	copy(cpy, am.wallets)
	return cpy
}

// Wallet检索与特定URL关联的钱包。
// Wallet retrieves the wallet associated with a particular URL.
func (am *Manager) Wallet(url string) (Wallet, error) {
	am.lock.RLock()
	defer am.lock.RUnlock()

	parsed, err := parseURL(url)
	if err != nil {
		return nil, err
	}
	for _, wallet := range am.Wallets() {
		if wallet.URL() == parsed {
			return wallet, nil
		}
	}
	return nil, ErrUnknownWallet
}

//查找尝试查找与特定帐户对应的钱包。 由于可以在钱包中动态添加和删除帐户，因此此方法在钱包数量方面具有线性运行时。
// Find attempts to locate the wallet corresponding to a specific account. Since
// accounts can be dynamically added to and removed from wallets, this method has
// a linear runtime in the number of wallets.
func (am *Manager) Find(account Account) (Wallet, error) {
	am.lock.RLock()
	defer am.lock.RUnlock()

	for _, wallet := range am.wallets {
		if wallet.Contains(account) {
			return wallet, nil
		}
	}
	return nil, ErrUnknownAccount
}

//订阅消息。向某个Manager对象订阅Wallet更新事件的，正是另外一个Manager对象，也就是<Backend>的实现类。
// Subscribe creates an async subscription to receive notifications when the
// manager detects the arrival or departure of a wallet from any of its backends.
func (am *Manager) Subscribe(sink chan<- WalletEvent) event.Subscription {
	return am.feed.Subscribe(sink)
}

// merge是钱包追加的排序模拟，其中通过在正确的位置插入新钱包来保留原始列表的顺序。
//假设原始切片已按URL排序。
// merge is a sorted analogue of append for wallets, where the ordering of the
// origin list is preserved by inserting new wallets at the correct position.
//
// The original slice is assumed to be already sorted by URL.
func merge(slice []Wallet, wallets ...Wallet) []Wallet {
	for _, wallet := range wallets {
		n := sort.Search(len(slice), func(i int) bool { return slice[i].URL().Cmp(wallet.URL()) >= 0 })
		if n == len(slice) {
			slice = append(slice, wallet)
			continue
		}
		slice = append(slice[:n], append([]Wallet{wallet}, slice[n:]...)...)
	}
	return slice
}

// drop是merge的对应物，它从已排序的缓存中查找钱包并删除指定的钱包。
// drop is the couterpart of merge, which looks up wallets from within the sorted
// cache and removes the ones specified.
func drop(slice []Wallet, wallets ...Wallet) []Wallet {
	for _, wallet := range wallets {
		n := sort.Search(len(slice), func(i int) bool { return slice[i].URL().Cmp(wallet.URL()) >= 0 })
		if n == len(slice) {
			// Wallet not found, may happen during startup//找不到钱包，可能在启动时发生
			continue
		}
		slice = append(slice[:n], slice[n+1:]...)
	}
	return slice
}
