# Event Catcher SDK Go

A Go SDK for interacting with the Event Catcher blockchain event monitoring system. This SDK provides a simple interface to connect to gateway services, manage node connections, subscribe to blockchain events, and register contract monitoring.

## Features

- **Gateway Integration**: Request available nodes from the gateway service
- **Authentication**: Sign login messages and validate client connections
- **Event Streaming**: Subscribe to real-time blockchain events via gRPC streams
- **Contract Management**: Register contracts for monitoring specific events

## Installation

```bash
go get github.com/u2u-labs/event-catcher
```

## Quick Start

### 1. Initialize the Client

```go
package main

import (
	"context"
	"time"

	"github.com/u2u-labs/event-catcher/client"
	"go.uber.org/zap"
)

func main() {
	client := client.NewClient(nil)

	// With custom configuration
	client := client.NewClient(&client.GatewayOpts{
		GatewayURL: "https://gateway.eventcatcher.api",
		Timeout:    30 * time.Second,
		Debug:      true,
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
signature, err := client.SignLoginMessage("your_private_key_hex")
if err != nil {
    panic(err)
}

auth, err := client.ValidateClient(
    context.Background(), 
    nodeURL, 
    signature, 
    time.Now().UTC().Format(time.RFC3339),
)
if err != nil || !auth.Success {
    panic(err)
}

authToken := auth.ConnectionToken
```

### 4. Subscribe to Events

```go
import "github.com/u2u-labs/event-catcher/proto/node"

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
    nodeURL, 
    authToken, 
    filter, 
    handler,
)
if err != nil {
    panic(err)
}

select {}
```

### 5. Register Contract for Monitoring

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

#### RetryConfig

```go
type RetryConfig struct {
    MaxAttempts int
    BaseDelay   time.Duration
    MaxDelay    time.Duration
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
    
    "github.com/u2u-labs/event-catcher/client"
    "github.com/u2u-labs/event-catcher/proto/node"
)

func main() {
    client := client.NewClient(&client.GatewayOpts{
        Debug: true,
    })
    defer client.Disconnect()
    
    nodes, err := client.RequestNodeFromGateway(context.Background(), "2484")
    if err != nil {
        log.Fatal(err)
    }
    
    if len(nodes.Nodes) == 0 {
        log.Fatal("No nodes available")
    }
    
    nodeURL := nodes.Nodes[0].Domain
    
    signature, err := client.SignLoginMessage("your_private_key_here")
    if err != nil {
        log.Fatal(err)
    }
    
    auth, err := client.ValidateClient(
        context.Background(), 
        nodeURL, 
        signature, 
        time.Now().UTC().Format(time.RFC3339),
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
        nodeURL, 
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
    
    select {}
}
```

## Error Handling

The SDK includes built-in retry logic for network operations. Configure retry behavior using the `RetryConfig` struct:

```go
client := client.NewClient(&client.GatewayOpts{
    RetryConfig: &client.RetryConfig{
        MaxAttempts: 5,
        BaseDelay:   2 * time.Second,
        MaxDelay:    60 * time.Second,
    },
})
```

## Logging

The SDK uses Uber's Zap logger. You can provide your own logger or use the default:

```go
logger, _ := zap.NewDevelopment()
client := client.NewClient(&client.GatewayOpts{
    Logger: logger.Sugar(),
    Debug:  true,
})
```

## License

This project is licensed under the MIT License - see the LICENSE file for details.

## Contributing

1. Fork the repository
2. Create your feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add some amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

## Support

For support and questions, please open an issue in the GitHub repository.