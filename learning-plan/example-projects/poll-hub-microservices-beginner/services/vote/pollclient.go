package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"time"
)

var errPollNotFound = errors.New("poll not found")

// PollClient is how vote-service learns about polls: it asks
// poll-service over HTTP instead of reading the polls table itself.
// Each service touches only the tables it owns.
type PollClient struct {
	baseURL string
	http    *http.Client
}

func NewPollClient(baseURL string) *PollClient {
	return &PollClient{
		baseURL: baseURL,
		http:    &http.Client{Timeout: 5 * time.Second},
	}
}

// pollInfo is the slice of poll-service's response we care about —
// decoding into a small struct simply ignores the other fields.
type pollInfo struct {
	ID      int64        `json:"id"`
	Status  string       `json:"status"`
	Options []pollOption `json:"options"`
}

type pollOption struct {
	ID int64 `json:"id"`
}

func (p *pollInfo) IsOpen() bool { return p.Status == "open" }

func (p *pollInfo) HasOption(id int64) bool {
	return slices.ContainsFunc(p.Options, func(o pollOption) bool { return o.ID == id })
}

// Get fetches one poll. errPollNotFound means the poll doesn't exist;
// any other error means poll-service couldn't be reached or misbehaved.
func (c *PollClient) Get(ctx context.Context, pollID int64) (*pollInfo, error) {
	url := fmt.Sprintf("%s/polls/%d", c.baseURL, pollID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("call poll-service: %w", err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
		var p pollInfo
		if err := json.NewDecoder(resp.Body).Decode(&p); err != nil {
			return nil, fmt.Errorf("decode poll-service response: %w", err)
		}
		return &p, nil
	case http.StatusNotFound:
		return nil, errPollNotFound
	default:
		return nil, fmt.Errorf("poll-service returned %s", resp.Status)
	}
}
