/*
	Copyright (C) 2021-2024  The YNOproject Developers

	This program is free software: you can redistribute it and/or modify
	it under the terms of the GNU Affero General Public License as published by
	the Free Software Foundation, either version 3 of the License, or
	(at your option) any later version.

	This program is distributed in the hope that it will be useful,
	but WITHOUT ANY WARRANTY; without even the implied warranty of
	MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
	GNU Affero General Public License for more details.

	You should have received a copy of the GNU Affero General Public License
	along with this program.  If not, see <http://www.gnu.org/licenses/>.
*/

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

func initHistory() {
	// Use main server to process chat message cleaning task for all games
	if isMainServer {
		logInitTask("history")

		scheduler.Cron("0 * * * *").Do(deleteOldChatMessages)
	}
}
