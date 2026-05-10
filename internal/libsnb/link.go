package libsnb

import (
	"fmt"
	"runtime"

	"github.com/0xveya/sme/internal/libsnb/errs"
	"github.com/vishvananda/netlink"
	"github.com/vishvananda/netns"
)

func (b *Bridge) Connect(pid int, hostSideName string, containerSideName string, mtu int) error {
	err := SetupLinkWithNames(pid, hostSideName, containerSideName, mtu)
	if err != nil {
		return err
	}

	cleanup, err := EnterNamespace(pid)
	if err != nil {
		return err
	}
	defer cleanup()

	link, err := netlink.LinkByName(containerSideName)
	if err != nil {
		return errs.ErrLinkNotFound
	}

	netlink.LinkSetMTU(link, mtu)

	return netlink.LinkSetUp(link)
}

func SetupLinkWithNames(pid int, hostName string, containerName string, mtu int) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	veth := &netlink.Veth{
		LinkAttrs: netlink.LinkAttrs{
			Name: hostName,
			MTU:  mtu,
		},
		PeerName: containerName,
	}

	if err := netlink.LinkAdd(veth); err != nil {
		return fmt.Errorf("%w: failed to create veth pair (%s <-> %s): %v", errs.ErrFailedToCreate, hostName, containerName, err)
	}

	peerLink, err := netlink.LinkByName(containerName)
	if err != nil {
		return fmt.Errorf("%w: failed to find peer interface: %v", errs.ErrFailedToFindPeer, err)
	}

	netlink.LinkSetMTU(peerLink, mtu)

	targetNs, err := netns.GetFromPid(pid)
	if err != nil {
		return fmt.Errorf("%w: failed to get namespace for PID %d: %v", errs.ErrNamespaceFailed, pid, err)
	}
	defer targetNs.Close()

	if err := netlink.LinkSetNsFd(peerLink, int(targetNs)); err != nil {
		return fmt.Errorf("%w: failed to move %s to PID %d: %v", errs.ErrNamespaceFailed, containerName, pid, err)
	}

	hostLink, err := netlink.LinkByName(hostName)
	if err == nil {
		netlink.LinkSetUp(hostLink)
	}

	return nil
}

func CleanupLink(hostName string) {
	link, err := netlink.LinkByName(hostName)
	if err == nil {
		netlink.LinkDel(link)
	}
}

func EnterNamespace(pid int) (func(), error) {
	runtime.LockOSThread()

	hostNS, err := netns.Get()
	if err != nil {
		runtime.UnlockOSThread()
		return nil, err
	}

	targetNS, err := netns.GetFromPid(pid)
	if err != nil {
		hostNS.Close()
		runtime.UnlockOSThread()
		return nil, err
	}

	if err := netns.Set(targetNS); err != nil {
		targetNS.Close()
		hostNS.Close()
		runtime.UnlockOSThread()
		return nil, err
	}

	return func() {
		defer runtime.UnlockOSThread()
		defer hostNS.Close()
		defer targetNS.Close()

		netns.Set(hostNS)
	}, nil
}
