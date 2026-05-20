package libsnb

import "net"

type RawSocketIO struct {
	Conn net.PacketConn
}

func (r *RawSocketIO) ReadPacket() ([]byte, error) {
	buf := make([]byte, 65536)

	n, _, err := r.Conn.ReadFrom(buf)
	if err != nil {
		return nil, err
	}

	return buf[:n], nil
}

func (r *RawSocketIO) WritePacket(data []byte) error {
	_, err := r.Conn.WriteTo(data, nil)
	return err
}

func (r *RawSocketIO) Close() error {
	return r.Conn.Close()
}
