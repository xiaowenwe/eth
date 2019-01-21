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

// Package accounts implements high level Ethereum account management.
package accounts

import (
	"math/big"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/event"
)

//一个账号是20个字节的数据。 URL是可选的字段。
// Account represents an Ethereum account located at a specific location defined
// by the optional URL field.
type Account struct {
	//20bytes长的地址变量外
	Address common.Address `json:"address"` // Ethereum account address derived from the key
	URL     URL            `json:"url"`     // Optional resource locator within a backend
	//URL，可以是网址，也可以是本地存储的路径+文件全名。在以网址形式存在时，URL.Scheme就是网络协议名，而作为本地存储文件时，URL.Scheme是字符串常量"keystore"。
}

//钱包。钱包应该是这里面最重要的一个接口了。 具体的钱包也是实现了这个接口。 钱包又有所谓的分层确定性钱包和普通钱包。
// Wallet represents a software or hardware wallet that might contain one or more
// accounts (derived from the same seed).
// Wallet 是指包含了一个或多个账户的软件钱包或者硬件钱包
type Wallet interface {
	// URL retrieves the canonical path under which this wallet is reachable. It is
	// user by upper layers to define a sorting order over all wallets from multiple
	// backends.
	// URL 用来获取这个钱包可以访问的规范路径。 它会被上层使用用来从所有的后端的钱包来排序。
	URL() URL

	// Status returns a textual status to aid the user in the current state of the
	// wallet. It also returns an error indicating any failure the wallet might have
	// encountered.
	// 用来返回一个文本值用来标识当前钱包的状态。 同时也会返回一个error用来标识钱包遇到的任何错误。
	Status() (string, error)

	// Open initializes access to a wallet instance. It is not meant to unlock or
	// decrypt account keys, rather simply to establish a connection to hardware
	// wallets and/or to access derivation seeds.
	//Open 初始化对钱包实例的访问。这个方法并不意味着解锁或者解密账户，而是简单地建立与硬件钱包的连接和/或访问衍生种子。
	// The passphrase parameter may or may not be used by the implementation of a
	// particular wallet instance. The reason there is no passwordless open method
	// is to strive towards a uniform wallet handling, oblivious to the different
	// backend providers.
	//passphrase参数可能在某些实现中并不需要。 没有提供一个无passphrase参数的Open方法的原因是为了提供一个统一的接口。
	// Please note, if you open a wallet, you must close it to release any allocated
	// resources (especially important when working with hardware wallets).
	//请注意，如果你open了一个钱包，你必须close它。不然有些资源可能没有释放。 特别是使用硬件钱包的时候需要特别注意。
	Open(passphrase string) error
	// Close 释放由Open方法占用的任何资源。
	// Close releases any resources held by an open wallet instance.
	Close() error

	// Accounts retrieves the list of signing accounts the wallet is currently aware
	// of. For hierarchical deterministic wallets, the list will not be exhaustive,
	// rather only contain the accounts explicitly pinned during account derivation.
	// Accounts用来获取钱包发现了账户列表。 对于分层次的钱包， 这个列表不会详尽的列出所有的账号， 而是只包含在帐户派生期间明确固定的帐户。

	Accounts() []Account
	// Contains 返回一个账号是否属于本钱包。
	// Contains returns whether an account is part of this particular wallet or not.
	Contains(account Account) bool

	// Derive attempts to explicitly derive a hierarchical deterministic account at
	// the specified derivation path. If requested, the derived account will be added
	// to the wallet's tracked account list.
	// Derive尝试在指定的派生路径上显式派生出分层确定性帐户。 如果pin为true，派生帐户将被添加到钱包的跟踪帐户列表中。
	Derive(path DerivationPath, pin bool) (Account, error)

	// SelfDerive sets a base account derivation path from which the wallet attempts
	// to discover non zero accounts and automatically add them to list of tracked
	// accounts.
	//SelfDerive设置一个基本帐户导出路径，从中钱包尝试发现非零帐户，并自动将其添加到跟踪帐户列表中。
	// Note, self derivaton will increment the last component of the specified path
	// opposed to decending into a child path to allow discovering accounts starting
	// from non zero components.
	//注意，SelfDerive将递增指定路径的最后一个组件，而不是下降到子路径，以允许从非零组件开始发现帐户。
	// You can disable automatic account discovery by calling SelfDerive with a nil
	// chain state reader.
	// 你可以通过传递一个nil的ChainStateReader来禁用自动账号发现。
	SelfDerive(base DerivationPath, chain ethereum.ChainStateReader)

	// SignHash requests the wallet to sign the given hash.
	//SignHash 请求钱包来给传入的hash进行签名。
	// It looks up the account specified either solely via its address contained within,
	// or optionally with the aid of any location metadata from the embedded URL field.
	//它可以通过其中包含的地址（或可选地借助嵌入式URL字段中的任何位置元数据）来查找指定的帐户。
	// If the wallet requires additional authentication to sign the request (e.g.
	// a password to decrypt the account, or a PIN code o verify the transaction),
	// an AuthNeededError instance will be returned, containing infos for the user
	// about which fields or actions are needed. The user may retry by providing
	// the needed details via SignHashWithPassphrase, or by other means (e.g. unlock
	// the account in a keystore).
	// 如果钱包需要额外的验证才能签名(比如说 需要密码来解锁账号， 或者是需要一个PIN 代码来验证交易。)
	// 会返回一个AuthNeededError的错误，里面包含了用户的信息，以及哪些字段或者操作需要提供。
	// 用户可以通过 SignHashWithPassphrase来签名或者通过其他手段(在keystore里面解锁账号)
	SignHash(account Account, hash []byte) ([]byte, error)
	//SignTx 请求钱包对指定的交易进行签名。
	// SignTx requests the wallet to sign the given transaction.
	//
	// It looks up the account specified either solely via its address contained within,
	// or optionally with the aid of any location metadata from the embedded URL field.
	//
	// If the wallet requires additional authentication to sign the request (e.g.
	// a password to decrypt the account, or a PIN code o verify the transaction),
	// an AuthNeededError instance will be returned, containing infos for the user
	// about which fields or actions are needed. The user may retry by providing
	// the needed details via SignTxWithPassphrase, or by other means (e.g. unlock
	// the account in a keystore).
	//它仅通过其中包含的地址查找指定的帐户，
	//或者可选地借助来自嵌入的URL字段的任何位置元数据。
	//如果钱包需要额外的验证来签署请求（例如，
	//解密账户密码或PIN码o验证交易），
	//将返回一个AuthNeededError实例，其中包含用户的信息
	//关于需要哪些字段或动作。 用户可以通过提供重试
	//通过SignTxWithPassphrase或通过其他方式（例如解锁）所需的详细信息
	//密钥库中的帐户）。
	SignTx(account Account, tx *types.Transaction, chainID *big.Int) (*types.Transaction, error)

	// SignHashWithPassphrase requests the wallet to sign the given hash with the
	// given passphrase as extra authentication information.
	//// SignHashWithPassphrase请求钱包使用给定的passphrase来签名给定的hash
	// It looks up the account specified either solely via its address contained within,
	// or optionally with the aid of any location metadata from the embedded URL field.
	//它只通过其中包含的地址查找指定的帐户，
	//或者可选地借助嵌入的URL字段中的任何位置元数据。
	SignHashWithPassphrase(account Account, passphrase string, hash []byte) ([]byte, error)

	// SignTxWithPassphrase requests the wallet to sign the given transaction, with the
	// given passphrase as extra authentication information.
	// SignHashWithPassphrase请求钱包使用给定的passphrase来签名给定的transaction
	// It looks up the account specified either solely via its address contained within,
	// or optionally with the aid of any location metadata from the embedded URL field.
	////它仅通过其中包含的地址查找指定的帐户，	或者可选地借助来自嵌入式URL字段的任何位置元数据。
	SignTxWithPassphrase(account Account, passphrase string, tx *types.Transaction, chainID *big.Int) (*types.Transaction, error)
}

// Backend is a "wallet provider" that may contain a batch of accounts they can
// sign transactions with and upon request, do so.
// Backend是一个钱包提供器。 可以包含一批账号。他们可以根据请求签署交易，这样做
type Backend interface {
	// Wallets retrieves the list of wallets the backend is currently aware of.
	// Wallets获取当前能够查找到的钱包
	// The returned wallets are not opened by default. For software HD wallets this
	// means that no base seeds are decrypted, and for hardware wallets that no actual
	// connection is established.
	//返回的钱包默认是没有打开的
	// The resulting wallet list will be sorted alphabetically based on its internal
	// URL assigned by the backend. Since wallets (especially hardware) may come and
	// go, the same wallet might appear at a different positions in the list during
	// subsequent retrievals.
	//所产生的钱包列表将根据后端分配的内部URL按字母顺序排序。 由于钱包（特别是硬件钱包）可能会打开和关闭，所以在随后的检索过程中，相同的钱包可能会出现在列表中的不同位置。
	Wallets() []Wallet
	// 订阅创建异步订阅，以便在后端检测到钱包的到达或离开时接收通知。
	// Subscribe creates an async subscription to receive notifications when the
	// backend detects the arrival or departure of a wallet.
	Subscribe(sink chan<- WalletEvent) event.Subscription
}

// WalletEventType表示钱包订阅子系统可以触发的不同事件类型。
// WalletEventType represents the different event types that can be fired by
// the wallet subscription subsystem.
type WalletEventType int

const ( //当通过USB或密钥库中的文件系统事件检测到新钱包时，会触发WalletArrived。
	// WalletArrived is fired when a new wallet is detected either via USB or via
	// a filesystem event in the keystore.
	WalletArrived WalletEventType = iota
	//成功打开钱包时会触发WalletOpened，目的是启动任何后台进程，例如自动密钥派生。
	// WalletOpened is fired when a wallet is successfully opened with the purpose
	// of starting any background processes such as automatic key derivation.
	WalletOpened
	//钱包丢了
	// WalletDropped
	WalletDropped
)

// WalletEvent是在检测到钱包到达或离开时由帐户后端触发的事件。
// WalletEvent is an event fired by an account backend when a wallet arrival or
// departure is detected.
type WalletEvent struct {
	Wallet Wallet          // Wallet instance arrived or departed钱包实例到达或离开
	Kind   WalletEventType // Event type that happened in the system系统中发生的事件类型
}
