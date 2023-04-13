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
	"fmt"
	"math"
	"math/rand"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

var db = getDatabaseConn()

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
		createPlayerData(ip, uuid, banned)

		// recheck moderation status
		if !banned {
			banned, muted = getPlayerModerationStatus(uuid)
		}
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
		return client.rank // return rank from session if client is connected
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
		client.muted = true
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
		client.muted = false
	}

	return nil
}

func tryChangePlayerUsername(senderUuid string, recipientUuid string, newUsername string) error { // called by api only
	if getPlayerRank(senderUuid) <= getPlayerRank(recipientUuid) {
		return errors.New("insufficient rank")
	}

	existingUuid, err := getUuidFromName(newUsername)
	if err != nil {
		return err
	}
	if existingUuid != "" {
		return errors.New("user with new username already exists")
	}

	_, err = db.Exec("UPDATE accounts SET user = ? WHERE uuid = ?", newUsername, recipientUuid)
	if err != nil {
		return err
	}

	if client, ok := clients.Load(recipientUuid); ok { // change client username if they're connected
		client.name = newUsername

		if client.rClient != nil {
			client.rClient.broadcast(buildMsg("name", client.id, newUsername)) // broadcast name change to room if client is in one
		}
	}

	return nil
}

func getPlayerMedals(uuid string) (medals [5]int) {
	if client, ok := clients.Load(uuid); ok {
		return client.medals // return medals from session if client is connected
	}

	err := db.QueryRow("SELECT pgd.medalCountBronze, pgd.medalCountSilver, pgd.medalCountGold, pgd.medalCountPlatinum, pgd.medalCountDiamond FROM players pd LEFT JOIN playerGameData pgd ON pgd.uuid = pd.uuid WHERE pd.uuid = ? AND pgd.game = ?", uuid, serverConfig.gameName).Scan(&medals[0], &medals[1], &medals[2], &medals[3], &medals[4])
	if err != nil {
		return [5]int{}
	}

	return medals
}

func getPlayerModerationStatus(uuid string) (banned bool, muted bool) {
	err := db.QueryRow("SELECT banned, muted FROM players WHERE uuid = ?", uuid).Scan(&banned, &muted)
	if err != nil {
		return false, false
	}

	return banned, muted
}

func createPlayerData(ip string, uuid string, banned bool) error {
	_, err := db.Exec("INSERT INTO players (ip, uuid, banned) VALUES (?, ?, ?)", ip, uuid, banned)
	if err != nil {
		return err
	}

	return nil
}

func getPlayerGameData(uuid string) (spriteName string, spriteIndex int, systemName string) {
	err := db.QueryRow("SELECT pgd.spriteName, pgd.spriteIndex, pgd.systemName FROM players pd LEFT JOIN playerGameData pgd ON pgd.uuid = pd.uuid WHERE pd.uuid = ? AND pgd.game = ?", uuid, serverConfig.gameName).Scan(&spriteName, &spriteIndex, &systemName)
	if err != nil {
		return "", 0, ""
	}

	return spriteName, spriteIndex, systemName
}

func (c *SessionClient) updatePlayerGameData() error {
	_, err := db.Exec("INSERT INTO playerGameData (uuid, game, name, systemName, spriteName, spriteIndex) VALUES (?, ?, ?, ?, ?, ?) ON DUPLICATE KEY UPDATE name = ?, systemName = ?, spriteName = ?, spriteIndex = ?", c.uuid, serverConfig.gameName, c.name, c.systemName, c.spriteName, c.spriteIndex, c.name, c.systemName, c.spriteName, c.spriteIndex)
	if err != nil {
		return err
	}

	return nil
}

func getPlayerInfo(ip string) (uuid string, name string, rank int) {
	err := db.QueryRow("SELECT pd.uuid, pgd.name, pd.rank FROM players pd LEFT JOIN playerGameData pgd ON pgd.uuid = pd.uuid WHERE pd.ip = ? AND (pgd.uuid IS NULL OR pgd.game = ?)", ip, serverConfig.gameName).Scan(&uuid, &name, &rank)
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

func updatePlayerActivity() error {
	_, err := db.Exec("UPDATE accounts SET inactive = CASE WHEN timestampLoggedIn IS NULL OR timestampLoggedIn < DATE_ADD(NOW(), INTERVAL -3 MONTH) THEN 1 ELSE 0 END")
	if err != nil {
		return err
	}

	return nil
}

func setPlayerBadge(uuid string, badge string) error {
	if client, ok := clients.Load(uuid); ok {
		client.badge = badge
	}

	_, err := db.Exec("UPDATE accounts SET badge = ? WHERE uuid = ?", badge, uuid)
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

func setPlayerBadgeSlot(uuid string, badgeId string, slotRow int, slotCol int) error {
	var slotCurrentBadgeId string
	err := db.QueryRow("SELECT badgeId FROM playerBadges WHERE uuid = ? AND slotRow = ? AND slotCol = ? LIMIT 1", uuid, slotRow, slotCol).Scan(&slotCurrentBadgeId)
	if err != nil {
		if err != sql.ErrNoRows {
			return err
		}
	} else if slotCurrentBadgeId == badgeId {
		return nil
	} else {
		if badgeId != "null" {
			var badgeCurrentSlotRow, badgeCurrentSlotCol int
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

func writeGlobalChatMessage(msgId, uuid, mapId, prevMapId, prevLocations string, x, y int, contents string) error {
	_, err := db.Exec("INSERT INTO chatMessages (msgId, game, uuid, mapId, prevMapId, prevLocations, x, y, contents) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)", msgId, serverConfig.gameName, uuid, mapId, prevMapId, prevLocations, x, y, contents)
	if err != nil {
		return err
	}

	msgSent = true

	return nil
}

func getLastMessageIds() (lastMsgIds map[int]string, err error) {
	lastMsgIds = make(map[int]string)

	results, err := db.Query("SELECT COALESCE(cm.partyId, 0), cm.msgId FROM chatMessages cm WHERE cm.timestamp = (SELECT MAX(cm2.timestamp) FROM chatMessages cm2 JOIN players pd ON pd.uuid = cm2.uuid WHERE cm2.game = ? AND pd.banned = 0 AND cm2.timestamp > DATE_ADD(UTC_TIMESTAMP(), INTERVAL -1 DAY) AND ((cm.partyId IS NULL AND cm2.partyId IS NULL) OR (cm2.partyId = cm.partyId))) GROUP BY COALESCE(cm.partyId, 0)", serverConfig.gameName)
	if err != nil {
		return lastMsgIds, nil
	}

	defer results.Close()

	for results.Next() {
		var partyId int
		var lastMsgId string

		results.Scan(&partyId, &lastMsgId)

		lastMsgIds[partyId] = lastMsgId
	}

	return lastMsgIds, nil
}

func updatePlayerLastChatMessage(uuid, lastMsgId string, party bool) error {
	query := "UPDATE playerGameData SET "

	if party {
		query += "lastPartyMsgId"
	} else {
		query += "lastGlobalMsgId"
	}

	query += " = ? WHERE uuid = ? AND game = ?"

	_, err := db.Exec(query, lastMsgId, uuid, serverConfig.gameName)
	if err != nil {
		return err
	}

	return nil
}

func getChatMessageHistory(uuid string, globalMsgLimit, partyMsgLimit int, lastMsgId string) (chatHistory *ChatHistory, err error) {
	chatHistory = &ChatHistory{}

	reconnectAfterRestart := lastMsgId != "" && !msgSent
	if reconnectAfterRestart {
		// Assume empty results if reconnecting after a disconnect with the last global message as the last message ID
		if lastGlobalMsgId, ok := lastMsgIds[0]; ok && lastGlobalMsgId == lastMsgId {
			return chatHistory, nil
		}
	}

	partyId, err := getPlayerPartyId(uuid)
	if err != nil {
		return chatHistory, err
	}

	if reconnectAfterRestart && partyId != 0 {
		// Assume empty results if reconnecting after a disconnect with the last party message in the user's party as the last message ID
		if lastPartyMsgId, ok := lastMsgIds[partyId]; ok && lastPartyMsgId == lastMsgId {
			return chatHistory, nil
		}
	}

	var query string

	selectClause := "SELECT cm.msgId, cm.uuid, cm.mapId, cm.prevMapId, cm.prevLocations, cm.x, cm.y, cm.contents, cm.timestamp, "
	globalSelectClause := selectClause + "0"
	partySelectClause := selectClause + "1"

	fromClause := " FROM chatMessages cm JOIN players pd ON pd.uuid = cm.uuid JOIN playerGameData pgd ON pgd.uuid = pd.uuid AND pgd.game = cm.game "

	whereClause := "WHERE cm.game = ? AND pd.banned = 0"

	if lastMsgId != "" {
		whereClause += " AND cm.timestamp > (SELECT cm2.timestamp FROM chatMessages cm2 WHERE cm2.msgId = ?)"
	}

	globalWhereClause := whereClause + " AND cm.partyId IS NULL AND (pgd.lastGlobalMsgId IS NULL OR cm.timestamp > (SELECT cmg.timestamp FROM chatMessages cmg WHERE cmg.msgId = pgd.lastGlobalMsgId)) ORDER BY 9 DESC"
	partyWhereClause := whereClause + " AND cm.partyId = ? AND (pgd.lastPartyMsgId IS NULL OR cm.timestamp > (SELECT cmp.timestamp FROM chatMessages cmp WHERE cmp.msgId = pgd.lastPartyMsgId)) ORDER BY 9 DESC"

	var messageQueryArgs []interface{}

	messageQueryArgs = append(messageQueryArgs, serverConfig.gameName)

	if lastMsgId != "" {
		messageQueryArgs = append(messageQueryArgs, lastMsgId)
	}

	messageQueryArgs = append(messageQueryArgs, globalMsgLimit)

	query += "("

	if partyId == 0 {
		query += globalSelectClause + fromClause + globalWhereClause + " LIMIT ?"
	} else {
		messageQueryArgs = append(messageQueryArgs, serverConfig.gameName)

		if lastMsgId != "" {
			messageQueryArgs = append(messageQueryArgs, lastMsgId)
		}

		messageQueryArgs = append(messageQueryArgs, partyId, partyMsgLimit)

		query += globalSelectClause + fromClause + globalWhereClause + " LIMIT ?) UNION (" + partySelectClause + fromClause + partyWhereClause + " LIMIT ?"
	}

	query += ") ORDER BY 9"

	messageResults, err := db.Query(query, messageQueryArgs...)
	if err != nil {
		return chatHistory, err
	}

	defer messageResults.Close()

	for messageResults.Next() {
		chatMessage := &ChatMessage{}
		err := messageResults.Scan(&chatMessage.MsgId, &chatMessage.Uuid, &chatMessage.MapId, &chatMessage.PrevMapId, &chatMessage.PrevLocations, &chatMessage.X, &chatMessage.Y, &chatMessage.Contents, &chatMessage.Timestamp, &chatMessage.Party)
		if err != nil {
			return chatHistory, err
		}
		chatHistory.Messages = append(chatHistory.Messages, chatMessage)
	}

	var firstTimestamp time.Time
	var lastTimestamp time.Time

	if len(chatHistory.Messages) != 0 {
		firstTimestamp = chatHistory.Messages[0].Timestamp
		lastTimestamp = chatHistory.Messages[len(chatHistory.Messages)-1].Timestamp
	}

	playersQuery := "SELECT DISTINCT pd.uuid, COALESCE(a.user, pgd.name), pd.rank, CASE WHEN a.user IS NULL THEN 0 ELSE 1 END, COALESCE(a.badge, ''), pgd.systemName, pgd.medalCountBronze, pgd.medalCountSilver, pgd.medalCountGold, pgd.medalCountPlatinum, pgd.medalCountDiamond FROM players pd JOIN playerGameData pgd ON pgd.uuid = pd.uuid LEFT JOIN accounts a ON a.uuid = pd.uuid WHERE pgd.game = ? AND EXISTS (SELECT cm.uuid FROM chatMessages cm WHERE cm.uuid = pd.uuid AND cm.game = pgd.game AND cm.timestamp BETWEEN ? AND ? "

	var playerQueryArgs []interface{}

	playerQueryArgs = append(playerQueryArgs, serverConfig.gameName, firstTimestamp, lastTimestamp)

	if partyId == 0 {
		playersQuery += "AND cm.partyId IS NULL"
	} else {
		playersQuery += "AND (cm.partyId IS NULL OR cm.partyId = ?)"

		playerQueryArgs = append(playerQueryArgs, partyId)
	}

	playersQuery += ")"

	playerResults, err := db.Query(playersQuery, playerQueryArgs...)
	if err != nil {
		return chatHistory, err
	}

	defer playerResults.Close()

	for playerResults.Next() {
		chatPlayer := &ChatPlayer{}
		err := playerResults.Scan(&chatPlayer.Uuid, &chatPlayer.Name, &chatPlayer.Rank, &chatPlayer.Account, &chatPlayer.Badge, &chatPlayer.SystemName, &chatPlayer.Medals[0], &chatPlayer.Medals[1], &chatPlayer.Medals[2], &chatPlayer.Medals[3], &chatPlayer.Medals[4])
		if err != nil {
			return chatHistory, err
		}
		chatHistory.Players = append(chatHistory.Players, chatPlayer)
	}

	return chatHistory, nil
}

func archiveChatMessages() error {
	var threshold time.Time

	err := db.QueryRow("SELECT DATE_ADD(UTC_TIMESTAMP(), INTERVAL -1 DAY)").Scan(&threshold)
	if err != nil {
		return err
	}

	_, err = db.Exec("INSERT INTO chatMessagesArchive (msgId, game, uuid, contents, mapId, prevMapId, prevLocations, x, y, partyId, timestamp) (SELECT cm.msgId, cm.game, cm.uuid, cm.contents, cm.mapId, cm.prevMapId, cm.prevLocations, cm.x, cm.y, cm.partyId, cm.timestamp FROM chatMessages cm WHERE cm.timestamp < ?)", threshold)
	if err != nil {
		return err
	}

	_, err = db.Exec("DELETE FROM chatMessages WHERE timestamp < ?", threshold)
	if err != nil {
		return err
	}

	return nil
}

func setCurrentEventPeriodId() error {
	var periodId int

	err := db.QueryRow("SELECT id FROM eventPeriods WHERE UTC_DATE() >= startDate AND UTC_DATE() < endDate").Scan(&periodId)
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
	err = db.QueryRow("SELECT ep.periodOrdinal, ep.endDate, gep.enableVms FROM eventPeriods ep JOIN gameEventPeriods gep ON gep.periodId = ep.id AND gep.game = ? WHERE UTC_DATE() >= ep.startDate AND UTC_DATE() < ep.endDate", serverConfig.gameName).Scan(&eventPeriod.PeriodOrdinal, &eventPeriod.EndDate, &eventPeriod.EnableVms)
	if err != nil {
		eventPeriod.PeriodOrdinal = -1
		if err == sql.ErrNoRows {
			return eventPeriod, nil
		}
		return eventPeriod, err
	}

	return eventPeriod, nil
}

func getGameCurrentEventPeriodsData() (gameEventPeriods map[string]*EventPeriod, err error) {
	gameEventPeriods = make(map[string]*EventPeriod)

	results, err := db.Query("SELECT gep.id, ep.periodOrdinal, ep.endDate, gep.enableVms, gep.game FROM eventPeriods ep JOIN gameEventPeriods gep ON gep.periodId = ep.id WHERE UTC_DATE() >= ep.startDate AND UTC_DATE() < ep.endDate")
	if err != nil {
		return gameEventPeriods, err
	}

	defer results.Close()

	for results.Next() {
		var gameId string
		eventPeriod := &EventPeriod{}

		err = results.Scan(&eventPeriod.Id, &eventPeriod.PeriodOrdinal, &eventPeriod.EndDate, &eventPeriod.EnableVms, &gameId)
		if err != nil {
			return gameEventPeriods, err
		}

		gameEventPeriods[gameId] = eventPeriod
	}

	return gameEventPeriods, nil
}

func setCurrentGameEventPeriodId() error {
	var gamePeriodId int

	err := db.QueryRow("SELECT id FROM gameEventPeriods WHERE game = ? AND periodId = ?", serverConfig.gameName, currentEventPeriodId).Scan(&gamePeriodId)
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

func getRandomGameForEventLocation(pool map[string][]*EventLocationData, eventLocationCountThreshold int) (gameId string, err error) {
	results, err := db.Query("SELECT CEIL(AVG(gpc.playerCount)), gpc.game FROM gamePlayerCounts gpc JOIN gameEventPeriods gep ON gep.periodId = ? AND gep.game = gpc.game GROUP BY gpc.game", currentEventPeriodId)
	if err != nil {
		return "", err
	}

	defer results.Close()

	var playerCounts []int
	var gameIds []string
	totalPlayerCount := 0

	for results.Next() {
		var currentPlayerCount int
		var currentGameId string

		err = results.Scan(&currentPlayerCount, &currentGameId)
		if err != nil {
			return "", err
		}

		// Ignore games with no event locations in the current pool
		if eventLocations, ok := pool[currentGameId]; currentGameId != "2kki" && (!ok || len(eventLocations) < eventLocationCountThreshold) {
			continue
		}

		if currentPlayerCount == 0 {
			currentPlayerCount = 1
		}

		totalPlayerCount += currentPlayerCount

		playerCounts = append(playerCounts, currentPlayerCount)
		gameIds = append(gameIds, currentGameId)
	}

	avgPlayerCount := int(math.Ceil(float64(totalPlayerCount) / float64(len(gameIds))))

	var poolThresholds []int
	gamePool := make(map[int]string)
	totalPoolValue := 0

	poolCommonValue := int(math.Ceil(float64(avgPlayerCount) * gameEventShareFactor))

	for i, count := range playerCounts {
		poolValue := int(math.Floor(float64(count)*(1-gameEventShareFactor))) + poolCommonValue
		poolThreshold := poolValue + totalPoolValue

		gamePool[poolThreshold] = gameIds[i]
		poolThresholds = append(poolThresholds, poolThreshold)

		totalPoolValue += poolValue
	}

	rand.Seed(time.Now().Unix())
	randValue := rand.Intn(totalPoolValue)

	for _, value := range poolThresholds {
		if randValue < value {
			gameId = gamePool[value]
			break
		}
	}

	return gameId, nil
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
	err = db.QueryRow("SELECT SUM(exp) FROM ((SELECT COALESCE(SUM(ec.exp), 0) exp FROM eventCompletions ec JOIN eventLocations el ON el.id = ec.eventId AND ec.type = 0 JOIN gameEventPeriods gep ON gep.id = el.gamePeriodId JOIN eventPeriods ep ON ep.id = gep.periodId WHERE ec.uuid = ?) UNION ALL (SELECT COALESCE(SUM(ec.exp), 0) exp FROM eventCompletions ec JOIN eventVms ev ON ev.id = ec.eventId AND ec.type = 2 JOIN gameEventPeriods gep ON gep.id = ev.gamePeriodId JOIN eventPeriods ep ON ep.id = gep.periodId WHERE ec.uuid = ?)) eventExp", playerUuid, playerUuid).Scan(&totalEventExp)
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
	// Relies on rankings but is much faster than calculating directly
	err = db.QueryRow("SELECT FLOOR(valueFloat * 100) FROM rankingEntries WHERE uuid = ? AND categoryId = 'eventLocationCompletion' AND subCategoryId = 'all'", playerUuid).Scan(&eventLocationCompletion)
	if err != nil {
		if err == sql.ErrNoRows {
			return 0, nil
		}
		return eventLocationCompletion, err
	}

	return eventLocationCompletion, nil
}

func getOrWriteLocationIdForEventLocation(gameEventPeriodId int, title string, titleJP string, depth int, minDepth int, mapIds []string) (locationId int, err error) {
	mapIdsJson, err := json.Marshal(mapIds)
	if err != nil {
		return locationId, err
	}

	_, err = db.Exec("INSERT INTO gameLocations (game, title, titleJP, depth, minDepth, mapIds) VALUES (?, ?, ?, ?, ?, ?) ON DUPLICATE KEY UPDATE titleJP = titleJP, depth = depth, minDepth = minDepth, mapIds = mapIds", serverConfig.gameName, title, titleJP, depth, minDepth, mapIdsJson)
	if err != nil {
		return locationId, err
	}

	db.QueryRow("SELECT l.id FROM gameLocations l JOIN gameEventPeriods gep ON gep.game = l.game WHERE gep.id = ? AND l.title = ?", gameEventPeriodId, title).Scan(&locationId)

	return locationId, nil
}

func getOrWriteLocationIdForPlayerEventLocation(gameEventPeriodId int, playerUuid string, title string, titleJP string, depth int, minDepth int, mapIds []string) (locationId int, err error) {
	var playerEventLocationQueueLength int
	db.QueryRow("SELECT COUNT(*) FROM playerEventLocationQueue WHERE game = ? AND date = UTC_DATE()", serverConfig.gameName).Scan(&playerEventLocationQueueLength)

	if playerEventLocationQueueLength > 0 {
		var currentPlayerEventLocationQueueLength int
		db.QueryRow("SELECT COUNT(*) FROM eventCompletions ec JOIN playerEventLocations pel ON pel.id = ec.eventId AND ec.type = 1 WHERE pel.gamePeriodId = ? AND pel.startDate = UTC_DATE() AND pel.uuid = ?", gameEventPeriodId, playerUuid).Scan(&currentPlayerEventLocationQueueLength)

		if currentPlayerEventLocationQueueLength < playerEventLocationQueueLength {
			db.QueryRow("SELECT locationId FROM playerEventLocationQueue WHERE game = ? AND date = UTC_DATE() AND queueIndex = ?", serverConfig.gameName, currentPlayerEventLocationQueueLength+1).Scan(&locationId)

			return locationId, nil
		}
	}

	locationId, err = getOrWriteLocationIdForEventLocation(gameEventPeriodId, title, titleJP, depth, minDepth, mapIds)
	if err != nil {
		return locationId, err
	}

	_, err = db.Exec("INSERT INTO playerEventLocationQueue (game, date, queueIndex, locationId) VALUES (?, UTC_DATE(), ?, ?)", serverConfig.gameName, playerEventLocationQueueLength+1, locationId)
	if err != nil {
		return locationId, err
	}

	return locationId, nil
}

func writeEventLocationData(gameEventPeriodId int, eventType int, title string, titleJP string, depth int, minDepth int, exp int, mapIds []string) error {
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

	locationId, err := getOrWriteLocationIdForEventLocation(gameEventPeriodId, title, titleJP, depth, minDepth, mapIds)
	if err != nil {
		return err
	}

	_, err = db.Exec("INSERT INTO eventLocations (locationId, gamePeriodId, type, exp, startDate, endDate) VALUES (?, ?, ?, ?, DATE_SUB(UTC_DATE(), INTERVAL ? DAY), DATE_ADD(UTC_DATE(), INTERVAL ? DAY))", locationId, gameEventPeriodId, eventType, exp, offsetDays, days)
	if err != nil {
		return err
	}

	return nil
}

func writePlayerEventLocationData(gameEventPeriodId int, playerUuid string, title string, titleJP string, depth int, minDepth int, mapIds []string) error {
	locationId, err := getOrWriteLocationIdForPlayerEventLocation(gameEventPeriodId, playerUuid, title, titleJP, depth, minDepth, mapIds)
	if err != nil {
		return err
	}

	_, err = db.Exec("INSERT INTO playerEventLocations (locationId, gamePeriodId, uuid, startDate, endDate) SELECT ?, ?, ?, UTC_DATE(), DATE_ADD(UTC_DATE(), INTERVAL 1 DAY) WHERE NOT EXISTS(SELECT * FROM playerEventLocations pel LEFT JOIN eventCompletions ec ON ec.eventId = pel.id AND ec.type = 1 AND ec.uuid = pel.uuid WHERE pel.uuid = ? AND pel.gamePeriodId = ? AND ec.uuid IS NULL AND UTC_DATE() >= pel.startDate AND UTC_DATE() < pel.endDate)", locationId, gameEventPeriodId, playerUuid, playerUuid, gameEventPeriodId)
	if err != nil {
		return err
	}

	return nil
}

func getCurrentPlayerEventLocationsData(playerUuid string) (eventLocations []*EventLocation, err error) {
	results, err := db.Query("SELECT el.id, el.type, gep.game, l.title, l.titleJP, l.depth, l.minDepth, el.exp, el.endDate, CASE WHEN ec.uuid IS NOT NULL THEN 1 ELSE 0 END FROM eventLocations el JOIN gameLocations l ON l.id = el.locationId JOIN gameEventPeriods gep ON gep.id = el.gamePeriodId LEFT JOIN eventCompletions ec ON ec.eventId = el.id AND ec.type = 0 AND ec.uuid = ? WHERE gep.periodId = ? AND UTC_DATE() >= el.startDate AND UTC_DATE() < el.endDate ORDER BY 2, 1", playerUuid, currentEventPeriodId)
	if err != nil {
		return eventLocations, err
	}

	defer results.Close()

	for results.Next() {
		eventLocation := &EventLocation{}

		var completeBin int

		err := results.Scan(&eventLocation.Id, &eventLocation.Type, &eventLocation.Game, &eventLocation.Title, &eventLocation.TitleJP, &eventLocation.Depth, &eventLocation.MinDepth, &eventLocation.Exp, &eventLocation.EndDate, &completeBin)
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

	results, err = db.Query("SELECT pel.id, gep.game, pl.title, pl.titleJP, pl.depth, pl.minDepth, pel.endDate FROM playerEventLocations pel JOIN gameLocations pl ON pl.id = pel.locationId JOIN gameEventPeriods gep ON gep.id = pel.gamePeriodId LEFT JOIN eventCompletions ec ON ec.eventId = pel.id AND ec.type = 1 AND ec.uuid = pel.uuid WHERE pel.uuid = ? AND gep.periodId = ? AND gep.game = ? AND ec.uuid IS NULL AND UTC_DATE() >= pel.startDate AND UTC_DATE() < pel.endDate ORDER BY 1", playerUuid, currentEventPeriodId, serverConfig.gameName)
	if err != nil {
		return eventLocations, err
	}

	defer results.Close()

	for results.Next() {
		eventLocation := &EventLocation{}

		err := results.Scan(&eventLocation.Id, &eventLocation.Game, &eventLocation.Title, &eventLocation.TitleJP, &eventLocation.Depth, &eventLocation.MinDepth, &eventLocation.EndDate)
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
		if client.rClient == nil {
			return -1, err
		}

		results, err := db.Query("SELECT el.id, el.type, el.exp, l.mapIds FROM eventLocations el JOIN gameLocations l ON l.id = el.locationId WHERE el.gamePeriodId = ? AND l.title = ? AND UTC_DATE() >= el.startDate AND UTC_DATE() < el.endDate ORDER BY 2", currentGameEventPeriodId, location)
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
		if client.rClient == nil {
			return false, err
		}

		// HACK: workaround for strange race condition
		// it's possible for a player to disconnect before the query finishes, causing a nil ptr
		clientMapId := client.rClient.mapId

		results, err := db.Query("SELECT pel.id, pl.mapIds FROM playerEventLocations pel JOIN gameLocations pl ON pl.id = pel.locationId WHERE pel.gamePeriodId = ? AND pl.title = ? AND pel.uuid = ? AND UTC_DATE() >= pel.startDate AND UTC_DATE() < pel.endDate ORDER BY 2", currentGameEventPeriodId, location, playerUuid)
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
				if clientMapId != mapId {
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
	results, err := db.Query("SELECT ev.id, gep.game, ev.exp, ev.endDate, CASE WHEN ec.uuid IS NOT NULL THEN 1 ELSE 0 END FROM eventVms ev JOIN gameEventPeriods gep ON gep.id = ev.gamePeriodId LEFT JOIN eventCompletions ec ON ec.eventId = ev.id AND ec.type = 2 AND ec.uuid = ? WHERE gep.periodId = ? AND UTC_DATE() >= ev.startDate AND UTC_DATE() < ev.endDate ORDER BY 2, 1", playerUuid, currentEventPeriodId)
	if err != nil {
		return eventVms, err
	}

	defer results.Close()

	for results.Next() {
		eventVm := &EventVm{}

		var completeBin int

		err := results.Scan(&eventVm.Id, &eventVm.Game, &eventVm.Exp, &eventVm.EndDate, &completeBin)
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

func writeEventVmData(mapId int, eventId int, exp int) error {
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

	_, err := db.Exec("INSERT INTO eventVms (gamePeriodId, mapId, eventId, exp, startDate, endDate) VALUES (?, ?, ?, ?, DATE_SUB(UTC_DATE(), INTERVAL ? DAY), DATE_ADD(UTC_DATE(), INTERVAL ? DAY))", currentGameEventPeriodId, mapId, eventId, exp, offsetDays, days)
	if err != nil {
		return err
	}

	return nil
}

func tryCompleteEventVm(playerUuid string, mapId int, eventId int) (exp int, err error) {
	if client, ok := clients.Load(playerUuid); ok {
		if client.rClient == nil {
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

			if client.rClient.mapId != fmt.Sprintf("%04d", eventMapId) {
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

func writeGameBadges() error {
	_, err := db.Exec("TRUNCATE TABLE badges")
	if err != nil {
		return err
	}

	for badgeGame := range badges {
		for badgeId, badge := range badges[badgeGame] {
			if _, ok := badges[serverConfig.gameName]; ok {
				badgeUnlockPercentage := badgeUnlockPercentages[badgeId]
				_, err = db.Exec("INSERT INTO badges (badgeId, game, bp, hidden, percentUnlocked) VALUES (?, ?, ?, ?, ?)", badgeId, badgeGame, badge.Bp, badge.Hidden || badge.Dev, badgeUnlockPercentage)
				if err != nil {
					return err
				}
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

func unlockPlayerBadge(playerUuid string, badgeId string) error {
	_, err := db.Exec("INSERT INTO playerBadges (uuid, badgeId, timestampUnlocked) VALUES (?, ?, ?) ON DUPLICATE KEY UPDATE badgeId = badgeId", playerUuid, badgeId, time.Now())
	if err != nil {
		return err
	}

	badgeUnlockPercentages[badgeId], err = getBadgeUnlockPercentage(badgeId)
	if err != nil {
		return err
	}

	return nil
}

func removePlayerBadge(playerUuid string, badgeId string) error {
	var slotRow int
	var slotCol int

	err := db.QueryRow("SELECT slotRow, slotCol FROM playerBadges WHERE uuid = ? AND badgeId = ?", playerUuid, badgeId).Scan(&slotRow, &slotCol)
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

func getBadgeUnlockPercentage(badgeId string) (unlockPercentage float32, err error) {
	err = db.QueryRow("SELECT COALESCE(COUNT(b.uuid) / aa.count, 0) * 100 FROM playerBadges b JOIN accounts a ON a.uuid = b.uuid JOIN (SELECT COUNT(aa.uuid) count FROM accounts aa WHERE EXISTS(SELECT * FROM playerBadges aab WHERE aab.uuid = aa.uuid AND aa.inactive = 0)) aa WHERE EXISTS(SELECT * FROM playerBadges ab WHERE ab.uuid = a.uuid AND a.inactive = 0) AND b.badgeId = ?", badgeId).Scan(&unlockPercentage)

	return unlockPercentage, err
}

func getBadgeUnlockPercentages() (unlockPercentages map[string]float32, err error) {
	results, err := db.Query("SELECT b.badgeId, (COUNT(b.uuid) / aa.count) * 100 FROM playerBadges b JOIN accounts a ON a.uuid = b.uuid JOIN (SELECT COUNT(aa.uuid) count FROM accounts aa WHERE EXISTS(SELECT * FROM playerBadges aab WHERE aab.uuid = aa.uuid AND aa.inactive = 0)) aa WHERE EXISTS(SELECT * FROM playerBadges ab WHERE ab.uuid = a.uuid AND a.inactive = 0) GROUP BY b.badgeId")
	if err != nil {
		return unlockPercentages, err
	}

	defer results.Close()

	unlockPercentages = make(map[string]float32)

	for results.Next() {
		var badgeId string
		var percentUnlocked float32

		err := results.Scan(&badgeId, &percentUnlocked)
		if err != nil {
			return unlockPercentages, err
		}

		unlockPercentages[badgeId] = percentUnlocked
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
		if client.rClient == nil {
			return false, nil
		}

		// Spare SQL having to deal with a duplicate record by checking player tags beforehand
		var tagExists bool
		for _, tag := range client.rClient.tags {
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
		_, err = db.Exec("UPDATE playerMinigameScores SET score = ?, timestampCompleted = ? WHERE uuid = ? AND game = ? AND minigameId = ?", score, time.Now(), playerUuid, serverConfig.gameName, minigameId)
		if err != nil {
			return false, err
		}
		return true, nil
	}

	_, err = db.Exec("INSERT INTO playerMinigameScores (uuid, game, minigameId, score, timestampCompleted) VALUES (?, ?, ?, ?, ?)", playerUuid, serverConfig.gameName, minigameId, score, time.Now())
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

func getUuidFromName(name string) (uuid string, err error) {
	err = db.QueryRow("SELECT uuid FROM accounts WHERE user = ?", name).Scan(&uuid)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", nil
		}
		return "", err
	}

	return uuid, nil
}

func getNameFromUuid(uuid string) (name string) {
	// get name from sessionClients if they're connected
	if client, ok := clients.Load(uuid); ok {
		return client.name
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
	_, err := db.Exec("INSERT INTO gamePlayerCounts (game, playerCount) VALUES (?, ?)", serverConfig.gameName, playerCount)
	if err != nil {
		return err
	}

	var playerCounts int
	err = db.QueryRow("SELECT COUNT(*) FROM gamePlayerCounts WHERE game = ?", serverConfig.gameName).Scan(&playerCounts)
	if err != nil {
		return err
	}

	if playerCounts > 28 {
		_, err = db.Exec("DELETE FROM gamePlayerCounts WHERE game = ? ORDER BY id LIMIT ?", serverConfig.gameName, playerCounts-28)
		if err != nil {
			return err
		}
	}

	return nil
}

func doCleanupQueries() error {
	// Remove player records with no game activity
	_, err := db.Exec("DELETE FROM players WHERE ip IS NOT NULL AND uuid NOT IN (SELECT uuid FROM playerGameData) AND uuid NOT IN (SELECT uuid FROM partyMembers)")
	if err != nil {
		return err
	}

	// Remove player sessions that have expired
	_, err = db.Exec("DELETE FROM playerSessions WHERE expiration < NOW()")
	if err != nil {
		return err
	}

	// Remove player expeditions that were never completed
	_, err = db.Exec("DELETE pel FROM playerEventLocations pel WHERE UTC_DATE() > pel.endDate AND NOT EXISTS (SELECT ec.eventId FROM eventCompletions ec WHERE ec.eventId = pel.id AND ec.type = 1)")
	if err != nil {
		return err
	}

	// Remove player event location queue for past dates
	_, err = db.Exec("DELETE FROM playerEventLocationQueue WHERE UTC_DATE() > date")
	if err != nil {
		return err
	}

	// Remove Yume 2kki Explorer API query cache records that have expired
	_, err = db.Exec("DELETE FROM 2kkiApiQueries WHERE timestampExpired < CURRENT_TIMESTAMP()")
	if err != nil {
		return err
	}

	return nil
}
