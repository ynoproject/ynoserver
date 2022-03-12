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

func readAllPartyData(publicOnly bool) (parties []Party) { //called by api only
	publicClause := ""
	if publicOnly {
		publicClause = " AND public = 1"
	}
	results, err := db.Query("SELECT id, owner, name, public, theme, description FROM partydata WHERE game = '" + config.gameName + "'" + publicClause)
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

	err, partyMembersByParty := readAllPartyMemberDataByParty(publicOnly)
	if err != nil {
		for _, party := range parties {
			partyMembers := partyMembersByParty[party.Id]
			for _, partyMember := range partyMembers {
				party.Members = append(party.Members, partyMember)
			}
		}
	}

	return parties
}

func readAllPartyMemberDataByParty(publicOnly bool) (err error, partyMembersByParty map[int][]PartyMember) {
	partyMembersByParty = make(map[int][]PartyMember)
	publicClause := ""
	if publicOnly {
		publicClause = " AND public = 1"
	}
	results, err := db.Query("SELECT pm.partyId, pm.uuid, pm.name, p.rank, pm.systemName, pm.spriteName, pm.spriteIndex FROM playergamedata pm JOIN partydata p ON p.id = pm.partyId JOIN playerdata pd ON pd.uuid = p.uuid WHERE game = '" + config.gameName + "'" + publicClause)
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
			// Will be used for party by ID which requires being in the party
			/*partyMember.MapId = client.mapId
			partyMember.PrevMapId = client.prevMapId
			partyMember.PrevLocations = client.prevLocations*/
			partyMember.Online = true
		}
		// Will be used for party by ID which requires being in the party
		/* else {
			partyMember.MapId = "0000"
			partyMember.PrevMapId = "0000"
		}*/
		partyMembersByParty[partyId] = append(partyMembersByParty[partyId], *partyMember)
	}

	defer results.Close()

	return nil, partyMembersByParty
}
