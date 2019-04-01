/*
	Copyright 2019 Andrew C. Young <andrew@vaelen.org>

	This file is part of Serial2Network.

	Serial2Network is free software: you can redistribute it and/or modify
	it under the terms of the GNU General Public License as published by
	the Free Software Foundation, either version 3 of the License, or
	(at your option) any later version.

	Serial2Network is distributed in the hope that it will be useful,
	but WITHOUT ANY WARRANTY; without even the implied warranty of
	MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
	GNU General Public License for more details.

	You should have received a copy of the GNU General Public License
	along with Serial2Network.  If not, see <https://www.gnu.org/licenses/>.
*/

package main

import (
	"context"
	"flag"
	"fmt"
	"github.com/vaelen/serial2network"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
)

type parityValue struct {
	parity serial2network.ParityMode
}

func (v *parityValue) String() string {
	switch v.parity {
	case serial2network.ParityNone:
		return "NONE"
	case serial2network.ParityOdd:
		return "ODD"
	case serial2network.ParityEven:
		return "EVEN"
	default:
		return ""
	}
}

func (v *parityValue) Set(s string) error {
	switch strings.ToUpper(s)[0] {
	case 'N':
		v.parity = serial2network.ParityNone
	case 'O':
		v.parity = serial2network.ParityOdd
	case 'E':
		v.parity = serial2network.ParityEven
	default:
		return fmt.Errorf("invalid parity mode: %v", strings.ToUpper(s))
	}
	return nil
}

func (v *parityValue) Type() string {
	return "mode"
}

type lineEndingValue struct {
	lineEnding serial2network.LineEnding
}

func (v *lineEndingValue) String() string {
	switch v.lineEnding {
	case serial2network.Raw:
		return "RAW"
	case serial2network.LF:
		return "LF"
	case serial2network.CR:
		return "CR"
	case serial2network.CRLF:
		return "CRLF"
	default:
		return ""
	}
}

func (v *lineEndingValue) Set(s string) error {
	switch strings.ToUpper(s) {
	case "RAW":
		v.lineEnding = serial2network.Raw
	case "LF":
		v.lineEnding = serial2network.LF
	case "CR":
		v.lineEnding = serial2network.CR
	case "CRLF":
		v.lineEnding = serial2network.CRLF
	default:
		return fmt.Errorf("invalid line ending: %v", strings.ToUpper(s))
	}
	return nil
}

func (v *lineEndingValue) Type() string {
	return "line ending"
}

func defaultConfig() serial2network.Config {
	return serial2network.Config{
		SerialPort: serial2network.SerialConfig{
			Device:      "/dev/ttyS0",
			BaudRate:    9600,
			Parity:      serial2network.ParityNone,
			DataBits:    8,
			StopBits:    1,
			FlowControl: false,
		},
		Network: serial2network.NetworkConfig{
			Address: ":5555",
		},
		Server: true,
	}
}

func parseCommandLine() serial2network.Config {
	conf := defaultConfig()

	parity := &parityValue{
		parity: conf.SerialPort.Parity,
	}

	lineEndingReading := &lineEndingValue{
		lineEnding: conf.SerialPort.LineEndingForReading,
	}

	lineEndingWriting := &lineEndingValue{
		lineEnding: conf.SerialPort.LineEndingForWriting,
	}

	client := !conf.Server

	flag.StringVar(&conf.SerialPort.Device, "d", conf.SerialPort.Device, "The serial port to connect to")
	flag.UintVar(&conf.SerialPort.BaudRate, "b", conf.SerialPort.BaudRate, "Serial port baud rate in bps")
	flag.Var(parity, "p", "Serial port parity setting: [O]DD, [E]VEN, [N]ONE")
	flag.UintVar(&conf.SerialPort.DataBits, "z", conf.SerialPort.DataBits, "Serial port data bits")
	flag.UintVar(&conf.SerialPort.StopBits, "s", conf.SerialPort.StopBits, "Serial port stop bits")
	flag.BoolVar(&conf.SerialPort.FlowControl, "f", conf.SerialPort.FlowControl, "Serial port flow control")

	flag.Var(lineEndingReading, "r", "Line ending for reading data: RAW, LF, CR, CRLF")
	flag.Var(lineEndingWriting, "w", "Line ending for writing data: RAW, LF, CR, CRLF")

	flag.StringVar(&conf.Network.Address, "a", conf.Network.Address, "Network address of the server")
	flag.BoolVar(&client, "c", client, "Start up in client mode rather than server mode")

	flag.Parse()

	conf.SerialPort.Parity = parity.parity
	conf.SerialPort.LineEndingForReading = lineEndingReading.lineEnding
	conf.SerialPort.LineEndingForWriting = lineEndingWriting.lineEnding
	conf.Server = !client

	return conf
}

func main() {
	log.Printf("Starting\n")
	defer func() {
		log.Printf("Exiting\n")
	}()
	conf := parseCommandLine()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigs := make(chan os.Signal)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		select {
		case <-sigs:
			cancel()
		case <-ctx.Done():
			return
		}
	}()

	serial2network.Start(ctx, conf)
}
