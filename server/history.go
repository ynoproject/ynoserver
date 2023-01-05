package server

import (
	"time"
)

type ChatPlayer struct {
	Uuid       string `json:"uuid"`
	Name       string `json:"name"`
	SystemName string `json:"systemName"`
	Rank       int    `json:"rank"`
	Account    bool   `json:"account"`
	Badge      string `json:"badge"`
	Medals     [5]int `json:"medals"`
}

type ChatMessage struct {
	MsgId         string    `json:"msgId"`
	Uuid          string    `json:"uuid"`
	MapId         string    `json:"mapId"`
	PrevMapId     string    `json:"prevMapId"`
	PrevLocations string    `json:"prevLocations"`
	X             int       `json:"x"`
	Y             int       `json:"y"`
	Contents      string    `json:"contents"`
	Timestamp     time.Time `json:"timestamp"`
	Party         bool      `json:"party"`
}

type ChatHistory struct {
	Players  []*ChatPlayer  `json:"players"`
	Messages []*ChatMessage `json:"messages"`
}

var (
	msgSent    bool
	lastMsgIds map[int]string
)

func initHistory() {
	lastMsgIds, _ = getLastMessageIds()
}
