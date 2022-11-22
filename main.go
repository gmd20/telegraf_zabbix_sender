package main

import (
	"bufio"
	"flag"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

func readLine() string {
	reader := bufio.NewReader(os.Stdin)
	l, _ := reader.ReadString('\n')
	return l
}

func main() {
	var lock sync.Mutex
	var server string
	var metrics []*Metric
	var cpuTemp int = 0
	var cpuTempClock int64 = 0

	flag.StringVar(&server, "s", "127.0.0.1:10051", "zabbix server address")
	flag.Parse()

	hostname, _ := os.Hostname()
	packet := NewPacket(metrics, time.Now().Unix())

	go func() {
		for {
			time.Sleep(10 * time.Second)
			lock.Lock()
			if cpuTemp > 0 {
				metrics = append(metrics, NewMetric(hostname, "cpu.temp", strconv.Itoa(cpuTemp), cpuTempClock))
				cpuTemp = 0
			}
			if len(metrics) > 0 {
				packet.AddMetrics(metrics, time.Now().Unix())
			}
			metrics = metrics[:0]
			lock.Unlock()

			ZabbixSend(server, packet)
		}
	}()

	for {
		line := readLine()
		f := strings.Fields(line)
		if len(f) != 3 {
			continue
		}

		lock.Lock()
		if f[0] == "temp.temp" { // duplicated name for each cpu cores
			t, err := strconv.Atoi(f[1])
			if err == nil && t > cpuTemp {
				cpuTemp = t
				cpuTempClock = time.Now().Unix()
			}
		} else {
			metrics = append(metrics, NewMetric(hostname, f[0], f[1], time.Now().Unix()))
		}
		lock.Unlock()
	}
}
