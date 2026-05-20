package libsnb

import "net"

type UDPIO struct {
	Conn       *net.UDPConn
	RemoteAddr *net.UDPAddr
}

func (u *UDPIO) ReadPacket() ([]byte, error) {
	buf := make([]byte, 65536)

	n, _, err := u.Conn.ReadFrom(buf)
	if err != nil {
		return nil, err
	}

	return buf[:n], nil
}

func (u *UDPIO) WritePacket(data []byte) error {
	_, err := u.Conn.WriteTo(data, u.RemoteAddr)
	return err
}

func (u *UDPIO) Close() error {
	return u.Conn.Close()
}
