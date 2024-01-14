package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

var urlReplacer = strings.NewReplacer("http://", "", "https://", "")

type WebhookRequest struct {
	Username        string `json:"username"`
	AvatarUrl       string `json:"avatar_url"`
	Content         string `json:"content"`
	AllowedMentions struct {
		Parse []string `json:"parse"`
	} `json:"allowed_mentions"`
}

func sendWebhookMessage(name, badge, contents string) error {
	var avatarUrl string
	if badge != "" {
		avatarUrl = fmt.Sprintf("https://ynoproject.net/%s/badges/%s.png", config.gameName, badge)
	}

	body, err := json.Marshal(WebhookRequest{
		Username:  name,
		AvatarUrl: avatarUrl,
		Content:   urlReplacer.Replace(contents),
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
