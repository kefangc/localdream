package comfy

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"localdream/internal/workflow"
)

const maxObjectInfoResponseBytes int64 = 4 << 20

type Client struct {
	baseURL    string
	httpClient *http.Client
}

func NewClient(baseURL string, httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = &http.Client{
			Timeout: 30 * time.Second,
		}
	}

	return &Client{
		baseURL:    strings.TrimRight(baseURL, "/"),
		httpClient: httpClient,
	}
}

func (c *Client) ObjectInfo(
	ctx context.Context,
	nodeName string,
) ([]byte, error) {
	if c.baseURL == "" {
		return nil, fmt.Errorf("ComfyUI base URL is required")
	}

	if strings.TrimSpace(nodeName) == "" {
		return nil, fmt.Errorf("ComfyUI node name is required")
	}

	endpoint := c.baseURL +
		"/object_info/" +
		url.PathEscape(nodeName)

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		endpoint,
		nil,
	)
	if err != nil {
		return nil, fmt.Errorf("create ComfyUI object info request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request ComfyUI object info: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(
		resp.Body,
		maxObjectInfoResponseBytes+1,
	))

	if err != nil {
		return nil, fmt.Errorf("read ComfyUI object info response: %w", err)
	}

	if int64(len(body)) > maxObjectInfoResponseBytes {
		return nil, fmt.Errorf(
			"ComfyUI object info response exceeds %d bytes",
			maxObjectInfoResponseBytes,
		)
	}

	if resp.StatusCode < http.StatusOK ||
		resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf(
			"ComfyUI object info returned %s : %s",
			resp.Status,
			strings.TrimSuffix(string(body), "\n"),
		)
	}

	return body, nil
}

func (c *Client) RuntimeAssetSlots(
	ctx context.Context,
	nodeName string,
) (workflow.RuntimeAssetSlots, error) {
	raw, err := c.ObjectInfo(ctx, nodeName)
	if err != nil {
		return workflow.RuntimeAssetSlots{}, err
	}

	slots, err := workflow.ParseRuntimeAssetSlots(
		raw,
		nodeName,
	)
	if err != nil {
		return workflow.RuntimeAssetSlots{}, fmt.Errorf(
			"parse ComfyUI runtime asset slots: %w",
			err,
		)
	}
	return slots, nil
}
