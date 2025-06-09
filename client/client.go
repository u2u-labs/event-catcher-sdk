package client

import (
	"context"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/google/uuid"
	eventcatcher "github.com/u2u-labs/event-catcher-sdk"
	node2 "github.com/u2u-labs/event-catcher-sdk/proto/node"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
)

const (
	LOGIN_MESSAGE = "logmein"
)

// Event represents a blockchain event
type Event struct {
	BlockNumber uint64 `json:"block_number"`
	TxHash      string `json:"tx_hash"`
	Data        string `json:"data"`
}

// EventHandler is called when a new event is received
type EventHandler func(event *Event)

type RegisterContract struct {
	ChainId         int64  `json:"chainId"`
	ContractAddress string `json:"contractAddress"`
	EventSignature  string `json:"eventSignature"`
	EventAbi        string `json:"eventAbi"`
	StartBlock      uint64 `json:"startBlock"`
}

// StreamSubscription represents an active event stream
type StreamSubscription struct {
	ID        string
	cancel    context.CancelFunc
	authToken string
	nodeUrl   string
	tlsConfig *tls.Config
}

// GatewayClient is the main SDK client
type GatewayClient struct {
	gatewayURL string

	// Stream management
	streamsMu sync.RWMutex
	streams   map[string]*StreamSubscription
	streamWg  sync.WaitGroup

	// Configuration
	retryConfig *RetryConfig
	timeout     time.Duration
	tlsConfig   *tls.Config

	// Internal
	httpClient *http.Client
	logger     *zap.SugaredLogger
}

// RetryConfig defines retry behavior
type RetryConfig struct {
	MaxAttempts int
	BaseDelay   time.Duration
	MaxDelay    time.Duration
}

// DefaultRetryConfig returns default retry configuration
func DefaultRetryConfig() *RetryConfig {
	return &RetryConfig{
		MaxAttempts: 3,
		BaseDelay:   time.Second,
		MaxDelay:    30 * time.Second,
	}
}

// GatewayOpts holds client configuration
type GatewayOpts struct {
	RetryConfig *RetryConfig
	Timeout     time.Duration
	Logger      *zap.SugaredLogger
	TLSConfig   *tls.Config
	Debug       bool
	Mode        *eventcatcher.Mode
}

// NewClient creates a new Event Catcher SDK client
func NewClient(config *GatewayOpts) *GatewayClient {
	if config == nil {
		config = &GatewayOpts{}
	}

	if config.RetryConfig == nil {
		config.RetryConfig = DefaultRetryConfig()
	}

	if config.Mode == nil {
		mode := eventcatcher.Sandbox
		config.Mode = &mode
	}

	if config.Timeout == 0 {
		config.Timeout = 30 * time.Second
	}

	if config.Logger == nil {
		zapConfig := zap.NewDevelopmentConfig()
		if config.Debug {
			zapConfig.Level = zap.NewAtomicLevelAt(zap.DebugLevel)
		} else {
			zapConfig.Level = zap.NewAtomicLevelAt(zap.InfoLevel)
		}
		zapConfig.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
		zLogger, err := zapConfig.Build()
		if err != nil {
			panic(err)
		}

		config.Logger = zLogger.Sugar()
	}

	httpClient := &http.Client{
		Timeout: config.Timeout,
	}

	var tlsConfig *tls.Config
	if config.TLSConfig != nil {
		httpClient.Transport = &http.Transport{
			TLSClientConfig: config.TLSConfig,
		}
		tlsConfig = config.TLSConfig
	}

	return &GatewayClient{
		gatewayURL:  eventcatcher.GetBaseURL(*config.Mode),
		retryConfig: config.RetryConfig,
		timeout:     config.Timeout,
		httpClient:  httpClient,
		logger:      config.Logger,
		tlsConfig:   tlsConfig,
		streams:     make(map[string]*StreamSubscription),
	}
}

type GetListNodeResponse struct {
	Success      bool           `json:"success"`
	Message      string         `json:"message"`
	ErrorMessage string         `json:"errorMessage"`
	Nodes        []NodeResponse `json:"nodes"`
}

type NodeResponse struct {
	Domain       string `json:"domain"`
	DomainHealth string `json:"domainHealth"`
	DomainRpc    string `json:"domainRpc"`
	Node         string `json:"node"`
	ChainId      string `json:"chainId"`
}

// RequestNodeFromGateway requests node address from gateway
func (c *GatewayClient) RequestNodeFromGateway(ctx context.Context, chainId string) (*GetListNodeResponse, error) {
	res, err := c.makeHTTPRequest(ctx, http.MethodGet, c.gatewayURL, "/v1/gateway/get_list_node?chain_id="+chainId, nil)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}

	if res.StatusCode < http.StatusOK || res.StatusCode >= http.StatusBadRequest {
		return nil, fmt.Errorf("failed to get node: %s, %s", res.Status, string(body))
	}
	result := &GetListNodeResponse{}
	if err = json.NewDecoder(res.Body).Decode(result); err != nil {
		return nil, err
	}

	return result, err
}

// newGRPCConn establishes gRPC connection
func newGRPCConn(url string, tlsConfig *tls.Config) (*grpc.ClientConn, error) {
	var tlsOpts grpc.DialOption
	if tlsConfig != nil {
		tlsOpts = grpc.WithTransportCredentials(credentials.NewTLS(tlsConfig))
	} else {
		tlsOpts = grpc.WithTransportCredentials(insecure.NewCredentials())
	}

	opts := []grpc.DialOption{
		tlsOpts,
	}

	conn, err := grpc.NewClient(url, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to dial gRPC server: %w", err)
	}

	return conn, nil
}

type Receipt struct {
	Node             string `json:"node"`
	TotalServedBytes string `json:"totalServedBytes"`
	Status           string `json:"status"`
	Nonce            string `json:"nonce"`
}

type ValidateClientResponse struct {
	Success         bool     `json:"success"`
	Message         string   `json:"message"`
	ConnectionToken string   `json:"connectionToken"`
	Receipt         *Receipt `json:"receipt"`
	ErrorMessage    string   `json:"errorMessage"`
}

/*
ValidateClient requests token for a node from gateway
*/
func (c *GatewayClient) ValidateClient(ctx context.Context, nodeAddress, signature, timestamp string) (*ValidateClientResponse, error) {
	body := map[string]string{
		"node":      nodeAddress,
		"signature": signature,
		"timestamp": timestamp,
	}
	res, err := c.makeHTTPRequest(ctx, http.MethodGet, c.gatewayURL, "/v1/gateway/validate_client", body)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	resData, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}

	if res.StatusCode < http.StatusOK || res.StatusCode >= http.StatusBadRequest {
		return nil, fmt.Errorf("failed to get node: %s, %s", res.Status, string(resData))
	}
	result := &ValidateClientResponse{}
	if err = json.NewDecoder(res.Body).Decode(result); err != nil {
		return nil, err
	}

	return result, err
}

/*
SubscribeEvent will spawn a goroutine to watch for events matching the filter for you

# You should pass a handler function to handle the events

# You need nodeURL returned from RequestNodeFromGateway and an authToken returned from ValidateClient

# Pass TLS config if you want to use TLS, leave it nil otherwise

# Example:
```go

	...
	client := NewClient(nil)
	nodes, err := client.RequestNodeFromGateway(context.Background(), 1)
	if err != nil {
		panic(err)
	}

	signature, _ = SignLoginMessage("0xExamplePrivateKey")
	auth, err := client.ValidateClient(context.Background(), nodeURL, "signature", "timestamp")
	if err != nil {
		panic(err)
	}

	subs, _ := SubscribeEvent(ctx, nodes.Nodes[0].Domain, auth.ConnectionToken...)

```
*/
func (c *GatewayClient) SubscribeEvent(ctx context.Context, tlsConfig *tls.Config, nodeURL, authToken string, filter *node2.StreamEventsRequest, handler EventHandler) (*StreamSubscription, error) {
	// Create cancellable context for this stream
	streamCtx, cancel := context.WithCancel(ctx)

	subscription := &StreamSubscription{
		ID:        uuid.New().String(),
		cancel:    cancel,
		authToken: authToken,
		nodeUrl:   nodeURL,
		tlsConfig: tlsConfig,
	}

	// Start stream
	c.streamWg.Add(1)
	go func() {
		defer c.streamWg.Done()
		client, err := NewNodeClient(nodeURL, tlsConfig)
		if err != nil {
			c.logger.Errorf("Failed to create node client: %v", err)
			return
		}

		header := &metadata.MD{
			"Authorization": []string{authToken},
		}
		stream, err := client.StreamEvents(streamCtx, filter, grpc.Header(header))
		if err != nil {
			c.logger.Errorf("Failed to start event stream: %v", err)
			return
		}
		defer stream.CloseSend()

		for {
			select {
			case <-streamCtx.Done():
				c.logger.Infof("Event stream closed: %s", streamCtx.Err())
				return
			default:
				msg, err := stream.Recv()
				if err != nil {
					c.logger.Errorf("Failed to receive event: %v", err)
					return
				}
				c.logger.Debugf("Received event: %v", msg)

				if handler != nil {
					handler(&Event{
						BlockNumber: uint64(msg.GetBlockNumber()),
						TxHash:      msg.GetTxHash(),
						Data:        msg.GetData(),
					})
				}
			}
		}
	}()

	// Add to active streams
	c.streamsMu.Lock()
	c.streams[subscription.ID] = subscription
	c.streamsMu.Unlock()

	c.logger.Infof("Started event stream: %s", subscription.ID)
	return subscription, nil
}

// NewStream creates a new stream to a node
func (c *GatewayClient) NewStream(ctx context.Context, tlsConfig *tls.Config, nodeURL, authToken string, filter *node2.StreamEventsRequest) (grpc.ServerStreamingClient[node2.Event], error) {
	client, err := NewNodeClient(nodeURL, tlsConfig)
	if err != nil {
		c.logger.Errorf("Failed to create node client: %v", err)
		return nil, err
	}

	header := &metadata.MD{
		"Authorization": []string{authToken},
	}
	stream, err := client.StreamEvents(ctx, filter, grpc.Header(header))
	if err != nil {
		c.logger.Errorf("Failed to start event stream: %v", err)
		return nil, err
	}

	return stream, nil
}

// Disconnect closes the connection and stops all streams
func (c *GatewayClient) Disconnect() error {
	c.logger.Info("Disconnecting...")

	// Cancel all active streams
	c.streamsMu.RLock()
	for _, subscription := range c.streams {
		subscription.cancel()
		go c.disconnectStream(subscription)
	}
	c.streamsMu.RUnlock()

	// Wait for all streams to finish
	c.streamWg.Wait()

	c.logger.Info("Disconnected successfully")
	return nil
}

// StopStream stops a specific event stream and commit the billing
func (c *GatewayClient) StopStream(subscriptionID string) (*node2.DisconnectStreamResponse, error) {
	c.streamsMu.RLock()
	subscription, exists := c.streams[subscriptionID]
	c.streamsMu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("stream %s does not exist", subscriptionID)
	}

	subscription.cancel()
	c.logger.Debugf("Stopped stream: %s", subscriptionID)

	return c.disconnectStream(subscription)
}

func (c *GatewayClient) disconnectStream(subscription *StreamSubscription) (*node2.DisconnectStreamResponse, error) {
	nodeClient, err := NewNodeClient(subscription.nodeUrl, subscription.tlsConfig)
	if err != nil {
		c.logger.Errorf("Failed to create node client: %v", err)
		return nil, err
	}

	header := &metadata.MD{
		"Authorization": []string{subscription.authToken},
	}
	res, err := nodeClient.DisconnectStream(context.Background(), &node2.DisconnectStreamRequest{}, grpc.Header(header))
	if err != nil {
		c.logger.Errorf("Failed to disconnect stream: %v", err)
		return nil, err
	}

	return res, nil
}

func (c *GatewayClient) RegisterNodeMonitorContract(ctx context.Context, nodeUrl string, data RegisterContract) (string, error) {
	res, err := c.makeHTTPRequest(ctx, "POST", nodeUrl, "/api/v1/contracts", data)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		return "", err
	}
	if res.StatusCode < http.StatusOK || res.StatusCode >= http.StatusBadRequest {
		return "", fmt.Errorf("failed to register contract: %s, %s", res.Status, string(body))
	}

	var id struct {
		ID string `json:"id"`
	}
	err = json.NewDecoder(res.Body).Decode(&id)
	if err != nil {
		return "", err
	}
	return id.ID, nil
}

type SyncStatusParams struct {
	ChainId         int    `json:"chainId"`
	ContractAddress string `json:"contractAddress"`
	EventName       string `json:"eventName"`
}

type SyncStatusResponse struct {
	ChainId            string   `json:"chainId"`
	ContractAddress    string   `json:"contractAddress"`
	EventName          string   `json:"eventName"`
	CurrentBlockNumber *big.Int `json:"currentBlockNumber"`
}

func (c *GatewayClient) GetEventCurrentSyncStatus(ctx context.Context, nodeUrl string, params SyncStatusParams) (*SyncStatusResponse, error) {
	res, err := c.makeHTTPRequest(
		ctx,
		"GET",
		nodeUrl,
		fmt.Sprintf("/api/v1/status/%d/%s/%s", params.ChainId, params.ContractAddress, params.EventName),
		nil)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}
	if res.StatusCode < http.StatusOK || res.StatusCode >= http.StatusBadRequest {
		return nil, fmt.Errorf("failed to get contract: %s, %s", res.Status, string(body))
	}

	var status SyncStatusResponse
	err = json.NewDecoder(res.Body).Decode(&status)
	if err != nil {
		return nil, err
	}

	return &status, nil
}

// NewNodeClient creates a new NodeServiceClient
func NewNodeClient(nodeUrl string, tlsConfig *tls.Config) (node2.EventServiceClient, error) {
	grpcConn, err := newGRPCConn(nodeUrl, tlsConfig)
	if err != nil {
		return nil, err
	}
	client := node2.NewEventServiceClient(grpcConn)
	return client, nil
}

// Helper methods

func (c *GatewayClient) makeHTTPRequest(ctx context.Context, method, url, path string, data any) (*http.Response, error) {
	var body io.Reader
	if data != nil {
		jsonData, err := json.Marshal(data)
		if err != nil {
			return nil, err
		}
		body = strings.NewReader(string(jsonData))
	}

	req, err := http.NewRequestWithContext(ctx, method, url+path, body)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "EventCatcher-SDK-Go/1.0")

	return c.httpClient.Do(req)
}

func SignLoginMessage(privateKey string, ts string) (string, error) {
	msgSignedOn := fmt.Sprintf("%s:%s", LOGIN_MESSAGE, ts)
	// Decode hex string private key (without "0x" prefix)
	keyBytes, err := hex.DecodeString(trimHexPrefix(privateKey))
	if err != nil {
		return "", fmt.Errorf("invalid private key: %w", err)
	}

	// Parse ECDSA private key
	privKey, err := crypto.ToECDSA(keyBytes)
	if err != nil {
		return "", fmt.Errorf("failed to parse private key: %w", err)
	}

	// Ethereum message prefix (standard for eth_sign)
	msg := []byte(fmt.Sprintf("\x19Ethereum Signed Message:\n%d%s", len(msgSignedOn), msgSignedOn))

	// Hash the message
	msgHash := crypto.Keccak256Hash(msg)

	// Sign the hashed message
	signature, err := crypto.Sign(msgHash.Bytes(), privKey)
	if err != nil {
		return "", fmt.Errorf("failed to sign message: %w", err)
	}

	// Add 27 to recovery ID to match Ethereum's "eth_sign" behavior (optional)
	signature[64] += 27

	// Return as 0x-prefixed hex string
	return "0x" + hex.EncodeToString(signature), nil
}

func trimHexPrefix(s string) string {
	if len(s) >= 2 && s[0:2] == "0x" {
		return s[2:]
	}
	return s
}
