/*
	Copyright (C) 2021-2023  The YNOproject Developers

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
	"database/sql"
	"encoding/json"
	"errors"
)

type Party struct {
	Id          int            `json:"id"`
	Name        string         `json:"name"`
	Public      bool           `json:"public"`
	Pass        string         `json:"-"`
	SystemName  string         `json:"systemName"`
	Description string         `json:"description"`
	OwnerUuid   string         `json:"ownerUuid"`
	Members     []*PartyMember `json:"members"`
}

type PartyMember struct {
	Uuid          string `json:"uuid"`
	Name          string `json:"name"`
	Rank          int    `json:"rank"`
	Account       bool   `json:"account"`
	Badge         string `json:"badge"`
	Medals        [5]int `json:"medals"`
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

var parties = make(map[int]*Party)

func sendPartyUpdate() {
	parties, err := getAllPartyData()
	if err != nil {
		return
	}

	for _, party := range parties { // for every party
		partyDataJson, err := json.Marshal(party)
		if err != nil {
			continue
		}

		for _, member := range party.Members { // for every member
			if member.Online {
				if client, ok := clients.Load(member.Uuid); ok {
					client.(*SessionClient).send <- buildMsg("pt", partyDataJson) // send JSON to client
				}
			}
		}
	}
}

func (c *SessionClient) cacheParty() error {
	partyId, err := getPlayerPartyId(c.uuid)
	if err != nil {
		return err
	}

	if _, ok := parties[partyId]; ok { // it's already in the cache
		return nil
	}

	party, err := getPartyDataFromDatabase(c.uuid)
	if err != nil {
		return err
	}

	parties[party.Id] = &party

	return nil
}

func getPlayerPartyId(uuid string) (partyId int, err error) {
	err = db.QueryRow("SELECT pm.partyId FROM partyMembers pm JOIN parties p ON p.id = pm.partyId WHERE pm.uuid = ? AND p.game = ?", uuid, serverConfig.GameName).Scan(&partyId)
	if err != nil {
		if err == sql.ErrNoRows {
			return 0, nil
		}

		return 0, err
	}

	return partyId, nil
}

func getPartyData(partyId int) (Party, error) {
	party, ok := parties[partyId]
	if !ok {
		return Party{}, errors.New("party id not in cache")
	}

	partyDeref := *party // dereference party so we don't edit the cached one

	for _, member := range partyDeref.Members {
		client, ok := clients.Load(member.Uuid)
		if !ok {
			continue
		}

		session := client.(*SessionClient)

		if session.name != "" {
			member.Name = session.name
		}
		if session.systemName != "" {
			member.SystemName = session.systemName
		}
		if session.spriteName != "" {
			member.SpriteName = session.spriteName
		}
		if session.spriteIndex > -1 {
			member.SpriteIndex = session.spriteIndex
		}

		if session.rClient != nil {
			member.MapId = session.rClient.mapId
			member.PrevMapId = session.rClient.prevMapId
			member.PrevLocations = session.rClient.prevLocations
			member.X = session.rClient.x
			member.Y = session.rClient.y
		}

		member.Online = true
	}

	return partyDeref, nil
}

func getAllPartyData() ([]Party, error) {
	var partyData []Party
	for partyId := range parties {
		party, err := getPartyData(partyId)
		if err != nil {
			return []Party{}, nil
		}

		partyData = append(partyData, party)
	}

	return partyData, nil
}

func getPartyDataFromDatabase(playerUuid string) (party Party, err error) {
	err = db.QueryRow("SELECT p.id, p.owner, p.name, p.public, p.pass, p.theme, p.description FROM parties p JOIN partyMembers pm ON pm.partyId = p.id JOIN playerGameData pgd ON pgd.uuid = pm.uuid AND pgd.game = p.game WHERE p.game = ? AND pm.uuid = ?", serverConfig.GameName, playerUuid).Scan(&party.Id, &party.OwnerUuid, &party.Name, &party.Public, &party.Pass, &party.SystemName, &party.Description)
	if err != nil {
		return party, err
	}

	partyMembers, err := getPartyMemberDataFromDatabase(party.Id)
	if err != nil {
		return party, err
	}

	party.Members = partyMembers

	return party, nil
}

func getPartyMemberDataFromDatabase(partyId int) (partyMembers []*PartyMember, err error) {
	results, err := db.Query("SELECT pm.partyId, pm.uuid, COALESCE(a.user, pgd.name), pd.rank, CASE WHEN a.user IS NULL THEN 0 ELSE 1 END, COALESCE(a.badge, ''), pgd.systemName, pgd.spriteName, pgd.spriteIndex, pgd.medalCountBronze, pgd.medalCountSilver, pgd.medalCountGold, pgd.medalCountPlatinum, pgd.medalCountDiamond FROM partyMembers pm JOIN playerGameData pgd ON pgd.uuid = pm.uuid JOIN players pd ON pd.uuid = pgd.uuid JOIN parties p ON p.id = pm.partyId LEFT JOIN accounts a ON a.uuid = pd.uuid WHERE pm.partyId = ? AND pgd.game = ? ORDER BY CASE WHEN p.owner = pm.uuid THEN 0 ELSE 1 END, pd.rank DESC, pm.id", partyId, serverConfig.GameName)
	if err != nil {
		return partyMembers, err
	}

	defer results.Close()

	for results.Next() {
		var partyId int
		var accountBin int

		partyMember := &PartyMember{
			MapId:     "0000",
			PrevMapId: "0000",
		}

		err := results.Scan(&partyId, &partyMember.Uuid, &partyMember.Name, &partyMember.Rank, &accountBin, &partyMember.Badge, &partyMember.SystemName, &partyMember.SpriteName, &partyMember.SpriteIndex, &partyMember.Medals[0], &partyMember.Medals[1], &partyMember.Medals[2], &partyMember.Medals[3], &partyMember.Medals[4])
		if err != nil {
			return partyMembers, err
		}

		partyMember.Account = accountBin == 1

		if _, ok := clients.Load(partyMember.Uuid); ok {
			partyMember.Online = true
		}

		partyMembers = append(partyMembers, partyMember)
	}

	return partyMembers, nil
}

func createPartyData(name string, public bool, pass string, theme string, description string, playerUuid string) (partyId int, err error) {
	results, err := db.Exec("INSERT INTO parties (game, owner, name, public, pass, theme, description) VALUES (?, ?, ?, ?, ?, ?, ?)", serverConfig.GameName, playerUuid, name, public, pass, theme, description)
	if err != nil {
		return 0, err
	}

	var partyId64 int64

	partyId64, err = results.LastInsertId()
	if err != nil {
		return 0, err
	}

	partyId = int(partyId64)

	return partyId, nil
}

func updatePartyData(partyId int, name string, public bool, pass string, theme string, description string, playerUuid string) error {
	_, err := db.Exec("UPDATE parties SET game = ?, owner = ?, name = ?, public = ?, pass = ?, theme = ?, description = ? WHERE id = ?", serverConfig.GameName, playerUuid, name, public, pass, theme, description, partyId)
	if err != nil {
		return err
	}

	party, ok := parties[partyId]
	if !ok {
		return errors.New("party id not in cache")
	}

	party.OwnerUuid = playerUuid
	party.Name = name
	party.Public = public
	party.Pass = pass
	party.SystemName = theme
	party.Description = description

	return nil
}

func joinPlayerParty(partyId int, playerUuid string) error {
	_, err := db.Exec("INSERT INTO partyMembers (partyId, uuid) VALUES (?, ?)", partyId, playerUuid)
	if err != nil {
		return err
	}

	_, err = db.Exec("UPDATE playerGameData pgd SET pgd.lastPartyMsgId = (SELECT MAX(cm.timestamp) FROM chatMessages cm WHERE cm.game = pgd.game AND cm.partyId = ?) WHERE pgd.uuid = ? AND pgd.game = ?", partyId, playerUuid, serverConfig.GameName)
	if err != nil {
		return err
	}

	party, ok := parties[partyId]
	if !ok {
		// this only happens when someone creates a party
		party, err := getPartyDataFromDatabase(playerUuid)
		if err != nil {
			return err
		}

		parties[partyId] = &party

		return nil
	}

	client, ok := clients.Load(playerUuid)
	if !ok {
		return errors.New("client not online")
	}

	session := client.(*SessionClient)

	party.Members = append(party.Members, &PartyMember{
		Uuid:        session.uuid,
		Name:        session.name,
		Rank:        session.rank,
		Account:     session.account,
		Badge:       session.badge,
		SystemName:  session.systemName,
		SpriteName:  session.spriteName,
		SpriteIndex: session.spriteIndex,
		Medals:      session.medals,
		MapId:       "0000", // initial value
		PrevMapId:   "0000", // initial value
	})

	return nil
}

func leavePlayerParty(playerUuid string) error {
	partyId, err := getPlayerPartyId(playerUuid) // get party id for later
	if err != nil {
		return err
	}

	_, err = db.Exec("DELETE pm FROM partyMembers pm JOIN parties p ON p.id = pm.partyId WHERE pm.uuid = ? AND p.game = ?", playerUuid, serverConfig.GameName)
	if err != nil {
		return err
	}

	_, err = db.Exec("UPDATE playerGameData SET lastPartyMsgId = NULL WHERE uuid = ? AND game = ?", playerUuid, serverConfig.GameName)
	if err != nil {
		return err
	}

	party, ok := parties[partyId]
	if !ok {
		return errors.New("party id not in cache")
	}

	// remove member from party cache
	if len(party.Members) <= 1 {
		party.Members = nil // probably not safe
	} else {
		for i, member := range party.Members {
			if member.Uuid == playerUuid {
				party.Members = append(party.Members[:i], party.Members[i+1:]...)
				break
			}
		}
	}

	return nil
}

func getPartyMemberUuids(partyId int) (partyMemberUuids []string, err error) {
	party, ok := parties[partyId]
	if !ok {
		return nil, errors.New("party id not in cache")
	}

	for _, member := range party.Members {
		partyMemberUuids = append(partyMemberUuids, member.Uuid)
	}

	return partyMemberUuids, nil
}

func getPartyOwnerUuid(partyId int) (ownerUuid string, err error) {
	party, ok := parties[partyId]
	if !ok {
		return "", errors.New("party id not in cache")
	}

	return party.OwnerUuid, nil
}

func assumeNextPartyOwner(partyId int) error {
	partyMemberUuids, err := getPartyMemberUuids(partyId)
	if err != nil {
		return err
	}

	var nextOnlinePlayerUuid string

	for _, uuid := range partyMemberUuids {
		if client, ok := clients.Load(uuid); ok {
			if client.(*SessionClient).rClient != nil {
				nextOnlinePlayerUuid = uuid
				break
			}
		}
	}

	if nextOnlinePlayerUuid != "" {
		err := setPartyOwner(partyId, nextOnlinePlayerUuid)
		if err != nil {
			return err
		}
	} else {
		_, err := db.Exec("UPDATE parties p SET p.owner = (SELECT pm.uuid FROM partyMembers pm JOIN players pd ON pd.uuid = pm.uuid WHERE pm.partyId = p.id ORDER BY pd.rank DESC, pm.id LIMIT 1) WHERE p.id = ?", partyId)
		if err != nil {
			return err
		}
	}

	return nil
}

func setPartyOwner(partyId int, playerUuid string) error {
	_, err := db.Exec("UPDATE parties SET owner = ? WHERE id = ?", playerUuid, partyId)
	if err != nil {
		return err
	}

	party, ok := parties[partyId]
	if !ok {
		return errors.New("party id not in cache")
	}

	party.OwnerUuid = playerUuid

	return nil
}

func checkDeleteOrphanedParty(partyId int) (deleted bool, err error) {
	party, ok := parties[partyId]
	if !ok {
		return false, errors.New("party id not in cache")
	}

	if len(party.Members) == 0 {
		_, err := db.Exec("DELETE FROM parties WHERE id = ?", partyId)
		if err != nil {
			return true, err
		}

		delete(parties, partyId)

		return true, nil
	}

	return false, nil
}

func deletePartyAndMembers(partyId int) error {
	_, err := db.Exec("DELETE FROM partyMembers WHERE partyId = ?", partyId)
	if err != nil {
		return err
	}

	_, err = db.Exec("DELETE FROM parties WHERE id = ?", partyId)
	if err != nil {
		return err
	}

	delete(parties, partyId)

	return nil
}

func writePartyChatMessage(msgId, uuid, mapId, prevMapId, prevLocations string, x, y int, contents string, partyId int) error {
	_, err := db.Exec("INSERT INTO chatMessages (msgId, game, uuid, mapId, prevMapId, prevLocations, x, y, contents, partyId) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)", msgId, serverConfig.GameName, uuid, mapId, prevMapId, prevLocations, x, y, contents, partyId)
	if err != nil {
		return err
	}

	msgSent = true

	return nil
}
