package server

import (
	"database/sql"
	"errors"
	"strconv"

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
	results := db.QueryRow("SELECT uuid, rank, banned FROM playerdata WHERE ip = '" + ip + "'")
	err := results.Scan(&uuid, &rank, &banned)

	if uuid == "" {
		if err != nil {
			return "", 0, false
		} else {
			uuid = randstr.String(16)
			banned, _ := isVpn(ip)
			createPlayerData(ip, uuid, 0, banned)
		}
	}

	return uuid, rank, banned
}

func readPlayerRank(uuid string) (rank int) {
	results := db.QueryRow("SELECT rank FROM playerdata WHERE uuid = '" + uuid + "'")
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

	_, err := db.Exec("UPDATE playerdata SET banned = true WHERE uuid = '" + recipientUUID + "'")
	if err != nil {
		return err
	}

	return nil
}

func createPlayerData(ip string, uuid string, rank int, banned bool) error {
	_, err := db.Exec("INSERT INTO playerdata (ip, uuid, rank, banned) VALUES ('" + ip + "', '" + uuid + "', " + strconv.Itoa(rank) + ", " + strconv.FormatBool(banned) + ") ON DUPLICATE KEY UPDATE uuid = '" + uuid + "', rank = " + strconv.Itoa(rank) + ", banned = " + strconv.FormatBool(banned))
	if err != nil {
		return err
	}

	return nil
}

func updatePlayerData(client *Client) error {
	_, err := db.Exec("UPDATE playerdata SET name = '" + client.name + "', systemName = '" + client.systemName + "', spriteName = '" + client.spriteName + "', spriteIndex = " + strconv.Itoa(client.spriteIndex) + " WHERE uuid = '" + client.uuid + "'")
	if err != nil {
		return err
	}

	return nil
}

func readAllPartyData(publicOnly bool, playerUuid string) (parties []Party) { //called by api only
	var results *sql.Rows
	var err error
	if publicOnly {
		results, err = db.Query("SELECT p.id, p.owner, p.name, p.public, p.theme, p.description FROM partydata p LEFT JOIN playergamedata pm ON pm.partyId = p.id WHERE p.game = ? AND (p.public = 1 OR pm.uuid = ?)", config.gameName, playerUuid)
	} else {
		results, err = db.Query("SELECT p.id, p.owner, p.name, p.public, p.theme, p.description FROM partydata p WHERE p.game = ?", config.gameName)
	}

	if err != nil {
		return parties
	}

	for results.Next() {
		party := &Party{}
		err := results.Scan(&party.Id, &party.OwnerUuid, &party.Name, &party.Public, &party.SystemName, &party.Description)
		if err != nil {
			continue
		}
		parties = append(parties, *party)
	}

	defer results.Close()

	err, partyMembersByParty := readAllPartyMemberDataByParty(publicOnly, playerUuid)
	if err == nil {
		for _, party := range parties {
			partyMembers := partyMembersByParty[party.Id]
			for _, partyMember := range partyMembers {
				party.Members = append(party.Members, partyMember)
			}
		}
	}

	return parties
}

func readAllPartyMemberDataByParty(publicOnly bool, playerUuid string) (err error, partyMembersByParty map[int][]PartyMember) {
	partyMembersByParty = make(map[int][]PartyMember)

	var results *sql.Rows
	if publicOnly {
		results, err = db.Query("SELECT pm.partyId, pm.uuid, pm.name, pd.rank, pm.systemName, pm.spriteName, pm.spriteIndex FROM playergamedata pm JOIN playerdata pd ON pd.uuid = pm.uuid JOIN partydata p ON p.id = pm.partyId WHERE pm.game = ? AND p.public = 1 OR EXISTS (SELECT * FROM playergamedata pm2 WHERE pm2.partyId = p.id AND pm2.uuid = ?)", config.gameName, playerUuid)
	} else {
		results, err = db.Query("SELECT pm.partyId, pm.uuid, pm.name, pd.rank, pm.systemName, pm.spriteName, pm.spriteIndex FROM playergamedata pm JOIN playerdata pd ON pd.uuid = pm.uuid WHERE pm.game = ?", config.gameName)
	}

	if err != nil {
		return err, partyMembersByParty
	}

	for results.Next() {
		var partyId int
		partyMember := &PartyMember{}
		err := results.Scan(&partyId, &partyMember.Uuid, &partyMember.Name, &partyMember.Rank, &partyMember.SystemName, &partyMember.SpriteName, &partyMember.SpriteIndex)
		if err != nil {
			continue
		}
		if client, ok := allClients[partyMember.Uuid]; ok {
			partyMember.Name = client.name
			partyMember.SystemName = client.systemName
			partyMember.SpriteName = client.spriteName
			partyMember.SpriteIndex = client.spriteIndex
			partyMember.Online = true
		}
		partyMembersByParty[partyId] = append(partyMembersByParty[partyId], *partyMember)
	}

	defer results.Close()

	return nil, partyMembersByParty
}

func readPartyData(partyId int, playerUuid string) (party Party) { //called by api only
	results := db.QueryRow("SELECT p.id, p.owner, p.name, p.public, p.theme, p.description FROM partydata p LEFT JOIN playerdata pd ON pd.partyId = p.id WHERE p.game = ? AND pd.uuid = ?", config.gameName, playerUuid)
	err := results.Scan(&party.Id, &party.OwnerUuid, &party.Name, &party.Public, &party.SystemName, &party.Description)
	if err != nil {
		return party
	}

	err, partyMembers := readPartyMemberData(party.Id)
	if err == nil {
		for _, partyMember := range partyMembers {
			party.Members = append(party.Members, partyMember)
		}
	}

	return party
}

func readPartyMemberData(partyId int) (err error, partyMembers []PartyMember) {
	results, err := db.Query("SELECT pm.partyId, pm.uuid, pm.name, pd.rank, pm.systemName, pm.spriteName, pm.spriteIndex FROM playergamedata pm JOIN playerdata pd ON pd.uuid = pm.uuid WHERE pm.partyId = ? AND pm.game = ?", partyId, config.gameName)
	if err != nil {
		return err, partyMembers
	}

	for results.Next() {
		var partyId int
		partyMember := &PartyMember{}
		err := results.Scan(&partyId, &partyMember.Uuid, &partyMember.Name, &partyMember.Rank, &partyMember.SystemName, &partyMember.SpriteName, &partyMember.SpriteIndex)
		if err != nil {
			continue
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
		partyMembers = append(partyMembers, *partyMember)
	}

	defer results.Close()

	return nil, partyMembers
}
