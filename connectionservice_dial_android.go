//go:build android

package main

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"

	"reManager/internal/usbnet"
)

func dialTablet(ctx context.Context, addr string, timeout time.Duration) (net.Conn, error) {
	link, err := userspaceUSBLink(addr)
	if err != nil {
		return nil, err
	}
	if link != nil {
		ctx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		return link.DialContext(ctx, addr)
	}
	d := net.Dialer{Timeout: timeout, Control: bindToLocalNetwork}
	conn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, err
	}
	return withQuickAck(conn), nil
}

var usbEthernet struct {
	mu   sync.Mutex
	link *usbnet.Link
}

// Some kernels name the tablet's USB interface usb0, and Android only makes a
// network of eth\d, so the tablet is reachable only by claiming the USB device
// and running the network stack in the app. Phones that do make a network of
// it keep the kernel path, which needs no USB permission prompt.
func userspaceUSBLink(addr string) (*usbnet.Link, error) {
	host, err := dialedHost(addr)
	if err != nil || !usbnet.InTabletSubnet(host) {
		return nil, nil
	}

	usbEthernet.mu.Lock()
	defer usbEthernet.mu.Unlock()
	if usbEthernet.link != nil && usbEthernet.link.Alive() {
		return usbEthernet.link, nil
	}
	usbEthernet.link = nil

	payload, err := json.Marshal(map[string]string{"address": host})
	if err != nil {
		return nil, err
	}
	var opened struct {
		Routed    bool   `json:"routed"`
		Absent    bool   `json:"absent"`
		Error     string `json:"error"`
		FD        int    `json:"fd"`
		In        uint8  `json:"in"`
		Out       uint8  `json:"out"`
		MaxPacket int    `json:"maxPacket"`
	}
	if err := json.Unmarshal([]byte(application.Android.OpenUSBEthernet(string(payload))), &opened); err != nil {
		return nil, errors.New("open USB Ethernet: " + err.Error())
	}
	switch {
	case opened.Error != "":
		return nil, errors.New("open USB Ethernet: " + opened.Error)
	case opened.Routed, opened.Absent:
		return nil, nil
	}

	link, err := usbnet.Open(usbnet.Device{FD: opened.FD, In: opened.In, Out: opened.Out, MaxPacket: opened.MaxPacket}, application.Android.CloseUSBEthernet)
	if err != nil {
		application.Android.CloseUSBEthernet()
		return nil, err
	}
	usbEthernet.link = link
	return link, nil
}

// Android sends every socket through the default network, so a tablet on a
// network with no internet, such as USB, cannot be reached while Wi-Fi or
// cellular is up unless the socket is bound to the tablet's network first.
func bindToLocalNetwork(_, address string, conn syscall.RawConn) error {
	host, err := dialedHost(address)
	if err != nil {
		return err
	}

	var bindErr error
	controlErr := conn.Control(func(fd uintptr) {
		payload, err := json.Marshal(map[string]any{"fd": int(fd), "address": host})
		if err != nil {
			bindErr = err
			return
		}
		if message := application.Android.BindSocketToLocalNetwork(string(payload)); message != "" {
			bindErr = errors.New("bind to local network: " + message)
		}
	})
	if controlErr != nil {
		return controlErr
	}
	return bindErr
}

// A failed or dropped connection looks the same whether the phone never saw
// the tablet, saw it but made no network of it, or routed around it, so the
// log records what Android sees of USB and its networks at that moment.
func (s *ConnectionService) logUSBNetworkDiagnostics(address string) {
	if s.logger == nil {
		return
	}
	host, err := dialedHost(address)
	if err != nil {
		host = address
	}
	payload, err := json.Marshal(map[string]string{"address": host})
	if err != nil {
		return
	}
	s.logger.LogEvent("NETDIAG", application.Android.USBNetworkDiagnostics(string(payload)))
}

func dialedHost(address string) (string, error) {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return "", err
	}
	host, _, _ = strings.Cut(host, "%")
	return host, nil
}
