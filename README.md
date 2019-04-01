# Serial2Network
A network server for accessing a serial port.

This code requires go 1.12 or newer to build properly.

To build, run:
```sh
go build cmd/serial2network/serial2network.go
go build cmd/client/client.go
```

To compile the protobuf (you should not need to do this), run:
```sh
cd api
protoc api.proto --go_out=plugins=grpc:.
```

Example usage:
```sh
# Run a serial server that talks to a /dev/ttyUSB0 at 9600bps and sends/receives data line-by-line 
#   with CR (\r) line terminators 
./serial2network -d /dev/ttyUSB0 -b 9600 -r CR -w CR


# Run a command line client that talks to the local server.
# NOTE: This will send and receive data an entire line at a time.
./client

# Run a serial client that talks to /dev/ttyUSB1 at 9600bps and sends/receives data line-by-line 
#   with CR (\r) line terminators.
# The difference between this and the command above is that whatever is read from /dev/ttypUSB1 
#   will be sent to whatever serial port the server is listening to.  
# In other words, this is a way to link two serial ports together.
./serial2network -c -d /dev/ttyUSB1 -b 9600 -r CR -w CR
```