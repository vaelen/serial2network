# Serial2Network
A network server for accessing a serial port.

This code requires go 1.12 or newer to build properly.

To build, run:
```
go build cmd/serial2network/serial2network.go
go build cmd/client/client.go
```

To compile the protobuf, run:
```
cd api
protoc api.proto --go_out=plugins=grpc:.
```
