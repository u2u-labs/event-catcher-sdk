# Event Catcher SDK Go

A Go SDK for interacting with the Event Catcher blockchain event monitoring system. This SDK provides a simple interface to connect to gateway services, manage node connections, subscribe to blockchain events, and register contract monitoring.

## Features

- **Gateway Integration**: Request available nodes from the gateway service
- **Authentication**: Sign login messages and validate client connections
- **Event Streaming**: Subscribe to real-time blockchain events via gRPC streams
- **Contract Management**: Register contracts for monitoring specific events

## Installation

```bash
go get github.com/u2u-labs/event-catcher-sdk
```

## Quick Start

### 1. Initialize the Client

```go
package main

import (
	"context"
	"crypto/tls"
	"time"

	"github.com/u2u-labs/event-catcher-sdk/client"
	"go.uber.org/zap"
)

func main() {
	client := client.NewClient(nil)

	// With custom configuration
	client := client.NewClient(&client.GatewayOpts{
		Timeout:    30 * time.Second,
	})
}
```

### 2. Request Available Nodes

```go
nodes, err := client.RequestNodeFromGateway(context.Background(), "2484")
if err != nil {
    panic(err)
}

if len(nodes.Nodes) == 0 {
    log.Fatal("No nodes available")
}

nodeURL := nodes.Nodes[0].Domain
```

### 3. Authenticate with Node

```go
timestamp := time.Now().UTC().Format(time.RFC3339)
signature, err := client.SignLoginMessage("your_private_key_hex", timestamp)
if err != nil {
    panic(err)
}

auth, err := client.ValidateClient(
    context.Background(), 
    nodeURL, 
    signature, 
    timestamp,
)
if err != nil || !auth.Success {
    panic(err)
}

authToken := auth.ConnectionToken
```

### 4. Subscribe to Events

```go
import "github.com/u2u-labs/event-catcher-sdk/proto/node"

filter := &node.StreamEventsRequest{
    ChainId:         2484,
    ContractAddress: "0x8B0b7E0c9C5a6B48F5bA0352713B85c2C4973B78",
    EventSignature:  "Transfer",
}

handler := func(event *client.Event) {
    log.Printf("New event - Block: %d, TxHash: %s, Data: %s", 
        event.BlockNumber, event.TxHash, event.Data)
}

subscription, err := client.SubscribeEvent(
    context.Background(), 
    nil, // TLS config (nil for no TLS)
    nodeURL, 
    authToken, 
    filter, 
    handler,
)
if err != nil {
    panic(err)
}

log.Printf("Subscribed to events: %s", subscription.ID)
// Keep the program running to receive events
select {}
```

### 5. Stop a Specific Stream

```go
response, err := client.StopStream(subscription.ID)
if err != nil {
    log.Printf("Error stopping stream: %v", err)
} else {
    log.Printf("Stream stopped successfully: %v", response)
}
```

### 6. Register Contract for Monitoring

```go
contractID, err := client.RegisterNodeMonitorContract(
    context.Background(), 
    nodeURL, 
    client.RegisterContract{
        ChainId:         2484,
        ContractAddress: "0x8B0b7E0c9C5a6B48F5bA0352713B85c2C4973B78",
        EventSignature:  "Transfer(address indexed from, address indexed to, uint256 value)",
        EventAbi:        "",
        StartBlock:      50035775,
    },
)
if err != nil {
    panic(err)
}

log.Printf("Contract registered with ID: %s", contractID)
```

## API Reference

### Client Configuration

#### GatewayOpts

```go
type GatewayOpts struct {
    GatewayURL  string
    RetryConfig *RetryConfig
    Timeout     time.Duration
    Logger      *zap.SugaredLogger
    TLSConfig   *tls.Config
    Debug       bool
}
```

### Data Types

#### Event
```go
type Event struct {
    BlockNumber uint64 `json:"block_number"`
    TxHash      string `json:"tx_hash"`
    Data        string `json:"data"`
}
```

#### RegisterContract
```go
type RegisterContract struct {
    ChainId         int64  `json:"chainId"`
    ContractAddress string `json:"contractAddress"`
    EventSignature  string `json:"eventSignature"`
    EventAbi        string `json:"eventAbi"`
    StartBlock      uint64 `json:"startBlock"`
}
```

## Complete Example

```go
package main

import (
    "context"
    "log"
    "time"
    
    "github.com/u2u-labs/event-catcher-sdk/client"
    "github.com/u2u-labs/event-catcher-sdk/proto/node"
)

func main() {
    client := client.NewClient(&client.GatewayOpts{})
    defer client.Disconnect()
    
    nodes, err := client.RequestNodeFromGateway(context.Background(), "2484")
    if err != nil {
        log.Fatal(err)
    }
    
    if len(nodes.Nodes) == 0 {
        log.Fatal("No nodes available")
    }
    
    nodeURL := nodes.Nodes[0].Domain
    rpcURL := nodes.Nodes[0].DomainRpc
    nodeAddress := "0x01857E2BCFcb8B4eF76Df6590F8dCd3bf736C9E9"
    
    timestamp := time.Now().UTC().Format(time.RFC3339)
    signature, err := client.SignLoginMessage("your_private_key_here", timestamp)
    if err != nil {
        log.Fatal(err)
    }
    
    auth, err := client.ValidateClient(
        context.Background(),
        nodeAddress, 
        signature, 
        timestamp,
    )
    if err != nil || !auth.Success {
        log.Fatal("Authentication failed")
    }
    
    contractID, err := client.RegisterNodeMonitorContract(
        context.Background(), 
        nodeURL, 
        client.RegisterContract{
            ChainId:         2484,
            ContractAddress: "0x8B0b7E0c9C5a6B48F5bA0352713B85c2C4973B78",
            EventSignature:  "Transfer(address indexed from, address indexed to, uint256 value)",
            EventAbi:        "[{\\\"anonymous\\\":false,\\\"inputs\\\":[{\\\"indexed\\\":true,\\\"internalType\\\":\\\"address\\\",\\\"name\\\":\\\"operator\\\",\\\"type\\\":\\\"address\\\"},{\\\"indexed\\\":true,\\\"internalType\\\":\\\"address\\\",\\\"name\\\":\\\"from\\\",\\\"type\\\":\\\"address\\\"},{\\\"indexed\\\":true,\\\"internalType\\\":\\\"address\\\",\\\"name\\\":\\\"to\\\",\\\"type\\\":\\\"address\\\"},{\\\"indexed\\\":false,\\\"internalType\\\":\\\"uint256\\\",\\\"name\\\":\\\"id\\\",\\\"type\\\":\\\"uint256\\\"},{\\\"indexed\\\":false,\\\"internalType\\\":\\\"uint256\\\",\\\"name\\\":\\\"value\\\",\\\"type\\\":\\\"uint256\\\"}],\\\"name\\\":\\\"TransferSingle\\\",\\\"type\\\":\\\"event\\\"}]",
            StartBlock:      50035775,
        },
    )
    if err != nil {
        log.Fatal(err)
    }
    log.Printf("Contract registered: %s", contractID)
    
    filter := &node.StreamEventsRequest{
        ChainId:         2484,
        ContractAddress: "0x8B0b7E0c9C5a6B48F5bA0352713B85c2C4973B78",
        EventSignature:  "Transfer",
    }
    
    subscription, err := client.SubscribeEvent(
        context.Background(), 
        nil, // TLS config
        rpcURL, 
        auth.ConnectionToken, 
        filter, 
        func(event *client.Event) {
            log.Printf("Event: Block %d, Tx %s", event.BlockNumber, event.TxHash)
        },
    )
    if err != nil {
        log.Fatal(err)
    }
    
    log.Printf("Subscribed to events: %s", subscription.ID)
    
    // Stop the stream after some time
    go func() {
        time.Sleep(60 * time.Second)
        response, err := client.StopStream(subscription.ID)
        if err != nil {
            log.Printf("Error stopping stream: %v", err)
        } else {
            log.Printf("Stream stopped: %v", response)
        }
    }()
    
    select {}
}
```

### Stream Management

- Each subscription now has a unique ID for management
- Individual streams can be stopped while keeping others active
- Billing is committed when streams are properly disconnected

## TLS Configuration

For secure connections, provide TLS configuration:

```go
import "crypto/tls"

tlsConfig := &tls.Config{}

subscription, err := client.SubscribeEvent(
    ctx,
    tlsConfig,
    rpcURL,
    authToken,
    filter,
    handler,
)
```

## License

This project is licensed under the MIT License - see the LICENSE file for details.

## Contributing

1. Fork the repository
2. Create your feature branch (`git checkout -b ft/amazing-feature`)
3. Commit your changes (`git commit -m 'update: add amazing feature'`)
4. Push to the branch (`git push origin ft/amazing-feature`)
5. Open a Pull Request

## Support

For support and questions, please open an issue in the GitHub repository.
