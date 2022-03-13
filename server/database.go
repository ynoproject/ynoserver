package server

import (
	"database/sql"
	"errors"

	_ "github.com/go-sql-driver/mysql"
	"github.com/thanhpk/randstr"
)

func getDatabaseHandle() *sql.DB {
	db, err := sql.Open("mysql", config.dbUser+":"+config.dbPass+"@tcp("+config.dbHost+")/"+config.dbName)
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

func readPlayerPartyId(uuid string) (partyId int, err error) {
	results := db.QueryRow("SELECT pm.partyId FROM partymemberdata pm JOIN partydata p ON p.id = pm.partyId WHERE pm.uuid = ? AND p.game = ?", uuid, config.gameName)
	err = results.Scan(&partyId)

	if err != nil {
		return 0, err
	}

	return partyId, nil
}

func readAllPartyData(publicOnly bool, playerUuid string) (parties []*Party, err error) { //called by api only
	var results *sql.Rows
	if publicOnly {
		results, err = db.Query("SELECT p.id, p.owner, p.name, p.public, p.theme, p.description FROM partydata p LEFT JOIN playergamedata pm ON pm.partyId = p.id WHERE p.game = ? AND (p.public = 1 OR pm.uuid = ?)", config.gameName, playerUuid)
	} else {
		results, err = db.Query("SELECT p.id, p.owner, p.name, p.public, p.theme, p.description FROM partydata p WHERE p.game = ?", config.gameName)
	}

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
		parties = append(parties, party)
	}

	partyMembersByParty, err := readAllPartyMemberDataByParty(publicOnly, playerUuid)
	if err != nil {
		return parties, err
	}

	for _, party := range parties {
		for _, partyMember := range partyMembersByParty[party.Id] {
			party.Members = append(party.Members, *partyMember)
		}
	}

	return parties, nil
}

func readAllPartyMemberDataByParty(publicOnly bool, playerUuid string) (partyMembersByParty map[int][]*PartyMember, err error) {
	partyMembersByParty = make(map[int][]*PartyMember)

	var results *sql.Rows
	if publicOnly {
		results, err = db.Query("SELECT pm.partyId, pm.uuid, pgd.name, pd.rank, pgd.systemName, pgd.spriteName, pgd.spriteIndex FROM partymemberdata pm JOIN playergamedata pgd ON pgd.uuid = pm.uuid JOIN playerdata pd ON pd.uuid = pgd.uuid JOIN partydata p ON p.id = pm.partyId WHERE pgd.game = ? AND p.public = 1 OR EXISTS (SELECT * FROM playergamedata pm2 WHERE pm2.partyId = p.id AND pm2.uuid = ?) ORDER BY CASE WHEN p.owner = pm.uuid THEN 0 ELSE 1 END, pm.id", config.gameName, playerUuid)
	} else {
		results, err = db.Query("SELECT pm.partyId, pm.uuid, pgd.name, pd.rank, pgd.systemName, pgd.spriteName,	pgd.spriteIndex FROM partymemberdata pm JOIN playergamedata pgd ON pgd.uuid = pm.uuid JOIN playerdata pd ON pd.uuid = pgd.uuid JOIN partydata p ON p.id = pm.partyId WHERE pm.partyId IS NOT NULL AND pgd.game = ? ORDER BY CASE WHEN p.owner = pm.uuid THEN 0 ELSE 1 END, pm.id", config.gameName)
	}

	if err != nil {
		return partyMembersByParty, err
	}

	defer results.Close()

	for results.Next() {
		var partyId int
		partyMember := &PartyMember{}
		err := results.Scan(&partyId, &partyMember.Uuid, &partyMember.Name, &partyMember.Rank, &partyMember.SystemName, &partyMember.SpriteName, &partyMember.SpriteIndex)
		if err != nil {
			return partyMembersByParty, err
		}

		if client, ok := allClients[partyMember.Uuid]; ok {
			partyMember.Name = client.name
			partyMember.SystemName = client.systemName
			partyMember.SpriteName = client.spriteName
			partyMember.SpriteIndex = client.spriteIndex
			partyMember.Online = true
		}
		partyMembersByParty[partyId] = append(partyMembersByParty[partyId], partyMember)
	}

	return partyMembersByParty, nil
}

func readPartyData(partyId int, playerUuid string) (party Party, err error) { //called by api only
	results := db.QueryRow("SELECT p.id, p.owner, p.name, p.public, p.theme, p.description FROM partydata p JOIN partymemberdata pm ON pm.partyId = p.id JOIN playergamedata pgd ON pgd.uuid = pm.uuid AND pgd.game = p.game WHERE p.game = ? AND pm.uuid = ?", config.gameName, playerUuid)
	err = results.Scan(&party.Id, &party.OwnerUuid, &party.Name, &party.Public, &party.SystemName, &party.Description)
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

func readPartyMemberData(partyId int) (partyMembers []*PartyMember, err error) {
	results, err := db.Query("SELECT pm.partyId, pm.uuid, pgd.name, pd.rank, pgd.systemName, pgd.spriteName, pgd.spriteIndex FROM partymemberdata pm JOIN playergamedata pgd ON pgd.uuid = pm.uuid JOIN playerdata pd ON pd.uuid = pgd.uuid JOIN partydata p ON p.id = pm.partyId WHERE pm.partyId = ? AND pgd.game = ? ORDER BY CASE WHEN p.owner = pm.uuid THEN 0 ELSE 1 END, pm.id", partyId, config.gameName)
	if err != nil {
		return partyMembers, err
	}

	defer results.Close()

	for results.Next() {
		var partyId int
		partyMember := &PartyMember{}
		err := results.Scan(&partyId, &partyMember.Uuid, &partyMember.Name, &partyMember.Rank, &partyMember.SystemName, &partyMember.SpriteName, &partyMember.SpriteIndex)
		if err != nil {
			return partyMembers, err
		}
		if client, ok := allClients[partyMember.Uuid]; ok {
			partyMember.Name = client.name
			partyMember.SystemName = client.systemName
			partyMember.SpriteName = client.spriteName
			partyMember.SpriteIndex = client.spriteIndex
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

func createPartyData(name string, public bool, theme string, description string, playerUuid string) (partyId int, err error) {
	res, err := db.Exec("INSERT INTO partydata (game, owner, name, public, theme, description) VALUES (?, ?, ?, ?, ?, ?)", config.gameName, playerUuid, name, public, theme, description)
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

func writePlayerParty(partyId int, playerUuid string) error {
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
	_, err := db.Exec("UPDATE partydata SET owner = (SELECT uuid FROM partymemberdata WHERE partyId = ? ORDER BY id LIMIT 1) WHERE partyId = ?", partyId, partyId)
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
		_, err := db.Exec("DELETE FROM partydata WHERE id = ? AND p.game = ?", partyId, config.gameName)
		if err != nil {
			return true, err
		}
		return true, nil
	}

	return false, nil
}
