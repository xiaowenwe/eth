package natpmp

import (
	"fmt"
	"net"
	"time"
)

// Implement the NAT-PMP protocol, typically supported by Apple routers and open source
// routers such as DD-WRT and Tomato.
//
// See http://tools.ietf.org/html/draft-cheshire-nat-pmp-03
//
// Usage:
//
//    client := natpmp.NewClient(gatewayIP)
//    response, err := client.GetExternalAddress()

// The recommended mapping lifetime for AddPortMapping
const RECOMMENDED_MAPPING_LIFETIME_SECONDS = 3600

//用于进行远程过程调用的接口。
// Interface used to make remote procedure calls.
type caller interface {
	call(msg []byte, timeout time.Duration) (result []byte, err error)
}

//客户端是NAT-PMP协议客户端。
// Client is a NAT-PMP protocol client.
type Client struct {
	caller  caller
	timeout time.Duration
}

//在网关处为NAT-PMP服务器创建一个NAT-PMP客户端。
//使用大约128秒的默认超时。
// Create a NAT-PMP client for the NAT-PMP server at the gateway.
// Uses default timeout which is around 128 seconds.
func NewClient(gateway net.IP) (nat *Client) {
	return &Client{&network{gateway}, 0}
}

// Create a NAT-PMP client for the NAT-PMP server at the gateway, with a timeout.
// Timeout defines the total amount of time we will keep retrying before giving up.
func NewClientWithTimeout(gateway net.IP, timeout time.Duration) (nat *Client) {
	return &Client{&network{gateway}, timeout}
}

// NAT-PMP GetExternalAddress操作的结果。
// Results of the NAT-PMP GetExternalAddress operation.
type GetExternalAddressResult struct {
	SecondsSinceStartOfEpoc uint32
	ExternalIPAddress       [4]byte
}

//获取路由器的外部地址。
// Get the external address of the router.
func (n *Client) GetExternalAddress() (result *GetExternalAddressResult, err error) {
	msg := make([]byte, 2)
	msg[0] = 0 // Version 0
	msg[1] = 0 // OP Code 0
	response, err := n.rpc(msg, 12)
	if err != nil {
		return
	}
	result = &GetExternalAddressResult{}
	result.SecondsSinceStartOfEpoc = readNetworkOrderUint32(response[4:8])
	copy(result.ExternalIPAddress[:], response[8:12])
	return
}

//NAT-PMP AddPortMapping操作的结果
// Results of the NAT-PMP AddPortMapping operation
type AddPortMappingResult struct {
	SecondsSinceStartOfEpoc      uint32
	InternalPort                 uint16
	MappedExternalPort           uint16
	PortMappingLifetimeInSeconds uint32
}

//添加（或删除）端口映射。 要删除映射，请将requestedExternalPort和lifetime设置为0
// Add (or delete) a port mapping. To delete a mapping, set the requestedExternalPort and lifetime to 0
func (n *Client) AddPortMapping(protocol string, internalPort, requestedExternalPort int, lifetime int) (result *AddPortMappingResult, err error) {
	var opcode byte
	if protocol == "udp" {
		opcode = 1
	} else if protocol == "tcp" {
		opcode = 2
	} else {
		err = fmt.Errorf("unknown protocol %v", protocol)
		return
	}
	msg := make([]byte, 12)
	msg[0] = 0 // Version 0
	msg[1] = opcode
	writeNetworkOrderUint16(msg[4:6], uint16(internalPort))
	writeNetworkOrderUint16(msg[6:8], uint16(requestedExternalPort))
	writeNetworkOrderUint32(msg[8:12], uint32(lifetime))
	response, err := n.rpc(msg, 16)
	if err != nil {
		return
	}
	result = &AddPortMappingResult{}
	result.SecondsSinceStartOfEpoc = readNetworkOrderUint32(response[4:8])
	result.InternalPort = readNetworkOrderUint16(response[8:10])
	result.MappedExternalPort = readNetworkOrderUint16(response[10:12])
	result.PortMappingLifetimeInSeconds = readNetworkOrderUint32(response[12:16])
	return
}

func (n *Client) rpc(msg []byte, resultSize int) (result []byte, err error) {
	result, err = n.caller.call(msg, n.timeout)
	if err != nil {
		return
	}
	err = protocolChecks(msg, resultSize, result)
	return
}

func protocolChecks(msg []byte, resultSize int, result []byte) (err error) {
	if len(result) != resultSize {
		err = fmt.Errorf("unexpected result size %d, expected %d", len(result), resultSize)
		return
	}
	if result[0] != 0 {
		err = fmt.Errorf("unknown protocol version %d", result[0])
		return
	}
	expectedOp := msg[1] | 0x80
	if result[1] != expectedOp {
		err = fmt.Errorf("Unexpected opcode %d. Expected %d", result[1], expectedOp)
		return
	}
	resultCode := readNetworkOrderUint16(result[2:4])
	if resultCode != 0 {
		err = fmt.Errorf("Non-zero result code %d", resultCode)
		return
	}
	// If we got here the RPC is good.
	return
}

func writeNetworkOrderUint16(buf []byte, d uint16) {
	buf[0] = byte(d >> 8)
	buf[1] = byte(d)
}

func writeNetworkOrderUint32(buf []byte, d uint32) {
	buf[0] = byte(d >> 24)
	buf[1] = byte(d >> 16)
	buf[2] = byte(d >> 8)
	buf[3] = byte(d)
}

func readNetworkOrderUint16(buf []byte) uint16 {
	return (uint16(buf[0]) << 8) | uint16(buf[1])
}

func readNetworkOrderUint32(buf []byte) uint32 {
	return (uint32(buf[0]) << 24) | (uint32(buf[1]) << 16) | (uint32(buf[2]) << 8) | uint32(buf[3])
}
