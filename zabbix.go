package main

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"time"
)

const headerSize = 4 + 1 + 4 + 4
const tcpProtocol = byte(0x01)
const zlibCompress = byte(0x02)

type Metric struct {
	Host  string `json:"host"`
	Key   string `json:"key"`
	Value string `json:"value"`
	Clock string `json:"clock"`
}

func NewMetric(host, key, value string, clock string) *Metric {
	m := &Metric{Host: host, Key: key, Value: value, Clock: clock}
	return m
}

type Packet struct {
	Request  string    `json:"request"`
	Data     []*Metric `json:"data"`
	Clock    int64     `json:"clock"`
	compress bool
	buf      *bytes.Buffer
}

func (p *Packet) AddMetrics(data []*Metric, clock int64) {
	p.Data = data
	p.Clock = clock

	p.buf.Reset()
	if len(data) == 0 {
		return
	}

	// Zabbix sender protocol:
	// https://www.zabbix.com/documentation/current/en/manual/appendix/items/trapper
	// https://www.zabbix.com/documentation/current/en/manual/appendix/protocols/header_datalen
	// https://github.com/zabbix/zabbix/blob/master/src/go/pkg/zbxcomms/comms.go

	jsonData, _ := json.Marshal(p)
	var dataLen uint32 = uint32(len(jsonData))
	p.buf.Write([]byte("ZBXD"))
	var flag byte = tcpProtocol
	if p.compress {
		flag |= zlibCompress

		p.buf.WriteByte(flag)
		binary.Write(p.buf, binary.LittleEndian, uint32(0))
		binary.Write(p.buf, binary.LittleEndian, dataLen)
		w := zlib.NewWriter(p.buf)
		w.Write(jsonData)
		w.Close()
		// update compressed dataLen
		dataLen = uint32(p.buf.Len()) - headerSize
		b := p.buf.Bytes()
		binary.LittleEndian.PutUint32(b[5:], dataLen)
	} else {
		p.buf.WriteByte(flag)
		binary.Write(p.buf, binary.LittleEndian, dataLen)
		binary.Write(p.buf, binary.LittleEndian, dataLen)
		p.buf.Write(jsonData)
	}
}

func NewPacket(compress bool, data []*Metric, clock int64) *Packet {
	p := &Packet{Request: `sender data`, compress: compress, buf: new(bytes.Buffer)}
	p.AddMetrics(data, clock)
	return p
}

func uncompress(data []byte, expLen uint32) ([]byte, error) {
	var b bytes.Buffer

	b.Grow(int(expLen))
	z, err := zlib.NewReader(bytes.NewReader(data))
	if nil != err {
		return nil, fmt.Errorf("Unable to uncompress message: '%s'", err)
	}
	len, err := b.ReadFrom(z)
	z.Close()
	if nil != err {
		return nil, fmt.Errorf("Unable to uncompress message: '%s'", err)
	}
	if len != int64(expLen) {
		return nil, fmt.Errorf("Uncompressed message size %d instead of expected %d", len, expLen)
	}
	return b.Bytes(), nil
}

func zabbixRecv(conn net.Conn) ([]byte, error) {
	var total int
	var b [4096]byte
	var reservedSize uint32

	s := b[:]

	for total < headerSize {
		n, err := conn.Read(s[total:])
		if err != nil && err != io.EOF {
			return nil, fmt.Errorf("Cannot read message: '%s'", err)
		}
		if n == 0 {
			break
		}
		total += n
	}

	if total < headerSize {
		return nil, fmt.Errorf("Message is missing header")
	}
	if !bytes.Equal(s[:4], []byte{'Z', 'B', 'X', 'D'}) {
		return nil, fmt.Errorf("Message is using unsupported protocol")
	}

	flags := s[4]
	if 0 == (flags & tcpProtocol) {
		return nil, fmt.Errorf("Message is using unsupported protocol version")
	}
	if 0 != (flags & zlibCompress) {
		reservedSize = binary.LittleEndian.Uint32(s[9:13])
	}
	expectedSize := binary.LittleEndian.Uint32(s[5:9])
	if expectedSize > uint32(len(b))-headerSize {
		return nil, fmt.Errorf("Message size %d exceeds the buffer len", expectedSize)
	}
	if int(expectedSize) < total-headerSize {
		return nil, fmt.Errorf("Message is longer than expected")
	}
	if int(expectedSize) == total-headerSize {
		if 0 != (flags & zlibCompress) {
			return uncompress(s[headerSize:total], reservedSize)
		}
		return s[headerSize:total], nil
	}

	s = b[headerSize:]
	total = total - headerSize

	for total < int(expectedSize) {
		n, err := conn.Read(s[total:])
		if err != nil && err != io.EOF {
			return nil, fmt.Errorf("Cannot read message: '%s'", err)
		}
		if n == 0 {
			break
		}
		total += n
	}

	if total != int(expectedSize) {
		return nil, fmt.Errorf("Message size is shorted or longer than expected")
	}

	if 0 != (flags & zlibCompress) {
		return uncompress(s[:total], reservedSize)
	}
	return s[:total], nil
}

func ZabbixSend(serverAddr string, packet *Packet) ([]byte, error) {
	if packet.buf.Len() == 0 {
		return nil, fmt.Errorf("empty packet")
	}

	conn, err := net.DialTimeout("tcp", serverAddr, 5*time.Second)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	conn.SetDeadline(time.Now().Add(5 * time.Second))
	_, err = conn.Write(packet.buf.Bytes())
	if err != nil {
		return nil, err
	}
	return zabbixRecv(conn)
}
