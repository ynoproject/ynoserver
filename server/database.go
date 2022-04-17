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
	result := db.QueryRow("SELECT a.uuid, a.user, pd.rank, pd.banned FROM accounts a JOIN players pd ON pd.uuid = a.uuid WHERE a.session = ?", session)
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

func updatePlayerGameData(client *Client) error {
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
	results := db.QueryRow("SELECT a.uuid, a.user, pd.rank FROM accounts a JOIN players pd ON pd.uuid = a.uuid WHERE a.session = ?", session)
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

	results, err := db.Query("SELECT pm.partyId, pm.uuid, COALESCE(a.user, pgd.name), pd.rank, CASE WHEN a.user IS NULL THEN 0 ELSE 1 END, pgd.systemName, pgd.spriteName, pgd.spriteIndex FROM partyMembers pm JOIN playerGameData pgd ON pgd.uuid = pm.uuid JOIN players pd ON pd.uuid = pgd.uuid JOIN parties p ON p.id = pm.partyId LEFT JOIN accounts a ON a.uuid = pd.uuid WHERE pm.partyId IS NOT NULL AND pgd.game = ? ORDER BY CASE WHEN p.owner = pm.uuid THEN 0 ELSE 1 END, pd.rank DESC, pm.id", config.gameName)

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
	results, err := db.Query("SELECT pm.partyId, pm.uuid, COALESCE(a.user, pgd.name), pd.rank, CASE WHEN a.user IS NULL THEN 0 ELSE 1 END, pgd.systemName, pgd.spriteName, pgd.spriteIndex FROM partyMembers pm JOIN playerGameData pgd ON pgd.uuid = pm.uuid JOIN players pd ON pd.uuid = pgd.uuid JOIN parties p ON p.id = pm.partyId LEFT JOIN accounts a ON a.uuid = pd.uuid WHERE pm.partyId = ? AND pgd.game = ? ORDER BY CASE WHEN p.owner = pm.uuid THEN 0 ELSE 1 END, pd.rank DESC, pm.id", partyId, config.gameName)
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

func readPlayerEventExpData(periodId int, playerUuid string) (eventExp EventExp, err error) {
	result := db.QueryRow("SELECT COALESCE(SUM(ec.exp), 0) FROM eventCompletions ec JOIN eventLocations el ON el.id = ec.eventId JOIN eventPeriods ep ON ep.id = el.periodId WHERE ep.game = ? AND ec.uuid = ? AND ec.playerEvent = 0", config.gameName, playerUuid)
	err = result.Scan(&eventExp.TotalExp)

	if err != nil {
		return eventExp, err
	}

	result = db.QueryRow("SELECT COALESCE(SUM(ec.exp), 0) FROM eventCompletions ec JOIN eventLocations el ON el.id = ec.eventId JOIN eventPeriods ep ON ep.id = el.periodId WHERE ep.id = ? AND ec.uuid = ? AND ec.playerEvent = 0", periodId, playerUuid)
	err = result.Scan(&eventExp.PeriodExp)

	if err != nil {
		return eventExp, err
	}

	weekEventExp, err := readWeekEventExp(periodId, playerUuid)
	if err != nil {
		return eventExp, err
	}

	eventExp.WeekExp = weekEventExp

	return eventExp, nil
}

func readWeekEventExp(periodId int, playerUuid string) (weekEventExp int, err error) {
	weekdayIndex := int(time.Now().UTC().Weekday())

	result := db.QueryRow("SELECT COALESCE(SUM(ec.exp), 0) FROM eventCompletions ec JOIN eventLocations el ON el.id = ec.eventId JOIN eventPeriods ep ON ep.id = el.periodId WHERE ep.id = ? AND ec.uuid = ? AND ec.playerEvent = 0 AND DATE_SUB(UTC_DATE(), INTERVAL ? DAY) <= el.startDate AND DATE_ADD(UTC_DATE(), INTERVAL ? DAY) >= el.endDate", periodId, playerUuid, weekdayIndex, 7-weekdayIndex)
	err = result.Scan(&weekEventExp)

	if err != nil {
		return weekEventExp, err
	}

	return weekEventExp, nil
}

func writeEventLocationData(periodId int, eventType int, title string, titleJP string, depth int, exp int, mapIds []string) (err error) {
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

	_, err = db.Exec("INSERT INTO eventLocations (periodId, type, title, titleJP, depth, exp, startDate, endDate, mapIds) VALUES (?, ?, ?, ?, ?, ?, DATE_SUB(UTC_DATE(), INTERVAL ? DAY), DATE_ADD(UTC_DATE(), INTERVAL ? DAY), ?)", periodId, eventType, title, titleJP, depth, exp, offsetDays, days, mapIdsJson)
	if err != nil {
		return err
	}

	return nil
}

func writePlayerEventLocationData(periodId int, playerUuid string, title string, titleJP string, depth int, mapIds []string) (err error) {
	mapIdsJson, err := json.Marshal(mapIds)
	if err != nil {
		return err
	}

	_, err = db.Exec("INSERT INTO playerEventLocations (periodId, uuid, title, titleJP, depth, startDate, endDate, mapIds) VALUES (?, ?, ?, ?, ?, UTC_DATE(), DATE_ADD(UTC_DATE(), INTERVAL 1 DAY), ?)", periodId, playerUuid, title, titleJP, depth, mapIdsJson)
	if err != nil {
		return err
	}

	return nil
}

func readCurrentPlayerEventLocationsData(periodId int, playerUuid string) (eventLocations []*EventLocation, err error) {
	results, err := db.Query("SELECT el.id, el.type, el.title, el.titleJP, el.depth, el.exp, el.endDate, CASE WHEN ec.uuid IS NOT NULL THEN 1 ELSE 0 END FROM eventLocations el LEFT JOIN eventCompletions ec ON ec.eventId = el.id AND ec.uuid = ? WHERE el.periodId = ? AND UTC_DATE() >= el.startDate AND UTC_DATE() < el.endDate ORDER BY 2, 1", playerUuid, periodId)

	if err != nil {
		return eventLocations, err
	}

	defer results.Close()

	for results.Next() {
		eventLocation := &EventLocation{}

		var completeBin int

		err := results.Scan(&eventLocation.Id, &eventLocation.Type, &eventLocation.Title, &eventLocation.TitleJP, &eventLocation.Depth, &eventLocation.Exp, &eventLocation.EndDate, &completeBin)
		if err != nil {
			return eventLocations, err
		}

		if completeBin == 1 {
			eventLocation.Complete = true
		}

		eventLocations = append(eventLocations, eventLocation)
	}

	results, err = db.Query("SELECT pel.id, pel.title, pel.titleJP, pel.depth, pel.endDate FROM playerEventLocations pel LEFT JOIN eventCompletions ec ON ec.eventId = pel.id AND ec.uuid = ? WHERE pel.periodId = ? AND pel.uuid = ? AND ec.uuid IS NULL AND UTC_DATE() >= pel.startDate AND UTC_DATE() < pel.endDate ORDER BY 1", playerUuid, periodId, playerUuid)

	for results.Next() {
		eventLocation := &EventLocation{}

		err := results.Scan(&eventLocation.Id, &eventLocation.Title, &eventLocation.TitleJP, &eventLocation.Depth, &eventLocation.EndDate)
		if err != nil {
			return eventLocations, err
		}

		eventLocation.Type = -1

		eventLocations = append(eventLocations, eventLocation)
	}

	return eventLocations, nil
}

func tryCompleteEventLocation(periodId int, playerUuid string, location string) (exp int, err error) {
	if client, ok := allClients[playerUuid]; ok {
		clientMapId := client.mapId

		results, err := db.Query("SELECT el.id, el.type, el.exp, el.mapIds FROM eventLocations el WHERE el.periodId = ? AND el.title = ? AND UTC_DATE() >= el.startDate AND UTC_DATE() < el.endDate ORDER BY 2", periodId, location)

		if err != nil {
			return -1, err
		}

		weekEventExp, err := readWeekEventExp(periodId, playerUuid)
		if err != nil {
			return -1, err
		}

		defer results.Close()

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

			for _, mapId := range mapIds {
				if clientMapId == mapId {
					if weekEventExp >= 20 {
						eventExp = 0
					} else if weekEventExp+eventExp > 20 {
						eventExp = 20 - weekEventExp
					}

					_, err = db.Exec("INSERT INTO eventCompletions (eventId, uuid, playerEvent, timestampCompleted, exp) VALUES (?, ?, 0, ?, ?)", eventId, playerUuid, time.Now(), eventExp)
					if err != nil {
						break
					}

					exp += eventExp
					weekEventExp += eventExp
					break
				}
			}
		}

		return exp, nil
	}

	return -1, err
}

func tryCompletePlayerEventLocation(periodId int, playerUuid string, location string) (complete bool, err error) {
	if client, ok := allClients[playerUuid]; ok {
		clientMapId := client.mapId

		results, err := db.Query("SELECT pel.id, pel.mapIds FROM playerEventLocations pel WHERE pel.periodId = ? AND pel.title = ? AND pel.uuid = ? AND UTC_DATE() >= pel.startDate AND UTC_DATE() < pel.endDate ORDER BY 2", periodId, location, playerUuid)

		if err != nil {
			return false, err
		}

		success := false

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

			for _, mapId := range mapIds {
				if clientMapId == mapId {
					_, err = db.Exec("INSERT INTO eventCompletions (eventId, uuid, playerEvent, timestampCompleted, exp) VALUES (?, ?, 1, ?, 0)", eventId, playerUuid, time.Now())
					if err != nil {
						break
					}

					success = true
					break
				}
			}
		}

		return success, nil
	}

	return false, err
}
