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
//它是accounts.<Wallet>的实现类，它有一个Account对象，用来表示自身的地址，并通过Account.URL()方法，来实现上层接口<Wallet>.URL()方法；另外有一个KeyStore{}对象，这是这组代码中的核心类。
package keystore

import (
	"math/big"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/core/types"
)

// keystoreWallet实现原始密钥库的accounts.Wallet接口。
// keystoreWallet implements the accounts.Wallet interface for the original
// keystore.
type keystoreWallet struct {
	account  accounts.Account // Single account contained in this wallet此钱包中包含的单个帐户
	keystore *KeyStore        // Keystore where the account originates from帐户所在的密钥库
}

// URL实现accounts.Wallet，返回帐户的URL。
// URL implements accounts.Wallet, returning the URL of the account within.
func (w *keystoreWallet) URL() accounts.URL {
	return w.account.URL
}

// Status实现accounts.Wallet，返回密钥库钱包持有的帐户是否已解锁。
// Status implements accounts.Wallet, returning whether the account held by the
// keystore wallet is unlocked or not.
func (w *keystoreWallet) Status() (string, error) {
	w.keystore.mu.RLock()
	defer w.keystore.mu.RUnlock()

	if _, ok := w.keystore.unlocked[w.account.Address]; ok {
		return "Unlocked", nil
	}
	return "Locked", nil
}

//打开实现accounts.Wallet，但是对于普通钱包是noop，因为访问帐户列表不需要连接或解密步骤。
// Open implements accounts.Wallet, but is a noop for plain wallets since there
// is no connection or decryption step necessary to access the list of accounts.
func (w *keystoreWallet) Open(passphrase string) error { return nil }

//关闭实现accounts.Wallet，但是对于普通钱包是noop，因为没有有意义的打开操作。
// Close implements accounts.Wallet, but is a noop for plain wallets since is no
// meaningful open operation.
func (w *keystoreWallet) Close() error { return nil }

// Accounts实现accounts.Wallet，返回一个帐户列表，该列表由普通kestore钱包包含的单个帐户组成。
// Accounts implements accounts.Wallet, returning an account list consisting of
// a single account that the plain kestore wallet contains.
func (w *keystoreWallet) Accounts() []accounts.Account {
	return []accounts.Account{w.account}
}

//包含implements accounts.Wallet，返回此钱包实例是否包含特定帐户。
// Contains implements accounts.Wallet, returning whether a particular account is
// or is not wrapped by this wallet instance.
func (w *keystoreWallet) Contains(account accounts.Account) bool {
	return account.Address == w.account.Address && (account.URL == (accounts.URL{}) || account.URL == w.account.URL)
}

// Derive实现accounts.Wallet，但是对于普通钱包是noop，因为普通密钥库帐户没有分层帐户派生的概念。
// Derive implements accounts.Wallet, but is a noop for plain wallets since there
// is no notion of hierarchical account derivation for plain keystore accounts.
func (w *keystoreWallet) Derive(path accounts.DerivationPath, pin bool) (accounts.Account, error) {
	return accounts.Account{}, accounts.ErrNotSupported
}

// SelfDerive实现accounts.Wallet，但是对于普通钱包是noop，因为普通密钥库帐户没有分层帐户派生的概念。
// SelfDerive implements accounts.Wallet, but is a noop for plain wallets since
// there is no notion of hierarchical account derivation for plain keystore accounts.
func (w *keystoreWallet) SelfDerive(base accounts.DerivationPath, chain ethereum.ChainStateReader) {}

// SignHash实现accounts.Wallet，尝试使用给定帐户签署给定哈希。 如果钱包没有包装此特定帐户，
// \则会返回错误以避免帐户泄漏（即使理论上我们可以通过我们的共享密钥库后端进行签名）。
// SignHash implements accounts.Wallet, attempting to sign the given hash with
// the given account. If the wallet does not wrap this particular account, an
// error is returned to avoid account leakage (even though in theory we may be
// able to sign via our shared keystore backend).
func (w *keystoreWallet) SignHash(account accounts.Account, hash []byte) ([]byte, error) {
	// Make sure the requested account is contained within//确保包含所请求的帐户
	if account.Address != w.account.Address {
		return nil, accounts.ErrUnknownAccount
	}
	if account.URL != (accounts.URL{}) && account.URL != w.account.URL {
		return nil, accounts.ErrUnknownAccount
	}
	// Account seems valid, request the keystore to sign//帐户似乎有效，请求密钥库签名
	return w.keystore.SignHash(account, hash)
}

// SignTx实现accounts.Wallet，尝试使用给定帐户签署给定事务。 如果钱包没有包装此特定帐户，
// 则会返回错误以避免帐户泄漏（即使理论上我们可以通过我们的共享密钥库后端进行签名）。
// SignTx implements accounts.Wallet, attempting to sign the given transaction
// with the given account. If the wallet does not wrap this particular account,
// an error is returned to avoid account leakage (even though in theory we may
// be able to sign via our shared keystore backend).
func (w *keystoreWallet) SignTx(account accounts.Account, tx *types.Transaction, chainID *big.Int) (*types.Transaction, error) {
	// Make sure the requested account is contained within
	if account.Address != w.account.Address {
		return nil, accounts.ErrUnknownAccount
	}
	if account.URL != (accounts.URL{}) && account.URL != w.account.URL {
		return nil, accounts.ErrUnknownAccount
	} //帐户似乎有效，请求密钥库签名
	// Account seems valid, request the keystore to sign
	return w.keystore.SignTx(account, tx, chainID)
}

// SignHashWithPassphrase使用密码作为额外身份验证，使用给定帐户实现给定哈希值。
// SignHashWithPassphrase implements accounts.Wallet, attempting to sign the
// given hash with the given account using passphrase as extra authentication.
func (w *keystoreWallet) SignHashWithPassphrase(account accounts.Account, passphrase string, hash []byte) ([]byte, error) {
	// Make sure the requested account is contained within/确保所包含的帐户包含在内
	if account.Address != w.account.Address {
		return nil, accounts.ErrUnknownAccount
	}
	if account.URL != (accounts.URL{}) && account.URL != w.account.URL {
		return nil, accounts.ErrUnknownAccount
	}
	// Account seems valid, request the keystore to sign//帐户似乎有效，请求密钥库签名
	return w.keystore.SignHashWithPassphrase(account, passphrase, hash)
}

// SignTxWithPassphrase实现accounts.Wallet，尝试使用密码短语作为额外身份验证，使用给定帐户签署给定事务。
// SignTxWithPassphrase implements accounts.Wallet, attempting to sign the given
// transaction with the given account using passphrase as extra authentication.
func (w *keystoreWallet) SignTxWithPassphrase(account accounts.Account, passphrase string, tx *types.Transaction, chainID *big.Int) (*types.Transaction, error) {
	// Make sure the requested account is contained within确保所包含的帐户包含在内
	if account.Address != w.account.Address {
		return nil, accounts.ErrUnknownAccount
	}
	if account.URL != (accounts.URL{}) && account.URL != w.account.URL {
		return nil, accounts.ErrUnknownAccount
	}
	// Account seems valid, request the keystore to sign帐户似乎有效，请求密钥库签名
	return w.keystore.SignTxWithPassphrase(account, passphrase, tx, chainID)
}
