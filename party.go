/*
	Copyright (C) 2021-2022  The YNOproject Developers

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

package main

import (
	"encoding/json"
)

type Party struct {
	Id          int           `json:"id"`
	Name        string        `json:"name"`
	Public      bool          `json:"public"`
	Pass        string        `json:"pass"`
	SystemName  string        `json:"systemName"`
	Description string        `json:"description"`
	OwnerUuid   string        `json:"ownerUuid"`
	Members     []PartyMember `json:"members"`
}

type PartyMember struct {
	Uuid          string `json:"uuid"`
	Name          string `json:"name"`
	Rank          int    `json:"rank"`
	Account       bool   `json:"account"`
	Badge         string `json:"badge"`
	SystemName    string `json:"systemName"`
	SpriteName    string `json:"spriteName"`
	SpriteIndex   int    `json:"spriteIndex"`
	MapId         string `json:"mapId,omitempty"`
	PrevMapId     string `json:"prevMapId,omitempty"`
	PrevLocations string `json:"prevLocations,omitempty"`
	X             int    `json:"x"`
	Y             int    `json:"y"`
	Online        bool   `json:"online"`
}

func sendPartyUpdate() {
	parties, err := getAllPartyData(false)
	if err != nil {
		return
	}

	for _, party := range parties { // for every party
		var partyPass string
		if party.Pass != "" {
			partyPass = party.Pass
			party.Pass = ""
		}
		partyDataJson, err := json.Marshal(party)
		if err != nil {
			continue
		}

		for _, member := range party.Members { // for every member
			if !member.Online {
				continue
			}
			if client, ok := clients.Load(member.Uuid); ok {
				var jsonData []byte
				if member.Uuid == party.OwnerUuid {
					// Expose password only for party owner
					party.Pass = partyPass
					ownerPartyDataJson, err := json.Marshal(party)
					party.Pass = ""
					if err != nil {
						continue
					}
					jsonData = ownerPartyDataJson
				} else {
					jsonData = partyDataJson
				}
				client.(*SessionClient).sendMsg("pt", jsonData) // send JSON to client
			}
		}
	}
}
