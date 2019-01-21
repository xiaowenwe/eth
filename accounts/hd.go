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
	"errors"
	"fmt"
	"math"
	"math/big"
	"strings"
)

// DefaultRootDerivationPath是自定义派生端点的根路径
//附加。 因此，第一个帐户将是m / 44'/ 60'/ 0'/ 0，第二个帐户
//在m / 44'/ 60'/ 0'/ 1等处
// DefaultRootDerivationPath is the root path to which custom derivation endpoints
// are appended. As such, the first account will be at m/44'/60'/0'/0, the second
// at m/44'/60'/0'/1, etc.
var DefaultRootDerivationPath = DerivationPath{0x80000000 + 44, 0x80000000 + 60, 0x80000000 + 0, 0}

// DefaultBaseDerivationPath是自定义派生端点递增的基本路径。 因此，第一个帐户将是m / 44'/ 60'/ 0'/ 0，第二个帐户将是m / 44'/ 60'/ 0'/ 1等。
// DefaultBaseDerivationPath is the base path from which custom derivation endpoints
// are incremented. As such, the first account will be at m/44'/60'/0'/0, the second
// at m/44'/60'/0'/1, etc.
var DefaultBaseDerivationPath = DerivationPath{0x80000000 + 44, 0x80000000 + 60, 0x80000000 + 0, 0, 0}

// DefaultLedgerBaseDerivationPath是自定义派生端点递增的基本路径。 因此，第一个帐户将是m / 44'/ 60'/ 0'/ 0，第二个帐户
//在m / 44'/ 60'/ 0'/ 1等处
// DefaultLedgerBaseDerivationPath is the base path from which custom derivation endpoints
// are incremented. As such, the first account will be at m/44'/60'/0'/0, the second
// at m/44'/60'/0'/1, etc.
var DefaultLedgerBaseDerivationPath = DerivationPath{0x80000000 + 44, 0x80000000 + 60, 0x80000000 + 0, 0}

// DerivationPath表示分层确定性钱包帐户派生路径的计算机友好版本。
//
// BIP-32规范https://github.com/bitcoin/bips/blob/master/bip-0032.mediawiki
//将派生路径定义为以下形式：
//
// m / purpose'/ coin_type'/ account'/ change / address_index
//
// BIP-44规范https://github.com/bitcoin/bips/blob/master/bip-0044.mediawiki
//为加密货币定义`目的'为44'（或0x8000002C）
// SLIP-44 https://github.com/satoshilabs/slips/blob/master/slip-0044.md指定
//“coin_type”60'（或0x8000003C）到以太坊。
//根据https://github.com/ethereum/EIPs/issues/84中的规范，以太坊的根路径为m / 44'/ 60'/ 0'/ 0，尽管它不是一成不变的 应该增加最后一个组件或其子组件。 我们将采用更简单的方法来递增最后一个组件
// DerivationPath represents the computer friendly version of a hierarchical
// deterministic wallet account derivaion path.
//
// The BIP-32 spec https://github.com/bitcoin/bips/blob/master/bip-0032.mediawiki
// defines derivation paths to be of the form:
//
//   m / purpose' / coin_type' / account' / change / address_index
//
// The BIP-44 spec https://github.com/bitcoin/bips/blob/master/bip-0044.mediawiki
// defines that the `purpose` be 44' (or 0x8000002C) for crypto currencies, and
// SLIP-44 https://github.com/satoshilabs/slips/blob/master/slip-0044.md assigns
// the `coin_type` 60' (or 0x8000003C) to Ethereum.
//
// The root path for Ethereum is m/44'/60'/0'/0 according to the specification
// from https://github.com/ethereum/EIPs/issues/84, albeit it's not set in stone
// yet whether accounts should increment the last component or the children of
// that. We will go with the simpler approach of incrementing the last component.
type DerivationPath []uint32

// ParseDerivationPath将用户指定的派生路径字符串转换为内部二进制表示。
//完全派生路径需要以`m /`前缀开头，相对派生路径（将附加到默认根路径）必须在第一个元素前面没有前缀。 空格被忽略。
// ParseDerivationPath converts a user specified derivation path string to the
// internal binary representation.
//
// Full derivation paths need to start with the `m/` prefix, relative derivation
// paths (which will get appended to the default root path) must not have prefixes
// in front of the first element. Whitespace is ignored.
func ParseDerivationPath(path string) (DerivationPath, error) {
	var result DerivationPath

	// Handle absolute or relative paths
	components := strings.Split(path, "/")
	switch {
	case len(components) == 0:
		return nil, errors.New("empty derivation path")

	case strings.TrimSpace(components[0]) == "":
		return nil, errors.New("ambiguous path: use 'm/' prefix for absolute paths, or no leading '/' for relative ones")

	case strings.TrimSpace(components[0]) == "m":
		components = components[1:]

	default:
		result = append(result, DefaultRootDerivationPath...)
	} //所有剩余的组件都是相对的，逐个追加
	// All remaining components are relative, append one by one
	if len(components) == 0 {
		return nil, errors.New("empty derivation path") // Empty relative paths
	}
	for _, component := range components {
		// Ignore any user added whitespace//忽略任何用户添加的空格
		component = strings.TrimSpace(component)
		var value uint32
		//处理硬化路径
		// Handle hardened paths
		if strings.HasSuffix(component, "'") {
			value = 0x80000000
			component = strings.TrimSpace(strings.TrimSuffix(component, "'"))
		} //处理非硬化组件
		// Handle the non hardened component
		bigval, ok := new(big.Int).SetString(component, 0)
		if !ok {
			return nil, fmt.Errorf("invalid component: %s", component)
		}
		max := math.MaxUint32 - value
		if bigval.Sign() < 0 || bigval.Cmp(big.NewInt(int64(max))) > 0 {
			if value == 0 {
				return nil, fmt.Errorf("component %v out of allowed range [0, %d]", bigval, max)
			}
			return nil, fmt.Errorf("component %v out of allowed hardened range [0, %d]", bigval, max)
		}
		value += uint32(bigval.Uint64())
		//追加并重复
		// Append and repeat
		result = append(result, value)
	}
	return result, nil
}

// String实现stringer接口，将二进制派生路径转换为其规范表示。
// String implements the stringer interface, converting a binary derivation path
// to its canonical representation.
func (path DerivationPath) String() string {
	result := "m"
	for _, component := range path {
		var hardened bool
		if component >= 0x80000000 {
			component -= 0x80000000
			hardened = true
		}
		result = fmt.Sprintf("%s/%d", result, component)
		if hardened {
			result += "'"
		}
	}
	return result
}
