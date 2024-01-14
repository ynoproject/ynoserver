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

func sendWebhookMessage(url, name, badge, message string, sanitize bool) error {
	var avatarUrl string
	if badge != "" {
		avatarUrl = fmt.Sprintf("https://ynoproject.net/%s/images/badge/%s.png", config.gameName, badge)
	}

	content := message
	if sanitize {
		content = urlReplacer.Replace(message)
	}

	body, err := json.Marshal(WebhookRequest{
		Username:  name,
		AvatarUrl: avatarUrl,
		Content:  content,
	})
	if err != nil {
		return err
	}

	resp, err := http.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		return err
	}

	defer resp.Body.Close()

	return nil
}
