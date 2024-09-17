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
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"
)

type ScheduleUpdate struct {
	SchedulePlatforms
	Name          string    `json:"name"`
	Description   string    `json:"description"`
	OwnerUuid     string    `json:"ownerUuid"`
	PartyId       string    `json:"partyId,omitempty"`
	Game          string    `json:"game"`
	Recurring     bool      `json:"recurring"`
	IntervalValue int       `json:"interval"`
	IntervalType  string    `json:"intervalType"`
	Datetime      time.Time `json:"datetime"`
	SystemName    string    `json:"systemName"`
}

type ScheduleDisplay struct {
	ScheduleUpdate
	Id            int    `json:"id,omitempty"`
	OwnerName     string `json:"ownerName"`
	FollowerCount int    `json:"followerCount"`
	PlayerLiked   bool   `json:"playerLiked"`
}

type SchedulePlatforms struct {
	Discord  string `json:"discord,omitempty"`
	Youtube  string `json:"youtube,omitempty"`
	Twitch   string `json:"twitch,omitempty"`
	Niconico string `json:"niconico,omitempty"`
	Openrec  string `json:"openrec,omitempty"`
	Bilibili string `json:"bilibili,omitempty"`
}

func initSchedules() {
	scheduler.Every(1).Day().At("06:00").Do(clearDoneSchedules)
}

func handleSchedules(w http.ResponseWriter, r *http.Request) {
	var uuid string
	var banned bool
	var rank int

	commandParam := r.URL.Query().Get("command")
	if commandParam == "" {
		handleError(w, r, "command not specified")
		return
	}
	token := r.Header.Get("Authorization")
	if token == "" {
		if commandParam == "list" {
			uuid, banned, _ = getOrCreatePlayerData(getIp(r))
		} else {
			handleError(w, r, "token not specified")
			return
		}
	} else {
		uuid, _, rank, _, banned, _ = getPlayerDataFromToken(token)
		if uuid == "" {
			handleError(w, r, "invalid token")
			return
		}
	}

	if banned {
		handleError(w, r, "player is banned")
		return
	}

	switch commandParam {
	case "list":
		schedules, err := listSchedules(uuid, rank)
		if err != nil {
			fmt.Printf("listSchedules: %s", err)
			handleError(w, r, "error listing schedules")
			return
		}
		schedulesJson, err := json.Marshal(schedules)
		if err != nil {
			handleError(w, r, "error marshalling schedules")
			return
		}
		w.Write(schedulesJson)
	case "update":
		var id int
		var err error
		query := r.URL.Query()
		if query.Has("id") {
			id, err = strconv.Atoi(query.Get("id"))
			if err != nil {
				handleError(w, r, "invalid schedule ID")
				return
			}
		}
		var interval int
		var intervalType string
		recurring := query.Get("recurring") == "true"
		if recurring {
			interval, err = strconv.Atoi(query.Get("interval"))
			if err != nil {
				handleError(w, r, "invalid interval")
				return
			}
			intervalType = query.Get("intervalType")
			if intervalType != "days" && intervalType != "months" && intervalType != "years" {
				handleError(w, r, "invalid interval type")
				return
			}
		}
		datetime, err := time.Parse(time.RFC3339, query.Get("datetime"))
		if err != nil {
			handleError(w, r, "invalid datetime `"+query.Get("datetime")+"`")
		}
		payload := &ScheduleUpdate{
			Name:          query.Get("name"),
			Description:   query.Get("description"),
			OwnerUuid:     query.Get("ownerUuid"),
			PartyId:       query.Get("partyId"),
			Game:          query.Get("game"),
			Recurring:     recurring,
			IntervalValue: interval,
			IntervalType:  intervalType,
			Datetime:      datetime,
			SystemName:    query.Get("systemName"),
			SchedulePlatforms: SchedulePlatforms{
				Discord:  query.Get("discord"),
				Youtube:  query.Get("youtube"),
				Twitch:   query.Get("twitch"),
				Niconico: query.Get("niconico"),
				Openrec:  query.Get("openrec"),
				Bilibili: query.Get("bilibili"),
			},
		}
		id, err = updateSchedule(id, rank, uuid, payload)
		if err != nil {
			fmt.Printf("updateSchedules: %s", err)
			handleError(w, r, "error updating schedule")
			return
		}
		w.Write([]byte(strconv.Itoa(id)))
	case "follow":
		query := r.URL.Query()
		scheduleId, err := strconv.Atoi(query.Get("scheduleId"))
		if err != nil {
			handleError(w, r, "invalid scheduleId")
			return
		}
		shouldFollow := query.Get("value") == "true"
		followCount, err := followSchedule(uuid, scheduleId, shouldFollow)
		if err != nil {
			fmt.Printf("followSchedules: %s", err)
			handleError(w, r, "error following schedule")
			return
		}
		w.Write([]byte(strconv.Itoa(followCount)))
	case "cancel":
		scheduleId, err := strconv.Atoi(r.URL.Query().Get("scheduleId"))
		if err != nil {
			handleError(w, r, "invalid scheduleId")
			return
		}
		err = cancelSchedule(uuid, rank, scheduleId)
		if err != nil {
			fmt.Printf("cancelSchedules: %s", err)
			handleError(w, r, "error cancelling schedule")
			return
		}
		w.Write([]byte("ok"))
	}
}

func listSchedules(uuid string, rank int) ([]*ScheduleDisplay, error) {
	var schedules []*ScheduleDisplay
	partyId, err := getPlayerPartyId(uuid)
	if err != nil {
		return schedules, err
	}

	selectClause := `
WITH tally AS (SELECT pf.scheduleId, COUNT(pf.uuid) AS followerCount FROM playerScheduleFollows pf GROUP BY pf.scheduleId),
	 likes AS (SELECT scheduleId FROM playerScheduleFollows WHERE uuid = ?)
SELECT s.id, s.name, s.description, s.ownerUuid, p.name AS ownerName, s.partyId, s.game, s.recurring, s.intervalValue,
	   s.intervalType, s.datetime, s.systemName, s.discord, s.youtube, s.twitch, s.niconico, s.openrec, s.bilibili,
	   tally.followerCount, IF(s.id IN likes, 1, 0) AS playerLiked
FROM schedules s
WHERE s.partyId IS NULL OR s.partyId = ? OR ?
LEFT JOIN players p ON p.uuid = s.ownerUuid
LEFT JOIN tally ON tally.scheduleId = s.id`

	results, err := db.Query(selectClause, uuid, partyId, rank > 0)
	if err != nil {
		return schedules, err
	}
	defer results.Close()
	for results.Next() {
		var s ScheduleDisplay
		err := results.Scan(&s.Id, &s.Name, &s.Description, &s.OwnerUuid, &s.OwnerName, &s.PartyId, &s.Game, &s.Recurring, &s.IntervalValue, &s.IntervalType, &s.Datetime, &s.SystemName, &s.Discord, &s.Youtube, &s.Twitch, &s.Niconico, &s.Openrec, &s.Bilibili, &s.FollowerCount, &s.PlayerLiked)
		if err != nil {
			return schedules, err
		}
		schedules = append(schedules, &s)
	}
	return schedules, nil
}

func updateSchedule(id int, rank int, uuid string, s *ScheduleUpdate) (int, error) {
	if id == 0 {
		query := `
INSERT INTO schedules
	(name, description, ownerUuid, partyId, game, recurring, intervalValue, intervalType, datetime, systemName, discord, youtube, twitch, niconico, openrec, bilibili)
VALUES
	(   ?,           ?,         ?,       ?,    ?,         ?,             ?,            ?,        ?,          ?,       ?,       ?,      ?,        ?,       ?,        ?)`
		results, err := db.Exec(query, s.Name, s.Description, s.OwnerUuid, s.PartyId, s.Game, s.Recurring, s.IntervalValue, s.IntervalType, s.Datetime, s.SystemName, s.Discord, s.Youtube, s.Twitch, s.Niconico, s.Openrec, s.Bilibili)
		if err != nil {
			return id, err
		}
		idLarge, err := results.LastInsertId()
		if err != nil {
			return id, err
		}
		return int(idLarge), nil
	}

	query := `
UPDATE schedules SET
	name = ?, description = ?, partyId = ?, ownerUuid = ?, game = ?, recurring = ?, intervalValue = ?, intervalType = ?, datetime = ?, systemName = ?,
	discord = ?, youtube = ?, twitch = ?, niconico = ?, openrec = ?, bilibili = ?
WHERE id = ? AND (? OR ownerUuid = ?)`
	results, err := db.Exec(query, s.Name, s.Description, s.PartyId, s.OwnerUuid, s.Game, s.Recurring, s.IntervalValue, s.IntervalType, s.Datetime, s.SystemName, s.Discord, s.Youtube, s.Twitch, s.Niconico, s.Openrec, s.Bilibili, id, rank > 0, uuid)

	if err != nil {
		return id, err
	}
	affected, err := results.RowsAffected()
	if affected < 1 {
		return id, errors.Join(err, errors.New("Did not update any schedules"))
	}
	return id, err
}

func followSchedule(uuid string, scheduleId int, shouldFollow bool) (followCount int, _ error) {
	if shouldFollow {
		_, err := db.Exec("INSERT IGNORE INTO playerScheduleFollows (uuid, scheduleId) VALUES (?, ?)", uuid, scheduleId)
		if err != nil {
			return followCount, err
		}
	} else {
		_, err := db.Exec("DELETE FROM playerScheduleFollows WHERE uuid = ? AND scheduleId = ?", uuid, scheduleId)
		if err != nil {
			return followCount, err
		}
	}

	err := db.QueryRow("SELECT COUNT(uuid) FROM playerScheduleFollows WHERE scheduleId = ?", scheduleId).Scan(&followCount)
	return followCount, err
}

func cancelSchedule(uuid string, rank int, scheduleId int) error {
	_, err := db.Exec("DELETE FROM schedules WHERE id = (SELECT id FROM schedules WHERE id = ? AND (? OR ownerUuid = ?))", scheduleId, rank > 0, uuid)
	return err
}

func clearDoneSchedules() {
	_, err := db.Exec("DELETE FROM schedules WHERE datetime < NOW() AND NOT recurring")
	if err != nil {
		fmt.Printf("error deleting non-recurring events: %s", err)
	}

	_, err = db.Exec(`
UPDATE schedules
SET datetime = CASE intervalType
    WHEN 'days' THEN DATE_ADD(datetime, INTERVAL intervalValue DAY)
    WHEN 'months' THEN DATE_ADD(datetime, INTERVAL intervalValue MONTH)
    WHEN 'years' THEN DATE_ADD(datetime, INTERVAL intervalValue YEAR)
    ELSE datetime
END WHERE recurring`)
	if err != nil {
		fmt.Printf("error calculating recurring events: %s", err)
	}
}
