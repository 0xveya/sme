package libsnb

import "github.com/google/gopacket/pcap"

type PcapIO struct {
	Handle *pcap.Handle
}

func (p *PcapIO) ReadPacket() ([]byte, error) {
	data, _, err := p.Handle.ReadPacketData()
	return data, err
}

func (p *PcapIO) WritePacket(data []byte) error {
	return p.Handle.WritePacketData(data)
}

func (p *PcapIO) Close() error {
	p.Handle.Close()
	return nil
}
