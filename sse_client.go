package flipswitch

import (
	"bufio"
	"context"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"encoding/json"
)

const (
	minRetryDelay = 1 * time.Second
	maxRetryDelay = 30 * time.Second
)

// SseClient handles SSE connections for real-time flag change notifications.
type SseClient struct {
	baseURL        string
	apiKey         string
	onFlagChange   FlagChangeHandler
	onStatusChange ConnectionStatusHandler
	httpClient     *http.Client

	status     ConnectionStatus
	retryDelay time.Duration
	closed     bool
	mu         sync.RWMutex
	ctx        context.Context
	cancel     context.CancelFunc
}

// NewSseClient creates a new SSE client.
func NewSseClient(
	baseURL string,
	apiKey string,
	onFlagChange FlagChangeHandler,
	onStatusChange ConnectionStatusHandler,
) *SseClient {
	ctx, cancel := context.WithCancel(context.Background())
	return &SseClient{
		baseURL:        strings.TrimSuffix(baseURL, "/"),
		apiKey:         apiKey,
		onFlagChange:   onFlagChange,
		onStatusChange: onStatusChange,
		httpClient: &http.Client{
			Timeout: 0, // No timeout for SSE
		},
		status:     StatusDisconnected,
		retryDelay: minRetryDelay,
		ctx:        ctx,
		cancel:     cancel,
	}
}

// Connect starts the SSE connection in a background goroutine.
func (c *SseClient) Connect() {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return
	}
	c.mu.Unlock()

	go c.connectLoop()
}

func (c *SseClient) connectLoop() {
	for {
		c.mu.RLock()
		closed := c.closed
		c.mu.RUnlock()

		if closed {
			return
		}

		err := c.connect()
		if err != nil {
			c.mu.RLock()
			closed := c.closed
			c.mu.RUnlock()

			if !closed {
				log.Printf("[Flipswitch] SSE connection error: %v", err)
				c.updateStatus(StatusError)
				c.scheduleReconnect()
			}
		}
	}
}

func (c *SseClient) connect() error {
	c.updateStatus(StatusConnecting)

	url := c.baseURL + "/api/v1/flags/events"

	req, err := http.NewRequestWithContext(c.ctx, "GET", url, nil)
	if err != nil {
		return err
	}

	req.Header.Set("X-API-Key", c.apiKey)
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Cache-Control", "no-cache")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return &sseError{statusCode: resp.StatusCode}
	}

	log.Println("[Flipswitch] SSE connection established")
	c.updateStatus(StatusConnected)

	c.mu.Lock()
	c.retryDelay = minRetryDelay
	c.mu.Unlock()

	reader := bufio.NewReader(resp.Body)
	var eventType, eventData string

	for {
		select {
		case <-c.ctx.Done():
			return nil
		default:
		}

		line, err := reader.ReadString('\n')
		if err != nil {
			c.mu.RLock()
			closed := c.closed
			c.mu.RUnlock()

			if !closed {
				log.Println("[Flipswitch] SSE connection closed")
				c.updateStatus(StatusDisconnected)
				c.scheduleReconnect()
			}
			return nil
		}

		line = strings.TrimSpace(line)

		if strings.HasPrefix(line, "event:") {
			eventType = strings.TrimSpace(line[6:])
		} else if strings.HasPrefix(line, "data:") {
			eventData = strings.TrimSpace(line[5:])
		} else if line == "" && eventData != "" {
			c.handleEvent(eventType, eventData)
			eventType = ""
			eventData = ""
		}
	}
}

type sseError struct {
	statusCode int
}

func (e *sseError) Error() string {
	return "SSE connection failed with status: " + intToString(e.statusCode)
}

func (c *SseClient) handleEvent(eventType, data string) {
	if eventType == "heartbeat" {
		return
	}

	if eventType == "flag-change" {
		var event FlagChangeEvent
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			log.Printf("[Flipswitch] Failed to parse flag-change event: %v", err)
			return
		}

		if c.onFlagChange != nil {
			c.onFlagChange(event)
		}
	}
}

func (c *SseClient) scheduleReconnect() {
	c.mu.RLock()
	closed := c.closed
	delay := c.retryDelay
	c.mu.RUnlock()

	if closed {
		return
	}

	log.Printf("[Flipswitch] Scheduling SSE reconnect in %v", delay)

	select {
	case <-time.After(delay):
	case <-c.ctx.Done():
		return
	}

	c.mu.Lock()
	if c.retryDelay < maxRetryDelay {
		c.retryDelay = c.retryDelay * 2
		if c.retryDelay > maxRetryDelay {
			c.retryDelay = maxRetryDelay
		}
	}
	c.mu.Unlock()
}

func (c *SseClient) updateStatus(status ConnectionStatus) {
	c.mu.Lock()
	c.status = status
	c.mu.Unlock()

	if c.onStatusChange != nil {
		c.onStatusChange(status)
	}
}

// GetStatus returns the current connection status.
func (c *SseClient) GetStatus() ConnectionStatus {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.status
}

// Close closes the SSE connection and stops reconnection attempts.
func (c *SseClient) Close() {
	c.mu.Lock()
	c.closed = true
	c.mu.Unlock()

	c.cancel()
	c.updateStatus(StatusDisconnected)
}
