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

	rawMode := false

	flag.BoolVar(&rawMode, "r", false, "Raw data mode (instead of line-by-line mode)")
	flag.StringVar(&address, "s", address, "Network address of the server")

	flag.Parse()

	if rawMode {
		log.Printf("Raw data mode\n")
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

	if rawMode {
		go func() {
			stdin := bufio.NewReader(os.Stdin)
			data := make([]byte, 4098)

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
					stream.Send(&wrappers.BytesValue{Value: data[:bytesRead]})
				}
			}
		}()
	} else {
		go func() {
			stdin := bufio.NewScanner(os.Stdin)
			for stdin.Scan() {
				data := stdin.Bytes()
				if len(data) > 0 {
					stream.Send(&wrappers.BytesValue{Value: data})
				}
			}
			err := stdin.Err()
			if err == io.EOF {
				log.Printf("Disconnecting\n")
				cancel()
				return
			}
			if err != nil {
				log.Fatalf("Error reading from STDIN: %v\n", err)
			}
		}()
	}

	go func() {
		stdout := bufio.NewWriter(os.Stdout)

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
			if !rawMode {
				stdout.WriteByte('\n')
			}
			stdout.Flush()
			if err != nil {
				log.Fatalf("Error writing to STDOUT: %v\n", err)
			}
		}
	}()

	<-ctx.Done()

}
