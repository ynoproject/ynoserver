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
