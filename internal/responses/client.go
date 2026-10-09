package responses

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Tool struct {
	Type        string `json:"type"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Strict      bool   `json:"strict"`
	Parameters  any    `json:"parameters"`
}

type Request struct {
	Model        string `json:"model"`
	Instructions string `json:"instructions"`
	Input        []any  `json:"input"`
	Tools        []Tool `json:"tools"`
	ToolChoice   string `json:"tool_choice"`
	Parallel     bool   `json:"parallel_tool_calls"`
	Store        bool   `json:"store"`
}

type Response struct {
	ID                string            `json:"id"`
	Status            string            `json:"status"`
	Output            []json.RawMessage `json:"output"`
	Error             json.RawMessage   `json:"error"`
	IncompleteDetails json.RawMessage   `json:"incomplete_details"`
}

type FunctionCall struct {
	Type      string `json:"type"`
	CallID    string `json:"call_id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

func (r Response) Calls() ([]FunctionCall, error) {
	calls := make([]FunctionCall, 0)
	for _, item := range r.Output {
		var call FunctionCall
		if err := json.Unmarshal(item, &call); err != nil {
			return nil, err
		}
		if call.Type != "function_call" {
			continue
		}
		if call.CallID == "" || call.Name == "" {
			return nil, errors.New("incomplete function call")
		}
		calls = append(calls, call)
	}
	return calls, nil
}

func (r Response) Text() string {
	var text strings.Builder
	for _, item := range r.Output {
		var message struct {
			Type    string `json:"type"`
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		}
		if json.Unmarshal(item, &message) != nil || message.Type != "message" {
			continue
		}
		for _, part := range message.Content {
			if part.Type == "output_text" {
				text.WriteString(part.Text)
			}
		}
	}
	return text.String()
}

type Client struct {
	HTTP *http.Client
}

func Endpoint(base string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(base))
	if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return "", errors.New("invalid Responses API base URL")
	}
	p := strings.TrimRight(u.Path, "/")
	switch {
	case strings.HasSuffix(p, "/v1/responses"):
	case strings.HasSuffix(p, "/v1"):
		p += "/responses"
	case strings.HasSuffix(p, "/responses"):
	default:
		p += "/v1/responses"
	}
	u.Path = p
	return u.String(), nil
}

func (c *Client) Create(ctx context.Context, baseURL, apiKey string, input Request) (Response, error) {
	endpoint, err := Endpoint(baseURL)
	if err != nil {
		return Response{}, err
	}
	if input.Model == "" {
		return Response{}, errors.New("model is required")
	}
	body, err := json.Marshal(input)
	if err != nil {
		return Response{}, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return Response{}, err
	}
	request.Header.Set("Content-Type", "application/json")
	if apiKey != "" {
		request.Header.Set("Authorization", "Bearer "+apiKey)
	}
	client := c.HTTP
	if client == nil {
		client = &http.Client{Timeout: 2 * time.Minute}
	}
	response, err := client.Do(request)
	if err != nil {
		return Response{}, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return Response{}, fmt.Errorf("Responses provider returned HTTP %d", response.StatusCode)
	}
	var result Response
	limited := io.LimitReader(response.Body, 8<<20)
	if err := json.NewDecoder(limited).Decode(&result); err != nil {
		return Response{}, fmt.Errorf("decode Responses result: %w", err)
	}
	if result.Status != "completed" {
		return Response{}, fmt.Errorf("Responses result is %q", result.Status)
	}
	return result, nil
}
