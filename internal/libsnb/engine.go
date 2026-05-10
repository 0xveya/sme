package libsnb

import (
	"net"
	"sync"

	"github.com/google/gopacket/pcap"
	"github.com/vishvananda/netns"
)

type Endpoint struct {
	Raw     net.PacketConn
	Pcap    *pcap.Handle
	UsePcap bool
}

type Bridge struct {
	Connections map[string]*Endpoint
	mu          sync.RWMutex
}

type NSManager struct {
	HostNS    netns.NsHandle
	TargetPID int
}

func (b *Bridge) Bind(ifaceName string, mtu int) error {
	// snaplen: mtu + Ethernet + VLAN + padding
	snaplen := int32(mtu + 32)

	handle, err := pcap.OpenLive(ifaceName, snaplen, true, pcap.BlockForever)
	if err != nil {
		return err
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	if b.Connections == nil {
		b.Connections = make(map[string]*Endpoint)
	}

	b.Connections[ifaceName] = &Endpoint{
		Pcap:    handle,
		UsePcap: true,
	}

	return nil
}

func (b *Bridge) Start(ifaceA, ifaceB string, mtu int) {
	go b.pipe(ifaceA, ifaceB, mtu)
	go b.pipe(ifaceB, ifaceA, mtu)
}

func (b *Bridge) pipe(srcName, dstName string, mtu int) {
	b.mu.RLock()
	src := b.Connections[srcName]
	dst := b.Connections[dstName]
	b.mu.RUnlock()

	if src == nil || dst == nil {
		return
	}

	for {
		data, _, err := src.Pcap.ReadPacketData()
		if err != nil {
			continue
		}

		if len(data) > mtu+32 {
			continue
		}

		if err := dst.Pcap.WritePacketData(data); err != nil {
			continue
		}
	}
}
