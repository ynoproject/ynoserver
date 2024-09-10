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
	"net/url"
)

type GameLocation struct {
	Id     int      `json:"id"`
	Game   string   `json:"game"`
	Name   string   `json:"name"`
	MapIds []string `json:"mapIds"`
}

func getNext2kkiLocations(originLocationName string, destLocationName string) ([]string, error) {
	response, err := query2kki("getNextLocations", "origin="+url.QueryEscape(originLocationName)+"&dest="+url.QueryEscape(destLocationName))
	if err != nil {
		return nil, err
	}

	var nextLocations []string
	err = json.Unmarshal([]byte(response), &nextLocations)
	if err != nil {
		return nil, err
	}

	return nextLocations, nil
}
