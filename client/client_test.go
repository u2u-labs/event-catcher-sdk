package client

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/u2u-labs/event-catcher-sdk/proto/node"
	"go.uber.org/zap"
)

func newTestClient() *GatewayClient {
	return NewClient(&GatewayOpts{
		GatewayURL: "http://127.0.0.1:3000",
		Logger:     zap.NewNop().Sugar(),
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
	req, err := http.NewRequest(http.MethodGet, nodeInfo.Nodes[0].Domain+"/health", nil)
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
	nodeUrl := "http://127.0.0.1:8080"

	// sign login request
	signature, err := SignLoginMessage("4da8145fbf8343a9f5c517644d229a63e3dd51266e096dc6dd6ac2dca9a737fe")
	if err != nil {
		t.Fatal(err)
	}

	// get auth token
	auth, err := client.ValidateClient(context.Background(), nodeUrl, signature, time.Now().UTC().Format(time.RFC3339))
	if err != nil || !auth.Success {
		t.Fatal(err)
	}

	// start event stream
	filter := &node.StreamEventsRequest{
		ChainId:         2484,
		ContractAddress: "0x8B0b7E0c9C5a6B48F5bA0352713B85c2C4973B78",
		EventSignature:  "NodeAdded",
	}
	streamSub, err := client.SubscribeEvent(context.Background(), nodeUrl, auth.ConnectionToken, filter, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Log(streamSub.ID)
}

func TestRegisterMonitorContract(t *testing.T) {
	client := newTestClient()
	nodeUrl := "http://127.0.0.1:8080"
	id, err := client.RegisterNodeMonitorContract(context.Background(), nodeUrl, RegisterContract{
		ChainId:         2484,
		ContractAddress: "0x8B0b7E0c9C5a6B48F5bA0352713B85c2C4973B78",
		EventSignature:  "NodeAdded(address indexed node)",
		EventAbi:        "",
		StartBlock:      50035775,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Log(id)
}
