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
	"time"
)

type ScreenshotData struct {
	Id        string    `json:"id"`
	Uuid      string    `json:"uuid"`
	Game      string    `json:"game"`
	Timestamp time.Time `json:"timestamp"`
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
	accountRequired := commandParam != "getPlayerScreenshots"

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
	case "deleteScreenshot":
		idParam := r.URL.Query().Get("id")
		if idParam == "" || !regexp.MustCompile("[0-9a-f]{16}").MatchString(idParam) {
			handleError(w, r, "invalid screenshot id")
			return
		}

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
	default:
		handleError(w, r, "unknown command")
		return
	}

	w.WriteHeader(http.StatusOK)
}
