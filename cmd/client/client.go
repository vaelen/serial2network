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
	"bufio"
	"context"
	"flag"
	"github.com/golang/protobuf/ptypes/wrappers"
	"github.com/vaelen/serial2network/api"
	"google.golang.org/grpc"
	"io"
	"log"
	"os"
	"os/signal"
	"syscall"
)

func main() {
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

	address := ":5555"

	convertLFtoCR := false
	convertLFtoCRLF := false

	flag.BoolVar(&convertLFtoCR, "r", false, "Convert LF (\\n) to CR (\\r) and vice-versa")
	flag.BoolVar(&convertLFtoCRLF, "n", false, "Convert LF (\\n) to CRLF (\\r\\n) and vice-versa")
	flag.StringVar(&address, "s", address, "Network address of the server")

	flag.Parse()

	if convertLFtoCR {
		log.Printf("Converting LF (\\n) to CR (\\r) and vice-versa\n")
	} else if convertLFtoCRLF {
		log.Printf("Converting LF (\\n) to CRLF (\\r\\n) and vice-versa\n")
	}
	log.Printf("Connecting to server: %v\n", address)

	conn, err := grpc.Dial(address, grpc.WithInsecure())
	if err != nil {
		log.Fatalf("Could not connect to server: %v\n", err)
	}
	defer conn.Close()
	client := api.NewSerialClient(conn)

	streamCtx, streamCancel := context.WithCancel(context.Background())
	defer streamCancel()

	stream, err := client.Open(streamCtx)
	if err != nil {
		log.Fatalf("Error opening data stream: %v\n", err)
	}
	defer stream.CloseSend()

	stdin := bufio.NewReader(os.Stdin)
	stdout := bufio.NewWriter(os.Stdout)

	go func() {
		data := make([]byte, 4098)
		convertedData := make([]byte, 4098*2)

		for {
			bytesRead, err := stdin.Read(data)
			if err == io.EOF {
				log.Printf("Disconnecting\n")
				cancel()
				return
			}
			if err != nil {
				log.Fatalf("Error reading from STDIN: %v\n", err)
			}
			if bytesRead > 0 {
				dataToSend := convertedData[:0]
				if convertLFtoCR || convertLFtoCRLF {
					for _, b := range data[:bytesRead] {
						if b == '\n' {
							if convertLFtoCR {
								dataToSend = append(dataToSend, '\r')
							} else if convertLFtoCRLF {
								dataToSend = append(dataToSend, '\r', '\n')
							}
						} else {
							dataToSend = append(dataToSend, b)
						}
					}
				} else {
					dataToSend = data[:bytesRead]
				}
				stream.Send(&wrappers.BytesValue{Value: dataToSend})
			}
		}
	}()

	go func() {
		for {
			v, err := stream.Recv()
			if err == io.EOF {
				log.Printf("Connection closed\n")
				cancel()
				return
			}
			if err != nil {
				log.Fatalf("Error receiving from stream: %v\n", err)
			}
			_, err = stdout.Write(v.Value)
			if err != nil {
				log.Fatalf("Error writing to STDOUT: %v\n", err)
			}
			stdout.Flush()
		}
	}()

	<-ctx.Done()

}
