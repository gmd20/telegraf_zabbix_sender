package main

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"io"
	"net"
	"time"
)

type Metric struct {
	Host  string `json:"host"`
	Key   string `json:"key"`
	Value string `json:"value"`
	Clock int64  `json:"clock"`
}

func NewMetric(host, key, value string, clock int64) *Metric {
	m := &Metric{Host: host, Key: key, Value: value}
	m.Clock = clock
	return m
}

type Packet struct {
	Request string    `json:"request"`
	Data    []*Metric `json:"data"`
	Clock   int64     `json:"clock"`
	buf     *bytes.Buffer
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

	p.buf.Write([]byte("ZBXD"))
	var flag byte = 1 // 0x01 - Zabbix communications protocol, 0x02 - compression
	p.buf.WriteByte(flag)
	dataPacket, _ := json.Marshal(p)
	binary.Write(p.buf, binary.LittleEndian, uint32(len(dataPacket)))
	var compressionLen uint32 = 0
	binary.Write(p.buf, binary.LittleEndian, compressionLen)
	p.buf.Write(dataPacket)
}

func NewPacket(data []*Metric, clock int64) *Packet {
	p := &Packet{Request: `sender data`, buf: new(bytes.Buffer)}
	p.AddMetrics(data, clock)
	return p
}

func ZabbixSend(serverAddr string, packet *Packet) ([]byte, error) {
	if packet.buf.Len() == 0 {
		return nil, nil
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
	return io.ReadAll(conn)
}
