/*
	Copyright (C) 2021-2022  The YNOproject Developers

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

type MinigameConfig struct {
	MinigameId     string `json:"minigameId"`
	VarId          int    `json:"varId"`
	InitialVarSync bool   `json:"initialVarSync"`
	SwitchId       int    `json:"switchId"`
	SwitchValue    bool   `json:"switchValue"`
	Dev            bool   `json:"dev"`
}

func getRoomMinigameConfigs(roomId int) (minigameConfigs []*MinigameConfig) {
	switch serverConfig.GameName {
	case "yume":
		if roomId == 155 {
			minigameConfigs = append(minigameConfigs, &MinigameConfig{MinigameId: "nasu", VarId: 88, SwitchId: 215})
		}
	case "2kki":
		switch roomId {
		case 102:
			minigameConfigs = append(minigameConfigs, &MinigameConfig{MinigameId: "rby", VarId: 1010, InitialVarSync: true})
		case 618:
			minigameConfigs = append(minigameConfigs, &MinigameConfig{MinigameId: "rby_ex", VarId: 79, InitialVarSync: true})
		case 341:
			minigameConfigs = append(minigameConfigs, &MinigameConfig{MinigameId: "fuji_ex", VarId: 3218, SwitchId: 3219, SwitchValue: true, Dev: true})
		}
	}
	return minigameConfigs
}
