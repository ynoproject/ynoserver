package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"
)

var urlReplacer = strings.NewReplacer("http://", "", "https://", "")

type WebhookRequest struct {
	Username string `json:"username"`
	Content string `json:"content"`
	AllowedMentions struct {
		Parse []string `json:"parse"`
	} `json:"allowed_mentions"`
}

func sendWebhookMessage(name, contents string) error {
	body, err := json.Marshal(WebhookRequest{
		Username: name,
		Content: urlReplacer.Replace(contents),
	})
	if err != nil {
		return err
	}

	resp, err := http.Post(config.webhookUrl, "application/json", bytes.NewReader(body))
	if err != nil {
		return err
	}

	defer resp.Body.Close()

	return nil
}
