package main

import (
	"encoding/json"
	"time"

	"github.com/go-co-op/gocron"
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
	MapId         string `json:"mapId"`
	PrevMapId     string `json:"prevMapId"`
	PrevLocations string `json:"prevLocations"`
	X             int    `json:"x"`
	Y             int    `json:"y"`
	Online        bool   `json:"online"`
}

func startPartyUpdateTimer() {
	s := gocron.NewScheduler(time.UTC)
	
	s.Every(5).Seconds().Do(sendPartyUpdate())
}

func sendPartyUpdate() error {
	parties, err := readAllPartyData()
	if err != nil {
		return err //unused
	}

	for _, party := range parties { //for every party
		partyDataJson, err := json.Marshal(party)
		if err != nil {
			continue
		}

		for _, member := range party.Members { //for every member
			if member.Online {
				if client, ok := sessionClients[member.Uuid]; ok {
					client.send <- []byte("p" + paramDelimStr + string(partyDataJson)) //send JSON to client
				}
			}
		}
	}

	return nil
}
