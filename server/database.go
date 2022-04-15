package server

import (
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/thanhpk/randstr"
)

func getDatabaseHandle() *sql.DB {
	db, err := sql.Open("mysql", config.dbUser+":"+config.dbPass+"@tcp("+config.dbHost+")/"+config.dbName+"?parseTime=true")
	if err != nil {
		return nil
	}

	return db
}

func readPlayerData(ip string) (uuid string, rank int, banned bool) {
	results := db.QueryRow("SELECT uuid, rank, banned FROM players WHERE ip = ?", ip)
	err := results.Scan(&uuid, &rank, &banned)

	if err != nil {
		if err == sql.ErrNoRows {
			uuid = randstr.String(16)
			banned, _ := isVpn(ip)
			createPlayerData(ip, uuid, 0, banned)
		} else {
			return "", 0, false
		}
	}

	return uuid, rank, banned
}

func readPlayerDataFromSession(session string) (uuid string, name string, rank int, banned bool) {
	result := db.QueryRow("SELECT ad.uuid, ad.user, pd.rank, pd.banned FROM accounts ad JOIN players pd ON pd.uuid = ad.uuid WHERE ad.session = ?", session)
	err := result.Scan(&uuid, &name, &rank, &banned)

	if err != nil {
		return "", "", 0, false
	}

	return uuid, name, rank, banned
}

func readPlayerRank(uuid string) (rank int) {
	results := db.QueryRow("SELECT rank FROM players WHERE uuid = ?", uuid)
	err := results.Scan(&rank)
	if err != nil {
		return 0
	}

	return rank
}

func tryBanPlayer(senderUUID string, recipientUUID string) error { //called by api only
	if readPlayerRank(senderUUID) <= readPlayerRank(recipientUUID) {
		return errors.New("insufficient rank")
	}

	if senderUUID == recipientUUID {
		return errors.New("attempted self-ban")
	}

	_, err := db.Exec("UPDATE players SET banned = true WHERE uuid = ?", recipientUUID)
	if err != nil {
		return err
	}

	return nil
}

func createPlayerData(ip string, uuid string, rank int, banned bool) error {
	_, err := db.Exec("INSERT INTO players (ip, uuid, rank, banned) VALUES (?, ?, ?, ?) ON DUPLICATE KEY UPDATE uuid = ?, rank = ?, banned = ?", ip, uuid, rank, banned, uuid, rank, banned)
	if err != nil {
		return err
	}

	return nil
}

func updatePlayerData(client *Client) error {
	_, err := db.Exec("INSERT INTO playerGameData (uuid, game, name, systemName, spriteName, spriteIndex) VALUES (?, ?, ?, ?, ?, ?) ON DUPLICATE KEY UPDATE name = ?, systemName = ?, spriteName = ?, spriteIndex = ?", client.uuid, config.gameName, client.name, client.systemName, client.spriteName, client.spriteIndex, client.name, client.systemName, client.spriteName, client.spriteIndex)
	if err != nil {
		return err
	}

	return nil
}

func readPlayerInfo(ip string) (uuid string, name string, rank int) {
	results := db.QueryRow("SELECT pd.uuid, pgd.name, pd.rank FROM players pd JOIN playerGameData pgd ON pgd.uuid = pd.uuid WHERE pd.ip = ? AND pgd.game = ?", ip, config.gameName)
	err := results.Scan(&uuid, &name, &rank)

	if err != nil {
		return "", "", 0
	}

	return uuid, name, rank
}

func readPlayerInfoFromSession(session string) (uuid string, name string, rank int) {
	results := db.QueryRow("SELECT ad.uuid, ad.user, pd.rank FROM accounts ad JOIN players pd ON pd.uuid = ad.uuid JOIN playerGameData pgd ON pgd.uuid = pd.uuid WHERE ad.session = ? AND pgd.game = ?", session, config.gameName)
	err := results.Scan(&uuid, &name, &rank)

	if err != nil {
		return "", "", 0
	}

	return uuid, name, rank
}

func readPlayerPartyId(uuid string) (partyId int, err error) {
	results := db.QueryRow("SELECT pm.partyId FROM partyMembers pm JOIN parties p ON p.id = pm.partyId WHERE pm.uuid = ? AND p.game = ?", uuid, config.gameName)
	err = results.Scan(&partyId)

	if err != nil {
		if err == sql.ErrNoRows {
			return 0, nil
		} else {
			return 0, err
		}
	}

	return partyId, nil
}

func readAllPartyData() (parties []*Party, err error) { //called by api only
	partyMembersByParty, err := readAllPartyMemberDataByParty()
	if err != nil {
		return parties, err
	}

	results, err := db.Query("SELECT p.id, p.owner, p.name, p.public, p.theme, p.description FROM parties p WHERE p.game = ?", config.gameName)

	if err != nil {
		return parties, err
	}

	defer results.Close()

	for results.Next() {
		party := &Party{}
		err := results.Scan(&party.Id, &party.OwnerUuid, &party.Name, &party.Public, &party.SystemName, &party.Description)
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
			parties = append(parties, party)
		}
	}

	return parties, nil
}

func readAllPartyMemberDataByParty() (partyMembersByParty map[int][]*PartyMember, err error) {
	partyMembersByParty = make(map[int][]*PartyMember)

	results, err := db.Query("SELECT pm.partyId, pm.uuid, COALESCE(ad.user, pgd.name), pd.rank, CASE WHEN ad.user IS NULL THEN 0 ELSE 1 END, pgd.systemName, pgd.spriteName, pgd.spriteIndex FROM partyMembers pm JOIN playerGameData pgd ON pgd.uuid = pm.uuid JOIN players pd ON pd.uuid = pgd.uuid JOIN parties p ON p.id = pm.partyId LEFT JOIN accounts ad ON ad.uuid = pd.uuid WHERE pm.partyId IS NOT NULL AND pgd.game = ? ORDER BY CASE WHEN p.owner = pm.uuid THEN 0 ELSE 1 END, pd.rank DESC, pm.id", config.gameName)

	if err != nil {
		return partyMembersByParty, err
	}

	defer results.Close()

	var offlinePartyMembersByParty map[int][]*PartyMember = make(map[int][]*PartyMember)

	for results.Next() {
		var partyId int
		var accountBin int
		partyMember := &PartyMember{}
		err := results.Scan(&partyId, &partyMember.Uuid, &partyMember.Name, &partyMember.Rank, &accountBin, &partyMember.SystemName, &partyMember.SpriteName, &partyMember.SpriteIndex)
		if err != nil {
			return partyMembersByParty, err
		}
		partyMember.Account = accountBin == 1

		if client, ok := allClients[partyMember.Uuid]; ok {
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
			partyMember.Online = true
			partyMembersByParty[partyId] = append(partyMembersByParty[partyId], partyMember)
		} else {
			offlinePartyMembersByParty[partyId] = append(offlinePartyMembersByParty[partyId], partyMember)
		}
	}

	for partyId, offlinePartyMembers := range offlinePartyMembersByParty {
		partyMembersByParty[partyId] = append(partyMembersByParty[partyId], offlinePartyMembers...)
	}

	return partyMembersByParty, nil
}

func readPartyData(playerUuid string) (party Party, err error) { //called by api only
	results := db.QueryRow("SELECT p.id, p.owner, p.name, p.public, p.pass, p.theme, p.description FROM parties p JOIN partyMembers pm ON pm.partyId = p.id JOIN playerGameData pgd ON pgd.uuid = pm.uuid AND pgd.game = p.game WHERE p.game = ? AND pm.uuid = ?", config.gameName, playerUuid)
	err = results.Scan(&party.Id, &party.OwnerUuid, &party.Name, &party.Public, &party.Pass, &party.SystemName, &party.Description)
	if err != nil {
		return party, err
	}

	partyMembers, err := readPartyMemberData(party.Id)
	if err != nil {
		return party, err
	}

	for _, partyMember := range partyMembers {
		party.Members = append(party.Members, *partyMember)
	}

	return party, nil
}

func readPartyDescription(partyId int) (description string, err error) { //called by api only
	results := db.QueryRow("SELECT description FROM parties WHERE id = ?", partyId)
	err = results.Scan(&description)
	if err != nil {
		return description, err
	}

	return description, nil
}

func readPartyPublic(partyId int) (public bool, err error) { //called by api only
	results := db.QueryRow("SELECT public FROM parties WHERE id = ?", partyId)
	err = results.Scan(&public)
	if err != nil {
		return public, err
	}

	return public, nil
}

func readPartyPass(partyId int) (pass string, err error) { //called by api only
	results := db.QueryRow("SELECT pass FROM parties WHERE id = ?", partyId)
	err = results.Scan(&pass)
	if err != nil {
		return pass, err
	}

	return pass, nil
}

func readPartyMemberData(partyId int) (partyMembers []*PartyMember, err error) {
	results, err := db.Query("SELECT pm.partyId, pm.uuid, COALESCE(ad.user, pgd.name), pd.rank, CASE WHEN ad.user IS NULL THEN 0 ELSE 1 END, pgd.systemName, pgd.spriteName, pgd.spriteIndex FROM partyMembers pm JOIN playerGameData pgd ON pgd.uuid = pm.uuid JOIN players pd ON pd.uuid = pgd.uuid JOIN parties p ON p.id = pm.partyId LEFT JOIN accounts ad ON ad.uuid = pd.uuid WHERE pm.partyId = ? AND pgd.game = ? ORDER BY CASE WHEN p.owner = pm.uuid THEN 0 ELSE 1 END, pd.rank DESC, pm.id", partyId, config.gameName)
	if err != nil {
		return partyMembers, err
	}

	defer results.Close()

	for results.Next() {
		var partyId int
		var accountBin int
		partyMember := &PartyMember{}
		err := results.Scan(&partyId, &partyMember.Uuid, &partyMember.Name, &partyMember.Rank, &accountBin, &partyMember.SystemName, &partyMember.SpriteName, &partyMember.SpriteIndex)
		if err != nil {
			return partyMembers, err
		}
		partyMember.Account = accountBin == 1
		if client, ok := allClients[partyMember.Uuid]; ok {
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
			partyMember.MapId = client.mapId
			partyMember.PrevMapId = client.prevMapId
			partyMember.PrevLocations = client.prevLocations
			partyMember.Online = true
		} else {
			partyMember.MapId = "0000"
			partyMember.PrevMapId = "0000"
		}
		partyMembers = append(partyMembers, partyMember)
	}

	return partyMembers, nil
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

func readPartyMemberUuids(partyId int) (partyMemberUuids []string, err error) {
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

func readPartyMemberCount(partyId int) (count int, err error) {
	results := db.QueryRow("SELECT COUNT(*) FROM partyMembers WHERE partyId = ?", partyId)
	err = results.Scan(&count)

	if err != nil {
		return count, err
	}

	return count, nil
}

func readPartyOwnerUuid(partyId int) (ownerUuid string, err error) {
	results := db.QueryRow("SELECT owner FROM parties WHERE id = ?", partyId)
	err = results.Scan(&ownerUuid)

	if err != nil {
		return ownerUuid, err
	}

	return ownerUuid, nil
}

func assumeNextPartyOwner(partyId int) error {
	partyMemberUuids, err := readPartyMemberUuids(partyId)
	if err != nil {
		return err
	}

	var nextOnlinePlayerUuid string

	for _, uuid := range partyMemberUuids {
		if _, ok := allClients[uuid]; ok {
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
	var partyMemberCount int
	partyMemberCount, err = readPartyMemberCount(partyId)
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

func readSaveDataTimestamp(playerUuid string) (timestamp time.Time, err error) { //called by api only
	result := db.QueryRow("SELECT timestamp FROM gameSaves WHERE uuid = ? AND game = ?", playerUuid, config.gameName)

	if err != nil {
		return timestamp, err
	}

	err = result.Scan(&timestamp)
	if err != nil {
		return timestamp, err
	}

	return timestamp, nil
}

func readSaveData(playerUuid string) (saveData string, err error) { //called by api only
	result := db.QueryRow("SELECT data FROM gameSaves WHERE uuid = ? AND game = ?", playerUuid, config.gameName)

	if err != nil {
		return saveData, err
	}

	err = result.Scan(&saveData)
	if err != nil {
		return saveData, err
	}

	return saveData, nil
}

func createGameSaveData(playerUuid string, timestamp time.Time, data string) (err error) { //called by api only
	_, err = db.Exec("INSERT INTO gameSaves (uuid, game, timestamp, data) VALUES (?, ?, ?, ?) ON DUPLICATE KEY UPDATE timestamp = ?, data = ?", playerUuid, config.gameName, timestamp, data, timestamp, data)
	if err != nil {
		return err
	}

	return nil
}

func readCurrentEventPeriodId() (periodId int, err error) {
	result := db.QueryRow("SELECT id FROM eventPeriods WHERE game = ? AND UTC_DATE() >= startDate AND UTC_DATE() < endDate", config.gameName)
	err = result.Scan(&periodId)

	if err != nil {
		if err == sql.ErrNoRows {
			return 0, nil
		}
		return 0, err
	}

	return periodId, nil
}

func readCurrentEventPeriodData() (eventPeriod EventPeriod, err error) {
	result := db.QueryRow("SELECT periodOrdinal, endDate FROM eventPeriods WHERE game = ? AND UTC_DATE() >= startDate AND UTC_DATE() < endDate", config.gameName)

	err = result.Scan(&eventPeriod.PeriodOrdinal, &eventPeriod.EndDate)

	if err != nil {
		eventPeriod.PeriodOrdinal = -1
		if err == sql.ErrNoRows {
			return eventPeriod, nil
		}
		return eventPeriod, err
	}

	return eventPeriod, nil
}

func readEventPointsData(periodId int, playerUuid string) (eventPoints EventPoints, err error) {
	result := db.QueryRow("SELECT COUNT(ecd.eventId) FROM eventCompletions ecd JOIN eventLocations ed ON ed.id = ecd.eventId JOIN eventPeriods epd ON epd.id = ed.periodId WHERE epd.game = ? AND ecd.uuid = ?", config.gameName, playerUuid)
	err = result.Scan(&eventPoints.TotalPoints)

	if err != nil {
		return eventPoints, err
	}

	result = db.QueryRow("SELECT COUNT(ecd.eventId) FROM eventCompletions ecd JOIN eventLocations ed ON ed.id = ecd.eventId JOIN eventPeriods epd ON epd.id = ed.periodId WHERE epd.id = ? AND ecd.uuid = ?", periodId, playerUuid)
	err = result.Scan(&eventPoints.PeriodPoints)

	if err != nil {
		return eventPoints, err
	}

	weekdayIndex := int(time.Now().UTC().Weekday())

	result = db.QueryRow("SELECT COUNT(ecd.eventId) FROM eventCompletions ecd JOIN eventLocations ed ON ed.id = ecd.eventId JOIN eventPeriods epd ON epd.id = ed.periodId WHERE epd.id = ? AND ecd.uuid = ? AND DATE_SUB(UTC_DATE(), INTERVAL ? DAY) <= ed.startDate AND DATE_ADD(UTC_DATE(), INTERVAL ? DAY) >= ed.endDate", periodId, playerUuid, weekdayIndex, 7-weekdayIndex)
	err = result.Scan(&eventPoints.WeekPoints)

	if err != nil {
		return eventPoints, err
	}

	return eventPoints, nil
}

func writeEventLocationData(periodId int, eventType int, title string, titleJP string, depth int, mapIds []string) (err error) {
	mapIdsJson, err := json.Marshal(mapIds)
	if err != nil {
		return err
	}

	var days int
	offsetDays := 0
	weekday := time.Now().UTC().Weekday()
	if eventType == 0 {
		days = 1
	} else if eventType == 1 {
		days = 7
		offsetDays = int(weekday)
	} else {
		if weekday == time.Friday || weekday == time.Saturday {
			days = 2
			offsetDays = int(weekday) - int(time.Friday)
		} else {
			return nil
		}
	}

	days -= offsetDays

	_, err = db.Exec("INSERT INTO eventLocations (periodId, type, title, titleJP, depth, startDate, endDate, mapIds) VALUES (?, ?, ?, ?, ?, DATE_SUB(UTC_DATE(), INTERVAL ? DAY), DATE_ADD(UTC_DATE(), INTERVAL ? DAY), ?)", periodId, eventType, title, titleJP, depth, offsetDays, days, mapIdsJson)
	if err != nil {
		return err
	}

	return nil
}

func readCurrentPlayerEventLocationsData(periodId int, playerUuid string) (eventLocations []*EventLocation, err error) {
	results, err := db.Query("SELECT ed.id, ed.type, ed.title, ed.titleJP, ed.depth, ed.endDate, CASE WHEN ecd.uuid IS NOT NULL THEN 1 ELSE 0 END FROM eventLocations ed LEFT JOIN eventCompletions ecd ON ecd.eventId = ed.id AND ecd.uuid = ? WHERE ed.periodId = ? AND UTC_DATE() >= ed.startDate AND UTC_DATE() < ed.endDate ORDER BY 2, 1", playerUuid, periodId)

	if err != nil {
		return eventLocations, err
	}

	defer results.Close()

	for results.Next() {
		eventLocation := &EventLocation{}

		var completeBin int

		err := results.Scan(&eventLocation.Id, &eventLocation.Type, &eventLocation.Title, &eventLocation.TitleJP, &eventLocation.Depth, &eventLocation.EndDate, &completeBin)
		if err != nil {
			return eventLocations, err
		}

		if completeBin == 1 {
			eventLocation.Complete = true
		}

		eventLocations = append(eventLocations, eventLocation)
	}

	return eventLocations, nil
}

func tryCompleteEventLocation(periodId int, playerUuid string, location string) (ep int, err error) {
	if client, ok := allClients[playerUuid]; ok {
		clientMapId := client.mapId

		results, err := db.Query("SELECT ed.id, ed.type, ed.mapIds FROM eventLocations ed WHERE ed.periodId = ? AND ed.title = ? AND UTC_DATE() >= ed.startDate AND UTC_DATE() < ed.endDate ORDER BY 2", periodId, location)

		if err != nil {
			return 0, err
		}

		defer results.Close()

		for results.Next() {
			var eventId string
			var eventType int
			var mapIdsJson string

			err := results.Scan(&eventId, &eventType, &mapIdsJson)
			if err != nil {
				return ep, err
			}

			var mapIds []string
			err = json.Unmarshal([]byte(mapIdsJson), &mapIds)

			for _, mapId := range mapIds {
				if clientMapId == mapId {

					_, err = db.Exec("INSERT INTO eventCompletions (eventId, uuid, timestampCompleted) VALUES (?, ?, ?)", eventId, playerUuid, time.Now())
					if err != nil {
						break
					}

					if eventType == 1 {
						ep += 5
					} else if eventType == 2 {
						ep += 3
					} else {
						ep++
					}
					break
				}
			}
		}

		return ep, nil
	}

	return 0, err
}
