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
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

type GameLocation struct {
	Id     int      `json:"id"`
	Game   string   `json:"game"`
	Name   string   `json:"name"`
	MapIds []string `json:"mapIds"`
}

type PathLocations struct {
	Locations []PathLocation `json:"locations"`
}

type PathLocation struct {
	Title      string                    `json:"title"`
	TitleJP    string                    `json:"titleJP"`
	ConnType   int                       `json:"connType"`
	TypeParams map[string]ConnTypeParams `json:"typeParams"`
	Depth      int                       `json:"depth"`
}

type ConnTypeParams struct {
	Params   string `json:"params"`
	ParamsJP string `json:"paramsJP"`
}

type LocationResponse struct {
	Title               string   `json:"title"`
	LocationImage       string   `json:"locationImage"`
	BackgroundColor     string   `json:"backgroundColor"`
	FontColor           string   `json:"fontColor"`
	OriginalName        string   `json:"originalName,omitempty"`
	PrimaryAuthor       string   `json:"primaryAuthor,omitempty"`
	ContributingAuthors []string `json:"contributingAuthors,omitempty"`
	VersionAdded        string   `json:"versionAdded"`
	VersionsUpdated     []string `json:"versionsUpdated"`
	VersionRemoved      string   `json:"versionRemoved,omitempty"`
	VersionGaps         []string `json:"versionGaps"`
}

type LocationsResponse struct {
	Locations   []LocationResponse `json:"locations"`
	Game        string             `json:"game"`
	ContinueKey string             `json:"continueKey,omitempty"`
}

type Location struct {
	Id                  int      `json:"id"`
	Title               string   `json:"title"`
	OriginalTitle       string   `json:"originalTitle"`
	Depth               int      `json:"depth"`
	MinDepth            int      `json:"minDepth"`
	Image               string   `json:"locationImage"`
	PrimaryAuthor       string   `json:"primaryAuthor,omitempty"`
	ContributingAuthors []string `json:"contributingAuthors,omitempty"`
	VersionAdded        string   `json:"versionAdded"`
	VersionsUpdated     []string `json:"versionsUpdated"`
	Secret              bool     `json:"secret"`
}

var locationCache []*Location
var locationPlayerCounts map[int]int
var locationPlayerCountsPayload []byte

func initLocations() {
	logInitTask("locations")

	locationPlayerCounts = make(map[int]int)

	scheduler.Every(6).Hours().Do(updateLocationCache)
	scheduler.Every(30).Seconds().Do(updateLocationPlayerCounts)

	go updateLocationCache()
}

func handleGameLocations(w http.ResponseWriter, r *http.Request) {
	gameLocationsJson, err := json.Marshal(locationCache)
	if err != nil {
		handleError(w, r, fmt.Sprintf("error while marshaling: %s", err.Error()))
		return
	}

	w.Write([]byte(gameLocationsJson))
}

func getNext2kkiLocations(originLocationName string, destLocationName string) (PathLocations, error) {
	var nextLocations PathLocations

	v := make(url.Values)
	v.Set("origin", originLocationName)
	v.Set("dest", destLocationName)

	response, err := query2kki("getNextLocations", v.Encode())
	if err != nil {
		return nextLocations, err
	}

	err = json.Unmarshal([]byte(response), &nextLocations.Locations)
	if err != nil {
		return nextLocations, err
	}

	return nextLocations, nil
}

func updateLocationCache() {
	var locations []*Location
	var wikiLocations []LocationResponse
	var locationsResponse LocationsResponse
	continueKey := "0"

	for continueKey != "" {
		response, err := queryWiki("locations", fmt.Sprintf("continueKey=%s", continueKey))
		if err != nil {
			writeErrLog("SERVER", "Locations", err.Error())
			return
		}

		err = json.Unmarshal([]byte(response), &locationsResponse)
		if err != nil {
			writeErrLog("SERVER", "Locations", err.Error())
			return
		}

		wikiLocations = append(wikiLocations, locationsResponse.Locations...)

		continueKey = locationsResponse.ContinueKey

		locationsResponse.ContinueKey = ""
	}

	locationsMap := make(map[string]*Location)

	results, err := db.Query("SELECT id, title, depth, minDepth, secret FROM gameLocations WHERE game = ?", config.gameName)
	if err != nil {
		writeErrLog("SERVER", "Locations", err.Error())
		return
	}

	defer results.Close()

	for results.Next() {
		location := &Location{}
		err = results.Scan(&location.Id, &location.Title, &location.Depth, &location.MinDepth, &location.Secret)
		if err != nil {
			writeErrLog("SERVER", "Locations", err.Error())
			return
		}

		locationsMap[location.Title] = location
	}

	for _, wikiLocation := range wikiLocations {
		if location, ok := locationsMap[wikiLocation.Title]; ok {
			location.Image = wikiLocation.LocationImage
			location.OriginalTitle = wikiLocation.OriginalName
			location.PrimaryAuthor = wikiLocation.PrimaryAuthor
			location.ContributingAuthors = wikiLocation.ContributingAuthors
			location.VersionAdded = wikiLocation.VersionAdded
			location.VersionsUpdated = wikiLocation.VersionsUpdated

			locations = append(locations, location)
		}
	}

	locationCache = locations
}

func updateLocationPlayerCounts() {
	for k := range locationPlayerCounts {
		delete(locationPlayerCounts, k)
	}

	for _, client := range clients.Get() {
		if client.private || client.hideLocation || client.roomC == nil {
			continue
		}
		for _, locationId := range client.roomC.locationIds {
			locationPlayerCounts[locationId]++
		}
	}

	playerCountsJson, err := json.Marshal(locationPlayerCounts)
	if err != nil {
		writeErrLog("SERVER", "Location Player Counts", err.Error())
		return
	}

	locationPlayerCountsPayload = playerCountsJson
}
