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
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// URL表示钱包或帐户的规范标识URL。
//
//它是url.URL的简化版本，具有重要的限制（这里被认为是特征），它只包含值可复制的组件，以及它不对特殊字符进行任何URL编码/解码。
//
//前者对于允许复制帐户而不留下对原始版本的实时引用很重要，而后者对于确保单个规范形式与RFC 3986规范中许多允许的形式相对是很重要的。
//因此，不应在以太坊钱包或帐户范围之外使用这些URL。
// URL represents the canonical identification URL of a wallet or account.
//
// It is a simplified version of url.URL, with the important limitations (which
// are considered features here) that it contains value-copyable components only,
// as well as that it doesn't do any URL encoding/decoding of special characters.
//
// The former is important to allow an account to be copied without leaving live
// references to the original version, whereas the latter is important to ensure
// one single canonical form opposed to many allowed ones by the RFC 3986 spec.
//
// As such, these URLs should not be used outside of the scope of an Ethereum
// wallet or account.
type URL struct {
	Scheme string // Protocol scheme to identify a capable account backend用于识别有能力的帐户后端的协议方案
	Path   string // Path for the backend to identify a unique entity后端识别唯一实体的路径
}

// parseURL将用户提供的URL转换为特定于帐户的结构。
// parseURL converts a user supplied URL into the accounts specific structure.
func parseURL(url string) (URL, error) {
	parts := strings.Split(url, "://")
	if len(parts) != 2 || parts[0] == "" {
		return URL{}, errors.New("protocol scheme missing")
	}
	return URL{
		Scheme: parts[0],
		Path:   parts[1],
	}, nil
}

// String实现stringer接口。
// String implements the stringer interface.
func (u URL) String() string {
	if u.Scheme != "" {
		return fmt.Sprintf("%s://%s", u.Scheme, u.Path)
	}
	return u.Path
}

// TerminalString实现log.TerminalStringer接口。
// TerminalString implements the log.TerminalStringer interface.
func (u URL) TerminalString() string {
	url := u.String()
	if len(url) > 32 {
		return url[:31] + "…"
	}
	return url
}

// MarshalJSON实现了json.Marshaller接口
// MarshalJSON implements the json.Marshaller interface.
func (u URL) MarshalJSON() ([]byte, error) {
	return json.Marshal(u.String())
}

// Cmp compares x and y and returns:Cmp比较x和y并返回：
//
//   -1 if x <  y
//    0 if x == y
//   +1 if x >  y
//
func (u URL) Cmp(url URL) int {
	if u.Scheme == url.Scheme {
		return strings.Compare(u.Path, url.Path)
	}
	return strings.Compare(u.Scheme, url.Scheme)
}
