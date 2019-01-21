// Copyright 2014 The go-ethereum Authors
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

package common

import (
	"fmt"
)

//StorageSize是支持用户友好/格式化的浮动值的包装器。
// StorageSize is a wrapper around a float value that supports user friendly
// formatting.
type StorageSize float64

// String实现stringer接口。
// String implements the stringer interface.
func (s StorageSize) String() string {
	if s > 1000000 {
		return fmt.Sprintf("%.2f mB", s/1000000)
	} else if s > 1000 {
		return fmt.Sprintf("%.2f kB", s/1000)
	} else {
		return fmt.Sprintf("%.2f B", s)
	}
}

// TerminalString实现log.TerminalStringer，在日志记录期间格式化控制台输出的字符串。
// TerminalString implements log.TerminalStringer, formatting a string for console
// output during logging.
func (s StorageSize) TerminalString() string {
	if s > 1000000 {
		return fmt.Sprintf("%.2fmB", s/1000000)
	} else if s > 1000 {
		return fmt.Sprintf("%.2fkB", s/1000)
	} else {
		return fmt.Sprintf("%.2fB", s)
	}
}
