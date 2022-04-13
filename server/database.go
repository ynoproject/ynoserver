package server

import (
	"database/sql"
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
	results := db.QueryRow("SELECT uuid, rank, banned FROM playerdata WHERE ip = ?", ip)
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
	result := db.QueryRow("SELECT ad.uuid, ad.user, pd.rank, pd.banned FROM accountdata ad JOIN playerdata pd ON pd.uuid = ad.uuid WHERE ad.session = ?", session)
	err := result.Scan(&uuid, &name, &rank, &banned)

	if err != nil {
		return "", "", 0, false
	}

	return uuid, name, rank, banned
}

func readPlayerRank(uuid string) (rank int) {
	results := db.QueryRow("SELECT rank FROM playerdata WHERE uuid = ?", uuid)
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

	_, err := db.Exec("UPDATE playerdata SET banned = true WHERE uuid = ?", recipientUUID)
	if err != nil {
		return err
	}

	return nil
}

func createPlayerData(ip string, uuid string, rank int, banned bool) error {
	_, err := db.Exec("INSERT INTO playerdata (ip, uuid, rank, banned) VALUES (?, ?, ?, ?) ON DUPLICATE KEY UPDATE uuid = ?, rank = ?, banned = ?", ip, uuid, rank, banned, uuid, rank, banned)
	if err != nil {
		return err
	}

	return nil
}

func updatePlayerData(client *Client) error {
	_, err := db.Exec("INSERT INTO playergamedata (uuid, game, name, systemName, spriteName, spriteIndex) VALUES (?, ?, ?, ?, ?, ?) ON DUPLICATE KEY UPDATE name = ?, systemName = ?, spriteName = ?, spriteIndex = ?", client.uuid, config.gameName, client.name, client.systemName, client.spriteName, client.spriteIndex, client.name, client.systemName, client.spriteName, client.spriteIndex)
	if err != nil {
		return err
	}

	return nil
}

func readPlayerInfo(ip string) (uuid string, name string, rank int) {
	results := db.QueryRow("SELECT pd.uuid, pgd.name, pd.rank FROM playerdata pd JOIN playergamedata pgd ON pgd.uuid = pd.uuid WHERE pd.ip = ? AND pgd.game = ?", ip, config.gameName)
	err := results.Scan(&uuid, &name, &rank)

	if err != nil {
		return "", "", 0
	}

	return uuid, name, rank
}

func readPlayerInfoFromSession(session string) (uuid string, name string, rank int) {
	results := db.QueryRow("SELECT ad.uuid, ad.user, pd.rank FROM accountdata ad JOIN playerdata pd ON pd.uuid = ad.uuid JOIN playergamedata pgd ON pgd.uuid = pd.uuid WHERE ad.session = ? AND pgd.game = ?", session, config.gameName)
	err := results.Scan(&uuid, &name, &rank)

	if err != nil {
		return "", "", 0
	}

	return uuid, name, rank
}

func readPlayerPartyId(uuid string) (partyId int, err error) {
	results := db.QueryRow("SELECT pm.partyId FROM partymemberdata pm JOIN partydata p ON p.id = pm.partyId WHERE pm.uuid = ? AND p.game = ?", uuid, config.gameName)
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

func readAllPartyData(playerUuid string) (parties []*Party, err error) { //called by api only
	partyMembersByParty, err := readAllPartyMemberDataByParty(playerUuid)
	if err != nil {
		return parties, err
	}

	results, err := db.Query("SELECT p.id, p.owner, p.name, p.public, p.theme, p.description FROM partydata p WHERE p.game = ?", config.gameName)

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

func readAllPartyMemberDataByParty(playerUuid string) (partyMembersByParty map[int][]*PartyMember, err error) {
	partyMembersByParty = make(map[int][]*PartyMember)

	results, err := db.Query("SELECT pm.partyId, pm.uuid, COALESCE(ad.user, pgd.name), pd.rank, CASE WHEN ad.user IS NULL THEN 0 ELSE 1 END, pgd.systemName, pgd.spriteName, pgd.spriteIndex FROM partymemberdata pm JOIN playergamedata pgd ON pgd.uuid = pm.uuid JOIN playerdata pd ON pd.uuid = pgd.uuid JOIN partydata p ON p.id = pm.partyId LEFT JOIN accountdata ad ON ad.uuid = pd.uuid WHERE pm.partyId IS NOT NULL AND pgd.game = ? ORDER BY CASE WHEN p.owner = pm.uuid THEN 0 ELSE 1 END, pd.rank DESC, pm.id", config.gameName)

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

func readPartyData(partyId int, playerUuid string) (party Party, err error) { //called by api only
	results := db.QueryRow("SELECT p.id, p.owner, p.name, p.public, p.pass, p.theme, p.description FROM partydata p JOIN partymemberdata pm ON pm.partyId = p.id JOIN playergamedata pgd ON pgd.uuid = pm.uuid AND pgd.game = p.game WHERE p.game = ? AND pm.uuid = ?", config.gameName, playerUuid)
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
	results := db.QueryRow("SELECT description FROM partydata WHERE id = ?", partyId)
	err = results.Scan(&description)
	if err != nil {
		return description, err
	}

	return description, nil
}

func readPartyPublic(partyId int) (public bool, err error) { //called by api only
	results := db.QueryRow("SELECT public FROM partydata WHERE id = ?", partyId)
	err = results.Scan(&public)
	if err != nil {
		return public, err
	}

	return public, nil
}

func readPartyPass(partyId int) (pass string, err error) { //called by api only
	results := db.QueryRow("SELECT pass FROM partydata WHERE id = ?", partyId)
	err = results.Scan(&pass)
	if err != nil {
		return pass, err
	}

	return pass, nil
}

func readPartyMemberData(partyId int) (partyMembers []*PartyMember, err error) {
	results, err := db.Query("SELECT pm.partyId, pm.uuid, COALESCE(ad.user, pgd.name), pd.rank, CASE WHEN ad.user IS NULL THEN 0 ELSE 1 END, pgd.systemName, pgd.spriteName, pgd.spriteIndex FROM partymemberdata pm JOIN playergamedata pgd ON pgd.uuid = pm.uuid JOIN playerdata pd ON pd.uuid = pgd.uuid JOIN partydata p ON p.id = pm.partyId LEFT JOIN accountdata ad ON ad.uuid = pd.uuid WHERE pm.partyId = ? AND pgd.game = ? ORDER BY CASE WHEN p.owner = pm.uuid THEN 0 ELSE 1 END, pd.rank DESC, pm.id", partyId, config.gameName)
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
	res, err := db.Exec("INSERT INTO partydata (game, owner, name, public, pass, theme, description) VALUES (?, ?, ?, ?, ?, ?, ?)", config.gameName, playerUuid, name, public, pass, theme, description)
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
	_, err = db.Exec("UPDATE partydata SET game = ?, owner = ?, name = ?, public = ?, pass = ?, theme = ?, description = ? WHERE id = ?", config.gameName, playerUuid, name, public, pass, theme, description, partyId)
	if err != nil {
		return err
	}

	return nil
}

func createPlayerParty(partyId int, playerUuid string) error {
	_, err := db.Exec("INSERT INTO partymemberdata (partyId, uuid) VALUES (?, ?)", partyId, playerUuid)
	if err != nil {
		return err
	}

	return nil
}

func clearPlayerParty(playerUuid string) error {
	_, err := db.Exec("DELETE pm FROM partymemberdata pm JOIN partydata p ON p.id = pm.partyId WHERE pm.uuid = ? AND p.game = ?", playerUuid, config.gameName)
	if err != nil {
		return err
	}

	return nil
}

func readPartyMemberUuids(partyId int) (partyMemberUuids []string, err error) {
	results, err := db.Query("SELECT pm.uuid FROM partymemberdata pm JOIN playerdata pd ON pd.uuid = pm.uuid WHERE pm.partyId = ? ORDER BY pd.rank DESC, pm.id", partyId)
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
	results := db.QueryRow("SELECT COUNT(*) FROM partymemberdata WHERE partyId = ?", partyId)
	err = results.Scan(&count)

	if err != nil {
		return count, err
	}

	return count, nil
}

func readPartyOwnerUuid(partyId int) (ownerUuid string, err error) {
	results := db.QueryRow("SELECT owner FROM partydata WHERE id = ?", partyId)
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
		_, err := db.Exec("UPDATE partydata p SET p.owner = (SELECT pm.uuid FROM partymemberdata pm JOIN playerdata pd ON pd.uuid = pm.uuid WHERE pm.partyId = p.id ORDER BY pd.rank DESC, pm.id LIMIT 1) WHERE p.id = ?", partyId)
		if err != nil {
			return err
		}
	}

	return nil
}

func setPartyOwner(partyId int, playerUuid string) error {
	_, err := db.Exec("UPDATE partydata SET owner = ? WHERE id = ?", playerUuid, partyId)
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
		_, err := db.Exec("DELETE FROM partydata WHERE id = ?", partyId)
		if err != nil {
			return true, err
		}
		return true, nil
	}

	return false, nil
}

func deletePartyAndMembers(partyId int) (err error) {
	_, err = db.Exec("DELETE FROM partymemberdata WHERE partyId = ?", partyId)
	if err != nil {
		return err
	}

	_, err = db.Exec("DELETE FROM partydata WHERE id = ?", partyId)
	if err != nil {
		return err
	}

	return nil
}

func readSaveDataTimestamp(playerUuid string) (timestamp time.Time, err error) { //called by api only
	result := db.QueryRow("SELECT timestamp FROM gamesavedata WHERE uuid = ? AND game = ?", playerUuid, config.gameName)

	if err != nil {
		return timestamp, err
	}

	err = result.Scan(&timestamp)
	if err != nil {
		return timestamp, err
	}

	return timestamp, nil
}

func readSaveData(playerUuid string) (saveData *SaveData, err error) { //called by api only
	result := db.QueryRow("SELECT timestamp, data FROM gamesavedata WHERE uuid = ? AND game = ?", playerUuid, config.gameName)

	if err != nil {
		return saveData, err
	}

	var timestamp time.Time
	saveData = &SaveData{}
	err = result.Scan(&timestamp, &saveData.Data)
	if err != nil {
		return saveData, err
	}
	saveData.Timestamp = timestamp.Format(time.RFC3339)

	return saveData, nil
}

func createGameSaveData(playerUuid string, timestamp time.Time, data string) (err error) { //called by api only
	_, err = db.Exec("INSERT INTO gamesavedata (uuid, game, timestamp, data) VALUES (?, ?, ?, ?) ON DUPLICATE KEY UPDATE timestamp = ?, data = ?", playerUuid, config.gameName, timestamp, data, timestamp, data)
	if err != nil {
		return err
	}

	return nil
}
