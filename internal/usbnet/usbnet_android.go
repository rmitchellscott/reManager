//go:build android

// Package usbnet carries TCP to a reMarkable over its USB CDC-ECM interface
// through a userspace network stack, for phones whose kernel brings the link
// up under a name Android's Ethernet service ignores.
package usbnet

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"runtime"
	"sync"
	"syscall"
	"unsafe"

	"gvisor.dev/gvisor/pkg/buffer"
	"gvisor.dev/gvisor/pkg/tcpip"
	"gvisor.dev/gvisor/pkg/tcpip/adapters/gonet"
	"gvisor.dev/gvisor/pkg/tcpip/link/channel"
	"gvisor.dev/gvisor/pkg/tcpip/link/ethernet"
	"gvisor.dev/gvisor/pkg/tcpip/network/arp"
	"gvisor.dev/gvisor/pkg/tcpip/network/ipv4"
	"gvisor.dev/gvisor/pkg/tcpip/stack"
	"gvisor.dev/gvisor/pkg/tcpip/transport/tcp"
)

const (
	nicID           = 1
	frameMTU        = 1514
	receiveSize     = 2048
	outboundQueue   = 256
	pollTimeoutMs   = 500
	sendTimeoutMs   = 2000
	tabletPrefixLen = 27
)

// The tablet is always 10.11.99.1 on its USB link and nothing else shares the
// cable, so the phone takes a fixed address instead of asking its DHCP server.
var (
	tabletSubnet = netip.MustParsePrefix("10.11.99.0/27")
	localAddress = netip.MustParseAddr("10.11.99.2")
	localMAC     = tcpip.LinkAddress("\x02\x72\x6d\x00\x00\x02")
)

func InTabletSubnet(host string) bool {
	addr, err := netip.ParseAddr(host)
	return err == nil && tabletSubnet.Contains(addr)
}

type Device struct {
	FD        int
	In        uint8
	Out       uint8
	MaxPacket int
}

type Link struct {
	device   Device
	endpoint *channel.Endpoint
	stack    *stack.Stack
	release  func()

	ctx       context.Context
	cancel    context.CancelFunc
	closeOnce sync.Once
}

// release runs once when the link closes, including when a transfer fails
// because the tablet was unplugged.
func Open(device Device, release func()) (*Link, error) {
	if device.MaxPacket <= 0 {
		return nil, fmt.Errorf("usbnet: invalid max packet size %d", device.MaxPacket)
	}

	netStack := stack.New(stack.Options{
		NetworkProtocols:   []stack.NetworkProtocolFactory{ipv4.NewProtocol, arp.NewProtocol},
		TransportProtocols: []stack.TransportProtocolFactory{tcp.NewProtocol},
	})
	endpoint := channel.New(outboundQueue, frameMTU, localMAC)
	if err := netStack.CreateNIC(nicID, ethernet.New(endpoint)); err != nil {
		netStack.Destroy()
		return nil, fmt.Errorf("usbnet: create NIC: %s", err)
	}
	local := tcpip.AddressWithPrefix{Address: tcpip.AddrFrom4(localAddress.As4()), PrefixLen: tabletPrefixLen}
	if err := netStack.AddProtocolAddress(nicID, tcpip.ProtocolAddress{Protocol: ipv4.ProtocolNumber, AddressWithPrefix: local}, stack.AddressProperties{}); err != nil {
		netStack.Destroy()
		return nil, fmt.Errorf("usbnet: add address: %s", err)
	}
	netStack.SetRouteTable([]tcpip.Route{{Destination: local.Subnet(), NIC: nicID}})

	ctx, cancel := context.WithCancel(context.Background())
	link := &Link{device: device, endpoint: endpoint, stack: netStack, release: release, ctx: ctx, cancel: cancel}
	go link.receive()
	go link.send()
	return link, nil
}

func (l *Link) Alive() bool {
	return l.ctx.Err() == nil
}

func (l *Link) DialContext(ctx context.Context, address string) (net.Conn, error) {
	addrPort, err := netip.ParseAddrPort(address)
	if err != nil {
		return nil, err
	}
	if !tabletSubnet.Contains(addrPort.Addr()) {
		return nil, fmt.Errorf("usbnet: %s is not on the tablet's USB subnet", addrPort.Addr())
	}
	remote := tcpip.FullAddress{NIC: nicID, Addr: tcpip.AddrFrom4(addrPort.Addr().As4()), Port: addrPort.Port()}
	return gonet.DialContextTCP(ctx, l.stack, remote, ipv4.ProtocolNumber)
}

func (l *Link) Close() {
	l.closeOnce.Do(func() {
		l.cancel()
		l.stack.Close()
		l.endpoint.Close()
		l.stack.Wait()
		if l.release != nil {
			l.release()
		}
	})
}

func (l *Link) receive() {
	frame := make([]byte, receiveSize)
	for l.Alive() {
		n, err := l.bulk(l.device.In, frame, pollTimeoutMs)
		if errors.Is(err, syscall.ETIMEDOUT) {
			continue
		}
		if err != nil {
			go l.Close()
			return
		}
		if n == 0 {
			continue
		}
		pkt := stack.NewPacketBuffer(stack.PacketBufferOptions{Payload: buffer.MakeWithData(append([]byte(nil), frame[:n]...))})
		l.endpoint.InjectInbound(0, pkt)
		pkt.DecRef()
	}
}

func (l *Link) send() {
	frame := make([]byte, 0, receiveSize)
	for {
		pkt := l.endpoint.ReadContext(l.ctx)
		if pkt == nil {
			return
		}
		frame = frame[:0]
		for _, slice := range pkt.AsSlices() {
			frame = append(frame, slice...)
		}
		pkt.DecRef()
		// ECM ends a frame with a short packet, so a frame that fills its last
		// packet exactly gets one byte of padding, which the IP length ignores.
		if len(frame)%l.device.MaxPacket == 0 {
			frame = append(frame, 0)
		}
		if _, err := l.bulk(l.device.Out, frame, sendTimeoutMs); err != nil && !errors.Is(err, syscall.ETIMEDOUT) {
			go l.Close()
			return
		}
	}
}

// usbdevfs_bulktransfer from linux/usbdevice_fs.h; Go lays out the pointer
// with the same alignment and padding as C on both 32- and 64-bit ARM.
type bulkTransfer struct {
	endpoint uint32
	length   uint32
	timeout  uint32
	data     unsafe.Pointer
}

var usbdevfsBulk = ioWR('U', 2, unsafe.Sizeof(bulkTransfer{}))

func ioWR(kind, number, size uintptr) uintptr {
	const read, write = 2, 1
	return (read|write)<<30 | size<<16 | kind<<8 | number
}

func (l *Link) bulk(endpoint uint8, data []byte, timeoutMs uint32) (int, error) {
	transfer := bulkTransfer{endpoint: uint32(endpoint), length: uint32(len(data)), timeout: timeoutMs, data: unsafe.Pointer(unsafe.SliceData(data))}
	n, _, errno := syscall.Syscall(syscall.SYS_IOCTL, uintptr(l.device.FD), usbdevfsBulk, uintptr(unsafe.Pointer(&transfer)))
	runtime.KeepAlive(data)
	if errno != 0 {
		return 0, errno
	}
	return int(n), nil
}
