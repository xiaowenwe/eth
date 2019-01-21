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

package nat

import (
	"net"
	"testing"
	"time"
)

//此测试检查autodisc不会挂起，并在多个goroutines同时调用其方法时返回一致的结果。
// This test checks that autodisc doesn't hang and returns
// consistent results when multiple goroutines call its methods
// concurrently.
func TestAutoDiscRace(t *testing.T) {
	ad := startautodisc("thing", func() Interface {
		time.Sleep(500 * time.Millisecond)
		return ExtIP{33, 44, 55, 66}
	})
	//产生几个并发呼叫到ad.ExternalIP。
	// Spawn a few concurrent calls to ad.ExternalIP.
	type rval struct {
		ip  net.IP
		err error
	}
	results := make(chan rval, 50)
	for i := 0; i < cap(results); i++ {
		go func() {
			ip, err := ad.ExternalIP()
			results <- rval{ip, err}
		}()
	}
	//检查他们是否在截止日期之内返回正确的结果。
	// Check that they all return the correct result within the deadline.
	deadline := time.After(2 * time.Second)
	for i := 0; i < cap(results); i++ {
		select {
		case <-deadline:
			t.Fatal("deadline exceeded")
		case rval := <-results:
			if rval.err != nil {
				t.Errorf("result %d: unexpected error: %v", i, rval.err)
			}
			wantIP := net.IP{33, 44, 55, 66}
			if !rval.ip.Equal(wantIP) {
				t.Errorf("result %d: got IP %v, want %v", i, rval.ip, wantIP)
			}
		}
	}
}
