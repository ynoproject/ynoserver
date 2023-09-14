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
	"bytes"
	"encoding/json"
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
	SystemName string    `json:"systemName"`
	Timestamp  time.Time `json:"timestamp"`
	Public     bool      `json:"public"`
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
	Timestamp time.Time        `json:"timestamp"`
	LikeCount int              `json:"likeCount"`
	Liked     bool             `json:"liked"`
}

const (
	defaultPlayerScreenshotLimit = 10
)

func initScreenshots() {
	logInitTask("screenshots")

	http.Handle("/screenshots/", http.StripPrefix("/screenshots", http.FileServer(http.Dir("./screenshots/"))))

	logTaskComplete()
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
		var (
			limit  int
			offset int
			err    error
		)

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
	case "upload":
		fallthrough
	case "uploadScreenshot":
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

		id := getNanoId()

		err = writeScreenshotData(id, uuid, config.gameName)
		if err != nil {
			handleInternalError(w, r, err)
			return
		}

		err = os.Mkdir("screenshots/"+uuid, 0755)
		if err != nil && os.IsNotExist(err) {
			handleInternalError(w, r, err)
			return
		}

		err = os.WriteFile("screenshots/"+uuid+"/"+id+".png", body, 0644)
		if err != nil {
			handleInternalError(w, r, err)
			return
		}
	case "setPublic":
		fallthrough
	case "setLike":
		fallthrough
	case "delete":
		fallthrough
	case "deleteScreenshot":
		idParam := r.URL.Query().Get("id")
		if idParam == "" || !regexp.MustCompile("[0-9a-f]{16}").MatchString(idParam) {
			handleError(w, r, "invalid screenshot id")
			return
		}

		if commandParam == "setPublic" || commandParam == "setLike" {
			valueParam := r.URL.Query().Get("value")

			value := valueParam == "1"

			if commandParam == "setPublic" {
				success, err := setPlayerScreenshotPublic(idParam, uuid, value)
				if err != nil {
					handleInternalError(w, r, err)
					return
				}

				if !success {
					handleError(w, r, "failed to update screenshot")
					return
				}
			} else {
				var err error
				var success bool
				if value {
					err, success = writeScreenshotLike(uuid, idParam)
				} else {
					err, success = deleteScreenshotLike(uuid, idParam)
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
			err := os.Remove("screenshots/" + uuid + "/" + idParam + ".png")
			if err != nil {
				handleInternalError(w, r, err)
				return
			}

			success, err := deleteScreenshot(idParam, uuid)
			if err != nil {
				handleInternalError(w, r, err)
				return
			}

			if !success {
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
