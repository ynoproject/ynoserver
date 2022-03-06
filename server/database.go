package server

import (
	"errors"
	"strconv"
	"database/sql"
	"github.com/thanhpk/randstr"
	_ "github.com/go-sql-driver/mysql"
)

type Database struct {
	handle *sql.DB
}

func getDatabaseHandle(c Config) (*sql.DB) {
	db, err := sql.Open("mysql", c.dbUser + ":" + c.dbPass + "@tcp(" + c.dbHost + ")/" + c.dbName)
	if err != nil {
		return nil
	}

	return db
}

func (d *Database) readPlayerData(ip string) (uuid string, rank int, banned bool) {
	results, err := d.queryDatabase("SELECT uuid, rank, banned FROM playerdata WHERE ip = '" + ip + "'")
	if err != nil {
		return "", 0, false
	}
	
	defer results.Close()

	if results.Next() {
		err := results.Scan(&uuid, &rank, &banned)
		if err != nil {
			return "", 0, false
		}
	} else {
		uuid = randstr.String(16)
		banned, _ := isVpn(ip)
		d.createPlayerData(ip, uuid, 0, banned)
	} 

	return uuid, rank, banned
}

func (d *Database) readPlayerRank(uuid string) (rank int) {
	results, err := d.queryDatabase("SELECT rank FROM playerdata WHERE uuid = '" + uuid + "'")
	if err != nil {
		return 0
	}
	
	defer results.Close()

	if results.Next() {
		err := results.Scan(&rank)
		if err != nil {
			return 0
		}
	}

	return rank
}

func (d *Database) tryBanPlayer(senderIp, uuid string) error {
	senderUUID, senderRank, _ := d.readPlayerData(senderIp)
	if senderUUID == uuid {
		return errors.New("attempted self-ban")
	}
	rank := d.readPlayerRank(uuid)
	if senderRank <= rank {
		return errors.New("unauthorized ban")
	}

	results, err := d.queryDatabase("UPDATE playerdata SET banned = true WHERE uuid = '" + uuid + "'")
	if err != nil {
		return err
	}
	
	defer results.Close()

	return nil
}

func (d *Database) createPlayerData(ip string, uuid string, rank int, banned bool) error {
	results, err := d.queryDatabase("INSERT INTO playerdata (ip, uuid, rank, banned) VALUES ('" + ip + "', '" + uuid + "', " + strconv.Itoa(rank) + ", " + strconv.FormatBool(banned) + ") ON DUPLICATE KEY UPDATE uuid = '" + uuid + "', rank = " + strconv.Itoa(rank) + ", banned = " + strconv.FormatBool(banned))
	if err != nil {
		return err
	}
	
	defer results.Close()

	return nil
}

func (d *Database) queryDatabase(query string) (*sql.Rows, error) {
	if d.handle == nil {
		return nil, nil
	}

	results, err := d.handle.Query(query)
	if err != nil {
		return nil, err
	}

	return results, err
}
