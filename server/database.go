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

package server

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

var (
	db = getDatabaseConn()
)

func getDatabaseConn() *sql.DB {
	conn, err := sql.Open("mysql", "yno@unix(/run/mysqld/mysqld.sock)/ynodb?parseTime=true")
	if err != nil {
		panic(err)
	}

	return conn
}

func getOrCreatePlayerData(ip string) (uuid string, banned bool, muted bool) {
	err := db.QueryRow("SELECT uuid, banned, muted FROM players WHERE ip = ?", ip).Scan(&uuid, &banned, &muted)
	if err != nil {
		if err != sql.ErrNoRows {
			return "", false, false
		}

		// create new guest account
		uuid = randString(16)
		banned = isVpn(ip)
		createPlayerData(ip, uuid, 0, banned)
	}

	return uuid, banned, muted
}

func getPlayerDataFromToken(token string) (uuid string, name string, rank int, badge string, banned bool, muted bool) {
	err := db.QueryRow("SELECT a.uuid, a.user, pd.rank, a.badge, pd.banned, pd.muted FROM accounts a JOIN playerSessions ps ON ps.uuid = a.uuid JOIN players pd ON pd.uuid = a.uuid WHERE ps.sessionId = ? AND NOW() < ps.expiration", token).Scan(&uuid, &name, &rank, &badge, &banned, &muted)
	if err != nil {
		return "", "", 0, "", false, false
	}

	return uuid, name, rank, badge, banned, muted
}

func getPlayerRank(uuid string) (rank int) {
	if client, ok := clients.Load(uuid); ok {
		return client.(*SessionClient).rank // return rank from session if client is connected
	}

	err := db.QueryRow("SELECT rank FROM players WHERE uuid = ?", uuid).Scan(&rank)
	if err != nil {
		return 0
	}

	return rank
}

func tryBanPlayer(senderUuid string, recipientUuid string) error { // called by api only
	if getPlayerRank(senderUuid) <= getPlayerRank(recipientUuid) {
		return errors.New("insufficient rank")
	}

	if senderUuid == recipientUuid {
		return errors.New("attempted self-ban")
	}

	_, err := db.Exec("UPDATE players SET banned = 1 WHERE uuid = ?", recipientUuid)
	if err != nil {
		return err
	}

	if client, ok := clients.Load(recipientUuid); ok {
		client := client.(*SessionClient)
		if client.rClient != nil {
			client.rClient.disconnect()
		}

		client.disconnect()
	}

	return nil
}

func tryUnbanPlayer(senderUuid string, recipientUuid string) error { // called by api only
	if getPlayerRank(senderUuid) <= getPlayerRank(recipientUuid) {
		return errors.New("insufficient rank")
	}

	if senderUuid == recipientUuid {
		return errors.New("attempted self-unban")
	}

	_, err := db.Exec("UPDATE players SET banned = 0 WHERE uuid = ?", recipientUuid)
	if err != nil {
		return err
	}

	return nil
}

func tryMutePlayer(senderUuid string, recipientUuid string) error { // called by api only
	if getPlayerRank(senderUuid) <= getPlayerRank(recipientUuid) {
		return errors.New("insufficient rank")
	}

	if senderUuid == recipientUuid {
		return errors.New("attempted self-mute")
	}

	_, err := db.Exec("UPDATE players SET muted = 1 WHERE uuid = ?", recipientUuid)
	if err != nil {
		return err
	}

	if client, ok := clients.Load(recipientUuid); ok { // mute client if they're connected
		client.(*SessionClient).muted = true
	}

	return nil
}

func tryUnmutePlayer(senderUuid string, recipientUuid string) error { // called by api only
	if getPlayerRank(senderUuid) <= getPlayerRank(recipientUuid) {
		return errors.New("insufficient rank")
	}

	if senderUuid == recipientUuid {
		return errors.New("attempted self-unmute")
	}

	_, err := db.Exec("UPDATE players SET muted = 0 WHERE uuid = ?", recipientUuid)
	if err != nil {
		return err
	}

	if client, ok := clients.Load(recipientUuid); ok { // unmute client if they're connected
		client.(*SessionClient).muted = false
	}

	return nil
}

func getPlayerMedals(uuid string) (medals [5]int) {
	if client, ok := clients.Load(uuid); ok {
		return client.(*SessionClient).medals // return medals from session if client is connected
	}

	err := db.QueryRow("SELECT pgd.medalCountBronze, pgd.medalCountSilver, pgd.medalCountGold, pgd.medalCountPlatinum, pgd.medalCountDiamond FROM players pd LEFT JOIN playerGameData pgd ON pgd.uuid = pd.uuid WHERE pd.uuid = ? AND pgd.game = ?", uuid, serverConfig.GameName).Scan(&medals[0], &medals[1], &medals[2], &medals[3], &medals[4])
	if err != nil {
		return [5]int{}
	}

	return medals
}

func createPlayerData(ip string, uuid string, rank int, banned bool) error {
	_, err := db.Exec("INSERT INTO players (ip, uuid, rank, banned) VALUES (?, ?, ?, ?)", ip, uuid, rank, banned)
	if err != nil {
		return err
	}

	return nil
}

func getPlayerGameData(uuid string) (spriteName string, spriteIndex int, systemName string) {
	err := db.QueryRow("SELECT pgd.spriteName, pgd.spriteIndex, pgd.systemName FROM players pd LEFT JOIN playerGameData pgd ON pgd.uuid = pd.uuid WHERE pd.uuid = ? AND pgd.game = ?", uuid, serverConfig.GameName).Scan(&spriteName, &spriteIndex, &systemName)
	if err != nil {
		return "", 0, ""
	}

	return spriteName, spriteIndex, systemName
}

func (c *SessionClient) updatePlayerGameData() error {
	_, err := db.Exec("INSERT INTO playerGameData (uuid, game, name, systemName, spriteName, spriteIndex) VALUES (?, ?, ?, ?, ?, ?) ON DUPLICATE KEY UPDATE name = ?, systemName = ?, spriteName = ?, spriteIndex = ?", c.uuid, serverConfig.GameName, c.name, c.systemName, c.spriteName, c.spriteIndex, c.name, c.systemName, c.spriteName, c.spriteIndex)
	if err != nil {
		return err
	}

	return nil
}

func getPlayerInfo(ip string) (uuid string, name string, rank int) {
	err := db.QueryRow("SELECT pd.uuid, pgd.name, pd.rank FROM players pd LEFT JOIN playerGameData pgd ON pgd.uuid = pd.uuid WHERE pd.ip = ? AND (pgd.uuid IS NULL OR pgd.game = ?)", ip, serverConfig.GameName).Scan(&uuid, &name, &rank)
	if err != nil {
		return "", "", 0
	}

	return uuid, name, rank
}

func getPlayerInfoFromToken(token string) (uuid string, name string, rank int, badge string, badgeSlotRows int, badgeSlotCols int) {
	err := db.QueryRow("SELECT a.uuid, a.user, pd.rank, a.badge, a.badgeSlotRows, a.badgeSlotCols FROM accounts a JOIN playerSessions ps ON ps.uuid = a.uuid JOIN players pd ON pd.uuid = a.uuid WHERE ps.sessionId = ? AND NOW() < ps.expiration", token).Scan(&uuid, &name, &rank, &badge, &badgeSlotRows, &badgeSlotCols)
	if err != nil {
		return "", "", 0, "", 0, 0
	}

	return uuid, name, rank, badge, badgeSlotRows, badgeSlotCols
}

func getPlayerBadgeSlotCounts(playerName string) (badgeSlotRows int, badgeSlotCols int) {
	err := db.QueryRow("SELECT badgeSlotRows, badgeSlotCols FROM accounts WHERE user = ?", playerName).Scan(&badgeSlotRows, &badgeSlotCols)
	if err != nil {
		return 1, 3
	}

	return badgeSlotRows, badgeSlotCols
}

func updatePlayerBadgeSlotCounts(uuid string) (err error) {
	query := "UPDATE accounts JOIN (SELECT pb.uuid, SUM(b.bp) bp, COUNT(b.badgeId) bc FROM playerBadges pb JOIN badges b ON b.badgeId = pb.badgeId AND b.hidden = 0 GROUP BY pb.uuid) AS pb ON pb.uuid = accounts.uuid SET badgeSlotRows = CASE WHEN bp < 300 THEN 1 WHEN bp < 1000 THEN 2 WHEN bp < 2000 THEN 3 WHEN bp < 4000 THEN 4 WHEN bp < 7500 THEN 5 WHEN bp < 12500 THEN 6 WHEN bp < 20000 THEN 7 WHEN bp < 30000 THEN 8 WHEN bp < 50000 THEN 9 ELSE 10 END, badgeSlotCols = CASE WHEN bc < 50 THEN 3 WHEN bc < 150 THEN 4 WHEN bc < 300 THEN 5 WHEN bc < 500 THEN 6 ELSE 7 END"
	if uuid == "" {
		_, err = db.Exec(query)
	} else {
		query += " WHERE accounts.uuid = ?"
		_, err = db.Exec(query, uuid)
	}
	if err != nil {
		return err
	}

	return nil
}

func setPlayerBadge(uuid string, badge string) (err error) {
	if client, ok := clients.Load(uuid); ok {
		client.(*SessionClient).badge = badge
	}

	_, err = db.Exec("UPDATE accounts SET badge = ? WHERE uuid = ?", badge, uuid)
	if err != nil {
		return err
	}

	return nil
}

func getPlayerBadgeSlots(playerName string, badgeSlotRows int, badgeSlotCols int) (badgeSlots [][]string, err error) {
	results, err := db.Query("SELECT pb.badgeId, pb.slotRow, pb.slotCol FROM playerBadges pb JOIN accounts a ON a.uuid = pb.uuid WHERE a.user = ? AND pb.slotRow BETWEEN 1 AND ? AND pb.slotCol BETWEEN 1 AND ? ORDER BY pb.slotRow, pb.slotCol", playerName, badgeSlotRows, badgeSlotCols)
	if err != nil {
		return badgeSlots, err
	}

	defer results.Close()

	var badgeId string
	var badgeRow int
	var badgeCol int

	for r := 1; r <= badgeSlotRows; r++ {
		var badgeSlotRow []string
		for c := 1; c <= badgeSlotCols; c++ {
			if badgeRow > r || (badgeRow == r && badgeCol >= c) {
				if badgeRow == r && badgeCol == c {
					badgeSlotRow = append(badgeSlotRow, badgeId)
				} else {
					badgeSlotRow = append(badgeSlotRow, "null")
				}
			} else {
				for {
					if !results.Next() {
						break
					}
					err := results.Scan(&badgeId, &badgeRow, &badgeCol)
					if err != nil {
						break
					}

					if badgeRow > r || (badgeRow == r && badgeCol >= c) {
						if badgeRow == r && badgeCol == c {
							badgeSlotRow = append(badgeSlotRow, badgeId)
						}
						break
					}
				}
				if len(badgeSlotRow) < c {
					badgeSlotRow = append(badgeSlotRow, "null")
				}
			}
		}
		badgeSlots = append(badgeSlots, badgeSlotRow)
	}

	return badgeSlots, nil
}

func setPlayerBadgeSlot(uuid string, badgeId string, slotRow int, slotCol int) (err error) {
	var slotCurrentBadgeId string
	err = db.QueryRow("SELECT badgeId FROM playerBadges WHERE uuid = ? AND slotRow = ? AND slotCol = ? LIMIT 1", uuid, slotRow, slotCol).Scan(&slotCurrentBadgeId)
	if err != nil {
		if err != sql.ErrNoRows {
			return err
		}
	} else if slotCurrentBadgeId == badgeId {
		return
	} else {
		if badgeId != "null" {
			var badgeCurrentSlotRow int
			var badgeCurrentSlotCol int
			err := db.QueryRow("SELECT slotRow, slotCol FROM playerBadges WHERE uuid = ? AND badgeId = ? LIMIT 1", uuid, badgeId).Scan(&badgeCurrentSlotRow, &badgeCurrentSlotCol)

			if err != nil && err != sql.ErrNoRows {
				return err
			} else {
				_, err = db.Exec("UPDATE playerBadges SET slotRow = ?, slotCol = ? WHERE uuid = ? AND badgeId = ?", badgeCurrentSlotRow, badgeCurrentSlotCol, uuid, slotCurrentBadgeId)
				if err != nil && err != sql.ErrNoRows {
					return err
				}
			}
		} else {
			_, err = db.Exec("UPDATE playerBadges SET slotRow = 0, slotCol = 0 WHERE uuid = ? AND slotRow = ? AND slotCol = ?", uuid, slotRow, slotCol)
			if err != nil && err != sql.ErrNoRows {
				return err
			}
		}
	}

	_, err = db.Exec("UPDATE playerBadges SET slotRow = ?, slotCol = ? WHERE uuid = ? AND badgeId = ?", slotRow, slotCol, uuid, badgeId)
	if err != nil {
		return err
	}

	return nil
}

func getPlayerPartyId(uuid string) (partyId int, err error) {
	err = db.QueryRow("SELECT pm.partyId FROM partyMembers pm JOIN parties p ON p.id = pm.partyId WHERE pm.uuid = ? AND p.game = ?", uuid, serverConfig.GameName).Scan(&partyId)
	if err != nil {
		if err == sql.ErrNoRows {
			return 0, nil
		} else {
			return 0, err
		}
	}

	return partyId, nil
}

func getAllPartyData(simple bool) (parties []*Party, err error) {
	partyMembersByParty, err := getAllPartyMemberDataByParty(simple)
	if err != nil {
		return parties, err
	}

	results, err := db.Query("SELECT p.id, p.owner, p.name, p.public, p.pass, p.theme, p.description FROM parties p WHERE p.game = ?", serverConfig.GameName)
	if err != nil {
		return parties, err
	}

	defer results.Close()

	for results.Next() {
		party := &Party{}
		err := results.Scan(&party.Id, &party.OwnerUuid, &party.Name, &party.Public, &party.Pass, &party.SystemName, &party.Description)
		if err != nil {
			return parties, err
		}

		var hasOnlineMember bool

		for _, partyMember := range partyMembersByParty[party.Id] {
			party.Members = append(party.Members, *partyMember)
			if partyMember.Online {
				hasOnlineMember = true
			}
		}

		if hasOnlineMember {
			if simple {
				party.Pass = ""
			}
			parties = append(parties, party)
		}
	}

	return parties, nil
}

func getAllPartyMemberDataByParty(simple bool) (partyMembersByParty map[int][]*PartyMember, err error) {
	partyMembersByParty = make(map[int][]*PartyMember)

	results, err := db.Query("SELECT pm.partyId, pm.uuid, COALESCE(a.user, pgd.name), pd.rank, CASE WHEN a.user IS NULL THEN 0 ELSE 1 END, COALESCE(a.badge, ''), pgd.systemName, pgd.spriteName, pgd.spriteIndex, pgd.medalCountBronze, pgd.medalCountSilver, pgd.medalCountGold, pgd.medalCountPlatinum, pgd.medalCountDiamond FROM partyMembers pm JOIN playerGameData pgd ON pgd.uuid = pm.uuid JOIN players pd ON pd.uuid = pgd.uuid JOIN parties p ON p.id = pm.partyId LEFT JOIN accounts a ON a.uuid = pd.uuid WHERE pgd.game = ? ORDER BY CASE WHEN p.owner = pm.uuid THEN 0 ELSE 1 END, pd.rank DESC, pm.id", serverConfig.GameName)
	if err != nil {
		return partyMembersByParty, err
	}

	defer results.Close()

	offlinePartyMembersByParty := make(map[int][]*PartyMember)

	for results.Next() {
		var partyId int
		var accountBin int
		partyMember := &PartyMember{}
		err := results.Scan(&partyId, &partyMember.Uuid, &partyMember.Name, &partyMember.Rank, &accountBin, &partyMember.Badge, &partyMember.SystemName, &partyMember.SpriteName, &partyMember.SpriteIndex, &partyMember.Medals[0], &partyMember.Medals[1], &partyMember.Medals[2], &partyMember.Medals[3], &partyMember.Medals[4])
		if err != nil {
			return partyMembersByParty, err
		}
		partyMember.Account = accountBin == 1

		if client, ok := clients.Load(partyMember.Uuid); ok {
			client := client.(*SessionClient)
			if client.name != "" {
				partyMember.Name = client.name
			}
			if client.systemName != "" {
				partyMember.SystemName = client.systemName
			}
			if client.spriteName != "" {
				partyMember.SpriteName = client.spriteName
			}
			if client.spriteIndex > -1 {
				partyMember.SpriteIndex = client.spriteIndex
			}
			if !simple && client.rClient != nil {
				partyMember.MapId = client.rClient.mapId
				partyMember.PrevMapId = client.rClient.prevMapId
				partyMember.PrevLocations = client.rClient.prevLocations
				partyMember.X = client.rClient.x
				partyMember.Y = client.rClient.y
			}
			partyMember.Online = true

			partyMembersByParty[partyId] = append(partyMembersByParty[partyId], partyMember)
		} else {
			if !simple {
				partyMember.MapId = "0000"
				partyMember.PrevMapId = "0000"
			}
			offlinePartyMembersByParty[partyId] = append(offlinePartyMembersByParty[partyId], partyMember)
		}
	}

	for partyId, offlinePartyMembers := range offlinePartyMembersByParty {
		partyMembersByParty[partyId] = append(partyMembersByParty[partyId], offlinePartyMembers...)
	}

	return partyMembersByParty, nil
}

func getPartyData(playerUuid string) (party Party, err error) { // called by api only
	err = db.QueryRow("SELECT p.id, p.owner, p.name, p.public, p.pass, p.theme, p.description FROM parties p JOIN partyMembers pm ON pm.partyId = p.id JOIN playerGameData pgd ON pgd.uuid = pm.uuid AND pgd.game = p.game WHERE p.game = ? AND pm.uuid = ?", serverConfig.GameName, playerUuid).Scan(&party.Id, &party.OwnerUuid, &party.Name, &party.Public, &party.Pass, &party.SystemName, &party.Description)
	if err != nil {
		return party, err
	}

	partyMembers, err := getPartyMemberData(party.Id)
	if err != nil {
		return party, err
	}

	for _, partyMember := range partyMembers {
		party.Members = append(party.Members, *partyMember)
	}

	return party, nil
}

func getPartyMemberData(partyId int) (partyMembers []*PartyMember, err error) {
	results, err := db.Query("SELECT pm.partyId, pm.uuid, COALESCE(a.user, pgd.name), pd.rank, CASE WHEN a.user IS NULL THEN 0 ELSE 1 END, COALESCE(a.badge, ''), pgd.systemName, pgd.spriteName, pgd.spriteIndex, pgd.medalCountBronze, pgd.medalCountSilver, pgd.medalCountGold, pgd.medalCountPlatinum, pgd.medalCountDiamond FROM partyMembers pm JOIN playerGameData pgd ON pgd.uuid = pm.uuid JOIN players pd ON pd.uuid = pgd.uuid JOIN parties p ON p.id = pm.partyId LEFT JOIN accounts a ON a.uuid = pd.uuid WHERE pm.partyId = ? AND pgd.game = ? ORDER BY CASE WHEN p.owner = pm.uuid THEN 0 ELSE 1 END, pd.rank DESC, pm.id", partyId, serverConfig.GameName)
	if err != nil {
		return partyMembers, err
	}

	defer results.Close()

	for results.Next() {
		var partyId int
		var accountBin int
		partyMember := &PartyMember{}
		err := results.Scan(&partyId, &partyMember.Uuid, &partyMember.Name, &partyMember.Rank, &accountBin, &partyMember.Badge, &partyMember.SystemName, &partyMember.SpriteName, &partyMember.SpriteIndex, &partyMember.Medals[0], &partyMember.Medals[1], &partyMember.Medals[2], &partyMember.Medals[3], &partyMember.Medals[4])
		if err != nil {
			return partyMembers, err
		}
		partyMember.Account = accountBin == 1

		if client, ok := clients.Load(partyMember.Uuid); ok {
			client := client.(*SessionClient)
			if client.name != "" {
				partyMember.Name = client.name
			}
			if client.systemName != "" {
				partyMember.SystemName = client.systemName
			}
			if client.spriteName != "" {
				partyMember.SpriteName = client.spriteName
			}
			if client.spriteIndex > -1 {
				partyMember.SpriteIndex = client.spriteIndex
			}
			if client.rClient != nil {
				partyMember.MapId = client.rClient.mapId
				partyMember.PrevMapId = client.rClient.prevMapId
				partyMember.PrevLocations = client.rClient.prevLocations
				partyMember.X = client.rClient.x
				partyMember.Y = client.rClient.y
			}
			partyMember.Online = true
		}
		if partyMember.MapId == "" {
			partyMember.MapId = "0000"
		}
		if partyMember.PrevMapId == "" {
			partyMember.PrevMapId = "0000"
		}
		partyMembers = append(partyMembers, partyMember)
	}

	return partyMembers, nil
}

func getPartyDescription(partyId int) (description string, err error) { // called by api only
	err = db.QueryRow("SELECT description FROM parties WHERE id = ?", partyId).Scan(&description)
	if err != nil {
		return description, err
	}

	return description, nil
}

func getPartyPublic(partyId int) (public bool, err error) { // called by api only
	err = db.QueryRow("SELECT public FROM parties WHERE id = ?", partyId).Scan(&public)
	if err != nil {
		return public, err
	}

	return public, nil
}

func getPartyPass(partyId int) (pass string, err error) { // called by api only
	err = db.QueryRow("SELECT pass FROM parties WHERE id = ?", partyId).Scan(&pass)
	if err != nil {
		return pass, err
	}

	return pass, nil
}

func createPartyData(name string, public bool, pass string, theme string, description string, playerUuid string) (partyId int, err error) {
	res, err := db.Exec("INSERT INTO parties (game, owner, name, public, pass, theme, description) VALUES (?, ?, ?, ?, ?, ?, ?)", serverConfig.GameName, playerUuid, name, public, pass, theme, description)
	if err != nil {
		return 0, err
	}

	var partyId64 int64

	partyId64, err = res.LastInsertId()
	if err != nil {
		return 0, err
	}

	partyId = int(partyId64)

	return partyId, nil
}

func updatePartyData(partyId int, name string, public bool, pass string, theme string, description string, playerUuid string) (err error) {
	_, err = db.Exec("UPDATE parties SET game = ?, owner = ?, name = ?, public = ?, pass = ?, theme = ?, description = ? WHERE id = ?", serverConfig.GameName, playerUuid, name, public, pass, theme, description, partyId)
	if err != nil {
		return err
	}

	return nil
}

func createPlayerParty(partyId int, playerUuid string) error {
	_, err := db.Exec("INSERT INTO partyMembers (partyId, uuid) VALUES (?, ?)", partyId, playerUuid)
	if err != nil {
		return err
	}

	return nil
}

func clearPlayerParty(playerUuid string) error {
	_, err := db.Exec("DELETE pm FROM partyMembers pm JOIN parties p ON p.id = pm.partyId WHERE pm.uuid = ? AND p.game = ?", playerUuid, serverConfig.GameName)
	if err != nil {
		return err
	}

	return nil
}

func getPartyMemberUuids(partyId int) (partyMemberUuids []string, err error) {
	results, err := db.Query("SELECT pm.uuid FROM partyMembers pm JOIN players pd ON pd.uuid = pm.uuid WHERE pm.partyId = ? ORDER BY pd.rank DESC, pm.id", partyId)
	if err != nil {
		return partyMemberUuids, err
	}

	defer results.Close()

	for results.Next() {
		var uuid string
		err := results.Scan(&uuid)
		if err != nil {
			return partyMemberUuids, err
		}
		partyMemberUuids = append(partyMemberUuids, uuid)
	}

	return partyMemberUuids, nil
}

func getPartyMemberCount(partyId int) (count int, err error) {
	err = db.QueryRow("SELECT COUNT(*) FROM partyMembers WHERE partyId = ?", partyId).Scan(&count)
	if err != nil {
		return count, err
	}

	return count, nil
}

func getPartyOwnerUuid(partyId int) (ownerUuid string, err error) {
	err = db.QueryRow("SELECT owner FROM parties WHERE id = ?", partyId).Scan(&ownerUuid)
	if err != nil {
		return ownerUuid, err
	}

	return ownerUuid, nil
}

func assumeNextPartyOwner(partyId int) error {
	partyMemberUuids, err := getPartyMemberUuids(partyId)
	if err != nil {
		return err
	}

	var nextOnlinePlayerUuid string

	for _, uuid := range partyMemberUuids {
		if client, ok := clients.Load(uuid); ok {
			if client.(*SessionClient).rClient == nil {
				continue
			}

			nextOnlinePlayerUuid = uuid
			break
		}
	}

	if nextOnlinePlayerUuid != "" {
		err = setPartyOwner(partyId, nextOnlinePlayerUuid)
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

	return nil
}

func checkDeleteOrphanedParty(partyId int) (deleted bool, err error) {
	partyMemberCount, err := getPartyMemberCount(partyId)
	if err != nil {
		return false, err
	}

	if partyMemberCount == 0 {
		_, err := db.Exec("DELETE FROM parties WHERE id = ?", partyId)
		if err != nil {
			return true, err
		}
		return true, nil
	}

	return false, nil
}

func deletePartyAndMembers(partyId int) (err error) {
	_, err = db.Exec("DELETE FROM partyMembers WHERE partyId = ?", partyId)
	if err != nil {
		return err
	}

	_, err = db.Exec("DELETE FROM parties WHERE id = ?", partyId)
	if err != nil {
		return err
	}

	return nil
}

func getSaveDataTimestamp(playerUuid string) (timestamp time.Time, err error) { // called by api only
	err = db.QueryRow("SELECT timestamp FROM playerGameSaves WHERE uuid = ? AND game = ?", playerUuid, serverConfig.GameName).Scan(&timestamp)
	if err != nil {
		return timestamp, err
	}

	return timestamp, nil
}

func getSaveData(playerUuid string) (saveData string, err error) { // called by api only
	err = db.QueryRow("SELECT data FROM playerGameSaves WHERE uuid = ? AND game = ?", playerUuid, serverConfig.GameName).Scan(&saveData)
	if err != nil {
		return saveData, err
	}

	return saveData, nil
}

func createGameSaveData(playerUuid string, timestamp time.Time, data string) (err error) { // called by api only
	_, err = db.Exec("INSERT INTO playerGameSaves (uuid, game, timestamp, data) VALUES (?, ?, ?, ?) ON DUPLICATE KEY UPDATE timestamp = ?, data = ?", playerUuid, serverConfig.GameName, timestamp, data, timestamp, data)
	if err != nil {
		return err
	}

	return nil
}

func clearGameSaveData(playerUuid string) (err error) { // called by api only
	_, err = db.Exec("DELETE FROM playerGameSaves WHERE uuid = ? AND game = ?", playerUuid, serverConfig.GameName)
	if err != nil {
		return err
	}

	return nil
}

func setCurrentEventPeriodId() (err error) {
	var periodId int

	err = db.QueryRow("SELECT id FROM eventPeriods WHERE game = ? AND UTC_DATE() >= startDate AND UTC_DATE() < endDate", serverConfig.GameName).Scan(&periodId)
	if err != nil {
		currentEventPeriodId = 0
		if err == sql.ErrNoRows {
			return nil
		}
		return err
	}

	currentEventPeriodId = periodId

	return nil
}

func getCurrentEventPeriodData() (eventPeriod EventPeriod, err error) {
	err = db.QueryRow("SELECT periodOrdinal, endDate, enableVms FROM eventPeriods WHERE UTC_DATE() >= startDate AND UTC_DATE() < endDate").Scan(&eventPeriod.PeriodOrdinal, &eventPeriod.EndDate, &eventPeriod.EnableVms)
	if err != nil {
		eventPeriod.PeriodOrdinal = -1
		if err == sql.ErrNoRows {
			return eventPeriod, nil
		}
		return eventPeriod, err
	}

	return eventPeriod, nil
}

func setCurrentGameEventPeriodId() (err error) {
	var gamePeriodId int

	err = db.QueryRow("SELECT id FROM gameEventPeriods WHERE game = ? AND periodId = ?", serverConfig.GameName, currentEventPeriodId).Scan(&gamePeriodId)
	if err != nil {
		currentGameEventPeriodId = 0
		if err == sql.ErrNoRows {
			return nil
		}
		return err
	}

	currentGameEventPeriodId = gamePeriodId

	return nil
}

func getPlayerEventExpData(playerUuid string) (eventExp EventExp, err error) {
	totalEventExp, err := getPlayerTotalEventExp(playerUuid)
	if err != nil {
		return eventExp, err
	}

	eventExp.TotalExp = totalEventExp

	periodEventExp, err := getPlayerPeriodEventExp(playerUuid)
	if err != nil {
		return eventExp, err
	}

	eventExp.PeriodExp = periodEventExp

	weekEventExp, err := getPlayerWeekEventExp(playerUuid)
	if err != nil {
		return eventExp, err
	}

	eventExp.WeekExp = weekEventExp

	return eventExp, nil
}

func getPlayerTotalEventExp(playerUuid string) (totalEventExp int, err error) {
	err = db.QueryRow("SELECT SUM(exp) FROM ((SELECT COALESCE(SUM(ec.exp), 0) exp FROM eventCompletions ec JOIN eventLocations el ON el.id = ec.eventId AND ec.type = 0 JOIN gameEventPeriods gep ON gep.id = el.gamePeriodId JOIN eventPeriods ep ON ep.id = gep.periodId WHERE ep.game = ? AND ec.uuid = ?) UNION ALL (SELECT COALESCE(SUM(ec.exp), 0) exp FROM eventCompletions ec JOIN eventVms ev ON ev.id = ec.eventId AND ec.type = 2 JOIN gameEventPeriods gep ON gep.id = ev.gamePeriodId JOIN eventPeriods ep ON ep.id = gep.periodId WHERE ep.game = ? AND ec.uuid = ?)) eventExp", serverConfig.GameName, playerUuid, serverConfig.GameName, playerUuid).Scan(&totalEventExp)
	if err != nil {
		return totalEventExp, err
	}

	return totalEventExp, nil
}

func getPlayerPeriodEventExp(playerUuid string) (periodEventExp int, err error) {
	err = db.QueryRow("SELECT SUM(exp) FROM ((SELECT COALESCE(SUM(ec.exp), 0) exp FROM eventCompletions ec JOIN eventLocations el ON el.id = ec.eventId AND ec.type = 0 JOIN gameEventPeriods gep ON gep.id = el.gamePeriodId JOIN eventPeriods ep ON ep.id = gep.periodId WHERE ep.id = ? AND ec.uuid = ?) UNION ALL (SELECT COALESCE(SUM(ec.exp), 0) exp FROM eventCompletions ec JOIN eventVms ev ON ev.id = ec.eventId AND ec.type = 2 JOIN gameEventPeriods gep ON gep.id = ev.gamePeriodId JOIN eventPeriods ep ON ep.id = gep.periodId WHERE ep.id = ? AND ec.uuid = ?)) eventExp", currentEventPeriodId, playerUuid, currentEventPeriodId, playerUuid).Scan(&periodEventExp)
	if err != nil {
		return periodEventExp, err
	}

	return periodEventExp, nil
}

func getPlayerWeekEventExp(playerUuid string) (weekEventExp int, err error) {
	weekdayIndex := int(time.Now().UTC().Weekday())

	err = db.QueryRow("SELECT SUM(exp) FROM ((SELECT COALESCE(SUM(ec.exp), 0) exp FROM eventCompletions ec JOIN eventLocations el ON el.id = ec.eventId AND ec.type = 0 JOIN gameEventPeriods gep ON gep.id = el.gamePeriodId JOIN eventPeriods ep ON ep.id = gep.periodId WHERE ep.id = ? AND ec.uuid = ? AND DATE_SUB(UTC_DATE(), INTERVAL ? DAY) <= el.startDate AND DATE_ADD(UTC_DATE(), INTERVAL ? DAY) >= el.endDate) UNION ALL (SELECT COALESCE(SUM(ec.exp), 0) exp FROM eventCompletions ec JOIN eventVms ev ON ev.id = ec.eventId AND ec.type = 2 JOIN gameEventPeriods gep ON gep.id = ev.gamePeriodId JOIN eventPeriods ep ON ep.id = gep.periodId WHERE ep.id = ? AND ec.uuid = ? AND DATE_SUB(UTC_DATE(), INTERVAL ? DAY) <= ev.startDate AND DATE_ADD(UTC_DATE(), INTERVAL ? DAY) >= ev.endDate)) eventExp", currentEventPeriodId, playerUuid, weekdayIndex, 7-weekdayIndex, currentEventPeriodId, playerUuid, weekdayIndex, 7-weekdayIndex).Scan(&weekEventExp)
	if err != nil {
		return weekEventExp, err
	}

	return weekEventExp, nil
}

func getPlayerEventLocationCount(playerUuid string) (eventLocationCount int, err error) {
	err = db.QueryRow("SELECT COUNT(eventId) FROM eventCompletions WHERE uuid = ? AND type < 2", playerUuid).Scan(&eventLocationCount)
	if err != nil {
		return eventLocationCount, err
	}

	return eventLocationCount, nil
}

func getPlayerEventLocationCompletion(playerUuid string) (eventLocationCompletion int, err error) {
	err = db.QueryRow("SELECT COALESCE(FLOOR((COUNT(DISTINCT COALESCE(el.title, pel.title)) / aec.count) * 100), 0) FROM eventCompletions ec LEFT JOIN eventLocations el ON el.id = ec.eventId AND ec.type = 0 LEFT JOIN playerEventLocations pel ON pel.id = ec.eventId AND ec.type = 1 JOIN (SELECT COUNT(DISTINCT COALESCE(ael.title, apel.title)) count FROM eventCompletions aec LEFT JOIN eventLocations ael ON ael.id = aec.eventId AND aec.type = 0 LEFT JOIN playerEventLocations	apel ON apel.id = aec.eventId AND aec.type = 1 WHERE (ael.title IS NOT NULL OR apel.title IS NOT NULL)) aec WHERE ec.uuid = ? AND (el.title IS NOT NULL OR pel.title IS NOT NULL)", playerUuid).Scan(&eventLocationCompletion)
	if err != nil {
		return eventLocationCompletion, err
	}

	return eventLocationCompletion, nil
}

func writeEventLocationData(eventType int, title string, titleJP string, depth int, minDepth int, exp int, mapIds []string) (err error) {
	mapIdsJson, err := json.Marshal(mapIds)
	if err != nil {
		return err
	}

	var days int
	var offsetDays int
	weekday := time.Now().UTC().Weekday()
	switch eventType {
	case 0:
		days = 1
	case 1:
		days = 7
		offsetDays = int(weekday)
	default:
		if weekday == time.Friday || weekday == time.Saturday {
			days = 2
			offsetDays = int(weekday - time.Friday)
		} else {
			return nil
		}
	}

	days -= offsetDays

	_, err = db.Exec("INSERT INTO eventLocations (gamePeriodId, type, title, titleJP, depth, minDepth, exp, startDate, endDate, mapIds) VALUES (?, ?, ?, ?, ?, ?, ?, DATE_SUB(UTC_DATE(), INTERVAL ? DAY), DATE_ADD(UTC_DATE(), INTERVAL ? DAY), ?)", currentGameEventPeriodId, eventType, title, titleJP, depth, minDepth, exp, offsetDays, days, mapIdsJson)
	if err != nil {
		return err
	}

	return nil
}

func writePlayerEventLocationData(playerUuid string, title string, titleJP string, depth int, minDepth int, mapIds []string) (err error) {
	mapIdsJson, err := json.Marshal(mapIds)
	if err != nil {
		return err
	}

	_, err = db.Exec("INSERT INTO playerEventLocations (gamePeriodId, uuid, title, titleJP, depth, minDepth, startDate, endDate, mapIds) SELECT ?, ?, ?, ?, ?, ?, UTC_DATE(), DATE_ADD(UTC_DATE(), INTERVAL 1 DAY), ? WHERE NOT EXISTS(SELECT * FROM playerEventLocations pel LEFT JOIN eventCompletions ec ON ec.eventId = pel.id AND ec.type = 1 AND ec.uuid = pel.uuid WHERE pel.uuid = ? AND pel.gamePeriodId = ? AND ec.uuid IS NULL AND UTC_DATE() >= pel.startDate AND UTC_DATE() < pel.endDate)", currentGameEventPeriodId, playerUuid, title, titleJP, depth, minDepth, mapIdsJson, playerUuid, currentGameEventPeriodId)
	if err != nil {
		return err
	}

	return nil
}

func getCurrentPlayerEventLocationsData(playerUuid string) (eventLocations []*EventLocation, err error) {
	results, err := db.Query("SELECT el.id, el.type, el.title, el.titleJP, el.depth, el.minDepth, el.exp, el.endDate, CASE WHEN ec.uuid IS NOT NULL THEN 1 ELSE 0 END FROM eventLocations el JOIN gamePeriods gep ON gep.id = el.gamePeriodId LEFT JOIN eventCompletions ec ON ec.eventId = el.id AND ec.type = 0 AND ec.uuid = ? WHERE gep.periodId = ? AND UTC_DATE() >= el.startDate AND UTC_DATE() < el.endDate ORDER BY 2, 1", playerUuid, currentEventPeriodId)
	if err != nil {
		return eventLocations, err
	}

	defer results.Close()

	for results.Next() {
		eventLocation := &EventLocation{}

		var completeBin int

		err := results.Scan(&eventLocation.Id, &eventLocation.Type, &eventLocation.Title, &eventLocation.TitleJP, &eventLocation.Depth, &eventLocation.MinDepth, &eventLocation.Exp, &eventLocation.EndDate, &completeBin)
		if err != nil {
			return eventLocations, err
		}

		if eventLocation.MinDepth == eventLocation.Depth {
			eventLocation.MinDepth = 0
		}

		if completeBin == 1 {
			eventLocation.Complete = true
		}

		eventLocations = append(eventLocations, eventLocation)
	}

	results, err = db.Query("SELECT pel.id, pel.title, pel.titleJP, pel.depth, pel.minDepth, pel.endDate FROM playerEventLocations pel JOIN gamePeriodIds gep ON gep.id = pel.gamePeriodId LEFT JOIN eventCompletions ec ON ec.eventId = pel.id AND ec.type = 1 AND ec.uuid = pel.uuid WHERE pel.uuid = ? AND gep.periodId = ? AND ec.uuid IS NULL AND UTC_DATE() >= pel.startDate AND UTC_DATE() < pel.endDate ORDER BY 1", playerUuid, currentEventPeriodId)
	if err != nil {
		return eventLocations, err
	}

	defer results.Close()

	for results.Next() {
		eventLocation := &EventLocation{}

		err := results.Scan(&eventLocation.Id, &eventLocation.Title, &eventLocation.TitleJP, &eventLocation.Depth, &eventLocation.MinDepth, &eventLocation.EndDate)
		if err != nil {
			return eventLocations, err
		}

		eventLocation.Type = -1

		if eventLocation.MinDepth == eventLocation.Depth {
			eventLocation.MinDepth = 0
		}

		eventLocations = append(eventLocations, eventLocation)
	}

	return eventLocations, nil
}

func tryCompleteEventLocation(playerUuid string, location string) (exp int, err error) {
	if client, ok := clients.Load(playerUuid); ok {
		client := client.(*SessionClient)
		if client.rClient == nil {
			return -1, err
		}

		results, err := db.Query("SELECT el.id, el.type, el.exp, el.mapIds FROM eventLocations el WHERE el.gamePeriodId = ? AND el.title = ? AND UTC_DATE() >= el.startDate AND UTC_DATE() < el.endDate ORDER BY 2", currentGameEventPeriodId, location)
		if err != nil {
			return -1, err
		}

		defer results.Close()

		weekEventExp, err := getPlayerWeekEventExp(playerUuid)
		if err != nil {
			return -1, err
		}

		for results.Next() {
			var eventId string
			var eventType int
			var eventExp int
			var mapIdsJson string

			err := results.Scan(&eventId, &eventType, &eventExp, &mapIdsJson)
			if err != nil {
				return exp, err
			}

			var mapIds []string
			err = json.Unmarshal([]byte(mapIdsJson), &mapIds)
			if err != nil {
				return exp, err
			}

			for _, mapId := range mapIds {
				if client.rClient.mapId != mapId {
					continue
				}
				if weekEventExp >= weeklyExpCap {
					eventExp = 0
				} else if weekEventExp+eventExp > weeklyExpCap {
					eventExp = weeklyExpCap - weekEventExp
				}

				_, err = db.Exec("INSERT INTO eventCompletions (eventId, uuid, type, timestampCompleted, exp) VALUES (?, ?, 0, ?, ?)", eventId, playerUuid, time.Now(), eventExp)
				if err != nil {
					break
				}

				exp += eventExp
				weekEventExp += eventExp
				break
			}
		}

		return exp, nil
	}

	return -1, err
}

func tryCompletePlayerEventLocation(playerUuid string, location string) (success bool, err error) {
	if client, ok := clients.Load(playerUuid); ok {
		client := client.(*SessionClient).rClient
		if client == nil {
			return false, err
		}

		results, err := db.Query("SELECT pel.id, pel.mapIds FROM playerEventLocations pel WHERE pel.gamePeriodId = ? AND pel.title = ? AND pel.uuid = ? AND UTC_DATE() >= pel.startDate AND UTC_DATE() < pel.endDate ORDER BY 2", currentGameEventPeriodId, location, playerUuid)
		if err != nil {
			return false, err
		}

		defer results.Close()

		for results.Next() {
			var eventId string
			var mapIdsJson string

			err := results.Scan(&eventId, &mapIdsJson)
			if err != nil {
				return false, err
			}

			var mapIds []string
			err = json.Unmarshal([]byte(mapIdsJson), &mapIds)
			if err != nil {
				return false, err
			}

			for _, mapId := range mapIds {
				if client.mapId != mapId {
					continue
				}

				_, err = db.Exec("INSERT INTO eventCompletions (eventId, uuid, type, timestampCompleted, exp) VALUES (?, ?, 1, ?, 0)", eventId, playerUuid, time.Now())
				if err != nil {
					break
				}

				success = true
				break
			}
		}

		return success, nil
	}

	return false, err
}

func getPlayerEventVmCount(playerUuid string) (eventVmCount int, err error) {
	err = db.QueryRow("SELECT COUNT(eventId) FROM eventCompletions WHERE uuid = ? AND type = 2", playerUuid).Scan(&eventVmCount)
	if err != nil {
		return eventVmCount, err
	}

	return eventVmCount, nil
}

func getCurrentPlayerEventVmsData(playerUuid string) (eventVms []*EventVm, err error) {
	results, err := db.Query("SELECT ev.id, ev.exp, ev.endDate, CASE WHEN ec.uuid IS NOT NULL THEN 1 ELSE 0 END FROM eventVms ev JOIN gamePeriods gep ON gep.id = ev.gamePeriodId LEFT JOIN eventCompletions ec ON ec.eventId = ev.id AND ec.type = 2 AND ec.uuid = ? WHERE gep.periodId = ? AND UTC_DATE() >= ev.startDate AND UTC_DATE() < ev.endDate ORDER BY 2, 1", playerUuid, currentEventPeriodId)
	if err != nil {
		return eventVms, err
	}

	defer results.Close()

	for results.Next() {
		eventVm := &EventVm{}

		var completeBin int

		err := results.Scan(&eventVm.Id, &eventVm.Exp, &eventVm.EndDate, &completeBin)
		if err != nil {
			return eventVms, err
		}

		if completeBin == 1 {
			eventVm.Complete = true
		}

		eventVms = append(eventVms, eventVm)
	}

	return eventVms, nil
}

func getEventVmInfo(id int) (mapId int, eventId int, err error) {
	err = db.QueryRow("SELECT mapId, eventId FROM eventVms WHERE id = ?", id).Scan(&mapId, &eventId)
	if err != nil {
		return 0, 0, err
	}

	return mapId, eventId, nil
}

func writeEventVmData(mapId int, eventId int, exp int) (err error) {

	var days int
	var offsetDays int
	weekday := time.Now().UTC().Weekday()

	switch weekday {
	case time.Sunday, time.Monday:
		days = 2
		offsetDays = int(weekday)
	case time.Tuesday, time.Wednesday, time.Thursday:
		days = 3
		offsetDays = int(weekday - time.Tuesday)
	case time.Friday, time.Saturday:
		days = 2
		offsetDays = int(weekday - time.Friday)
	}

	days -= offsetDays

	_, err = db.Exec("INSERT INTO eventVms (gamePeriodId, mapId, eventId, exp, startDate, endDate) VALUES (?, ?, ?, ?, DATE_SUB(UTC_DATE(), INTERVAL ? DAY), DATE_ADD(UTC_DATE(), INTERVAL ? DAY))", currentGameEventPeriodId, mapId, eventId, exp, offsetDays, days)
	if err != nil {
		return err
	}

	return nil
}

func tryCompleteEventVm(playerUuid string, mapId int, eventId int) (exp int, err error) {
	if client, ok := clients.Load(playerUuid); ok {
		client := client.(*SessionClient).rClient
		if client == nil {
			return -1, err
		}

		results, err := db.Query("SELECT ev.id, ev.mapId, ev.eventId, ev.exp FROM eventVms ev JOIN gameEventPeriods gep ON gep.id = ev.gamePeriodId WHERE gep.periodId = ? AND ev.mapId = ? AND ev.eventId = ? AND UTC_DATE() >= ev.startDate AND UTC_DATE() < ev.endDate ORDER BY 2", currentEventPeriodId, mapId, eventId)
		if err != nil {
			return -1, err
		}

		defer results.Close()

		currentEventVmsData, err := getCurrentPlayerEventVmsData(playerUuid)
		if err != nil {
			return -1, err
		}

		weekEventExp, err := getPlayerWeekEventExp(playerUuid)
		if err != nil {
			return -1, err
		}

		for results.Next() {
			var eventId int
			var eventMapId int
			var eventEvId int
			var eventExp int

			err := results.Scan(&eventId, &eventMapId, &eventEvId, &eventExp)
			if err != nil {
				return exp, err
			}

			for _, eventVm := range currentEventVmsData {
				if eventVm.Id == eventId {
					if eventVm.Complete {
						return -1, nil
					}
					break
				}
			}

			if client.mapId != fmt.Sprintf("%04d", eventMapId) {
				continue
			}
			if weekEventExp >= weeklyExpCap {
				eventExp = 0
			} else if weekEventExp+eventExp > weeklyExpCap {
				eventExp = weeklyExpCap - weekEventExp
			}

			_, err = db.Exec("INSERT INTO eventCompletions (eventId, uuid, type, timestampCompleted, exp) VALUES (?, ?, 2, ?, ?)", eventId, playerUuid, time.Now(), eventExp)
			if err != nil {
				break
			}

			exp += eventExp
			weekEventExp += eventExp
		}

		return exp, nil
	}

	return -1, err
}

func writeGameBadges() (err error) {
	_, err = db.Exec("TRUNCATE TABLE badges")
	if err != nil {
		return err
	}

	for badgeGame := range badges {
		for badgeId, badge := range badges[badgeGame] {
			_, err = db.Exec("INSERT INTO badges (badgeId, game, bp, hidden) VALUES (?, ?, ?, ?)", badgeId, badgeGame, badge.Bp, badge.Hidden || badge.Dev)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func getPlayerUnlockedBadgeIds(playerUuid string) (unlockedBadgeIds []string, err error) {
	results, err := db.Query("SELECT badgeId FROM playerBadges WHERE uuid = ?", playerUuid)
	if err != nil {
		return unlockedBadgeIds, err
	}

	defer results.Close()

	for results.Next() {
		var badgeId string
		err := results.Scan(&badgeId)
		if err != nil {
			return unlockedBadgeIds, err
		}
		unlockedBadgeIds = append(unlockedBadgeIds, badgeId)
	}

	return unlockedBadgeIds, nil
}

func unlockPlayerBadge(playerUuid string, badgeId string) (err error) {
	_, err = db.Exec("INSERT INTO playerBadges (uuid, badgeId, timestampUnlocked) VALUES (?, ?, ?) ON DUPLICATE KEY UPDATE badgeId = badgeId", playerUuid, badgeId, time.Now())
	if err != nil {
		return err
	}

	return nil
}

func removePlayerBadge(playerUuid string, badgeId string) (err error) {
	var slotRow int
	var slotCol int

	err = db.QueryRow("SELECT slotRow, slotCol FROM playerBadges WHERE uuid = ? AND badgeId = ?", playerUuid, badgeId).Scan(&slotRow, &slotCol)
	if err != nil {
		return err
	}

	_, err = db.Exec("DELETE FROM playerBadges WHERE uuid = ? AND badgeId = ?", playerUuid, badgeId)
	if err != nil {
		return err
	}

	_, err = db.Exec("UPDATE accounts SET badge = 'null' WHERE uuid = ? AND badge = ?", playerUuid, badgeId)
	if err != nil {
		return err
	}

	return nil
}

func getBadgeUnlockPercentages() (unlockPercentages []*BadgePercentUnlocked, err error) {
	results, err := db.Query("SELECT b.badgeId, (COUNT(b.uuid) / aa.count) * 100 FROM playerBadges b JOIN accounts a ON a.uuid = b.uuid JOIN (SELECT COUNT(aa.uuid) count FROM accounts aa WHERE EXISTS(SELECT * FROM playerBadges aab WHERE aab.uuid = aa.uuid AND aa.timestampLoggedIn >= DATE_ADD(NOW(), INTERVAL -3 MONTH))) aa WHERE EXISTS(SELECT * FROM playerBadges ab WHERE ab.uuid = a.uuid AND a.timestampLoggedIn >= DATE_ADD(NOW(), INTERVAL -3 MONTH)) GROUP BY b.badgeId")
	if err != nil {
		return unlockPercentages, err
	}

	defer results.Close()

	for results.Next() {
		percentUnlocked := &BadgePercentUnlocked{}

		err := results.Scan(&percentUnlocked.BadgeId, &percentUnlocked.Percent)
		if err != nil {
			return unlockPercentages, err
		}

		unlockPercentages = append(unlockPercentages, percentUnlocked)
	}

	return unlockPercentages, nil
}

func getPlayerTags(playerUuid string) (tags []string, err error) {
	results, err := db.Query("SELECT name FROM playerTags WHERE uuid = ?", playerUuid)
	if err != nil {
		return tags, err
	}

	defer results.Close()

	for results.Next() {
		var tagName string
		err := results.Scan(&tagName)
		if err != nil {
			return tags, err
		}
		tags = append(tags, tagName)
	}

	return tags, nil
}

func tryWritePlayerTag(playerUuid string, name string) (success bool, err error) {
	if client, ok := clients.Load(playerUuid); ok { // Player must be online to add a tag
		client := client.(*SessionClient).rClient
		if client == nil {
			return false, nil
		}

		// Spare SQL having to deal with a duplicate record by checking player tags beforehand
		var tagExists bool
		for _, tag := range client.tags {
			if tag == name {
				tagExists = true
				break
			}
		}
		if !tagExists {
			_, err = db.Exec("INSERT INTO playerTags (uuid, name, timestampUnlocked) VALUES (?, ?, ?) ON DUPLICATE KEY UPDATE name = name", playerUuid, name, time.Now())
			if err != nil {
				return false, err
			}
			return true, nil
		}
	}

	return false, nil
}

/*func getTimeTrialMapIds() (mapIds []int, err error) {
	results, err := db.Query("SELECT mapId FROM playerTimeTrials GROUP BY mapId ORDER BY MIN(seconds)")
	if err != nil {
		return mapIds, err
	}

	defer results.Close()

	for results.Next() {
		var mapId int
		err := results.Scan(&mapId)
		if err != nil {
			return mapIds, err
		}

		mapIds = append(mapIds, mapId)
	}

	return mapIds, nil
}*/

func getPlayerTimeTrialRecords(playerUuid string) (timeTrialRecords []*TimeTrialRecord, err error) {
	results, err := db.Query("SELECT mapId, MIN(seconds) FROM playerTimeTrials WHERE uuid = ? GROUP BY mapId", playerUuid)
	if err != nil {
		return timeTrialRecords, err
	}

	defer results.Close()

	for results.Next() {
		timeTrialRecord := &TimeTrialRecord{}

		err := results.Scan(&timeTrialRecord.MapId, &timeTrialRecord.Seconds)
		if err != nil {
			return timeTrialRecords, err
		}

		timeTrialRecords = append(timeTrialRecords, timeTrialRecord)
	}

	return timeTrialRecords, nil
}

func tryWritePlayerTimeTrial(playerUuid string, mapId int, seconds int) (success bool, err error) {
	var prevSeconds int
	err = db.QueryRow("SELECT seconds FROM playerTimeTrials WHERE uuid = ? AND mapId = ?", playerUuid, mapId).Scan(&prevSeconds)
	if err != nil {
		if err != sql.ErrNoRows {
			return false, err
		}
	} else if seconds >= prevSeconds {
		return false, nil
	} else {
		_, err = db.Exec("UPDATE playerTimeTrials SET seconds = ?, timestampCompleted = ? WHERE uuid = ? AND mapId = ?", seconds, time.Now(), playerUuid, mapId)
		if err != nil {
			return false, err
		}
		return true, nil
	}

	_, err = db.Exec("INSERT INTO playerTimeTrials (uuid, mapId, seconds, timestampCompleted) VALUES (?, ?, ?, ?)", playerUuid, mapId, seconds, time.Now())
	if err != nil {
		return false, err
	}

	return true, nil
}

/*func getGameMinigameIds() (minigameIds []string, err error) {
	results, err := db.Query("SELECT DISTINCT minigameId FROM playerMinigameScores WHERE game = ? ORDER BY minigameId", serverConfig.GameName)
	if err != nil {
		return minigameIds, err
	}

	defer results.Close()

	for results.Next() {
		var minigameId string
		err := results.Scan(&minigameId)
		if err != nil {
			return minigameIds, err
		}

		minigameIds = append(minigameIds, minigameId)
	}

	return minigameIds, nil
}*/

func getPlayerMinigameScore(playerUuid string, minigameId string) (score int, err error) {
	err = db.QueryRow("SELECT score FROM playerMinigameScores WHERE uuid = ? AND minigameId = ?", playerUuid, minigameId).Scan(&score)
	if err != nil {
		if err == sql.ErrNoRows {
			return 0, nil
		}
		return 0, err
	}

	return score, nil
}

func tryWritePlayerMinigameScore(playerUuid string, minigameId string, score int) (success bool, err error) {
	if score <= 0 {
		return false, nil
	}

	prevScore, err := getPlayerMinigameScore(playerUuid, minigameId)
	if err != nil {
		return false, err
	} else if score <= prevScore {
		return false, nil
	} else if prevScore > 0 {
		_, err = db.Exec("UPDATE playerMinigameScores SET score = ?, timestampCompleted = ? WHERE uuid = ? AND game = ? AND minigameId = ?", score, time.Now(), playerUuid, serverConfig.GameName, minigameId)
		if err != nil {
			return false, err
		}
		return true, nil
	}

	_, err = db.Exec("INSERT INTO playerMinigameScores (uuid, game, minigameId, score, timestampCompleted) VALUES (?, ?, ?, ?, ?)", playerUuid, serverConfig.GameName, minigameId, score, time.Now())
	if err != nil {
		return false, err
	}

	return true, nil
}

func getModeratedPlayers(action int) (players []PlayerInfo) {
	var actionStr string

	if action == 0 {
		actionStr = "banned"
	} else {
		actionStr = "muted"
	}

	results, err := db.Query("SELECT uuid, rank FROM players WHERE " + actionStr + " = 1")
	if err != nil {
		return players
	}

	defer results.Close()

	for results.Next() {
		var uuid string
		var rank int

		err := results.Scan(&uuid, &rank)
		if err != nil {
			return players
		}

		players = append(players, PlayerInfo{
			Uuid: uuid,
			Name: getNameFromUuid(uuid),
			Rank: rank,
		})
	}

	return players
}

func getNameFromUuid(uuid string) (name string) {
	// get name from sessionClients if they're connected
	if client, ok := clients.Load(uuid); ok {
		return client.(*SessionClient).name
	}

	// otherwise check accounts
	err := db.QueryRow("SELECT user FROM accounts WHERE uuid = ?", uuid).Scan(&name)
	if err != nil {
		return ""
	}

	// if we got a name then return it
	if name != "" {
		return name
	}

	// otherwise check playergamedata
	err = db.QueryRow("SELECT name FROM playerGameData WHERE uuid = ?").Scan(&name)
	if err != nil {
		return ""
	}

	return name
}

func isIpBanned(ip string) bool {
	var banned int

	// check if account is banned
	err := db.QueryRow("SELECT banned FROM players WHERE uuid IN (SELECT uuid FROM accounts WHERE ip = ?)", ip).Scan(&banned)
	if err != nil {
		return false
	}

	if banned == 1 {
		return true
	}

	// check if guest account is banned
	err = db.QueryRow("SELECT banned FROM players WHERE ip = ?", ip).Scan(&banned)
	if err != nil {
		return false
	}

	return banned == 1
}

func getUuidFromToken(token string) (uuid string) {
	db.QueryRow("SELECT uuid FROM playerSessions WHERE sessionId = ? AND NOW() < expiration", token).Scan(&uuid)

	return uuid
}

func writeGamePlayerCount(playerCount int) error {
	_, err := db.Exec("INSERT INTO gamePlayerCounts (game, playerCount) VALUES (?, ?)", serverConfig.GameName, playerCount)
	if err != nil {
		return err
	}

	var playerCounts int
	err = db.QueryRow("SELECT COUNT(*) FROM gamePlayerCounts WHERE game = ?", serverConfig.GameName).Scan(&playerCounts)
	if err != nil {
		return err
	}

	if playerCounts > 28 {
		_, err = db.Exec("DELETE FROM gamePlayerCounts WHERE game = ? ORDER BY id LIMIT ?", serverConfig.GameName, playerCounts-28)
		if err != nil {
			return err
		}
	}

	return nil
}
