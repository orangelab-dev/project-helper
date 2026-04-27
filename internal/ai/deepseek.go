package ai

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/cloudwego/eino/schema"

	"project-helper/internal/config"
)

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type StreamEvent struct {
	Type string `json:"type"`
	Data string `json:"data"`
}

type Client interface {
	Generate(ctx context.Context, messages []Message) (string, error)
	Stream(ctx context.Context, messages []Message) (<-chan StreamEvent, <-chan error)
}

type DeepSeekClient struct {
	cfg        config.DeepSeekConfig
	httpClient *http.Client
}

func NewDeepSeekClient(cfg config.DeepSeekConfig) *DeepSeekClient {
	return &DeepSeekClient{
		cfg: cfg,
		httpClient: &http.Client{
			Timeout: 10 * time.Minute,
		},
	}
}

func (c *DeepSeekClient) Generate(ctx context.Context, messages []Message) (string, error) {
	if err := c.ready(); err != nil {
		return "", err
	}
	reqBody := chatRequest{Model: c.cfg.Model, Messages: messages, Stream: false}
	var resp chatResponse
	if err := c.doJSON(ctx, reqBody, &resp); err != nil {
		return "", err
	}
	if len(resp.Choices) == 0 {
		return "", errors.New("DeepSeek 未返回内容")
	}
	return resp.Choices[0].Message.Content, nil
}

func (c *DeepSeekClient) Stream(ctx context.Context, messages []Message) (<-chan StreamEvent, <-chan error) {
	events := make(chan StreamEvent)
	errs := make(chan error, 1)
	go func() {
		defer close(events)
		defer close(errs)
		if err := c.ready(); err != nil {
			errs <- err
			return
		}
		body, _ := json.Marshal(chatRequest{Model: c.cfg.Model, Messages: messages, Stream: true})
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.cfg.BaseURL+"/chat/completions", bytes.NewReader(body))
		if err != nil {
			errs <- err
			return
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+c.cfg.APIKey)
		resp, err := c.httpClient.Do(req)
		if err != nil {
			errs <- err
			return
		}
		defer resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			payload, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
			errs <- fmt.Errorf("DeepSeek 请求失败: %s %s", resp.Status, strings.TrimSpace(string(payload)))
			return
		}
		scanner := bufio.NewScanner(resp.Body)
		buf := make([]byte, 0, 1024*1024)
		scanner.Buffer(buf, 1024*1024)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" || !strings.HasPrefix(line, "data:") {
				continue
			}
			payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			if payload == "[DONE]" {
				events <- StreamEvent{Type: "done"}
				return
			}
			var chunk streamChunk
			if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
				continue
			}
			for _, choice := range chunk.Choices {
				if choice.Delta.ReasoningContent != "" {
					events <- StreamEvent{Type: "reasoning", Data: choice.Delta.ReasoningContent}
				}
				if choice.Delta.Content != "" {
					events <- StreamEvent{Type: "token", Data: choice.Delta.Content}
				}
			}
		}
		if err := scanner.Err(); err != nil {
			errs <- err
		}
	}()
	return events, errs
}

func (c *DeepSeekClient) ready() error {
	if c.cfg.APIKey == "" || c.cfg.APIKey == "你的_key" {
		return errors.New("请先在 .env 中配置 DEEPSEEK_API_KEY")
	}
	if c.cfg.Model == "" {
		return errors.New("请配置 DEEPSEEK_MODEL")
	}
	return nil
}

func (c *DeepSeekClient) doJSON(ctx context.Context, reqBody chatRequest, target any) error {
	body, _ := json.Marshal(reqBody)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.cfg.BaseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.cfg.APIKey)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		payload, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("DeepSeek 请求失败: %s %s", resp.Status, strings.TrimSpace(string(payload)))
	}
	return json.NewDecoder(resp.Body).Decode(target)
}

func ToEinoMessages(messages []Message) []*schema.Message {
	out := make([]*schema.Message, 0, len(messages))
	for _, msg := range messages {
		role := schema.User
		switch msg.Role {
		case "system":
			role = schema.System
		case "assistant":
			role = schema.Assistant
		}
		out = append(out, &schema.Message{Role: role, Content: msg.Content})
	}
	return out
}

type chatRequest struct {
	Model    string    `json:"model"`
	Messages []Message `json:"messages"`
	Stream   bool      `json:"stream"`
}

type chatResponse struct {
	Choices []struct {
		Message Message `json:"message"`
	} `json:"choices"`
}

type streamChunk struct {
	Choices []struct {
		Delta struct {
			Content          string `json:"content"`
			ReasoningContent string `json:"reasoning_content"`
		} `json:"delta"`
	} `json:"choices"`
}
