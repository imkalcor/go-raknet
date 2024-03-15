# go-raknet
RakNet is networking middleware developed by Oculus VR, Inc. for use in the video game industry. RakNet was originally authored by Jenkins Software LLC.
This repository is an implementation of the original raknet in Go which was written in C++.

This library aims to provide a basic implementation of Raknet which is built on top of the UDP (user datagram protocol). It is used for networking and communication in real time
game servers. However this library contains functionality specific in context for Minecraft: Pocket Edition (now known as Bedrock Edition) servers.

The library provides an easy to use API to read and write (reliable ordered by default) game packets (compressed + encrypted usually). It is built on top of the binary library which
provides a custom implementation of a fast buffer to read and write various datatypes over the wire.

This library is highly optimised and highly concurrent. It is multithreaded wherever required to distribute the processing load on other threads if the work is substantial enough 
to cover the cost of spawning a goroutine.

## Creating a Server

```go
  listener, err := raknet.Listen("127.0.0.1:19132")
```

The above code creates and binds a Raknet Listener to the provided IP address and returns an error if the address was either invalid or if there was a process already bound
to that address.

```go
  conn := listener.Accept()
```

## Creating a Client

```go
  conn, err := raknet.Connect("hivebedrock.network:19132")
```

The above code is blocking as it waits until a full connection is established to the provided remote server. It may return an error depending upon whether
either address was invalid, or connection timed out, or connection failed due to other reasons.

## Sending and receiving packets

The above line is blocking as it waits until a connection is available for it to be returned from the Listener. Usually, a for loop is created to keep accepting new connections from the
listener and then handling of packets from those connections is done on different threads by spawning a goroutine for each one of them.

```go
  pk := conn.Read()
```

The above line is also blocking as it waits until a connection sends a packet and then returns one when one is available.

```go
  resp := []byte{0x00, 0x01, 0x02, 0x03}
  conn.Write(resp)
```

The above code is non blocking as it schedules a packet to be dispatched to the other end of the connection and returns immediately.

Packets sent through raknet are batched together to allow network efficient delivery of the packets while saving bandwidth. They may either be sent to the peer by the end of the 
raknet tick OR may be sent earlier if the current batch we're writing to does not have enough space for accomodating successive packets, we flush the batch, reset it and then start
writing the next packets.

TL;DR: Sending time of a packet may vary but it may never exceed the configured raknet TPS.
