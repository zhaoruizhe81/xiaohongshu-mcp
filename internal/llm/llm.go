package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config OpenAI 兼容接口配置（单多模态模型）。
type Config struct {
	BaseURL     string
	APIKey      string
	Model       string
	Temperature float64
	Timeout     time.Duration
}

// FromEnv 从环境变量读取配置。
// 兼容两套命名：优先 LLM_*，回退标准 OPENAI_*（LLM_BASE_URL/OPENAI_BASE_URL 等）。
func FromEnv() Config {
	temp := 0.85
	if v := firstNonEmpty(os.Getenv("LLM_TEMPERATURE"), os.Getenv("OPENAI_TEMPERATURE")); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			temp = f
		}
	}
	return Config{
		BaseURL:     strings.TrimRight(firstNonEmpty(os.Getenv("LLM_BASE_URL"), os.Getenv("OPENAI_BASE_URL"), "https://api.openai.com/v1"), "/"),
		APIKey:      firstNonEmpty(os.Getenv("LLM_API_KEY"), os.Getenv("OPENAI_API_KEY")),
		Model:       firstNonEmpty(os.Getenv("LLM_MODEL"), os.Getenv("OPENAI_MODEL")),
		Temperature: temp,
		Timeout:     60 * time.Second,
	}
}

func firstNonEmpty(ss ...string) string {
	for _, s := range ss {
		if s != "" {
			return s
		}
	}
	return ""
}

// Enabled 配置齐全才可用。
func (c Config) Enabled() bool { return c.APIKey != "" && c.Model != "" }

// Client 简单 OpenAI 兼容 chat 客户端。
type Client struct {
	cfg Config
	hc  *http.Client
}

func New(cfg Config) *Client {
	return &Client{cfg: cfg, hc: &http.Client{Timeout: cfg.Timeout}}
}

type imageURL struct {
	URL string `json:"url"`
}

// part 是 OpenAI 多模态 content 的一段（text 或 image_url）。
type part struct {
	Type     string    `json:"type"`
	Text     string    `json:"text,omitempty"`
	ImageURL *imageURL `json:"image_url,omitempty"`
}

type message struct {
	Role    string `json:"role"`
	Content any    `json:"content"`
}

type chatRequest struct {
	Model       string    `json:"model"`
	Temperature float64   `json:"temperature"`
	Messages    []message `json:"messages"`
}

type chatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// Chat 发一次对话；text 与图片(image data URL)一起作为 user content。
func (c *Client) Chat(ctx context.Context, system, userText string, imageDataURLs []string) (string, error) {
	parts := []part{{Type: "text", Text: userText}}
	for _, u := range imageDataURLs {
		parts = append(parts, part{Type: "image_url", ImageURL: &imageURL{URL: u}})
	}
	body := chatRequest{
		Model:       c.cfg.Model,
		Temperature: c.cfg.Temperature,
		Messages: []message{
			{Role: "system", Content: system},
			{Role: "user", Content: parts},
		},
	}
	b, err := json.Marshal(body)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.cfg.BaseURL+"/chat/completions", bytes.NewReader(b))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.cfg.APIKey)

	resp, err := c.hc.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("LLM HTTP %d: %s", resp.StatusCode, string(raw))
	}
	var cr chatResponse
	if err := json.Unmarshal(raw, &cr); err != nil {
		return "", err
	}
	if cr.Error != nil {
		return "", fmt.Errorf("LLM error: %s", cr.Error.Message)
	}
	if len(cr.Choices) == 0 {
		return "", fmt.Errorf("LLM 返回空 choices")
	}
	return strings.TrimSpace(cr.Choices[0].Message.Content), nil
}
