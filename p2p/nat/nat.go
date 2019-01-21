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

// Package nat provides access to common network port mapping protocols.
package nat

import (
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/log"
	"github.com/jackpal/go-nat-pmp"
)

// nat.Interface的实现可以将本地端口映射到可从Internet访问的端口。
// An implementation of nat.Interface can map local ports to ports
// accessible from the Internet.
type Interface interface {
	//这些方法管理本地计算机上的端口与可以从互联网连接的端口之间的映射。
	//协议是“UDP”或“TCP”。 一些实现允许为映射设置显示名称。 当网关的生存期结束时，该映射可能会被网关删除。
	// These methods manage a mapping between a port on the local
	// machine to a port that can be connected to from the internet.
	//
	// protocol is "UDP" or "TCP". Some implementations allow setting
	// a display name for the mapping. The mapping may be removed by
	// the gateway when its lifetime ends.
	AddMapping(protocol string, extport, intport int, name string, lifetime time.Duration) error
	DeleteMapping(protocol string, extport, intport int) error
	//此方法应返回网关设备的外部（面向Internet）地址。
	// This method should return the external (Internet-facing)
	// address of the gateway device.
	ExternalIP() (net.IP, error)
	//应该返回方法的名称。 这用于记录。
	// Should return name of the method. This is used for logging.
	String() string
}

//解析解析NAT接口描述。
//目前接受以下格式。
//请注意，机制名称不区分大小写。
//“”或“none”返回零
//“extip：77.12.33.4”将假定本地机器可以在给定的IP上访问
//“any”使用第一个自动检测机制
//“upnp”使用通用即插即用协议
//“pmp”使用带有自动检测网关地址的NAT-PMP
//“pmp：192.168.0.1”使用具有给定网关地址的NAT-PMP
// Parse parses a NAT interface description.
// The following formats are currently accepted.
// Note that mechanism names are not case-sensitive.
//
//     "" or "none"         return nil
//     "extip:77.12.33.4"   will assume the local machine is reachable on the given IP
//     "any"                uses the first auto-detected mechanism
//     "upnp"               uses the Universal Plug and Play protocol
//     "pmp"                uses NAT-PMP with an auto-detected gateway address
//     "pmp:192.168.0.1"    uses NAT-PMP with the given gateway address
func Parse(spec string) (Interface, error) {
	var (
		parts = strings.SplitN(spec, ":", 2)
		mech  = strings.ToLower(parts[0])
		ip    net.IP
	)
	if len(parts) > 1 {
		ip = net.ParseIP(parts[1])
		if ip == nil {
			return nil, errors.New("invalid IP address")
		}
	}
	switch mech {
	case "", "none", "off":
		return nil, nil
	case "any", "auto", "on":
		return Any(), nil
	case "extip", "ip":
		if ip == nil {
			return nil, errors.New("missing IP address")
		}
		return EExtIP(ip), nil
	case "upnp":
		return UPnP(), nil
	case "pmp", "natpmp", "nat-pmp":
		return PMP(ip), nil
	default:
		return nil, fmt.Errorf("unknown mechanism %q", parts[0])
	}
}

const (
	mapTimeout        = 20 * time.Minute
	mapUpdateInterval = 15 * time.Minute
)

// Map在m上添加一个端口映射，并保持它活动直到c关闭。
//此函数通常在其自己的goroutine中调用。
// Map adds a port mapping on m and keeps it alive until c is closed.
// This function is typically invoked in its own goroutine.
func Map(m Interface, c chan struct{}, protocol string, extport, intport int, name string) {
	log := log.New("proto", protocol, "extport", extport, "intport", intport, "interface", m)
	refresh := time.NewTimer(mapUpdateInterval)
	defer func() {
		refresh.Stop()
		log.Debug("Deleting port mapping")
		m.DeleteMapping(protocol, extport, intport)
	}()
	if err := m.AddMapping(protocol, extport, intport, name, mapTimeout); err != nil {
		log.Debug("Couldn't add port mapping", "err", err)
	} else {
		log.Info("Mapped network port")
	}
	for {
		select {
		case _, ok := <-c:
			if !ok {
				return
			}
		case <-refresh.C:
			log.Trace("Refreshing port mapping")
			if err := m.AddMapping(protocol, extport, intport, name, mapTimeout); err != nil {
				log.Debug("Couldn't add port mapping", "err", err)
			}
			refresh.Reset(mapUpdateInterval)
		}
	}
}

// ExtIP假定本地机器可以在给定的情况下到达
//外部IP地址，并且手动映射了所有必需的端口。
//映射操作不会返回错误，但实际上不会做任何事情。
// EExtIP assumes that the local machine is reachable on the given
// external IP address, and that any required ports were mapped manually.
// Mapping operations will not return an error but won't actually do anything.
func EExtIP(ip net.IP) Interface {
	if ip == nil {
		panic("IP must not be nil")
	}
	return ExtIP(ip)
}

type ExtIP net.IP

func (n ExtIP) ExternalIP() (net.IP, error) { return net.IP(n), nil }
func (n ExtIP) String() string              { return fmt.Sprintf("EExtIP(%v)", net.IP(n)) }

//这些什么都不做。
// These do nothing.
func (ExtIP) AddMapping(string, int, int, string, time.Duration) error { return nil }
func (ExtIP) DeleteMapping(string, int, int) error                     { return nil }

//任何返回的端口映射器都会尝试发现本地网络上的任何支持的机制。
// Any returns a port mapper that tries to discover any supported
// mechanism on the local network.
func Any() Interface {
	// TODO: attempt to discover whether the local machine has an
	//互联网级地址。 在这种情况下返回ExtIP。
	// Internet-class address. Return EExtIP in this case.
	return startautodisc("UPnP or NAT-PMP", func() Interface {
		found := make(chan Interface, 2)
		go func() { found <- discoverUPnP() }()
		go func() { found <- discoverPMP() }()
		for i := 0; i < cap(found); i++ {
			if c := <-found; c != nil {
				return c
			}
		}
		return nil
	})
}

// UPnP返回使用UPnP的端口映射器。 它会尝试
//使用UDP广播发现你的路由器的地址。
// UPnP returns a port mapper that uses UPnP. It will attempt to
// discover the address of your router using UDP broadcasts.
func UPnP() Interface {
	return startautodisc("UPnP", discoverUPnP)
}

// PMP返回使用NAT-PMP的端口映射器。 提供的网关地址应该是路由器的IP。
// 如果给定网关地址为零，则PMP将尝试自动发现路由器。
// PMP returns a port mapper that uses NAT-PMP. The provided gateway
// address should be the IP of your router. If the given gateway
// address is nil, PMP will attempt to auto-discover the router.
func PMP(gateway net.IP) Interface {
	if gateway != nil {
		return &pmp{gw: gateway, c: natpmp.NewClient(gateway)}
	}
	return startautodisc("NAT-PMP", discoverPMP)
}

// autodisc表示仍在被自动发现的端口映射机制。 调用此类型的接口方法将一直等到发现完成，然后调用已发现机制的方法。
//这种类型很有用，因为发现可能需要一段时间，但我们希望立即从UPnP，PMP和Auto中返回一个Interface值
// autodisc represents a port mapping mechanism that is still being
// auto-discovered. Calls to the Interface methods on this type will
// wait until the discovery is done and then call the method on the
// discovered mechanism.
//
// This type is useful because discovery can take a while but we
// want return an Interface value from UPnP, PMP and Auto immediately.
type autodisc struct {
	what string // type of interface being autodiscovered//被自动发现的接口类型
	once sync.Once
	doit func() Interface

	mu    sync.Mutex
	found Interface
}

func Startautodisc(what string, doit func() Interface) Interface {
	// TODO: monitor network configuration and rerun doit when it changes.// TODO：监控网络配置并在更改时重新运行doit。
	return startautodisc(what, doit)
}
func startautodisc(what string, doit func() Interface) Interface {
	// TODO: monitor network configuration and rerun doit when it changes.// TODO：监控网络配置并在更改时重新运行doit。
	return &autodisc{what: what, doit: doit}
}

func (n *autodisc) AddMapping(protocol string, extport, intport int, name string, lifetime time.Duration) error {
	if err := n.wait(); err != nil {
		return err
	}
	return n.found.AddMapping(protocol, extport, intport, name, lifetime)
}

func (n *autodisc) DeleteMapping(protocol string, extport, intport int) error {
	if err := n.wait(); err != nil {
		return err
	}
	return n.found.DeleteMapping(protocol, extport, intport)
}

func (n *autodisc) ExternalIP() (net.IP, error) {
	if err := n.wait(); err != nil {
		return nil, err
	}
	return n.found.ExternalIP()
}

func (n *autodisc) String() string {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.found == nil {
		return n.what
	} else {
		return n.found.String()
	}
}

//等待块，直到执行自动发现。
// wait blocks until auto-discovery has been performed.
func (n *autodisc) wait() error {
	n.once.Do(func() {
		n.mu.Lock()
		n.found = n.doit()
		n.mu.Unlock()
	})
	if n.found == nil {
		return fmt.Errorf("no %s router discovered", n.what)
	}
	return nil
}
