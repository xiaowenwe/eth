package main

import (
	"fmt"
	"github.com/ethereum/go-ethereum/p2p/nat"
	"net"
	"time"
)

func main() {
	ad := nat.Startautodisc("thing", func() nat.Interface {
		time.Sleep(500 * time.Millisecond)
		return nat.ExtIP{33, 44, 55, 66}
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
			fmt.Println("deadline exceeded")
		case rval := <-results:
			fmt.Println(rval)
			if rval.err != nil {
				fmt.Println("result %d: unexpected error: %v", i, rval.err)
			}
			wantIP := net.IP{33, 44, 55, 66}
			if !rval.ip.Equal(wantIP) {
				fmt.Println("result %d: got IP %v, want %v", i, rval.ip, wantIP)
			}
		}
	}
}
