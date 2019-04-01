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
	"bytes"
	"context"
	"github.com/golang/protobuf/ptypes/wrappers"
	"github.com/google/uuid"
	"github.com/jacobsa/go-serial/serial"
	"github.com/vaelen/serial2network/api"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"io"
	"log"
	"net"
	"sync"
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

// LineEnding defines a specific line ending mode
type LineEnding uint8

const (
	// Raw line ending mode performs no transformation.
	// Data will NOT be sent or received on a line-by-line basis.
	Raw LineEnding = 0
	// LF denotes that line endings should be LF (\n) only.
	// Data will be sent or received on a line-by-line basis.
	LF LineEnding = 1
	// CR denotes that line endings should be CR (\r) only.
	// Data will be sent or received on a line-by-line basis.
	CR LineEnding = 2
	// CRLF denotes taht line endings should be CRLF (\r\n).
	// Data will be sent or received on a line-by-line basis.
	CRLF LineEnding = 3
)

// Bytes returns the line ending as a byte slice
func (le LineEnding) Bytes() []byte {
	switch le {
	case Raw:
		return []byte{}
	case LF:
		return []byte{'\n'}
	case CR:
		return []byte{'\r'}
	case CRLF:
		return []byte{'\r', '\n'}
	default:
		return []byte{}
	}
}

func (le LineEnding) String() string {
	switch le {
	case Raw:
		return "Raw"
	case LF:
		return "LF"
	case CR:
		return "CR"
	case CRLF:
		return "CRLF"
	default:
		return ""
	}

}

// SerialConfig provides the serial port configuration
type SerialConfig struct {
	Device               string
	BaudRate             uint
	Parity               ParityMode
	DataBits             uint
	StopBits             uint
	FlowControl          bool
	LineEndingForReading LineEnding
	LineEndingForWriting LineEnding
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

	if conf.Server {
		startServer(ctx, conf, serialPort)
		return
	}

	lineEndingIn := conf.SerialPort.LineEndingForReading
	lineEndingOut := conf.SerialPort.LineEndingForWriting

	startClient(ctx, conf, serialPort, lineEndingIn, lineEndingOut)
}

func startServer(ctx context.Context, conf Config, serialPort io.ReadWriter) {
	log.Printf("Starting server\n")

	l, err := net.Listen("tcp", conf.Network.Address)
	if err != nil {
		log.Fatalf("Couldn't start server: %v\n", err)
	}

	lineEndingIn := conf.SerialPort.LineEndingForReading
	lineEndingOut := conf.SerialPort.LineEndingForWriting

	server := newServer(ctx, serialPort, lineEndingIn, lineEndingOut)
	grpcServer := grpc.NewServer()

	defer func() {
		log.Printf("Shutting down server\n")
		grpcServer.Stop()
	}()

	api.RegisterSerialServer(grpcServer, server)

	go grpcServer.Serve(l)

	select {
	case <-ctx.Done():
		return
	}

}

func startClient(ctx context.Context, conf Config, serialPort io.ReadWriter, lineEndingIn LineEnding, lineEndingOut LineEnding) {
	log.Printf("Connecting to server\n")

	defer func() {
		log.Printf("Disconnecting from server\n")
	}()

	conn, err := grpc.Dial(conf.Network.Address, grpc.WithInsecure())
	if err != nil {
		log.Fatalf("Could not create client: %v\n", err)
	}
	defer conn.Close()
	client := api.NewSerialClient(conn)

	stream, err := client.Open(ctx)
	if err != nil {
		log.Fatalf("Error opening data stream: %v\n", err)
	}

	fromNetwork := make(chan []byte)
	toNetwork := make(chan []byte)

	if lineEndingIn == Raw {
		go serialPortReader(serialPort, toNetwork)
	} else {
		go serialPortLineReader(serialPort, toNetwork, lineEndingIn.Bytes())
	}

	go func() {
		defer func() {
			log.Printf("Closing stream reader\n")
			close(fromNetwork)
		}()
		log.Printf("Starting stream reader")
		for {
			v, err := stream.Recv()
			if err == io.EOF {
				return
			}
			if err != nil {
				log.Fatalf("Error receiving from stream: %v\n", err)
			}
			fromNetwork <- v.Value
		}
	}()

	serialPortProcessingLoop(ctx, serialPort, lineEndingOut, fromNetwork, toNetwork, func(data []byte) error {
		return stream.Send(&wrappers.BytesValue{
			Value: data,
		})
	})

}

func serialPortReader(serialPort io.Reader, toNetwork chan []byte) error {
	log.Printf("Starting serial port reader\n")

	defer func() {
		log.Printf("Closing serial port reader\n")
		close(toNetwork)
	}()

	serialPortIn := bufio.NewReader(serialPort)
	data := make([]byte, 4098)

	for {
		bytesRead, err := serialPortIn.Read(data)
		if err != nil {
			log.Fatalf("Error reading from serial port: %v\n", err)
		}
		if bytesRead > 0 {
			log.Printf("Read from serial port: %q\n", string(data[:bytesRead]))

			toNetwork <- data[:bytesRead]
		}
	}
}

func serialPortLineReader(serialPort io.Reader, toNetwork chan []byte, lineEnding []byte) {
	log.Printf("Starting serial port line reader\n")

	defer func() {
		log.Printf("Closing serial port line reader\n")
		close(toNetwork)
	}()

	s := bufio.NewScanner(serialPort)
	s.Split(createSplitFunc(lineEnding))

	for s.Scan() {
		b := s.Bytes()
		if len(b) > 0 {
			log.Printf("Read from serial port: %q\n", string(b))

			toNetwork <- b
		}
	}
	err := s.Err()
	if err != nil && err != io.EOF {
		log.Fatalf("Error reading from serial port: %v\n", err)
	}
}

func createSplitFunc(lineEnding []byte) bufio.SplitFunc {
	return func(data []byte, atEOF bool) (advance int, token []byte, err error) {
		for i := range data {
			end := i + 1
			start := end - len(lineEnding)
			if start < 0 {
				continue
			}
			ending := data[start:end]
			if bytes.Equal(ending, lineEnding) {
				advance = end
				token = data[0:start]
				return
			}
		}
		if atEOF && len(data) > 0 {
			// Return whatever is left
			advance = len(data)
			token = data
		}
		return
	}
}

type dataSender func([]byte) error

func serialPortProcessingLoop(ctx context.Context, serialPort io.Writer, lineEnding LineEnding, fromNetwork chan []byte, toNetwork chan []byte, f dataSender) {
	serialPortOut := bufio.NewWriter(serialPort)
	for {
		select {
		case <-ctx.Done():
			return
		case data := <-fromNetwork:
			if data == nil {
				return
			}
			_, err := serialPortOut.Write(data)
			serialPortOut.Write(lineEnding.Bytes())
			serialPortOut.Flush()
			if err != nil {
				log.Fatalf("Error writing to serial port: %v\n", err)
			}
			log.Printf("Written to serial port: %q\n", string(data))
		case data := <-toNetwork:
			if data == nil {
				return
			}
			err := f(data)
			if err != nil {
				log.Fatalf("Error sending data: %v\n", err)
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

func newServer(ctx context.Context, serialPort io.ReadWriter, lineEndingIn LineEnding, lineEndingOut LineEnding) *serialServer {
	s := &serialServer{
		ctx:         ctx,
		fromNetwork: make(chan []byte),
		toNetwork:   make(chan []byte),
		listeners:   &sync.Map{},
	}
	if lineEndingIn == Raw {
		go serialPortReader(serialPort, s.toNetwork)
	} else {
		go serialPortLineReader(serialPort, s.toNetwork, lineEndingIn.Bytes())
	}
	go serialPortProcessingLoop(s.ctx, serialPort, lineEndingOut, s.fromNetwork, s.toNetwork, func(data []byte) error {
		return s.Send(data)
	})
	return s
}

// Server implementation
type serialServer struct {
	ctx           context.Context
	fromNetwork   chan []byte
	toNetwork     chan []byte
	listeners     *sync.Map
	lineEndingIn  LineEnding
	lineEndingOut LineEnding
}

func (s *serialServer) Open(stream api.Serial_OpenServer) error {
	id := uuid.New().String()

	log.Printf("Client connected: %s\n", id)
	defer log.Printf("Client disconnected: %s\n", id)

	var sender dataSender = func(data []byte) error {
		return stream.Send(&wrappers.BytesValue{
			Value: data,
		})
	}

	s.listeners.Store(id, sender)
	defer s.listeners.Delete(id)

	for {
		v, err := stream.Recv()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			code := status.Code(err)
			if code != codes.Canceled {
				log.Printf("Error receiving from stream %s: %v\n", id, err)
				return err
			}
			return nil
		}
		s.fromNetwork <- v.Value
	}
}

func (s *serialServer) Send(data []byte) error {
	s.listeners.Range(func(key interface{}, value interface{}) bool {
		err := (value.(dataSender))(data)
		if err != nil {
			log.Printf("Error sending to stream %s: %v\n", key, err)
		}
		return true
	})
	return nil
}
