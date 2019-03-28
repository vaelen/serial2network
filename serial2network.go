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

package serial2network

import (
	"bufio"
	"context"
	"fmt"
	"github.com/golang/protobuf/ptypes/wrappers"
	"github.com/jacobsa/go-serial/serial"
	"google.golang.org/grpc"
	"io"
	"log"
	"net"
)

// ParityMode represents the various serial device parity modes.
type ParityMode int

const (
	// ParityNone - No parity bit
	ParityNone ParityMode = 0
	// ParityOdd - Odd parity bit
	ParityOdd ParityMode = 1
	// ParityEven - Even parity bit
	ParityEven ParityMode = 2
)

func (p ParityMode) String() string {
	switch p {
	case ParityNone:
		return "None"
	case ParityOdd:
		return "Odd"
	case ParityEven:
		return "Even"
	default:
		return ""
	}
}

// SerialConfig provides the serial port configuration
type SerialConfig struct {
	Device      string
	BaudRate    uint
	Parity      ParityMode
	DataBits    uint
	StopBits    uint
	FlowControl bool
}

// NetworkConfig provides the network configuration
type NetworkConfig struct {
	Address string
}

// Config provides the complete configuration
type Config struct {
	SerialPort SerialConfig
	Network    NetworkConfig
	Server     bool
}

// Start either the server or client, depending on the configuration
func Start(ctx context.Context, conf Config) {
	log.Printf("Configuration: %+v\n", conf)

	serialPort := open(conf.SerialPort)
	defer func() {
		log.Printf("Closing serial port\n")
		if serialPort != nil {
			serialPort.Close()
		}
	}()

	bufferedSerialPort := bufio.NewReadWriter(
		bufio.NewReader(serialPort),
		bufio.NewWriter(serialPort),
	)

	if conf.Server {
		startServer(ctx, conf, bufferedSerialPort)
		return
	}

	startClient(ctx, conf, bufferedSerialPort)
}

func startServer(ctx context.Context, conf Config, serialPort io.ReadWriter) {
	log.Printf("Starting server\n")

	l, err := net.Listen("tcp", conf.Network.Address)
	if err != nil {
		log.Fatalf("Couldn't start server: %v\n", err)
	}

	server := &serialServer{serialPort: serialPort, ctx: ctx}
	grpcServer := grpc.NewServer()

	defer func() {
		log.Printf("Shutting down server\n")
		grpcServer.Stop()
	}()

	RegisterSerialServer(grpcServer, server)

	go grpcServer.Serve(l)

	select {
	case <-ctx.Done():
		return
	}

}

func startClient(ctx context.Context, conf Config, serialPort io.ReadWriter) {
	log.Printf("Connecting to server\n")

	defer func() {
		log.Printf("Disconnecting from server\n")
	}()

	conn, err := grpc.Dial(conf.Network.Address)
	if err != nil {
		log.Fatalf("Could not create client: %v\n", err)
	}
	defer conn.Close()
	client := NewSerialClient(conn)

	stream, err := client.Open(ctx)
	if err != nil {
		log.Fatalf("Error opening data stream: %v\n", err)
	}

	fromNetwork := make(chan []byte)
	toNetwork := make(chan []byte)

	go serialPortReader(serialPort, fromNetwork, toNetwork)

	err = serialPortProcessingLoop(ctx, serialPort, fromNetwork, toNetwork, func(data []byte) error {
		return stream.Send(&wrappers.BytesValue{
			Value: data,
		})
	})
	if err != nil {
		log.Fatalf("%v\n", err)
	}
}

func serialPortReader(serialPort io.ReadWriter, fromNetwork chan []byte, toNetwork chan []byte) {
	log.Printf("Starting serial port reader\n")

	defer func() {
		log.Printf("Closing serial port reader\n")
		close(fromNetwork)
		close(toNetwork)
	}()

	data := make([]byte, 4098)

	for {
		bytesRead, err := serialPort.Read(data)
		if err != nil {
			log.Fatalf("Error reading from serial port: %v\n", err)
		}
		if bytesRead > 0 {
			toNetwork <- data[:bytesRead]
		}
	}
}

type dataSender func([]byte) error

func serialPortProcessingLoop(ctx context.Context, serialPort io.ReadWriter, fromNetwork chan []byte, toNetwork chan []byte, f dataSender) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case data := <-fromNetwork:
			if data == nil {
				return nil
			}
			_, err := serialPort.Write(data)
			if err != nil {
				return fmt.Errorf("error receiving data: %v", err)
			}
		case data := <-toNetwork:
			if data == nil {
				return nil
			}
			err := f(data)
			if err != nil {
				return fmt.Errorf("error sending data: %v", err)
			}
		}
	}
}

func open(conf SerialConfig) io.ReadWriteCloser {
	log.Printf("Opening serial port\n")

	options := serial.OpenOptions{
		PortName:          conf.Device,
		BaudRate:          conf.BaudRate,
		DataBits:          conf.DataBits,
		StopBits:          conf.StopBits,
		RTSCTSFlowControl: conf.FlowControl,
		MinimumReadSize:   1,
	}

	switch conf.Parity {
	case ParityNone:
		options.ParityMode = serial.PARITY_NONE
	case ParityOdd:
		options.ParityMode = serial.PARITY_ODD
	default:
		options.ParityMode = serial.PARITY_NONE
	}

	serialPort, err := serial.Open(options)

	if err != nil {
		log.Fatalf("Couldn't open serial port: %v\n", err)
	}

	return serialPort
}

// Server implementation
type serialServer struct {
	serialPort io.ReadWriter
	ctx        context.Context
}

func (s *serialServer) Open(stream Serial_OpenServer) error {
	fromNetwork := make(chan []byte)
	toNetwork := make(chan []byte)

	go serialPortReader(s.serialPort, fromNetwork, toNetwork)

	return serialPortProcessingLoop(s.ctx, s.serialPort, fromNetwork, toNetwork, func(data []byte) error {
		return stream.Send(&wrappers.BytesValue{
			Value: data,
		})
	})
}
