// Copyright 2016 The go-ethereum Authors
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
//包netutil包含net包的扩展。
// Package netutil contains extensions to the net package.
package netutil

import (
	"bytes"
	"errors"
	"fmt"
	"net"
	"sort"
	"strings"
)

var lan4, lan6, special4, special6 Netlist

func init() {
	// Lists from RFC 5735, RFC 5156,
	// https://www.iana.org/assignments/iana-ipv4-special-registry/
	lan4.Add("0.0.0.0/8")              // "This" network
	lan4.Add("10.0.0.0/8")             // Private Use
	lan4.Add("172.16.0.0/12")          // Private Use
	lan4.Add("192.168.0.0/16")         // Private Use
	lan6.Add("fe80::/10")              // Link-Local
	lan6.Add("fc00::/7")               // Unique-Local
	special4.Add("192.0.0.0/29")       // IPv4 Service Continuity
	special4.Add("192.0.0.9/32")       // PCP Anycast
	special4.Add("192.0.0.170/32")     // NAT64/DNS64 Discovery
	special4.Add("192.0.0.171/32")     // NAT64/DNS64 Discovery
	special4.Add("192.0.2.0/24")       // TEST-NET-1
	special4.Add("192.31.196.0/24")    // AS112
	special4.Add("192.52.193.0/24")    // AMT
	special4.Add("192.88.99.0/24")     // 6to4 Relay Anycast
	special4.Add("192.175.48.0/24")    // AS112
	special4.Add("198.18.0.0/15")      // Device Benchmark Testing
	special4.Add("198.51.100.0/24")    // TEST-NET-2
	special4.Add("203.0.113.0/24")     // TEST-NET-3
	special4.Add("255.255.255.255/32") // Limited Broadcast

	// http://www.iana.org/assignments/iana-ipv6-special-registry/
	special6.Add("100::/64")
	special6.Add("2001::/32")
	special6.Add("2001:1::1/128")
	special6.Add("2001:2::/48")
	special6.Add("2001:3::/32")
	special6.Add("2001:4:112::/48")
	special6.Add("2001:5::/32")
	special6.Add("2001:10::/28")
	special6.Add("2001:20::/28")
	special6.Add("2001:db8::/32")
	special6.Add("2002::/16")
}

//网表是IP网络的列表。
// Netlist is a list of IP networks.
type Netlist []net.IPNet

// ParseNetlist分析逗号分隔的CIDR掩码列表。
//空白和额外的逗号被忽略。
// ParseNetlist parses a comma-separated list of CIDR masks.
// Whitespace and extra commas are ignored.
func ParseNetlist(s string) (*Netlist, error) {
	ws := strings.NewReplacer(" ", "", "\n", "", "\t", "")
	masks := strings.Split(ws.Replace(s), ",")
	l := make(Netlist, 0)
	for _, mask := range masks {
		if mask == "" {
			continue
		}
		_, n, err := net.ParseCIDR(mask)
		if err != nil {
			return nil, err
		}
		l = append(l, *n)
	}
	return &l, nil
}

// MarshalTOML implements toml.MarshalerRec.
func (l Netlist) MarshalTOML() interface{} {
	list := make([]string, 0, len(l))
	for _, net := range l {
		list = append(list, net.String())
	}
	return list
}

// UnmarshalTOML implements toml.UnmarshalerRec.
func (l *Netlist) UnmarshalTOML(fn func(interface{}) error) error {
	var masks []string
	if err := fn(&masks); err != nil {
		return err
	}
	for _, mask := range masks {
		_, n, err := net.ParseCIDR(mask)
		if err != nil {
			return err
		}
		*l = append(*l, *n)
	}
	return nil
}

// Add分析CIDR掩码并将其附加到列表中。 它惊慌失措的面具，是打算用于设置静态列表。
// Add parses a CIDR mask and appends it to the list. It panics for invalid masks and is
// intended to be used for setting up static lists.
func (l *Netlist) Add(cidr string) {
	_, n, err := net.ParseCIDR(cidr)
	if err != nil {
		panic(err)
	}
	*l = append(*l, *n)
}

//包含报告给定IP是否包含在列表中。
// Contains reports whether the given IP is contained in the list.
func (l *Netlist) Contains(ip net.IP) bool {
	if l == nil {
		return false
	}
	for _, net := range *l {
		if net.Contains(ip) {
			return true
		}
	}
	return false
}

// IsLAN报告IP是否是本地网络地址。
// IsLAN reports whether an IP is a local network address.
func IsLAN(ip net.IP) bool {
	if ip.IsLoopback() {
		return true
	}
	if v4 := ip.To4(); v4 != nil {
		return lan4.Contains(v4)
	}
	return lan6.Contains(ip)
}

// IsSpecialNetwork报告IP是否位于特殊用途网络范围内
//这包括广播，多播和文档地址。
// IsSpecialNetwork reports whether an IP is located in a special-use network range
// This includes broadcast, multicast and documentation addresses.
func IsSpecialNetwork(ip net.IP) bool {
	if ip.IsMulticast() {
		return true
	}
	if v4 := ip.To4(); v4 != nil {
		return special4.Contains(v4)
	}
	return special6.Contains(ip)
}

var (
	errInvalid     = errors.New("invalid IP")
	errUnspecified = errors.New("zero address")
	errSpecial     = errors.New("special network")
	errLoopback    = errors.New("loopback address from non-loopback host")
	errLAN         = errors.New("LAN address from WAN host")
)

// CheckRelayIP报告是否从给定的发送方IP中继IP
//是一个有效的连接目标。
//
//有四条规则：
// - 特殊网络地址永远无效。
// - 如果通过回送主机进行中继，回送地址可以。
// - 如果LAN主机中继，则LAN地址正常。
// - 所有其他地址总是可以接受的。
// CheckRelayIP reports whether an IP relayed from the given sender IP
// is a valid connection target.
//
// There are four rules:
//   - Special network addresses are never valid.
//   - Loopback addresses are OK if relayed by a loopback host.
//   - LAN addresses are OK if relayed by a LAN host.
//   - All other addresses are always acceptable.
func CheckRelayIP(sender, addr net.IP) error {
	if len(addr) != net.IPv4len && len(addr) != net.IPv6len {
		return errInvalid
	}
	if addr.IsUnspecified() {
		return errUnspecified
	}
	if IsSpecialNetwork(addr) {
		return errSpecial
	}
	if addr.IsLoopback() && !sender.IsLoopback() {
		return errLoopback
	}
	if IsLAN(addr) && !IsLAN(sender) {
		return errLAN
	}
	return nil
}

// SameNet报告两个IP地址是否具有给定位长度的相同前缀。
// SameNet reports whether two IP addresses have an equal prefix of the given bit length.
func SameNet(bits uint, ip, other net.IP) bool {
	ip4, other4 := ip.To4(), other.To4()
	switch {
	case (ip4 == nil) != (other4 == nil):
		return false
	case ip4 != nil:
		return sameNet(bits, ip4, other4)
	default:
		return sameNet(bits, ip.To16(), other.To16())
	}
}

func sameNet(bits uint, ip, other net.IP) bool {
	nb := int(bits / 8)
	mask := ^byte(0xFF >> (bits % 8))
	if mask != 0 && nb < len(ip) && ip[nb]&mask != other[nb]&mask {
		return false
	}
	return nb <= len(ip) && bytes.Equal(ip[:nb], other[:nb])
}

// DistinctNetSet跟踪IP，确保至多N个IP
//落入相同的网络范围。
// DistinctNetSet tracks IPs, ensuring that at most N of them
// fall into the same network range.
type DistinctNetSet struct {
	Subnet uint // number of common prefix bits
	Limit  uint // maximum number of IPs in each subnet

	members map[string]uint
	buf     net.IP
}

//添加将IP地址添加到集合。 如果定义范围内现有IP的数量超过限制，它将返回false（并且不会添加IP）。
// Add adds an IP address to the set. It returns false (and doesn't add the IP) if the
// number of existing IPs in the defined range exceeds the limit.
func (s *DistinctNetSet) Add(ip net.IP) bool {
	key := s.key(ip)
	n := s.members[string(key)]
	if n < s.Limit {
		s.members[string(key)] = n + 1
		return true
	}
	return false
}

//删除从集合中删除一个IP
// Remove removes an IP from the set.
func (s *DistinctNetSet) Remove(ip net.IP) {
	key := s.key(ip)
	if n, ok := s.members[string(key)]; ok {
		if n == 1 {
			delete(s.members, string(key))
		} else {
			s.members[string(key)] = n - 1
		}
	}
}

//包含给定的IP是否包含在集合中。
// Contains whether the given IP is contained in the set.
func (s DistinctNetSet) Contains(ip net.IP) bool {
	key := s.key(ip)
	_, ok := s.members[string(key)]
	return ok
}

// Len returns the number of tracked IPs.
func (s DistinctNetSet) Len() int {
	n := uint(0)
	for _, i := range s.members {
		n += i
	}
	return int(n)
}

//将地址键映射到临时缓冲区。
//密钥的第一个字节是'4'或'6'来区分IPv4 / IPv6地址类型。
//关键字的其余部分是IP，截断到位数
// key encodes the map key for an address into a temporary buffer.
//
// The first byte of key is '4' or '6' to distinguish IPv4/IPv6 address types.
// The remainder of the key is the IP, truncated to the number of bits.
func (s *DistinctNetSet) key(ip net.IP) net.IP {
	// Lazily initialize storage.
	if s.members == nil {
		s.members = make(map[string]uint)
		s.buf = make(net.IP, 17)
	}
	//规范ip和bits。
	// Canonicalize ip and bits.
	typ := byte('6')
	if ip4 := ip.To4(); ip4 != nil {
		typ, ip = '4', ip4
	}
	bits := s.Subnet
	if bits > uint(len(ip)*8) {
		bits = uint(len(ip) * 8)
	}
	//将前缀编码到s.buf中
	// Encode the prefix into s.buf.
	nb := int(bits / 8)
	mask := ^byte(0xFF >> (bits % 8))
	s.buf[0] = typ
	buf := append(s.buf[:1], ip[:nb]...)
	if nb < len(ip) && mask != 0 {
		buf = append(buf, ip[nb]&mask)
	}
	return buf
}

// String实现fmt.Stringer
// String implements fmt.Stringer
func (s DistinctNetSet) String() string {
	var buf bytes.Buffer
	buf.WriteString("{")
	keys := make([]string, 0, len(s.members))
	for k := range s.members {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for i, k := range keys {
		var ip net.IP
		if k[0] == '4' {
			ip = make(net.IP, 4)
		} else {
			ip = make(net.IP, 16)
		}
		copy(ip, k[1:])
		fmt.Fprintf(&buf, "%v×%d", ip, s.members[k])
		if i != len(keys)-1 {
			buf.WriteString(" ")
		}
	}
	buf.WriteString("}")
	return buf.String()
}
