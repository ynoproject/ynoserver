/*
	Copyright (C) 2021-2024  The YNOproject Developers

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
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"image/png"
	"io"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"time"
)

type PlayerScreenshotData struct {
	Id         string    `json:"id"`
	Uuid       string    `json:"uuid"`
	Game       string    `json:"game"`
	MapId      string    `json:"mapId"`
	MapX       int       `json:"mapX"`
	MapY       int       `json:"mapY"`
	SystemName string    `json:"systemName"`
	Timestamp  time.Time `json:"timestamp"`
	Public     bool      `json:"public"`
	Spoiler    bool      `json:"spoiler"`
	LikeCount  int       `json:"likeCount"`
	Liked      bool      `json:"liked"`
}

type ScreenshotOwner struct {
	Uuid       string `json:"uuid"`
	Name       string `json:"name"`
	Rank       int    `json:"rank"`
	Badge      string `json:"badge"`
	SystemName string `json:"systemName"`
}

type ScreenshotData struct {
	Id        string           `json:"id"`
	Owner     *ScreenshotOwner `json:"owner"`
	Game      string           `json:"game"`
	MapId     string           `json:"mapId"`
	MapX      int              `json:"mapX"`
	MapY      int              `json:"mapY"`
	Timestamp time.Time        `json:"timestamp"`
	Public    bool             `json:"public"`
	Spoiler   bool             `json:"spoiler"`
	LikeCount int              `json:"likeCount"`
	Liked     bool             `json:"liked"`
}

const (
	defaultPlayerScreenshotLimit = 10
)

func initScreenshots() {
	// Use main server to process temp screenshot cleaning task for all games
	if isMainServer {
		logInitTask("screenshots")

		scheduler.Cron("0 * * * *").Do(deleteTempScreenshots)
	}
}

func handleScreenshot(w http.ResponseWriter, r *http.Request) {
	commandParam := r.URL.Query().Get("command")
	if commandParam == "" {
		handleError(w, r, "command not specified")
		return
	}

	token := r.Header.Get("Authorization")
	accountRequired := commandParam != "getScreenshotFeed" && commandParam != "getPlayerScreenshots" && commandParam != "getScreenshotGames"

	if token == "" && accountRequired {
		handleError(w, r, "token not specified")
		return
	}

	var uuid string

	if token != "" {
		uuid = getUuidFromToken(token)

		if uuid == "" && accountRequired {
			handleError(w, r, "invalid token")
			return
		}
	}

	switch commandParam {
	case "getScreenshotFeed":
		var limit, offset int
		var err error

		limitParam := r.URL.Query().Get("limit")
		if limitParam != "" {
			limit, err = strconv.Atoi(limitParam)
			if err != nil {
				handleError(w, r, "invalid limit")
				return
			}
			if limit > 50 {
				limit = 50
			}
		} else {
			limit = 10
		}

		offsetParam := r.URL.Query().Get("offset")
		if offsetParam != "" {
			offset, err = strconv.Atoi(offsetParam)
			if err != nil {
				handleError(w, r, "invalid offset")
				return
			}
		} else {
			offset = 0
		}

		offsetIdParam := r.URL.Query().Get("offsetId")
		if offsetIdParam != "" && !regexp.MustCompile("[0-9a-f]{16}").MatchString(offsetIdParam) {
			offsetIdParam = ""
		}

		gameParam := r.URL.Query().Get("game")

		sortOrderParam := r.URL.Query().Get("sortOrder")
		switch sortOrderParam {
		case "recent":
		case "likes":
		default:
			sortOrderParam = "recent"
		}

		intervalParam := r.URL.Query().Get("interval")
		switch intervalParam {
		case "day":
			fallthrough
		case "week":
			fallthrough
		case "month":
			fallthrough
		case "year":
		case "":
			intervalParam = "all"
		default:
			intervalParam = "day"
		}

		screenshots, err := getScreenshotFeed(uuid, limit, offset, offsetIdParam, gameParam, sortOrderParam, intervalParam)
		if err != nil {
			handleInternalError(w, r, err)
			return
		}

		screenshotsJson, err := json.Marshal(screenshots)
		if err != nil {
			handleError(w, r, "error while marshaling")
			return
		}

		w.Write(screenshotsJson)
		return
	case "getPlayerScreenshots":
		uuidParam := r.URL.Query().Get("uuid")
		if uuidParam == "" {
			if uuid == "" {
				handleError(w, r, "invalid token")
				return
			}
			uuidParam = uuid
		}

		playerScreenshots, err := getPlayerScreenshots(uuidParam)
		if err != nil {
			handleInternalError(w, r, err)
			return
		}

		playerScreenshotsJson, err := json.Marshal(playerScreenshots)
		if err != nil {
			handleError(w, r, "error while marshaling")
			return
		}

		w.Write(playerScreenshotsJson)
		return
	case "getScreenshotGames":
		screenshotGames, err := getScreenshotGames()
		if err != nil {
			handleInternalError(w, r, err)
			return
		}

		screenshotGamesJson, err := json.Marshal(screenshotGames)
		if err != nil {
			handleError(w, r, "error while marshaling")
			return
		}

		w.Write(screenshotGamesJson)
		return
	case "upload":
		body, err := io.ReadAll(r.Body)
		if err != nil {
			handleError(w, r, "failed to read body")
			return
		}

		img, err := png.Decode(bytes.NewReader(body))
		if err != nil {
			handleError(w, r, "invalid png")
			return
		}

		if bounds := img.Bounds(); !(bounds.Dx() == 320 && bounds.Dy() == 240) {
			handleError(w, r, "invalid dimensions")
			return
		}

		mapIdParam := r.URL.Query().Get("mapId")
		if mapIdParam != "" {
			_, err = strconv.Atoi(mapIdParam)
			if err != nil || len(mapIdParam) != 4 {
				handleError(w, r, "invalid mapId")
				return
			}
		} else {
			mapIdParam = "0000"
		}

		mapXParam := r.URL.Query().Get("mapX")
		mapYParam := r.URL.Query().Get("mapY")

		var mapX, mapY int

		if mapXParam != "" && mapYParam != "" {
			mapX, err = strconv.Atoi(mapXParam)
			if err != nil || mapX < 0 || mapX >= 500 {
				mapX = 0
			}
			mapY, err = strconv.Atoi(mapYParam)
			if err != nil || mapY < 0 || mapY >= 500 {
				mapY = 0
			}
		}

		temp := r.URL.Query().Get("temp") == "1"

		id := getNanoId()

		err = writeScreenshotData(id, uuid, config.gameName, mapIdParam, mapX, mapY, temp)
		if err != nil {
			handleInternalError(w, r, err)
			return
		}

		directory := "screenshots/"
		if temp {
			directory += "temp/"
		}
		directory += uuid

		err = os.Mkdir(directory, 0755)
		if err != nil && os.IsNotExist(err) {
			handleInternalError(w, r, err)
			return
		}

		err = os.WriteFile(directory+"/"+id+".png", body, 0644)
		if err != nil {
			handleInternalError(w, r, err)
			return
		}

		w.Write([]byte(id))
		return
	case "setPublic":
		fallthrough
	case "setSpoiler":
		fallthrough
	case "setLike":
		fallthrough
	case "delete":
		idParam := r.URL.Query().Get("id")
		if idParam == "" || !regexp.MustCompile("[0-9a-f]{16}").MatchString(idParam) {
			handleError(w, r, "invalid screenshot id")
			return
		}

		if commandParam == "setPublic" || commandParam == "setSpoiler" || commandParam == "setLike" {
			valueParam := r.URL.Query().Get("value")

			value := valueParam == "1"

			if commandParam == "setPublic" || commandParam == "setSpoiler" {
				var success bool
				var err error
				if commandParam == "setPublic" {
					success, err = setPlayerScreenshotPublic(idParam, uuid, value)
				} else {
					success, err = setPlayerScreenshotSpoiler(idParam, uuid, value)
				}
				if err != nil {
					handleInternalError(w, r, err)
					return
				}

				if !success {
					handleError(w, r, "failed to update screenshot")
					return
				}

				if commandParam == "setPublic" && valueParam == "1" {
					_, name, _, badge, _, _ := getPlayerDataFromToken(r.Header.Get("Authorization"))

					err = sendWebhookMessage(config.screenshotWebhook, name, badge, fmt.Sprintf("https://connect.ynoproject.net/%s/screenshots/%s/%s.png", config.gameName, uuid, idParam), false)
					if err != nil {
						handleError(w, r, "failed to send to webhook")
						return
					}
				}
			} else {
				var err error
				var success bool
				if value {
					success, err = writePlayerScreenshotLike(idParam, uuid)
				} else {
					success, err = deletePlayerScreenshotLike(idParam, uuid)
				}
				if err != nil {
					handleInternalError(w, r, err)
					return
				}

				if !success {
					handleError(w, r, "failed to update screenshot like")
					return
				}
			}
		} else {
			temp := r.URL.Query().Get("temp") == "1"

			var ownerUuid string

			uuidParam := r.URL.Query().Get("uuid")
			if uuidParam == "" {
				ownerUuid = uuid
			} else {
				ownerUuid = uuidParam
			}

			success, err := deleteScreenshot(idParam, uuid)
			if err != nil {
				handleInternalError(w, r, err)
				return
			}

			if success {
				directory := "screenshots/"
				if temp {
					directory += "temp/"
				}
				directory += ownerUuid + "/"
				err := os.Remove(directory + idParam + ".png")
				if err != nil {
					handleInternalError(w, r, err)
					return
				}
			} else {
				handleError(w, r, "failed to delete screenshot")
				return
			}
		}
	default:
		handleError(w, r, "unknown command")
		return
	}

	w.WriteHeader(http.StatusOK)
}

func getPlayerScreenshotLimit(uuid string) (screenshotLimit int) {
	err := db.QueryRow("SELECT screenshotLimit FROM accounts WHERE uuid = ?", uuid).Scan(&screenshotLimit)
	if err != nil {
		return defaultPlayerScreenshotLimit
	}

	return screenshotLimit
}

func getScreenshotFeed(uuid string, limit int, offset int, offsetId string, game string, sortOrder string, intervalType string) ([]*ScreenshotData, error) {
	var screenshots []*ScreenshotData

	var queryArgs []any

	var cteClause string
	var selectClause string
	var fromJoinClause string
	var whereClause string
	var orderByClause string

	selectClause = "SELECT ps.id, op.uuid, oa.user, op.rank, COALESCE(oa.badge, ''), opgd.systemName, ps.game, ps.mapId, ps.mapX, ps.mapY, ps.publicTimestamp, ps.spoiler, (SELECT COUNT(*) FROM playerScreenshotLikes psl WHERE psl.screenshotId = ps.id) AS likeCount, CASE WHEN upsl.uuid IS NULL THEN 0 ELSE 1 END"

	fromJoinClause = " FROM playerScreenshots ps JOIN players op ON op.uuid = ps.uuid JOIN accounts oa ON oa.uuid = op.uuid JOIN playerGameData opgd ON opgd.uuid = op.uuid AND opgd.game = ps.game LEFT JOIN playerScreenshotLikes upsl ON upsl.screenshotId = ps.id AND upsl.uuid = ? "

	if offsetId != "" && sortOrder != "likes" {
		cteClause = "WITH offsetScreenshot AS (SELECT publicTimestamp FROM playerScreenshots WHERE id = ?) "
		fromJoinClause += "JOIN offsetScreenshot ops ON ops.publicTimestamp >= ps.publicTimestamp "
		queryArgs = append(queryArgs, offsetId)
	}

	queryArgs = append(queryArgs, uuid)

	whereClause = "WHERE ps.temp = 0 AND ps.public = 1 AND op.banned = 0 "

	if game != "" {
		whereClause += "AND ps.game = ? "
		queryArgs = append(queryArgs, game)
	}

	if intervalType != "all" {
		whereClause += "AND ps.publicTimestamp >= DATE_SUB(NOW(), INTERVAL 1 " + intervalType + ") "
	}

	orderByClause = "ORDER BY "

	if sortOrder == "likes" {
		if intervalType == "all" {
			orderByClause += "likeCount"
		} else {
			orderByClause += "(SELECT COUNT(*) FROM playerScreenshotLikes ipsl WHERE ipsl.screenshotId = ps.id AND ipsl.timestamp >= DATE_SUB(NOW(), INTERVAL 1 " + intervalType + "))"
		}
		orderByClause += " DESC, "
	}

	orderByClause += "ps.publicTimestamp DESC, op.uuid, ps.id DESC "

	query := cteClause + selectClause + fromJoinClause + whereClause + orderByClause + "LIMIT ?, ?"

	queryArgs = append(queryArgs, offset, limit)

	results, err := db.Query(query, queryArgs...)
	if err != nil {
		return screenshots, err
	}

	defer results.Close()

	for results.Next() {
		var screenshot ScreenshotData
		var owner ScreenshotOwner

		err := results.Scan(&screenshot.Id, &owner.Uuid, &owner.Name, &owner.Rank, &owner.Badge, &owner.SystemName, &screenshot.Game, &screenshot.MapId, &screenshot.MapX, &screenshot.MapY, &screenshot.Timestamp, &screenshot.Spoiler, &screenshot.LikeCount, &screenshot.Liked)
		if err != nil {
			return screenshots, err
		}

		screenshot.Public = true
		screenshot.Owner = &owner
		screenshots = append(screenshots, &screenshot)
	}

	return screenshots, nil
}

func getScreenshotInfo(uuid string, ownerUuid string, id string) (*ScreenshotData, error) {
	screenshot := &ScreenshotData{}
	owner := &ScreenshotOwner{}

	query := "SELECT ps.id, op.uuid, oa.user, op.rank, COALESCE(oa.badge, ''), opgd.systemName, ps.game, ps.mapId, ps.mapX, ps.mapY, ps.publicTimestamp, ps.public, ps.spoiler, (SELECT COUNT(*) FROM playerScreenshotLikes psl WHERE psl.screenshotId = ps.id) AS likeCount, CASE WHEN upsl.uuid IS NULL THEN 0 ELSE 1 END FROM playerScreenshots ps JOIN players op ON op.uuid = ps.uuid JOIN accounts oa ON oa.uuid = op.uuid JOIN playerGameData opgd ON opgd.uuid = op.uuid AND opgd.game = ps.game LEFT JOIN playerScreenshotLikes upsl ON upsl.screenshotId = ps.id AND upsl.uuid = ? WHERE ps.uuid = ? AND ps.id = ?"
	err := db.QueryRow(query, uuid, ownerUuid, id).Scan(&screenshot.Id, &owner.Uuid, &owner.Name, &owner.Rank, &owner.Badge, &owner.SystemName, &screenshot.Game, &screenshot.MapId, &screenshot.MapX, &screenshot.MapY, &screenshot.Timestamp, &screenshot.Public, &screenshot.Spoiler, &screenshot.LikeCount, &screenshot.Liked)
	if err != nil {
		return nil, err
	}

	if uuid != ownerUuid && !screenshot.Public {
		return nil, nil
	}

	screenshot.Owner = owner

	return screenshot, nil
}

func getPlayerScreenshots(uuid string) ([]*PlayerScreenshotData, error) {
	var playerScreenshots []*PlayerScreenshotData

	results, err := db.Query("SELECT ps.id, ps.uuid, ps.game, ps.mapId, ps.mapX, ps.mapY, opgd.systemName, ps.timestamp, ps.public, ps.spoiler, (SELECT COUNT(*) FROM playerScreenshotLikes psl WHERE psl.screenshotId = ps.id), CASE WHEN upsl.uuid IS NULL THEN 0 ELSE 1 END FROM playerScreenshots ps JOIN playerGameData opgd ON opgd.uuid = ps.uuid AND opgd.game = ps.game LEFT JOIN playerScreenshotLikes upsl ON upsl.screenshotId = ps.id AND upsl.uuid = ps.uuid WHERE ps.uuid = ? AND ps.temp = 0 ORDER BY ps.timestamp DESC, ps.id", uuid)
	if err != nil {
		return playerScreenshots, err
	}

	defer results.Close()

	for results.Next() {
		var screenshot PlayerScreenshotData

		err := results.Scan(&screenshot.Id, &screenshot.Uuid, &screenshot.Game, &screenshot.MapId, &screenshot.MapX, &screenshot.MapY, &screenshot.SystemName, &screenshot.Timestamp, &screenshot.Public, &screenshot.Spoiler, &screenshot.LikeCount, &screenshot.Liked)
		if err != nil {
			return playerScreenshots, err
		}

		playerScreenshots = append(playerScreenshots, &screenshot)
	}

	return playerScreenshots, nil
}

func getScreenshotGames() ([]string, error) {
	var screenshotGames []string

	results, err := db.Query("SELECT DISTINCT game FROM playerScreenshots")
	if err != nil {
		return screenshotGames, err
	}

	defer results.Close()

	for results.Next() {
		var game string
		err := results.Scan(&game)
		if err != nil {
			return screenshotGames, err
		}
		screenshotGames = append(screenshotGames, game)
	}

	return screenshotGames, nil
}

func writeScreenshotData(id string, uuid string, game string, mapId string, mapX int, mapY int, temp bool) error {
	var playerScreenshotCount int
	err := db.QueryRow("SELECT COUNT(*) FROM playerScreenshots WHERE uuid = ? AND temp = ?", uuid, temp).Scan(&playerScreenshotCount)
	if err != nil {
		return err
	} else {
		if temp {
			if playerScreenshotCount >= 100 {
				return errors.New("screenshot limit exceeded")
			}
		} else {
			playerScreenshotLimit := getPlayerScreenshotLimit(uuid)
			if playerScreenshotCount >= playerScreenshotLimit {
				return errors.New("screenshot limit exceeded")
			}
		}
	}

	_, err = db.Exec("INSERT INTO playerScreenshots (id, uuid, game, mapId, mapX, mapY, public, publicTimestamp, temp) VALUES (?, ?, ?, ?, ?, ?, ?, CASE WHEN ? = 1 THEN UTC_TIMESTAMP() ELSE NULL END, ?)", id, uuid, game, mapId, mapX, mapY, temp, temp, temp)
	if err != nil {
		return err
	}

	return nil
}

func setPlayerScreenshotPublic(id string, uuid string, value bool) (bool, error) {
	results, err := db.Exec("UPDATE playerScreenshots SET public = ?, publicTimestamp = COALESCE(publicTimestamp, NOW()) WHERE id = ? AND EXISTS (SELECT * FROM playerScreenshots ps JOIN players p ON p.uuid = ? JOIN players op ON op.uuid = ps.uuid WHERE p.uuid = op.uuid OR p.rank > op.rank)", value, id, uuid)
	if err != nil {
		return false, err
	}

	updatedRows, err := results.RowsAffected()
	if err != nil {
		return false, err
	}

	return updatedRows > 0, nil
}

func setPlayerScreenshotSpoiler(id string, uuid string, value bool) (bool, error) {
	results, err := db.Exec("UPDATE playerScreenshots SET spoiler = ? WHERE id = ? AND EXISTS (SELECT * FROM playerScreenshots ps JOIN players p ON p.uuid = ? JOIN players op ON op.uuid = ps.uuid WHERE p.uuid = op.uuid OR p.rank > op.rank)", value, id, uuid)
	if err != nil {
		return false, err
	}

	updatedRows, err := results.RowsAffected()
	if err != nil {
		return false, err
	}

	return updatedRows > 0, nil
}

func writePlayerScreenshotLike(id string, uuid string) (bool, error) {
	results, err := db.Exec("INSERT IGNORE INTO playerScreenshotLikes (screenshotId, uuid) VALUES (?, ?)", id, uuid)
	if err != nil {
		return false, err
	}

	insertedRows, err := results.RowsAffected()
	if err != nil {
		return false, err
	}

	return insertedRows > 0, nil
}

func deletePlayerScreenshotLike(id string, uuid string) (bool, error) {
	results, err := db.Exec("DELETE FROM playerScreenshotLikes WHERE screenshotId = ? AND uuid = ?", id, uuid)
	if err != nil {
		return false, err
	}

	deletedRows, err := results.RowsAffected()
	if err != nil {
		return false, err
	}

	return deletedRows > 0, nil
}

func deleteScreenshot(id string, uuid string) (bool, error) {
	results, err := db.Exec("DELETE FROM playerScreenshots WHERE id = ? AND EXISTS (SELECT * FROM playerScreenshots ps JOIN players p ON p.uuid = ? JOIN players op ON op.uuid = ps.uuid WHERE p.uuid = op.uuid OR p.rank > op.rank)", id, uuid)
	if err != nil {
		return false, err
	}

	deletedRows, err := results.RowsAffected()
	if err != nil {
		return false, err
	}

	return deletedRows > 0, nil
}

func deleteTempScreenshots() error {
	results, err := db.Query("SELECT id, uuid FROM playerScreenshots WHERE temp = 1 AND timestamp < DATE_SUB(UTC_TIMESTAMP(), INTERVAL 1 DAY)")
	if err != nil {
		return err
	}

	defer results.Close()

	for results.Next() {
		var screenshotId string
		var uuid string
		err = results.Scan(&screenshotId, &uuid)
		if err != nil {
			continue
		}

		os.Remove("screenshots/temp/" + uuid + "/" + screenshotId + ".png")
	}

	_, err = db.Exec("DELETE FROM playerScreenshots WHERE temp = 1 AND timestamp < DATE_SUB(UTC_TIMESTAMP(), INTERVAL 1 DAY)")
	if err != nil {
		return err
	}

	return nil
}
