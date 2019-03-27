# Serial2Network
A network server for accessing a serial port.

This code requires go 1.12 or newer to build properly.

To build, run:
```
go build cmd/serial2network
```

To compile the protobuf, run:
```
protoc api/serial2network.proto --go_out=plugins=grpc:./
```
