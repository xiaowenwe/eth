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

// Package keystore implements encrypted storage of secp256k1 private keys.
//
// Keys are stored as encrypted JSON files according to the Web3 Secret Storage specification.
// See https://github.com/ethereum/wiki/wiki/Web3-Secret-Storage-Definition for more information.
////KeyStore{}：它为keystoreWallet结构体提供所有与Account相关的实质性的数据和操作。KeyStore{}内部有两个作数据缓存用的成员：
//accountCache类型的成员cache，是所有待查找的地址信息(Account{}类型)集合；
//map[Address]unlocked{}形式的成员unlocked，由于unlocked{}结构体仅仅简单封装了Key{}对象(Key{}中显式含有数字签名公钥密钥对)，
// 所以map[]中可通过Address变量查找到该地址对应的原始公钥以及密钥。
//另外，KeyStore{}中有一个<keyStore>接口类型的成员storage，用来对存储在本地文件中的公钥信息Key做操作

package keystore

import (
	"crypto/ecdsa"
	crand "crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/event"
)

var (
	ErrLocked  = accounts.NewAuthNeededError("password or unlock")
	ErrNoMatch = errors.New("no key for given address or file")
	ErrDecrypt = errors.New("could not decrypt key with given passphrase")
)

// KeyStoreType是密钥库后端的反射类型。
// KeyStoreType is the reflect type of a keystore backend.
var KeyStoreType = reflect.TypeOf(&KeyStore{})

// KeyStoreScheme是为帐户和钱包URL添加前缀的协议方案。
// KeyStoreScheme is the protocol scheme prefixing account and wallet URLs.
var KeyStoreScheme = "keystore"

//钱包刷新之间的最长时间（如果文件系统通知不起作用）。
// Maximum time between wallet refreshes (if filesystem notifications don't work).
const walletRefreshCycle = 3 * time.Second

// KeyStore管理磁盘上的密钥存储目录。
// KeyStore manages a key storage directory on disk.
type KeyStore struct {
	storage keyStore      // Storage backend, might be cleartext or encrypted存储后端可能是明文或加密的
	cache   *accountCache // In-memory account cache over the filesystem storage内存帐户缓存在文件系统存储上
	changes chan struct{} // Channel receiving change notifications from the cache通道从缓存接收更改通知
	//公钥密钥数据类Key{}的封装类，其内部成员除了Key{}之外，还提供了一个chan类型变量abort，它会在KeyStore对于公钥密钥信息的管理机制中发挥作用。
	unlocked map[common.Address]*unlocked // Currently unlocked account (decrypted private keys)当前未锁定的帐户（解密的私钥）

	wallets     []accounts.Wallet       // Wallet wrappers around the individual key files围绕各个密钥文件的钱包包装
	updateFeed  event.Feed              // Event feed to notify wallet additions/removals用于通知钱包添加/删除的事件订阅源
	updateScope event.SubscriptionScope // Subscription scope tracking current live listeners订阅范围跟踪当前的实时监听器
	updating    bool                    // Whether the event notification loop is running事件通知循环是否正在运行

	mu sync.RWMutex
}

type unlocked struct {
	*Key
	abort chan struct{}
}

// NewKeyStore为给定目录创建密钥库。
// NewKeyStore creates a keystore for the given directory.
func NewKeyStore(keydir string, scryptN, scryptP int) *KeyStore {
	keydir, _ = filepath.Abs(keydir)
	ks := &KeyStore{storage: &keyStorePassphrase{keydir, scryptN, scryptP}}
	ks.init(keydir)
	return ks
}

// NewPlaintextKeyStore为给定目录创建密钥库。
//不推荐使用：使用NewKeyStore。
// NewPlaintextKeyStore creates a keystore for the given directory.
// Deprecated: Use NewKeyStore.
func NewPlaintextKeyStore(keydir string) *KeyStore {
	keydir, _ = filepath.Abs(keydir)
	ks := &KeyStore{storage: &keyStorePlain{keydir}}
	ks.init(keydir)
	return ks
}

func (ks *KeyStore) init(keydir string) {
	//锁定互斥锁，因为帐户缓存可能会回调事件
	// Lock the mutex since the account cache might call back with events
	ks.mu.Lock()
	defer ks.mu.Unlock()
	//初始化一组未锁定的密钥和帐户缓存
	// Initialize the set of unlocked keys and the account cache
	ks.unlocked = make(map[common.Address]*unlocked)
	ks.cache, ks.changes = newAccountCache(keydir)
	//到ks。 addressCache没有保留引用但是解锁的键没有，
	//所以在所有定时解锁都已过期之前，终结器不会触发。
	// TODO: In order for this finalizer to work, there must be no references
	// to ks. addressCache doesn't keep a reference but unlocked keys do,
	// so the finalizer will not trigger until all timed unlocks have expired.
	// TODO：为了使这个终结器工作，必须没有对ks的引用。 addressCache没有保留引用但是解锁键没有，所以在所有定时解锁都到期之前，终结器不会触发。
	runtime.SetFinalizer(ks, func(m *KeyStore) { //在垃圾回收之前执行的函数，关闭通知和定时器
		m.cache.close()
	}) //从缓存中创建钱包的初始列表
	// Create the initial list of wallets from the cache
	accs := ks.cache.accounts()
	ks.wallets = make([]accounts.Wallet, len(accs))
	for i := 0; i < len(accs); i++ {
		ks.wallets[i] = &keystoreWallet{account: accs[i], keystore: ks}
	}
}

// Wallets实现accounts.Backend，从keystore目录返回所有单键钱包。
// Wallets implements accounts.Backend, returning all single-key wallets from the
// keystore directory.
func (ks *KeyStore) Wallets() []accounts.Wallet {
	// Make sure the list of wallets is in sync with the account cache
	ks.refreshWallets()

	ks.mu.RLock()
	defer ks.mu.RUnlock()

	cpy := make([]accounts.Wallet, len(ks.wallets))
	copy(cpy, ks.wallets)
	return cpy
}

// refreshWallets检索当前帐户列表，并根据该列表进行任何必要的钱包刷新。
// refreshWallets retrieves the current account list and based on that does any
// necessary wallet refreshes.
func (ks *KeyStore) refreshWallets() {
	// Retrieve the current list of accounts检索当前的帐户列表
	ks.mu.Lock()
	accs := ks.cache.accounts()
	//将当前的钱包列表转换为新的钱包
	// Transform the current list of wallets into the new one
	wallets := make([]accounts.Wallet, 0, len(accs))
	events := []accounts.WalletEvent{}

	for _, account := range accs {
		// Drop wallets while they were in front of the next account//当他们在下一个帐户前面时丢弃钱包
		for len(ks.wallets) > 0 && ks.wallets[0].URL().Cmp(account.URL) < 0 {
			events = append(events, accounts.WalletEvent{Wallet: ks.wallets[0], Kind: accounts.WalletDropped})
			ks.wallets = ks.wallets[1:]
		}
		// If there are no more wallets or the account is before the next, wrap new wallet//如果没有钱包或帐户在下一个之前，请换新钱包
		if len(ks.wallets) == 0 || ks.wallets[0].URL().Cmp(account.URL) > 0 {
			wallet := &keystoreWallet{account: account, keystore: ks}

			events = append(events, accounts.WalletEvent{Wallet: wallet, Kind: accounts.WalletArrived})
			wallets = append(wallets, wallet)
			continue
		}
		// If the account is the same as the first wallet, keep it//如果帐户与第一个钱包相同，请保留该帐户
		if ks.wallets[0].Accounts()[0] == account {
			wallets = append(wallets, ks.wallets[0])
			ks.wallets = ks.wallets[1:]
			continue
		}
	} //删除任何剩余的钱包并设置新批次
	// Drop any leftover wallets and set the new batch
	for _, wallet := range ks.wallets {
		events = append(events, accounts.WalletEvent{Wallet: wallet, Kind: accounts.WalletDropped})
	}
	ks.wallets = wallets
	ks.mu.Unlock()
	//解雇所有钱包事件并返回
	// Fire all wallet events and return
	for _, event := range events {
		ks.updateFeed.Send(event)
	}
}

//订阅实现accounts.Backend，创建异步订阅以接收有关添加或删除密钥库钱包的通知。
// Subscribe implements accounts.Backend, creating an async subscription to
// receive notifications on the addition or removal of keystore wallets.
func (ks *KeyStore) Subscribe(sink chan<- accounts.WalletEvent) event.Subscription {
	// We need the mutex to reliably start/stop the update loop//我们需要互斥锁来可靠地启动/停止更新循环
	ks.mu.Lock()
	defer ks.mu.Unlock()

	// Subscribe the caller and track the subscriber count//订阅呼叫者并跟踪订户数量
	sub := ks.updateScope.Track(ks.updateFeed.Subscribe(sink))
	//订阅者需要一个活动的通知循环，启动它
	// Subscribers require an active notification loop, start it
	if !ks.updating {
		ks.updating = true
		go ks.updater()
	}
	return sub
}

// updater负责维护存储在密钥库中的最新钱包列表，以及用于解除钱包添加/删除事件。 它从底层帐户缓存中侦听帐户更改事件，并定期强制手动刷新（仅对文件系统通知程序未运行的系统触发）。
// updater is responsible for maintaining an up-to-date list of wallets stored in
// the keystore, and for firing wallet addition/removal events. It listens for
// account change events from the underlying account cache, and also periodically
// forces a manual refresh (only triggers for systems where the filesystem notifier
// is not running).
func (ks *KeyStore) updater() {
	for {
		// Wait for an account update or a refresh timeout等待帐户更新或刷新超时
		select {
		case <-ks.changes:
		case <-time.After(walletRefreshCycle):
		}
		// Run the wallet refresher//运行钱包刷新器
		ks.refreshWallets()

		// If all our subscribers left, stop the updater//如果我们的所有订阅者都离开了，请停止更新程序
		ks.mu.Lock()
		if ks.updateScope.Count() == 0 {
			ks.updating = false
			ks.mu.Unlock()
			return
		}
		ks.mu.Unlock()
	}
}

// HasAddress报告是否存在具有给定地址的密钥。
// HasAddress reports whether a key with the given address is present.
func (ks *KeyStore) HasAddress(addr common.Address) bool {
	return ks.cache.hasAddress(addr)
}

// Accounts返回目录中存在的所有密钥文件。
// Accounts returns all key files present in the directory.
func (ks *KeyStore) Accounts() []accounts.Account {
	return ks.cache.accounts()
}

//如果密码正确，则删除删除帐户匹配的密钥。
//如果帐户不包含文件名，则地址必须与唯一键匹配。
// Delete deletes the key matched by account if the passphrase is correct.
// If the account contains no filename, the address must match a unique key.
func (ks *KeyStore) Delete(a accounts.Account, passphrase string) error {
	// Decrypting the key isn't really necessary, but we do
	// it anyway to check the password and zero out the key
	// immediately afterwards.
	//解密密钥并不是必需的，但我们确实如此
	//无论如何，检查密码并立即将密钥归零。
	a, key, err := ks.getDecryptedKey(a, passphrase)
	if key != nil {
		zeroKey(key.PrivateKey)
	}
	if err != nil {
		return err
	} //订单在这里至关重要 文件消失后，密钥将从缓存中删除，以便在其间发生重新加载，不会再将其插入缓存中。
	// The order is crucial here. The key is dropped from the
	// cache after the file is gone so that a reload happening in
	// between won't insert it into the cache again.
	err = os.Remove(a.URL.Path)
	if err == nil {
		ks.cache.delete(a)
		ks.refreshWallets()
	}
	return err
}

// SignHash为给定的哈希计算ECDSA签名。 生成的签名位于[R || S || V]格式，其中V为0或1。
// SignHash calculates a ECDSA signature for the given hash. The produced
// signature is in the [R || S || V] format where V is 0 or 1.
func (ks *KeyStore) SignHash(a accounts.Account, hash []byte) ([]byte, error) {
	// Look up the key to sign with and abort if it cannot be found//查找要签名的密钥，如果找不到，则中止
	ks.mu.RLock()
	defer ks.mu.RUnlock()

	unlockedKey, found := ks.unlocked[a.Address]
	if !found {
		return nil, ErrLocked
	}
	// Sign the hash using plain ECDSA operations//使用普通ECDSA操作签署哈希
	return crypto.Sign(hash, unlockedKey.PrivateKey)
}

// SignTx用请求的账户签署给定的交易。
// SignTx signs the given transaction with the requested account.
func (ks *KeyStore) SignTx(a accounts.Account, tx *types.Transaction, chainID *big.Int) (*types.Transaction, error) {
	// Look up the key to sign with and abort if it cannot be found//查找要签名的密钥，如果找不到，则中止
	ks.mu.RLock()
	defer ks.mu.RUnlock()

	unlockedKey, found := ks.unlocked[a.Address]
	if !found {
		return nil, ErrLocked
	}
	// Depending on the presence of the chain ID, sign with EIP155 or homestead//根据链ID的存在，使用EIP155或宅基地签名
	if chainID != nil {
		return types.SignTx(tx, types.NewEIP155Signer(chainID), unlockedKey.PrivateKey)
	}
	return types.SignTx(tx, types.HomesteadSigner{}, unlockedKey.PrivateKey)
}

//如果匹配给定地址的私钥可以使用给定的密码解密，则SignHashWithPassphrase签名哈希。 制作的签名在
// [R || S || V]格式，其中V为0或1。
// SignHashWithPassphrase signs hash if the private key matching the given address
// can be decrypted with the given passphrase. The produced signature is in the
// [R || S || V] format where V is 0 or 1.
func (ks *KeyStore) SignHashWithPassphrase(a accounts.Account, passphrase string, hash []byte) (signature []byte, err error) {
	_, key, err := ks.getDecryptedKey(a, passphrase)
	if err != nil {
		return nil, err
	}
	defer zeroKey(key.PrivateKey)
	return crypto.Sign(hash, key.PrivateKey)
}

//如果可以使用给定的密码短语解密与给定地址匹配的私钥，则SignTxWithPassphrase对事务进行签名。
// SignTxWithPassphrase signs the transaction if the private key matching the
// given address can be decrypted with the given passphrase.
func (ks *KeyStore) SignTxWithPassphrase(a accounts.Account, passphrase string, tx *types.Transaction, chainID *big.Int) (*types.Transaction, error) {
	_, key, err := ks.getDecryptedKey(a, passphrase)
	if err != nil {
		return nil, err
	}
	defer zeroKey(key.PrivateKey)
	//根据链ID的存在，使用EIP155或宅基地签名
	// Depending on the presence of the chain ID, sign with EIP155 or homestead
	if chainID != nil {
		return types.SignTx(tx, types.NewEIP155Signer(chainID), key.PrivateKey)
	}
	return types.SignTx(tx, types.HomesteadSigner{}, key.PrivateKey)
}

// Unlock无限期解锁给定帐户。
// Unlock unlocks the given account indefinitely.
func (ks *KeyStore) Unlock(a accounts.Account, passphrase string) error {
	return ks.TimedUnlock(a, passphrase, 0)
}

// Lock从内存中删除具有给定地址的私钥。
// Lock removes the private key with the given address from memory.
func (ks *KeyStore) Lock(addr common.Address) error {
	ks.mu.Lock()
	if unl, found := ks.unlocked[addr]; found {
		ks.mu.Unlock()
		ks.expire(addr, unl, time.Duration(0)*time.Nanosecond)
	} else {
		ks.mu.Unlock()
	}
	return nil
}

//TimedUnlock()函数会在给定的时限到达后，立即将已知Account对应的unlocked对象中的PrivateKey的私钥销毁(逐个bit清0)，
// 并将该unlocked对象从KeyStore成员中删除。而Unlock()函数会将该unlocked对象一直公开，直到程序退出。
// 注意，这里的清理工作仅仅是针对内存中的Key对象，而以加密方式存在本地的key文件不受影响。
// TimedUnlock unlocks the given account with the passphrase. The account
// stays unlocked for the duration of timeout. A timeout of 0 unlocks the account
// until the program exits. The account must match a unique key file.
//
//如果帐户地址已解锁一段时间，TimedUnlock会延长或缩短活动解锁超时。 如果地址先前无限期解锁，则不会更改超时。
// If the account address is already unlocked for a duration, TimedUnlock extends or
// shortens the active unlock timeout. If the address was previously unlocked
// indefinitely the timeout is not altered.
func (ks *KeyStore) TimedUnlock(a accounts.Account, passphrase string, timeout time.Duration) error {
	a, key, err := ks.getDecryptedKey(a, passphrase)
	if err != nil {
		return err
	}

	ks.mu.Lock()
	defer ks.mu.Unlock()
	u, found := ks.unlocked[a.Address]
	if found {
		if u.abort == nil {
			// The address was unlocked indefinitely, so unlocking
			// it with a timeout would be confusing.
			//地址被无限期解锁，因此用超时解锁会令人困惑。
			zeroKey(key.PrivateKey)
			return nil
		} //终止过期goroutine并在下面替换它。
		// Terminate the expire goroutine and replace it below.
		close(u.abort)
	}
	if timeout > 0 {
		u = &unlocked{Key: key, abort: make(chan struct{})}
		go ks.expire(a.Address, u, timeout)
	} else {
		u = &unlocked{Key: key}
	}
	ks.unlocked[a.Address] = u
	return nil
}

// Find将给定帐户解析为密钥库中的唯一条目。
// Find resolves the given account into a unique entry in the keystore.
func (ks *KeyStore) Find(a accounts.Account) (accounts.Account, error) {
	ks.cache.maybeReload()
	ks.cache.mu.Lock()
	a, err := ks.cache.find(a)
	ks.cache.mu.Unlock()
	return a, err
}

func (ks *KeyStore) getDecryptedKey(a accounts.Account, auth string) (accounts.Account, *Key, error) {
	a, err := ks.Find(a)
	if err != nil {
		return a, nil, err
	}
	key, err := ks.storage.GetKey(a.Address, a.URL.Path, auth)
	return a, key, err
}

func (ks *KeyStore) expire(addr common.Address, u *unlocked, timeout time.Duration) {
	t := time.NewTimer(timeout)
	defer t.Stop()
	select {
	case <-u.abort:
		// just quit
	case <-t.C:
		ks.mu.Lock()
		//如果它仍然是与dropLater一起启动的同一个键实例，则仅删除。 我们可以检查使用指针相等，因为每次解锁键时映射都会存储一个新指针。
		// only drop if it's still the same key instance that dropLater
		// was launched with. we can check that using pointer equality
		// because the map stores a new pointer every time the key is
		// unlocked.
		if ks.unlocked[addr] == u {
			zeroKey(u.PrivateKey)
			delete(ks.unlocked, addr)
		}
		ks.mu.Unlock()
	}
}

// NewAccount生成一个新密钥并将其存储到密钥目录中，并使用密码对其进行加密。
// NewAccount generates a new key and stores it into the key directory,
// encrypting it with the passphrase.
func (ks *KeyStore) NewAccount(passphrase string) (accounts.Account, error) {
	_, account, err := storeNewKey(ks.storage, crand.Reader, passphrase)
	if err != nil {
		return accounts.Account{}, err
	} //立即将帐户添加到缓存中，而不是等待文件系统通知来获取它。
	// Add the account to the cache immediately rather
	// than waiting for file system notifications to pick it up.
	ks.cache.add(account)
	ks.refreshWallets()
	return account, nil
}

//将导出导出为JSON密钥，使用newPassphrase加密。
// Export exports as a JSON key, encrypted with newPassphrase.
func (ks *KeyStore) Export(a accounts.Account, passphrase, newPassphrase string) (keyJSON []byte, err error) {
	_, key, err := ks.getDecryptedKey(a, passphrase)
	if err != nil {
		return nil, err
	}
	var N, P int
	if store, ok := ks.storage.(*keyStorePassphrase); ok {
		N, P = store.scryptN, store.scryptP
	} else {
		N, P = StandardScryptN, StandardScryptP
	}
	return EncryptKey(key, newPassphrase, N, P)
}

// Import将给定的加密JSON密钥存储到密钥目录中。
// Import stores the given encrypted JSON key into the key directory.
func (ks *KeyStore) Import(keyJSON []byte, passphrase, newPassphrase string) (accounts.Account, error) {
	key, err := DecryptKey(keyJSON, passphrase)
	if key != nil && key.PrivateKey != nil {
		defer zeroKey(key.PrivateKey)
	}
	if err != nil {
		return accounts.Account{}, err
	}
	return ks.importKey(key, newPassphrase)
}

// ImportECDSA将给定密钥存储到密钥目录中，并使用密码对其进行加密。
// ImportECDSA stores the given key into the key directory, encrypting it with the passphrase.
func (ks *KeyStore) ImportECDSA(priv *ecdsa.PrivateKey, passphrase string) (accounts.Account, error) {
	key := newKeyFromECDSA(priv)
	if ks.cache.hasAddress(key.Address) {
		return accounts.Account{}, fmt.Errorf("account already exists")
	}
	return ks.importKey(key, passphrase)
}

func (ks *KeyStore) importKey(key *Key, passphrase string) (accounts.Account, error) {
	a := accounts.Account{Address: key.Address, URL: accounts.URL{Scheme: KeyStoreScheme, Path: ks.storage.JoinPath(keyFileName(key.Address))}}
	if err := ks.storage.StoreKey(a.URL.Path, key, passphrase); err != nil {
		return accounts.Account{}, err
	}
	ks.cache.add(a)
	ks.refreshWallets()
	return a, nil
}

//更新会更改现有帐户的密码。
// Update changes the passphrase of an existing account.
func (ks *KeyStore) Update(a accounts.Account, passphrase, newPassphrase string) error {
	a, key, err := ks.getDecryptedKey(a, passphrase)
	if err != nil {
		return err
	}
	return ks.storage.StoreKey(a.URL.Path, key, newPassphrase)
}

// ImportPreSaleKey解密给定的以太坊预售钱包并将密钥文件存储在密钥目录中。 密钥文件使用相同的密码加密。
// ImportPreSaleKey decrypts the given Ethereum presale wallet and stores
// a key file in the key directory. The key file is encrypted with the same passphrase.
func (ks *KeyStore) ImportPreSaleKey(keyJSON []byte, passphrase string) (accounts.Account, error) {
	a, _, err := importPreSaleKey(ks.storage, keyJSON, passphrase)
	if err != nil {
		return a, err
	}
	ks.cache.add(a)
	ks.refreshWallets()
	return a, nil
}

// zeroKey将内存中的私钥清零。
// zeroKey zeroes a private key in memory.
func zeroKey(k *ecdsa.PrivateKey) {
	b := k.D.Bits()
	for i := range b {
		b[i] = 0
	}
}
