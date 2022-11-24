package main

import (
	"bufio"
	"flag"
	"log"
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
	var compress bool
	var logfile string
	var metrics []*Metric
	var cpuTemp int = 0
	var cpuTempClock string

	flag.StringVar(&server, "s", "127.0.0.1:10051", "zabbix server address")
	flag.BoolVar(&compress, "c", false, "set true to enable compress protocol")
	flag.StringVar(&logfile, "l", "", "log server response into file")
	flag.Parse()

	if len(logfile) > 0 {
		if logfile != "stdout" {
			f, err := os.OpenFile(logfile, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
			if err != nil {
				log.Fatalf("error opening file: %v", err)
			}
			defer f.Close()
			log.SetOutput(f)
		}
		log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds | log.Lshortfile)
	}

	hostname, _ := os.Hostname()
	packet := NewPacket(compress, metrics, time.Now().Unix())

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

			resp, err := ZabbixSend(server, packet)
			if len(logfile) > 0 {
				if err != nil {
					log.Println(err)
				} else if resp != nil {
					log.Println(string(resp))
				}
			}
		}
	}()

	for {
		line := readLine()
		f := strings.Fields(line)
		if len(f) != 3 {
			continue
		}
		clock := f[2]

		lock.Lock()
		if f[0] == "temp.temp" { // duplicated name for each cpu cores
			t, err := strconv.Atoi(f[1])
			if err == nil && t > cpuTemp {
				cpuTemp = t
				cpuTempClock = clock
			}
		} else {
			metrics = append(metrics, NewMetric(hostname, f[0], f[1], clock))
		}
		lock.Unlock()
	}
}
