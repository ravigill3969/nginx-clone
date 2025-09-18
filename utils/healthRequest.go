package utils

import (
	"fmt"
	"net/http"
	"time"
)

type BackendURL string

func (b BackendURL) Validate() error {
	if b == "" {
		return fmt.Errorf("backend URL is empty")
	}

	client := http.Client{Timeout: 2 * time.Second}

	resp, err := client.Get(string(b))

	if err != nil {
		return fmt.Errorf("backend unreachable: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("received %d from %s", resp.StatusCode, string(b))
	}
	return nil
}
