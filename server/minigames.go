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
	"database/sql"
	"time"
)

type Minigame struct {
	Id             string `json:"id"`
	VarId          int    `json:"varId"`
	InitialVarSync bool   `json:"initialVarSync"`
	SwitchId       int    `json:"switchId"`
	SwitchValue    bool   `json:"switchValue"`
	Dev            bool   `json:"dev"`
}

func getRoomMinigames(roomId int) (minigames []*Minigame) {
	switch config.gameName {
	case "yume":
		if roomId == 155 {
			minigames = append(minigames, &Minigame{Id: "nasu", VarId: 88, SwitchId: 215})
		}
	case "2kki":
		switch roomId {
		case 102:
			minigames = append(minigames, &Minigame{Id: "rby", VarId: 1010, InitialVarSync: true})
		case 618:
			minigames = append(minigames, &Minigame{Id: "rby_ex", VarId: 79, InitialVarSync: true})
		case 344:
			minigames = append(minigames, &Minigame{Id: "fuji_ex", VarId: 3218, SwitchId: 3219, SwitchValue: true})
		case 1899:
			minigames = append(minigames, &Minigame{Id: "hozo", VarId: 4268, SwitchId: 5019, SwitchValue: true})
		}
	//case "amillusion":
	//	if roomId == 185 {
	//		minigames = append(minigames, &Minigame{Id: "cartoonboy", VarId: 86, SwitchId: 137, SwitchValue: true})
	//	}
	case "mikan":
		switch roomId {
		case 6:
			minigames = append(minigames, &Minigame{Id: "ta_be", VarId: 17, SwitchId: 14, SwitchValue: true})
		case 86:
			minigames = append(minigames, &Minigame{Id: "ta_be_hardcore", VarId: 17, SwitchId: 14, SwitchValue: true})
		}
	case "ultraviolet":
		if roomId == 118 {
			minigames = append(minigames, &Minigame{Id: "panerabbit", VarId: 152, SwitchId: 302, SwitchValue: true})
		}
	}
	return minigames
}

func getPlayerMinigameScore(playerUuid string, minigameId string) (score int, err error) {
	err = db.QueryRow("SELECT score FROM playerMinigameScores WHERE uuid = ? AND minigameId = ?", playerUuid, minigameId).Scan(&score)
	if err != nil {
		if err == sql.ErrNoRows {
			return 0, nil
		}
		return 0, err
	}

	return score, nil
}

func tryWritePlayerMinigameScore(playerUuid string, minigameId string, score int) (success bool, err error) {
	if score <= 0 {
		return false, nil
	}

	prevScore, err := getPlayerMinigameScore(playerUuid, minigameId)
	if err != nil {
		return false, err
	} else if score <= prevScore {
		return false, nil
	} else if prevScore > 0 {
		_, err = db.Exec("UPDATE playerMinigameScores SET score = ?, timestampCompleted = ? WHERE uuid = ? AND game = ? AND minigameId = ?", score, time.Now(), playerUuid, config.gameName, minigameId)
		if err != nil {
			return false, err
		}
		return true, nil
	}

	_, err = db.Exec("INSERT INTO playerMinigameScores (uuid, game, minigameId, score, timestampCompleted) VALUES (?, ?, ?, ?, ?)", playerUuid, config.gameName, minigameId, score, time.Now())
	if err != nil {
		return false, err
	}

	return true, nil
}
