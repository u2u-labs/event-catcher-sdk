package client

import (
	"context"
	"crypto/tls"
	"net/http"
	"testing"
	"time"

	eventcatcher "github.com/u2u-labs/event-catcher-sdk"
	"github.com/u2u-labs/event-catcher-sdk/proto/node"
	"go.uber.org/zap"
)

func newTestClient() *GatewayClient {
	return NewClient(&GatewayOpts{
		Logger:    zap.NewNop().Sugar(),
		TLSConfig: &tls.Config{}, // set nil for gateway with non-TLS connection
	})
}

func TestPingNode(t *testing.T) {
	client := newTestClient()
	nodeInfo, err := client.RequestNodeFromGateway(context.Background(), "2484")
	if err != nil {
		t.Fatal(err)
	}
	t.Log(nodeInfo)

	if len(nodeInfo.Nodes) == 0 {
		t.Log("no nodes available")
		return
	}
	// check healthy node get request
	req, err := http.NewRequest(http.MethodGet, nodeInfo.Nodes[0].DomainHealth, nil)
	if err != nil {
		t.Fatal(err)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("expected status code %d, got %d", http.StatusOK, res.StatusCode)
	}
}

func TestSubscribeEventStream(t *testing.T) {
	client := newTestClient()
	nodeRPCUrl := ""
	nodeAddress := "0x01857E2BCFcb8B4eF76Df6590F8dCd3bf736C9E9"
	ts := time.Now().UTC().Format(time.RFC3339)

	// sign login request
	signature, err := SignLoginMessage("<your-private-key>", ts)
	if err != nil {
		t.Fatal(err)
	}

	t.Logf("signature: %s", signature)
	// get auth token
	auth, err := client.ValidateClient(context.Background(), nodeAddress, signature, ts)
	if err != nil || !auth.Success {
		t.Fatal(err)
	}

	// start event stream
	filter := &node.StreamEventsRequest{
		ChainId:         2484,
		ContractAddress: "0x8B0b7E0c9C5a6B48F5bA0352713B85c2C4973B78",
		EventSignature:  "NodeAdded",
	}
	streamSub, err := client.SubscribeEvent(context.Background(), &tls.Config{}, nodeRPCUrl, auth.ConnectionToken, filter, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Log(streamSub.ID)
}

func TestRegisterMonitorContract(t *testing.T) {
	client := newTestClient()
	nodeUrl := eventcatcher.GetBaseURL(eventcatcher.Sandbox)
	id, err := client.RegisterNodeMonitorContract(context.Background(), nodeUrl, RegisterContract{
		ChainId:         2484,
		ContractAddress: "0x8B0b7E0c9C5a6B48F5bA0352713B85c2C4973B78",
		EventSignature:  "NodeAdded(address indexed node)",
		EventAbi:        "[{\\\"anonymous\\\":false,\\\"inputs\\\":[{\\\"indexed\\\":true,\\\"internalType\\\":\\\"address\\\",\\\"name\\\":\\\"operator\\\",\\\"type\\\":\\\"address\\\"},{\\\"indexed\\\":true,\\\"internalType\\\":\\\"address\\\",\\\"name\\\":\\\"from\\\",\\\"type\\\":\\\"address\\\"},{\\\"indexed\\\":true,\\\"internalType\\\":\\\"address\\\",\\\"name\\\":\\\"to\\\",\\\"type\\\":\\\"address\\\"},{\\\"indexed\\\":false,\\\"internalType\\\":\\\"uint256\\\",\\\"name\\\":\\\"id\\\",\\\"type\\\":\\\"uint256\\\"},{\\\"indexed\\\":false,\\\"internalType\\\":\\\"uint256\\\",\\\"name\\\":\\\"value\\\",\\\"type\\\":\\\"uint256\\\"}],\\\"name\\\":\\\"TransferSingle\\\",\\\"type\\\":\\\"event\\\"}]",
		StartBlock:      50035775,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Log(id)
}

func TestGetEventCurrentSyncStatus(t *testing.T) {
	client := newTestClient()
	nodeUrl := eventcatcher.GetBaseURL(eventcatcher.Sandbox)
	rs, err := client.GetEventCurrentSyncStatus(context.Background(), nodeUrl, SyncStatusParams{
		ChainId:         2484,
		ContractAddress: "0x8B0b7E0c9C5a6B48F5bA0352713B85c2C4973B78",
		EventName:       "NodeAdded(address indexed node)",
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Log(rs)
}
