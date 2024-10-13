package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

var (
	urlReplacer = strings.NewReplacer("http://", "", "https://", "")
	gameIdToName = map[string]string{
		"2kki": "Yume 2kki",
		"amillusion": "Amillusion",
		"braingirl": "Braingirl",
		"deepdreams": "Deep Dreams",
		"flow": ".flow",
		"genie": "Dream Genie",
		"if": "If",
		"mikan": "Mikan Muzou",
		"muma": "Muma|Rope",
		"nostalgic": "nostAlgic",
		"oneshot": "OneShot",
		"oversomnia": "Oversomnia",
		"prayers": "Answered Prayers",
		"sheawaits": "She Awaits",
		"someday": "Someday",
		"tsushin": "Yume Tsushin",
		"ultraviolet": "Ultra Violet",
		"unconscious": "Collective Unconscious",
		"unevendream": "Uneven Dream",
		"yume": "Yume Nikki",
	}
)

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
