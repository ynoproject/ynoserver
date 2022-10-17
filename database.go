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
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

var (
	db = getDatabaseConn()
)

func getDatabaseConn() *sql.DB {
	conn, err := sql.Open("mysql", "yno@unix(/run/mysqld/mysqld.sock)/ynodb?parseTime=true")
	if err != nil {
		return nil
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

func createPlayerData(ip string, uuid string, rank int, banned bool) error {
	_, err := db.Exec("INSERT INTO players (ip, uuid, rank, banned) VALUES (?, ?, ?, ?)", ip, uuid, rank, banned)
	if err != nil {
		return err
	}

	return nil
}

func getPlayerGameData(uuid string) (spriteName string, spriteIndex int, systemName string) {
	err := db.QueryRow("SELECT pgd.spriteName, pgd.spriteIndex, pgd.systemName FROM players pd LEFT JOIN playerGameData pgd ON pgd.uuid = pd.uuid WHERE pd.uuid = ? AND pgd.game = ?", uuid, config.gameName).Scan(&spriteName, &spriteIndex, &systemName)
	if err != nil {
		return "", 0, ""
	}

	return spriteName, spriteIndex, systemName
}

func updatePlayerGameData(client *SessionClient) error {
	_, err := db.Exec("INSERT INTO playerGameData (uuid, game, name, systemName, spriteName, spriteIndex) VALUES (?, ?, ?, ?, ?, ?) ON DUPLICATE KEY UPDATE name = ?, systemName = ?, spriteName = ?, spriteIndex = ?", client.uuid, config.gameName, client.name, client.systemName, client.spriteName, client.spriteIndex, client.name, client.systemName, client.spriteName, client.spriteIndex)
	if err != nil {
		return err
	}

	return nil
}

func getPlayerInfo(ip string) (uuid string, name string, rank int) {
	err := db.QueryRow("SELECT pd.uuid, pgd.name, pd.rank FROM players pd LEFT JOIN playerGameData pgd ON pgd.uuid = pd.uuid WHERE pd.ip = ? AND (pgd.uuid IS NULL OR pgd.game = ?)", ip, config.gameName).Scan(&uuid, &name, &rank)
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
	query := "UPDATE accounts JOIN (SELECT pb.uuid, SUM(b.bp) bp, COUNT(b.badgeId) bc FROM playerBadges pb JOIN badges b ON b.badgeId = pb.badgeId GROUP BY pb.uuid) AS pb ON pb.uuid = accounts.uuid SET badgeSlotRows = CASE WHEN bp < 1000 THEN 1 WHEN bp < 2500 THEN 2 WHEN bp < 5000 THEN 3 WHEN bp < 10000 THEN 4 WHEN bp < 17500 THEN 5 WHEN bp < 30000 THEN 6 ELSE 7 END, badgeSlotCols = CASE WHEN bc < 50 THEN 3 WHEN bc < 150 THEN 4 WHEN bc < 300 THEN 5 WHEN bc < 500 THEN 6 ELSE 7 END"
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

	results.Close()

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
	err = db.QueryRow("SELECT pm.partyId FROM partyMembers pm JOIN parties p ON p.id = pm.partyId WHERE pm.uuid = ? AND p.game = ?", uuid, config.gameName).Scan(&partyId)
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

	results, err := db.Query("SELECT p.id, p.owner, p.name, p.public, p.pass, p.theme, p.description FROM parties p WHERE p.game = ?", config.gameName)
	if err != nil {
		return parties, err
	}

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

	results.Close()

	return parties, nil
}

func getAllPartyMemberDataByParty(simple bool) (partyMembersByParty map[int][]*PartyMember, err error) {
	partyMembersByParty = make(map[int][]*PartyMember)

	results, err := db.Query("SELECT pm.partyId, pm.uuid, COALESCE(a.user, pgd.name), pd.rank, CASE WHEN a.user IS NULL THEN 0 ELSE 1 END, COALESCE(a.badge, ''), pgd.systemName, pgd.spriteName, pgd.spriteIndex FROM partyMembers pm JOIN playerGameData pgd ON pgd.uuid = pm.uuid JOIN players pd ON pd.uuid = pgd.uuid JOIN parties p ON p.id = pm.partyId LEFT JOIN accounts a ON a.uuid = pd.uuid WHERE pgd.game = ? ORDER BY CASE WHEN p.owner = pm.uuid THEN 0 ELSE 1 END, pd.rank DESC, pm.id", config.gameName)
	if err != nil {
		return partyMembersByParty, err
	}

	offlinePartyMembersByParty := make(map[int][]*PartyMember)

	for results.Next() {
		var partyId int
		var accountBin int
		partyMember := &PartyMember{}
		err := results.Scan(&partyId, &partyMember.Uuid, &partyMember.Name, &partyMember.Rank, &accountBin, &partyMember.Badge, &partyMember.SystemName, &partyMember.SpriteName, &partyMember.SpriteIndex)
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

	results.Close()

	for partyId, offlinePartyMembers := range offlinePartyMembersByParty {
		partyMembersByParty[partyId] = append(partyMembersByParty[partyId], offlinePartyMembers...)
	}

	return partyMembersByParty, nil
}

func getPartyData(playerUuid string) (party Party, err error) { // called by api only
	err = db.QueryRow("SELECT p.id, p.owner, p.name, p.public, p.pass, p.theme, p.description FROM parties p JOIN partyMembers pm ON pm.partyId = p.id JOIN playerGameData pgd ON pgd.uuid = pm.uuid AND pgd.game = p.game WHERE p.game = ? AND pm.uuid = ?", config.gameName, playerUuid).Scan(&party.Id, &party.OwnerUuid, &party.Name, &party.Public, &party.Pass, &party.SystemName, &party.Description)
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
	results, err := db.Query("SELECT pm.partyId, pm.uuid, COALESCE(a.user, pgd.name), pd.rank, CASE WHEN a.user IS NULL THEN 0 ELSE 1 END, COALESCE(a.badge, ''), pgd.systemName, pgd.spriteName, pgd.spriteIndex FROM partyMembers pm JOIN playerGameData pgd ON pgd.uuid = pm.uuid JOIN players pd ON pd.uuid = pgd.uuid JOIN parties p ON p.id = pm.partyId LEFT JOIN accounts a ON a.uuid = pd.uuid WHERE pm.partyId = ? AND pgd.game = ? ORDER BY CASE WHEN p.owner = pm.uuid THEN 0 ELSE 1 END, pd.rank DESC, pm.id", partyId, config.gameName)
	if err != nil {
		return partyMembers, err
	}

	for results.Next() {
		var partyId int
		var accountBin int
		partyMember := &PartyMember{}
		err := results.Scan(&partyId, &partyMember.Uuid, &partyMember.Name, &partyMember.Rank, &accountBin, &partyMember.Badge, &partyMember.SystemName, &partyMember.SpriteName, &partyMember.SpriteIndex)
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

	results.Close()

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
	res, err := db.Exec("INSERT INTO parties (game, owner, name, public, pass, theme, description) VALUES (?, ?, ?, ?, ?, ?, ?)", config.gameName, playerUuid, name, public, pass, theme, description)
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
	_, err = db.Exec("UPDATE parties SET game = ?, owner = ?, name = ?, public = ?, pass = ?, theme = ?, description = ? WHERE id = ?", config.gameName, playerUuid, name, public, pass, theme, description, partyId)
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
	_, err := db.Exec("DELETE pm FROM partyMembers pm JOIN parties p ON p.id = pm.partyId WHERE pm.uuid = ? AND p.game = ?", playerUuid, config.gameName)
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

	for results.Next() {
		var uuid string
		err := results.Scan(&uuid)
		if err != nil {
			return partyMemberUuids, err
		}
		partyMemberUuids = append(partyMemberUuids, uuid)
	}

	results.Close()

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
	err = db.QueryRow("SELECT timestamp FROM playerGameSaves WHERE uuid = ? AND game = ?", playerUuid, config.gameName).Scan(&timestamp)
	if err != nil {
		return timestamp, err
	}

	return timestamp, nil
}

func getSaveData(playerUuid string) (saveData string, err error) { // called by api only
	err = db.QueryRow("SELECT data FROM playerGameSaves WHERE uuid = ? AND game = ?", playerUuid, config.gameName).Scan(&saveData)
	if err != nil {
		return saveData, err
	}

	return saveData, nil
}

func createGameSaveData(playerUuid string, timestamp time.Time, data string) (err error) { // called by api only
	_, err = db.Exec("INSERT INTO playerGameSaves (uuid, game, timestamp, data) VALUES (?, ?, ?, ?) ON DUPLICATE KEY UPDATE timestamp = ?, data = ?", playerUuid, config.gameName, timestamp, data, timestamp, data)
	if err != nil {
		return err
	}

	return nil
}

func clearGameSaveData(playerUuid string) (err error) { // called by api only
	_, err = db.Exec("DELETE FROM playerGameSaves WHERE uuid = ? AND game = ?", playerUuid, config.gameName)
	if err != nil {
		return err
	}

	return nil
}

func getCurrentEventPeriodId() (periodId int, err error) {
	if currentEventPeriodId > -1 {
		return currentEventPeriodId, nil
	}

	err = db.QueryRow("SELECT id FROM eventPeriods WHERE game = ? AND UTC_DATE() >= startDate AND UTC_DATE() < endDate", config.gameName).Scan(&periodId)
	if err != nil {
		currentEventPeriodId = 0
		if err == sql.ErrNoRows {
			return 0, nil
		}
		return 0, err
	}

	currentEventPeriodId = periodId

	return periodId, nil
}

func getEventPeriodData() (eventPeriods []*EventPeriod, err error) {
	results, err := db.Query("SELECT periodOrdinal, endDate, enableVms FROM eventPeriods WHERE game = ? AND periodOrdinal > 0", config.gameName)
	if err != nil {
		return eventPeriods, err
	}

	for results.Next() {
		eventPeriod := &EventPeriod{}

		err := results.Scan(&eventPeriod.PeriodOrdinal, &eventPeriod.EndDate, &eventPeriod.EnableVms)
		if err != nil {
			return eventPeriods, err
		}

		eventPeriods = append(eventPeriods, eventPeriod)
	}

	results.Close()

	return eventPeriods, nil
}

func getCurrentEventPeriodData() (eventPeriod EventPeriod, err error) {
	err = db.QueryRow("SELECT periodOrdinal, endDate, enableVms FROM eventPeriods WHERE game = ? AND UTC_DATE() >= startDate AND UTC_DATE() < endDate", config.gameName).Scan(&eventPeriod.PeriodOrdinal, &eventPeriod.EndDate, &eventPeriod.EnableVms)
	if err != nil {
		eventPeriod.PeriodOrdinal = -1
		if err == sql.ErrNoRows {
			return eventPeriod, nil
		}
		return eventPeriod, err
	}

	return eventPeriod, nil
}

func getPlayerEventExpData(periodId int, playerUuid string) (eventExp EventExp, err error) {
	totalEventExp, err := getPlayerTotalEventExp(playerUuid)
	if err != nil {
		return eventExp, err
	}

	eventExp.TotalExp = totalEventExp

	periodEventExp, err := getPlayerPeriodEventExp(periodId, playerUuid)
	if err != nil {
		return eventExp, err
	}

	eventExp.PeriodExp = periodEventExp

	weekEventExp, err := getPlayerWeekEventExp(periodId, playerUuid)
	if err != nil {
		return eventExp, err
	}

	eventExp.WeekExp = weekEventExp

	return eventExp, nil
}

func getPlayerTotalEventExp(playerUuid string) (totalEventExp int, err error) {
	err = db.QueryRow("SELECT SUM(exp) FROM ((SELECT COALESCE(SUM(ec.exp), 0) exp FROM eventCompletions ec JOIN eventLocations el ON el.id = ec.eventId AND ec.type = 0 JOIN eventPeriods ep ON ep.id = el.periodId WHERE ep.game = ? AND ec.uuid = ?) UNION ALL (SELECT COALESCE(SUM(ec.exp), 0) exp FROM eventCompletions ec JOIN eventVms ev ON ev.id = ec.eventId AND ec.type = 2 JOIN eventPeriods ep ON ep.id = ev.periodId WHERE ep.game = ? AND ec.uuid = ?)) eventExp", config.gameName, playerUuid, config.gameName, playerUuid).Scan(&totalEventExp)
	if err != nil {
		return totalEventExp, err
	}

	return totalEventExp, nil
}

func getPlayerPeriodEventExp(periodId int, playerUuid string) (periodEventExp int, err error) {
	err = db.QueryRow("SELECT SUM(exp) FROM ((SELECT COALESCE(SUM(ec.exp), 0) exp FROM eventCompletions ec JOIN eventLocations el ON el.id = ec.eventId AND ec.type = 0 JOIN eventPeriods ep ON ep.id = el.periodId WHERE ep.id = ? AND ec.uuid = ?) UNION ALL (SELECT COALESCE(SUM(ec.exp), 0) exp FROM eventCompletions ec JOIN eventVms ev ON ev.id = ec.eventId AND ec.type = 2 JOIN eventPeriods ep ON ep.id = ev.periodId WHERE ep.id = ? AND ec.uuid = ?)) eventExp", periodId, playerUuid, periodId, playerUuid).Scan(&periodEventExp)
	if err != nil {
		return periodEventExp, err
	}

	return periodEventExp, nil
}

func getPlayerWeekEventExp(periodId int, playerUuid string) (weekEventExp int, err error) {
	weekdayIndex := int(time.Now().UTC().Weekday())

	err = db.QueryRow("SELECT SUM(exp) FROM ((SELECT COALESCE(SUM(ec.exp), 0) exp FROM eventCompletions ec JOIN eventLocations el ON el.id = ec.eventId AND ec.type = 0 JOIN eventPeriods ep ON ep.id = el.periodId WHERE ep.id = ? AND ec.uuid = ? AND DATE_SUB(UTC_DATE(), INTERVAL ? DAY) <= el.startDate AND DATE_ADD(UTC_DATE(), INTERVAL ? DAY) >= el.endDate) UNION ALL (SELECT COALESCE(SUM(ec.exp), 0) exp FROM eventCompletions ec JOIN eventVms ev ON ev.id = ec.eventId AND ec.type = 2 JOIN eventPeriods ep ON ep.id = ev.periodId WHERE ep.id = ? AND ec.uuid = ? AND DATE_SUB(UTC_DATE(), INTERVAL ? DAY) <= ev.startDate AND DATE_ADD(UTC_DATE(), INTERVAL ? DAY) >= ev.endDate)) eventExp", periodId, playerUuid, weekdayIndex, 7-weekdayIndex, periodId, playerUuid, weekdayIndex, 7-weekdayIndex).Scan(&weekEventExp)
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

func writeEventLocationData(periodId int, eventType int, title string, titleJP string, depth int, minDepth int, exp int, mapIds []string) (err error) {
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

	_, err = db.Exec("INSERT INTO eventLocations (periodId, type, title, titleJP, depth, minDepth, exp, startDate, endDate, mapIds) VALUES (?, ?, ?, ?, ?, ?, ?, DATE_SUB(UTC_DATE(), INTERVAL ? DAY), DATE_ADD(UTC_DATE(), INTERVAL ? DAY), ?)", periodId, eventType, title, titleJP, depth, minDepth, exp, offsetDays, days, mapIdsJson)
	if err != nil {
		return err
	}

	return nil
}

func writePlayerEventLocationData(periodId int, playerUuid string, title string, titleJP string, depth int, minDepth int, mapIds []string) (err error) {
	mapIdsJson, err := json.Marshal(mapIds)
	if err != nil {
		return err
	}

	_, err = db.Exec("INSERT INTO playerEventLocations (periodId, uuid, title, titleJP, depth, minDepth, startDate, endDate, mapIds) VALUES (?, ?, ?, ?, ?, ?, UTC_DATE(), DATE_ADD(UTC_DATE(), INTERVAL 1 DAY), ?)", periodId, playerUuid, title, titleJP, depth, minDepth, mapIdsJson)
	if err != nil {
		return err
	}

	return nil
}

func getCurrentPlayerEventLocationsData(periodId int, playerUuid string) (eventLocations []*EventLocation, err error) {
	results, err := db.Query("SELECT el.id, el.type, el.title, el.titleJP, el.depth, el.minDepth, el.exp, el.endDate, CASE WHEN ec.uuid IS NOT NULL THEN 1 ELSE 0 END FROM eventLocations el LEFT JOIN eventCompletions ec ON ec.eventId = el.id AND ec.type = 0 AND ec.uuid = ? WHERE el.periodId = ? AND UTC_DATE() >= el.startDate AND UTC_DATE() < el.endDate ORDER BY 2, 1", playerUuid, periodId)
	if err != nil {
		return eventLocations, err
	}

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

	results.Close()

	results, err = db.Query("SELECT pel.id, pel.title, pel.titleJP, pel.depth, pel.minDepth, pel.endDate FROM playerEventLocations pel LEFT JOIN eventCompletions ec ON ec.eventId = pel.id AND ec.type = 1 AND ec.uuid = pel.uuid WHERE pel.uuid = ? AND pel.periodId = ? AND ec.uuid IS NULL AND UTC_DATE() >= pel.startDate AND UTC_DATE() < pel.endDate ORDER BY 1", playerUuid, periodId)
	if err != nil {
		return eventLocations, err
	}

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

	results.Close()

	return eventLocations, nil
}

func tryCompleteEventLocation(periodId int, playerUuid string, location string) (exp int, err error) {
	if client, ok := clients.Load(playerUuid); ok {
		client := client.(*SessionClient)
		if client.rClient == nil {
			return -1, err
		}

		results, err := db.Query("SELECT el.id, el.type, el.exp, el.mapIds FROM eventLocations el WHERE el.periodId = ? AND el.title = ? AND UTC_DATE() >= el.startDate AND UTC_DATE() < el.endDate ORDER BY 2", periodId, location)
		if err != nil {
			return -1, err
		}

		weekEventExp, err := getPlayerWeekEventExp(periodId, playerUuid)
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

				rankingsMtx.Lock()
				_, err = db.Exec("INSERT INTO eventCompletions (eventId, uuid, type, timestampCompleted, exp) VALUES (?, ?, 0, ?, ?)", eventId, playerUuid, time.Now(), eventExp)
				if err != nil {
					rankingsMtx.Unlock()
					break
				}
				rankingsMtx.Unlock()

				exp += eventExp
				weekEventExp += eventExp
				break
			}
		}

		results.Close()

		return exp, nil
	}

	return -1, err
}

func tryCompletePlayerEventLocation(periodId int, playerUuid string, location string) (complete bool, err error) {
	if client, ok := clients.Load(playerUuid); ok {
		client := client.(*SessionClient).rClient
		if client == nil {
			return false, err
		}

		results, err := db.Query("SELECT pel.id, pel.mapIds FROM playerEventLocations pel WHERE pel.periodId = ? AND pel.title = ? AND pel.uuid = ? AND UTC_DATE() >= pel.startDate AND UTC_DATE() < pel.endDate ORDER BY 2", periodId, location, playerUuid)
		if err != nil {
			return false, err
		}

		var success bool

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

				rankingsMtx.Lock()
				_, err = db.Exec("INSERT INTO eventCompletions (eventId, uuid, type, timestampCompleted, exp) VALUES (?, ?, 1, ?, 0)", eventId, playerUuid, time.Now())
				if err != nil {
					rankingsMtx.Unlock()
					break
				}
				rankingsMtx.Unlock()

				success = true
				break
			}
		}

		results.Close()

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

func getCurrentPlayerEventVmsData(periodId int, playerUuid string) (eventVms []*EventVm, err error) {
	results, err := db.Query("SELECT ev.id, ev.exp, ev.endDate, CASE WHEN ec.uuid IS NOT NULL THEN 1 ELSE 0 END FROM eventVms ev LEFT JOIN eventCompletions ec ON ec.eventId = ev.id AND ec.type = 2 AND ec.uuid = ? WHERE ev.periodId = ? AND UTC_DATE() >= ev.startDate AND UTC_DATE() < ev.endDate ORDER BY 2, 1", playerUuid, periodId)
	if err != nil {
		return eventVms, err
	}

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

	results.Close()

	return eventVms, nil
}

func getEventVmInfo(id int) (mapId int, eventId int, err error) {
	err = db.QueryRow("SELECT mapId, eventId FROM eventVms WHERE id = ?", id).Scan(&mapId, &eventId)
	if err != nil {
		return 0, 0, err
	}

	return mapId, eventId, nil
}

func writeEventVmData(periodId int, mapId int, eventId int, exp int) (err error) {
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

	_, err = db.Exec("INSERT INTO eventVms (periodId, mapId, eventId, exp, startDate, endDate) VALUES (?, ?, ?, ?, DATE_SUB(UTC_DATE(), INTERVAL ? DAY), DATE_ADD(UTC_DATE(), INTERVAL ? DAY))", periodId, mapId, eventId, exp, offsetDays, days)
	if err != nil {
		return err
	}

	return nil
}

func tryCompleteEventVm(periodId int, playerUuid string, mapId int, eventId int) (exp int, err error) {
	if client, ok := clients.Load(playerUuid); ok {
		client := client.(*SessionClient).rClient
		if client == nil {
			return -1, err
		}

		results, err := db.Query("SELECT ev.id, ev.mapId, ev.eventId, ev.exp FROM eventVms ev WHERE ev.periodId = ? AND ev.mapId = ? AND ev.eventId = ? AND UTC_DATE() >= ev.startDate AND UTC_DATE() < ev.endDate ORDER BY 2", periodId, mapId, eventId)
		if err != nil {
			return -1, err
		}

		currentEventVmsData, err := getCurrentPlayerEventVmsData(periodId, playerUuid)
		if err != nil {
			return -1, err
		}

		weekEventExp, err := getPlayerWeekEventExp(periodId, playerUuid)
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

			rankingsMtx.Lock()
			_, err = db.Exec("INSERT INTO eventCompletions (eventId, uuid, type, timestampCompleted, exp) VALUES (?, ?, 2, ?, ?)", eventId, playerUuid, time.Now(), eventExp)
			if err != nil {
				rankingsMtx.Unlock()
				break
			}
			rankingsMtx.Unlock()

			exp += eventExp
			weekEventExp += eventExp
		}

		results.Close()

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

	for results.Next() {
		var badgeId string
		err := results.Scan(&badgeId)
		if err != nil {
			return unlockedBadgeIds, err
		}
		unlockedBadgeIds = append(unlockedBadgeIds, badgeId)
	}

	results.Close()

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
	results, err := db.Query("SELECT b.badgeId, (COUNT(b.uuid) / aa.count) * 100 FROM playerBadges b JOIN accounts a ON a.uuid = b.uuid JOIN (SELECT COUNT(aa.uuid) count FROM accounts aa WHERE aa.timestampLoggedIn IS NOT NULL) aa WHERE a.timestampLoggedIn IS NOT NULL GROUP BY b.badgeId")
	if err != nil {
		return unlockPercentages, err
	}

	for results.Next() {
		percentUnlocked := &BadgePercentUnlocked{}

		err := results.Scan(&percentUnlocked.BadgeId, &percentUnlocked.Percent)
		if err != nil {
			return unlockPercentages, err
		}

		unlockPercentages = append(unlockPercentages, percentUnlocked)
	}

	results.Close()

	return unlockPercentages, nil
}

func getPlayerTags(playerUuid string) (tags []string, err error) {
	results, err := db.Query("SELECT name FROM playerTags WHERE uuid = ?", playerUuid)
	if err != nil {
		return tags, err
	}

	for results.Next() {
		var tagName string
		err := results.Scan(&tagName)
		if err != nil {
			return tags, err
		}
		tags = append(tags, tagName)
	}

	results.Close()

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

func getTimeTrialMapIds() (mapIds []int, err error) {
	results, err := db.Query("SELECT mapId FROM playerTimeTrials GROUP BY mapId ORDER BY MIN(seconds)")
	if err != nil {
		return mapIds, err
	}

	for results.Next() {
		var mapId int
		err := results.Scan(&mapId)
		if err != nil {
			return mapIds, err
		}

		mapIds = append(mapIds, mapId)
	}

	results.Close()

	return mapIds, nil
}

func getPlayerTimeTrialRecords(playerUuid string) (timeTrialRecords []*TimeTrialRecord, err error) {
	results, err := db.Query("SELECT mapId, MIN(seconds) FROM playerTimeTrials WHERE uuid = ? GROUP BY mapId", playerUuid)
	if err != nil {
		return timeTrialRecords, err
	}

	for results.Next() {
		timeTrialRecord := &TimeTrialRecord{}

		err := results.Scan(&timeTrialRecord.MapId, &timeTrialRecord.Seconds)
		if err != nil {
			return timeTrialRecords, err
		}

		timeTrialRecords = append(timeTrialRecords, timeTrialRecord)
	}

	results.Close()

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

func getGameMinigameIds() (minigameIds []string, err error) {
	results, err := db.Query("SELECT DISTINCT minigameId FROM playerMinigameScores WHERE game = ? ORDER BY minigameId", config.gameName)
	if err != nil {
		return minigameIds, err
	}

	for results.Next() {
		var minigameId string
		err := results.Scan(&minigameId)
		if err != nil {
			return minigameIds, err
		}

		minigameIds = append(minigameIds, minigameId)
	}

	results.Close()

	return minigameIds, nil
}

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
		_, err = db.Exec("UPDATE playerMinigameScores SET score = ?, timestampCompleted = ? WHERE uuid = ? AND game = ? AND minigameId = ?", score, time.Now(), playerUuid, config.gameName, minigameId)
		if err != nil {
			return false, err
		}
		return true, nil
	}

	_, err = db.Exec("INSERT INTO playerMinigameScores (uuid, game, minigameId, score, timestampCompleted) VALUES (?, ?, ?, ?, ?)", playerUuid, config.gameName, minigameId, score, time.Now())
	if err != nil {
		return false, err
	}

	return true, nil
}

func getRankingCategories() (rankingCategories []*RankingCategory, err error) {
	results, err := db.Query("SELECT categoryId, game FROM rankingCategories WHERE game IN ('', ?) ORDER BY ordinal", config.gameName)
	if err != nil {
		return rankingCategories, err
	}

	for results.Next() {
		rankingCategory := &RankingCategory{}

		err := results.Scan(&rankingCategory.CategoryId, &rankingCategory.Game)
		if err != nil {
			return rankingCategories, err
		}

		rankingCategories = append(rankingCategories, rankingCategory)
	}

	results.Close()

	results, err = db.Query("SELECT sc.categoryId, sc.subCategoryId, sc.game, CEILING(COUNT(r.uuid) / 25) FROM rankingSubCategories sc JOIN rankingEntries r ON r.categoryId = sc.categoryId AND r.subCategoryId = sc.subCategoryId WHERE sc.game IN ('', ?) GROUP BY sc.categoryId, sc.subCategoryId, sc.game ORDER BY 1, sc.ordinal", config.gameName)
	if err != nil {
		return rankingCategories, err
	}

	var lastCategoryId string
	var lastCategory *RankingCategory

	for results.Next() {
		rankingSubCategory := &RankingSubCategory{}

		var categoryId string
		err := results.Scan(&categoryId, &rankingSubCategory.SubCategoryId, &rankingSubCategory.Game, &rankingSubCategory.PageCount)
		if err != nil {
			return rankingCategories, err
		}

		if lastCategoryId != categoryId {
			lastCategoryId = categoryId
			for _, rankingCategory := range rankingCategories {
				if rankingCategory.CategoryId == lastCategoryId {
					lastCategory = rankingCategory
				}
			}
		}

		lastCategory.SubCategories = append(lastCategory.SubCategories, *rankingSubCategory)
	}

	results.Close()

	return rankingCategories, nil
}

func writeRankingCategory(categoryId string, game string, order int) (err error) {
	_, err = db.Exec("INSERT INTO rankingCategories (categoryId, game, ordinal) VALUES (?, ?, ?) ON DUPLICATE KEY UPDATE ordinal = ?", categoryId, game, order, order)
	if err != nil {
		return err
	}

	return nil
}

func writeRankingSubCategory(categoryId string, subCategoryId string, game string, order int) (err error) {
	_, err = db.Exec("INSERT INTO rankingSubCategories (categoryId, subCategoryId, game, ordinal) VALUES (?, ?, ?, ?) ON DUPLICATE KEY UPDATE ordinal = ?", categoryId, subCategoryId, game, order, order)
	if err != nil {
		return err
	}

	return nil
}

func getRankingEntryPage(playerUuid string, categoryId string, subCategoryId string) (page int, err error) {
	err = db.QueryRow("SELECT FLOOR(r.rowNum / 25) + 1 FROM (SELECT r.uuid, ROW_NUMBER() OVER (ORDER BY r.position) rowNum FROM rankingEntries r WHERE r.categoryId = ? AND r.subCategoryId = ?) r WHERE r.uuid = ?", categoryId, subCategoryId, playerUuid).Scan(&page)
	if err != nil {
		if err == sql.ErrNoRows {
			return 1, nil
		}
		return 1, err
	}

	return page, nil
}

func getRankingsPaged(categoryId string, subCategoryId string, page int) (rankings []*Ranking, err error) {
	var valueType string
	switch categoryId {
	case "eventLocationCompletion":
		valueType = "Float"
	default:
		valueType = "Int"
	}

	results, err := db.Query("SELECT r.position, a.user, pd.rank, a.badge, COALESCE(pgd.systemName, ''), r.value"+valueType+" FROM rankingEntries r JOIN accounts a ON a.uuid = r.uuid JOIN players pd ON pd.uuid = a.uuid LEFT JOIN playerGameData pgd ON pgd.uuid = pd.uuid AND pgd.game = ? WHERE r.categoryId = ? AND r.subCategoryId = ? ORDER BY 1, r.timestamp LIMIT "+strconv.Itoa((page-1)*25)+", 25", config.gameName, categoryId, subCategoryId)
	if err != nil {
		return rankings, err
	}

	for results.Next() {
		ranking := &Ranking{}

		if valueType == "Int" {
			err = results.Scan(&ranking.Position, &ranking.Name, &ranking.Rank, &ranking.Badge, &ranking.SystemName, &ranking.ValueInt)
		} else {
			err = results.Scan(&ranking.Position, &ranking.Name, &ranking.Rank, &ranking.Badge, &ranking.SystemName, &ranking.ValueFloat)
		}
		if err != nil {
			return rankings, err
		}

		rankings = append(rankings, ranking)
	}

	results.Close()

	return rankings, nil
}

func updateRankingEntries(categoryId string, subCategoryId string) (err error) {
	var valueType string
	switch categoryId {
	case "eventLocationCompletion":
		valueType = "Float"
	default:
		valueType = "Int"
	}

	_, err = db.Exec("DELETE FROM rankingEntries WHERE categoryId = ? AND subCategoryId = ?", categoryId, subCategoryId)
	if err != nil {
		return err
	}

	isFiltered := subCategoryId != "all"
	var isUnion bool

	query := "INSERT INTO rankingEntries (categoryId, subCategoryId, position, uuid, value" + valueType + ", timestamp) "

	switch categoryId {
	case "badgeCount":
		query += "SELECT ?, ?, RANK() OVER (ORDER BY COUNT(pb.uuid) DESC), a.uuid, COUNT(pb.uuid), (SELECT MAX(apb.timestampUnlocked) FROM playerBadges apb WHERE apb.uuid = a.uuid AND apb.badgeId = b.badgeId) FROM playerBadges pb JOIN accounts a ON a.uuid = pb.uuid JOIN badges b ON b.badgeId = pb.badgeId WHERE b.hidden = 0"
		if isFiltered {
			query += " AND b.game = ?"
		}
		query += " GROUP BY a.uuid ORDER BY 5 DESC, 6"
	case "bp":
		query += "SELECT ?, ?, RANK() OVER (ORDER BY SUM(b.bp) DESC), a.uuid, SUM(b.bp), (SELECT MAX(apb.timestampUnlocked) FROM playerBadges apb WHERE apb.uuid = a.uuid AND apb.badgeId = b.badgeId) FROM playerBadges pb JOIN accounts a ON a.uuid = pb.uuid JOIN badges b ON b.badgeId = pb.badgeId"
		if isFiltered {
			query += " WHERE b.game = ?"
		}
		query += " GROUP BY a.uuid ORDER BY 5 DESC, 6"
	case "exp":
		query += "SELECT ?, ?, RANK() OVER (ORDER BY SUM(ec.exp) DESC), ec.uuid, SUM(ec.exp), (SELECT MAX(aec.timestampCompleted) FROM eventCompletions aec WHERE aec.uuid = ec.uuid) FROM ((SELECT ec.uuid, ec.exp FROM eventCompletions ec JOIN eventLocations el ON el.id = ec.eventId AND ec.type = 0"
		if isFiltered {
			query += " JOIN eventPeriods ep ON ep.id = el.periodId AND ep.periodOrdinal = ?"
		}
		query += ") UNION ALL (SELECT ec.uuid, ec.exp FROM eventCompletions ec JOIN eventVms ev ON ev.id = ec.eventId AND ec.type = 2"
		if isFiltered {
			query += " JOIN eventPeriods ep ON ep.id = ev.periodId AND ep.periodOrdinal = ?"
		}
		query += ")) ec GROUP BY ec.uuid ORDER BY 5 DESC, 6"
		isUnion = true
	case "eventLocationCount", "freeEventLocationCount":
		isFree := categoryId == "freeEventLocationCount"
		query += "SELECT ?, ?, RANK() OVER (ORDER BY COUNT(ec.uuid) DESC), ec.uuid, COUNT(ec.uuid), (SELECT MAX(aec.timestampCompleted) FROM eventCompletions aec WHERE aec.uuid = ec.uuid) FROM eventCompletions ec "
		if isFiltered {
			if isFree {
				query += "JOIN playerEventLocations el"
			} else {
				query += "JOIN eventLocations el"
			}
			query += " ON el.id = ec.eventId JOIN eventPeriods ep ON ep.id = el.periodId AND ep.periodOrdinal = ? "
		}
		query += "WHERE ec.type = "
		if isFree {
			query += "1"
		} else {
			query += "0"
		}
		query += " GROUP BY ec.uuid ORDER BY 5 DESC, 6"
	case "eventLocationCompletion":
		query += "SELECT ?, ?, RANK() OVER (ORDER BY COUNT(DISTINCT COALESCE(el.title, pel.title)) / aec.count DESC), a.uuid, COUNT(DISTINCT COALESCE(el.title, pel.title)) / aec.count, (SELECT MAX(aect.timestampCompleted) FROM eventCompletions aect WHERE aect.uuid = ec.uuid) FROM eventCompletions ec JOIN accounts a ON a.uuid = ec.uuid LEFT JOIN eventLocations el ON el.id = ec.eventId AND ec.type = 0 LEFT JOIN playerEventLocations pel ON pel.id = ec.eventId AND ec.type = 1 JOIN (SELECT COUNT(DISTINCT COALESCE(ael.title, apel.title)) count FROM eventCompletions aec LEFT JOIN eventLocations ael ON ael.id = aec.eventId AND aec.type = 0 LEFT JOIN playerEventLocations apel ON apel.id = aec.eventId AND aec.type = 1 WHERE (ael.title IS NOT NULL OR apel.title IS NOT NULL)) aec"
		if isFiltered {
			query += " JOIN eventPeriods ep ON ep.id = COALESCE(el.periodId, pel.periodId) AND ep.periodOrdinal = ?"
		}
		query += " GROUP BY a.user ORDER BY 5 DESC, 6"
	case "eventVmCount":
		query += "SELECT ?, ?, RANK() OVER (ORDER BY COUNT(ec.uuid) DESC), ec.uuid, COUNT(ec.uuid), (SELECT MAX(aec.timestampCompleted) FROM eventCompletions aec WHERE aec.uuid = ec.uuid) FROM eventCompletions ec "
		if isFiltered {
			query += "JOIN eventVms ev ON ev.id = ec.eventId JOIN eventPeriods ep ON ep.id = ev.periodId AND ep.periodOrdinal = ? "
		}
		query += "WHERE ec.type = 2 GROUP BY ec.uuid ORDER BY 5 DESC, 6"
	case "timeTrial":
		query += "SELECT ?, ?, RANK() OVER (ORDER BY MIN(tt.seconds)), tt.uuid, MIN(tt.seconds), (SELECT MAX(att.timestampCompleted) FROM playerTimeTrials att WHERE att.uuid = tt.uuid AND att.mapId = tt.mapId AND att.seconds = tt.seconds) FROM playerTimeTrials tt WHERE tt.mapId = ? GROUP BY tt.uuid ORDER BY 5, 6"
	case "minigame":
		query += "SELECT ?, ?, RANK() OVER (ORDER BY MAX(ms.score) DESC), ms.uuid, MAX(ms.score), (SELECT MAX(ams.timestampCompleted) FROM playerMinigameScores ams WHERE ams.uuid = ms.uuid AND ams.minigameId = ms.minigameId AND ams.score = ms.score) FROM playerMinigameScores ms WHERE ms.minigameId = ? GROUP BY ms.uuid ORDER BY 5 DESC, 6"
	}

	if isFiltered {
		if isUnion {
			_, err = db.Exec(query, categoryId, subCategoryId, subCategoryId, subCategoryId)
		} else {
			_, err = db.Exec(query, categoryId, subCategoryId, subCategoryId)
		}
	} else {
		_, err = db.Exec(query, categoryId, subCategoryId)
	}

	if err != nil {
		return err
	}

	return nil
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

	results.Close()

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
	results, err := db.Query("SELECT name FROM playerGameData WHERE uuid = ?")
	if err != nil {
		return ""
	}

	for results.Next() {
		err := results.Scan(&name)
		if err != nil {
			return ""
		}

		if name != "" {
			return name
		}
	}

	// couldn't find a name
	return ""
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
