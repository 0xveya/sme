package libsnb

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net"
	"os"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/google/gopacket/pcap"
	"github.com/vishvananda/netns"

	"github.com/0xveya/sme/internal/libsnb/errs"
)

type PacketIO interface {
	ReadPacket() ([]byte, error)
	WritePacket([]byte) error
	Close() error
}

// i shall consider improving this so pcap can run over udp

type Endpoint struct {
	Name       string
	ID         int // unsued for now will be useful later maybe once a uuid idk
	MTU        int
	IO         PacketIO
	RemoteAddr net.Addr
}

type Bridge struct {
	Connections map[string]*Endpoint
	mu          sync.RWMutex
	ctx         context.Context
	cancel      context.CancelFunc
	wg          sync.WaitGroup
}

type NSManager struct {
	HostNS    netns.NsHandle
	TargetPID int
}

func NewBridge() *Bridge {
	ctx, cancel := context.WithCancel(context.Background())
	return &Bridge{
		Connections: make(map[string]*Endpoint),
		ctx:         ctx,
		cancel:      cancel,
	}
}

func (b *Bridge) Bind(ifaceName string, mtu int, usePcap, immediateMode bool) error {
	if mtu+32 > math.MaxInt32 {
		return errs.ErrMTUOverflow
	}
	snaplen := int32(mtu + 32) // #nosec G115 ts alr checked

	b.mu.Lock()
	defer b.mu.Unlock()

	if b.Connections == nil {
		b.Connections = make(map[string]*Endpoint)
	}

	if after, ok := strings.CutPrefix(ifaceName, "udp://"); ok {
		remoteAddrStr := after
		remoteAddr, errResolve := net.ResolveUDPAddr("udp", remoteAddrStr)
		if errResolve != nil {
			return fmt.Errorf("failed to resolve worker address: %w", errResolve)
		}

		udpConn, errListen := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4zero, Port: 0})
		if errListen != nil {
			return fmt.Errorf("failed to open transport socket: %w", errListen)
		}

		b.Connections[ifaceName] = &Endpoint{
			Name: ifaceName,
			IO: &UDPIO{
				Conn:       udpConn,
				RemoteAddr: remoteAddr,
			},
			MTU:        mtu,
			RemoteAddr: remoteAddr,
		}
		return nil
	}

	if usePcap {
		inactive, errInactive := pcap.NewInactiveHandle(ifaceName)
		if errInactive != nil {
			return fmt.Errorf("failed to create inactive handle: %w", errInactive)
		}
		defer inactive.CleanUp()

		if errMode := inactive.SetImmediateMode(immediateMode); errMode != nil {
			return fmt.Errorf("failed to set immediate mode: %w", errMode)
		}
		if errSnap := inactive.SetSnapLen(int(snaplen)); errSnap != nil {
			return fmt.Errorf("failed to set snaplen: %w", errSnap)
		}
		if errPromisc := inactive.SetPromisc(true); errPromisc != nil {
			return fmt.Errorf("failed to set promisc: %w", errPromisc)
		}
		if errTimeout := inactive.SetTimeout(1 * time.Millisecond); errTimeout != nil {
			return fmt.Errorf("failed to set timeout: %w", errTimeout)
		}

		handle, errActivate := inactive.Activate()
		if errActivate != nil {
			return fmt.Errorf("failed to activate handle: %w", errActivate)
		}
		localIP, _ := net.ResolveIPAddr("ip", "127.0.0.1")

		b.Connections[ifaceName] = &Endpoint{
			Name:       ifaceName,
			IO:         &PcapIO{Handle: handle},
			MTU:        mtu,
			RemoteAddr: localIP,
		}
		return nil
	}

	ifi, errIface := net.InterfaceByName(ifaceName)
	if errIface != nil {
		return fmt.Errorf("failed to find interface %s: %w", ifaceName, errIface)
	}

	packetProto := htons(syscall.ETH_P_ALL)

	packetSock, errSock := syscall.Socket(syscall.AF_PACKET, syscall.SOCK_RAW, int(packetProto))
	if errSock != nil {
		return fmt.Errorf("failed to open raw packet socket: %w", errSock)
	}

	addr := &syscall.SockaddrLinklayer{
		Protocol: packetProto,
		Ifindex:  ifi.Index,
	}
	if errBind := syscall.Bind(packetSock, addr); errBind != nil {
		closeErr := syscall.Close(packetSock)
		if closeErr != nil {
			return fmt.Errorf("failed to close socket: %w", closeErr)
		}
		return fmt.Errorf("failed to bind raw socket to interface %s: %w", ifaceName, errBind)
	}

	ptr, errHandle := handleSocket(packetSock)
	if errHandle != nil {
		return fmt.Errorf("failed to handle socket: %w", errHandle)
	}
	fileConn, errFile := net.FilePacketConn(os.NewFile(ptr, ifaceName))
	if errFile != nil {
		closeErr := syscall.Close(packetSock)
		if closeErr != nil {
			return fmt.Errorf("failed to close socket: %w", closeErr)
		}
		return fmt.Errorf("failed to convert descriptor to packet connection: %w", errFile)
	}

	b.Connections[ifaceName] = &Endpoint{
		Name:       ifaceName,
		MTU:        mtu,
		IO:         &RawSocketIO{Conn: fileConn},
		RemoteAddr: fileConn.LocalAddr(),
	}
	return nil
}

func (b *Bridge) Start(ifaceA, ifaceB string, mtu int) {
	b.wg.Add(2)
	go func() {
		b.pipe(ifaceA, ifaceB, mtu)
		b.wg.Done()
	}()

	go func() {
		b.pipe(ifaceB, ifaceA, mtu)
		b.wg.Done()
	}()
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
		select {
		case <-b.ctx.Done():
			return
		default:
			data, err := src.IO.ReadPacket()
			if err != nil {
				select {
				case <-b.ctx.Done():
					return
				default:
					time.Sleep(10 * time.Millisecond)
					continue
				}
			}

			if len(data) > mtu+32 {
				continue
			}

			if err := dst.IO.WritePacket(data); err != nil {
				continue
			}
		}
	}
}

func (b *Bridge) Close() error {
	b.cancel()

	b.wg.Wait()

	b.mu.Lock()
	defer b.mu.Unlock()

	for _, endpoint := range b.Connections {
		if endpoint != nil {
			_ = endpoint.IO.Close()
		}
	}

	return nil
}

func htons(i uint16) uint16 {
	return (i<<8)&0xff00 | i>>8
}

func handleSocket(packetSock int) (uintptr, error) {
	if packetSock < 0 {
		return 0, errors.New("invalid socket: cannot be negative")
	}

	if uint64(packetSock) > uint64(^uintptr(0)) {
		return 0, errs.ErrSockOverflow
	}

	ptr := uintptr(packetSock) // #nosec G115 bounds are checked above
	return ptr, nil
}
