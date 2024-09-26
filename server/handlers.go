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
	"strconv"
	"strings"
)

func (c *RoomClient) handleSr(msg []string) error {
	if len(msg) != 2 {
		return errors.New("segment count mismatch")
	}

	roomId, errconv := strconv.Atoi(msg[1])
	if errconv != nil {
		return errconv
	}

	room, ok := rooms[roomId]
	if !ok {
		return errors.New("invalid room id")
	}

	c.leaveRoom()
	c.joinRoom(room)

	return nil
}

func (c *RoomClient) handleM(msg []string) error {
	if len(msg) != 3 {
		return errors.New("segment count mismatch")
	}

	// check if the coordinates are valid
	x, errconv := strconv.Atoi(msg[1])
	if errconv != nil || x < 0 {
		return errconv
	}
	y, errconv := strconv.Atoi(msg[2])
	if errconv != nil || y < 0 {
		return errconv
	}

	if msg[0] == "m" {
		switch {
		case c.y < y:
			c.facing = 0 // up
		case c.x > x:
			c.facing = 1 // right
		case c.y > y:
			c.facing = 2 // down
		case c.x < x:
			c.facing = 3 // left
		}
	}

	c.x = x
	c.y = y

	if msg[0] == "tp" {
		c.checkRoomConditions("teleport", "")
	}

	if c.syncCoords {
		c.checkRoomConditions("coords", "")
	}

	if msg[0] == "jmp" {
		c.broadcast(buildMsg("jmp", c.session.id, msg[1:])) // user %id% jumped to x y
	} else {
		c.broadcast(buildMsg("m", c.session.id, msg[1:])) // user %id% moved to x y
	}

	return nil
}

func (c *RoomClient) handleF(msg []string) error {
	if len(msg) != 2 {
		return errors.New("segment count mismatch")
	}

	// check if direction is valid
	facing, errconv := strconv.Atoi(msg[1])
	if errconv != nil || facing < 0 || facing > 3 {
		return errconv
	}

	c.facing = facing

	c.broadcast(buildMsg("f", c.session.id, msg[1])) // user %id% facing changed to f

	return nil
}

func (c *RoomClient) handleSpd(msg []string) error {
	if len(msg) != 2 {
		return errors.New("segment count mismatch")
	}

	spd, errconv := strconv.Atoi(msg[1])
	if errconv != nil || spd < 0 || spd > 10 {
		return errconv
	}

	c.speed = spd

	c.broadcast(buildMsg("spd", c.session.id, msg[1]))

	return nil
}

func (c *RoomClient) handleSpr(msg []string) error {
	if len(msg) != 3 {
		return errors.New("segment count mismatch")
	}

	if !assets.IsValidSprite(msg[1]) {
		return errors.New("invalid sprite")
	}

	if config.gameName == "2kki" && !isValid2kkiSprite(msg[1], c.room.id) {
		return errors.New("invalid 2kki sprite")
	}

	index, errconv := strconv.Atoi(msg[2])
	if errconv != nil || index < 0 {
		return errconv
	}

	c.session.sprite = msg[1]
	c.session.spriteIndex = index

	c.broadcast(buildMsg("spr", c.session.id, msg[1:]))

	return nil
}

func (c *RoomClient) handleFl(msg []string) error {
	if len(msg) != 6 {
		return errors.New("segment count mismatch")
	}

	red, errconv := strconv.Atoi(msg[1])
	if errconv != nil || red < 0 || red > 255 {
		return errconv
	}
	green, errconv := strconv.Atoi(msg[2])
	if errconv != nil || green < 0 || green > 255 {
		return errconv
	}
	blue, errconv := strconv.Atoi(msg[3])
	if errconv != nil || blue < 0 || blue > 255 {
		return errconv
	}
	power, errconv := strconv.Atoi(msg[4])
	if errconv != nil || power < 0 {
		return errconv
	}
	frames, errconv := strconv.Atoi(msg[5])
	if errconv != nil || frames < 0 {
		return errconv
	}

	if msg[0] == "rfl" {
		c.flash[0] = red
		c.flash[1] = green
		c.flash[2] = blue
		c.flash[3] = power
		c.flash[4] = frames
		c.repeatingFlash = true
	}

	c.broadcast(buildMsg(msg[0], c.session.id, msg[1:]))

	return nil
}

func (c *RoomClient) handleRrfl() (err error) {
	c.repeatingFlash = false
	c.flash = [5]int{}

	c.broadcast(buildMsg("rrfl", c.session.id))

	return nil
}

func (c *RoomClient) handleTr(msg []string) error {
	if len(msg) != 2 {
		return errors.New("segment count mismatch")
	}

	transparency, errconv := strconv.Atoi(msg[1])
	if errconv != nil || transparency < 0 || transparency > 7 {
		return errconv
	}

	c.transparency = transparency

	c.broadcast(buildMsg(msg[0], c.session.id, msg[1]))

	return nil
}

func (c *RoomClient) handleH(msg []string) error {
	if len(msg) != 2 {
		return errors.New("segment count mismatch")
	}

	c.hidden = msg[1] != "0"

	c.broadcast(buildMsg(msg[0], c.session.id, msg[1]))

	return nil
}

func (c *RoomClient) handleSys(msg []string) (err error) {
	if len(msg) != 2 {
		return errors.New("segment count mismatch")
	}

	if !assets.IsValidSystem(msg[1], false) {
		return err
	}

	c.session.system = msg[1]

	c.broadcast(buildMsg("sys", c.session.id, msg[1]))

	return nil
}

func (c *RoomClient) handleSe(msg []string) error {
	if len(msg) != 5 {
		return errors.New("segment count mismatch")
	}

	if !assets.IsValidSound(msg[1]) {
		return errors.New("invalid sound")
	}

	volume, errconv := strconv.Atoi(msg[2])
	if errconv != nil || volume < 0 || volume > 100 {
		return errconv
	}
	tempo, errconv := strconv.Atoi(msg[3])
	if errconv != nil || tempo < 10 || tempo > 400 {
		return errconv
	}
	balance, errconv := strconv.Atoi(msg[4])
	if errconv != nil || balance < 0 || balance > 100 {
		return errconv
	}

	c.broadcast(buildMsg("se", c.session.id, msg[1:]))

	return nil
}

func (c *RoomClient) handleP(msg []string) error {
	isShow := msg[0] == "ap"
	msgLength := 18
	if isShow {
		msgLength = 32
	}
	if len(msg) != msgLength {
		return errors.New("segment count mismatch")
	}

	if isShow {
		c.checkRoomConditions("picture", msg[17])
		if !assets.IsValidPicture(msg[17]) {
			return errors.New("invalid picture")
		}
	}

	id, errconv := strconv.Atoi(msg[1])
	if errconv != nil || id <= 0 || id > maxPictures {
		return errconv
	}

	posX, errconv := strconv.Atoi(msg[2])
	if errconv != nil {
		return errconv
	}
	posY, errconv := strconv.Atoi(msg[3])
	if errconv != nil {
		return errconv
	}
	mapX, errconv := strconv.Atoi(msg[4])
	if errconv != nil {
		return errconv
	}
	mapY, errconv := strconv.Atoi(msg[5])
	if errconv != nil {
		return errconv
	}
	panX, errconv := strconv.Atoi(msg[6])
	if errconv != nil {
		return errconv
	}
	panY, errconv := strconv.Atoi(msg[7])
	if errconv != nil {
		return errconv
	}

	magnify, errconv := strconv.Atoi(msg[8])
	if errconv != nil || magnify < 0 {
		return errconv
	}
	topTrans, errconv := strconv.Atoi(msg[9])
	if errconv != nil || topTrans < 0 {
		return errconv
	}
	bottomTrans, errconv := strconv.Atoi(msg[10])
	if errconv != nil || bottomTrans < 0 {
		return errconv
	}

	red, errconv := strconv.Atoi(msg[11])
	if errconv != nil || red < 0 || red > 200 {
		return errconv
	}
	green, errconv := strconv.Atoi(msg[12])
	if errconv != nil || green < 0 || green > 200 {
		return errconv
	}
	blue, errconv := strconv.Atoi(msg[13])
	if errconv != nil || blue < 0 || blue > 200 {
		return errconv
	}
	saturation, errconv := strconv.Atoi(msg[14])
	if errconv != nil || saturation < 0 || saturation > 200 {
		return errconv
	}

	effectMode, errconv := strconv.Atoi(msg[15])
	if errconv != nil || effectMode < 0 {
		return errconv
	}
	effectPower, errconv := strconv.Atoi(msg[16])
	if errconv != nil {
		return errconv
	}

	var pic *Picture
	if isShow {
		picName := msg[17]
		if picName == "" {
			return errors.New("no pic name")
		}

		pic = &Picture{
			name:                picName,
			useTransparentColor: msg[18] != "0",
			fixedToMap:          msg[19] != "0",
			spritesheetPlayOnce: msg[24] != "0",
			flipX:               msg[29] != "0",
			flipY:               msg[30] != "0",
		}

		if ptr := c.pictures[id-1]; ptr != nil {
			err := c.processMsg("rp" + delim + msg[1])
			if err != nil {
				return err
			}
		}

		spritesheetCols, errconv := strconv.Atoi(msg[20])
		if errconv != nil {
			return errconv
		}
		spritesheetRows, errconv := strconv.Atoi(msg[21])
		if errconv != nil {
			return errconv
		}
		spritesheetFrame, errconv := strconv.Atoi(msg[22])
		if errconv != nil {
			return errconv
		}
		spritesheetSpeed, errconv := strconv.Atoi(msg[23])
		if errconv != nil {
			return errconv
		}
		mapLayer, errconv := strconv.Atoi(msg[25])
		if errconv != nil {
			return errconv
		}
		battleLayer, errconv := strconv.Atoi(msg[26])
		if errconv != nil {
			return errconv
		}
		flags, errconv := strconv.Atoi(msg[27])
		if errconv != nil {
			return errconv
		}
		blendMode, errconv := strconv.Atoi(msg[28])
		if errconv != nil {
			return errconv
		}
		origin, errconv := strconv.Atoi(msg[31])
		if errconv != nil {
			return errconv
		}

		pic.spritesheetCols = spritesheetCols
		pic.spritesheetRows = spritesheetRows
		pic.spritesheetFrame = spritesheetFrame
		pic.spritesheetSpeed = spritesheetSpeed
		pic.mapLayer = mapLayer
		pic.battleLayer = battleLayer
		pic.flags = flags
		pic.blendMode = blendMode
		pic.origin = origin
	} else {
		if ptr := c.pictures[id-1]; ptr != nil {
			duration, errconv := strconv.Atoi(msg[17])
			if errconv != nil || duration < 0 {
				return errconv
			}

			pic = c.pictures[id-1]
		} else {
			return nil
		}
	}

	pic.posX = posX
	pic.posY = posY
	pic.mapX = mapX
	pic.mapY = mapY
	pic.panX = panX
	pic.panY = panY
	pic.magnify = magnify
	pic.topTrans = topTrans
	pic.bottomTrans = bottomTrans
	pic.red = red
	pic.blue = blue
	pic.green = green
	pic.saturation = saturation
	pic.effectMode = effectMode
	pic.effectPower = effectPower

	c.pictures[id-1] = pic

	c.broadcast(buildMsg(msg[0], c.session.id, msg[1:]))

	return nil
}

func (c *RoomClient) handleRp(msg []string) error {
	if len(msg) != 2 {
		return errors.New("segment count mismatch")
	}

	id, errconv := strconv.Atoi(msg[1])
	if errconv != nil || id == 0 || id > maxPictures {
		return errconv
	}

	c.pictures[id-1] = nil

	c.broadcast(buildMsg("rp", c.session.id, msg[1]))

	return nil
}

func (c *RoomClient) handleBa(msg []string) error {
	if len(msg) != 2 {
		return errors.New("segment count mismatch")
	}

	id, errconv := strconv.Atoi(msg[1])
	if errconv != nil {
		return errconv
	}

	if !config.battleAnimIds[id] {
		return errors.New("invalid battle animation id")
	}

	c.broadcast(buildMsg("ba", c.session.id, msg[1]))

	return nil
}

func (c *RoomClient) handleSs(msg []string) error {
	if len(msg) != 3 {
		return errors.New("segment count mismatch")
	}

	switchId, errconv := strconv.Atoi(msg[1])
	if errconv != nil {
		return errconv
	}

	value := msg[2] == "1"

	if config.gameName == "2kki" && c.session.rank == 0 && switchId == 11 && value {
		c.session.cancel()
	}

	c.switchCache[switchId] = value
	if switchId == 1430 && config.gameName == "2kki" { // time trial mode
		if value {
			c.outbox <- buildMsg("sv", 88, 0) // time elapsed
		}
	} else {
		if len(c.room.minigames) != 0 {
			for m, minigame := range c.room.minigames {
				if minigame.Dev && c.session.rank < 1 {
					continue
				}
				if minigame.SwitchId == switchId && minigame.SwitchValue == value && c.minigameScores[m] < c.varCache[minigame.VarId] {
					tryWritePlayerMinigameScore(c.session.uuid, minigame.Id, c.varCache[minigame.VarId])
				}
			}
		}

		for _, condition := range append(globalConditions, c.room.conditions...) {
			validVars := !condition.VarTrigger
			if condition.VarTrigger {
				if condition.VarId > 0 {
					if value, ok := c.varCache[condition.VarId]; ok {
						if validVar, _ := condition.checkVar(condition.VarId, value); validVar {
							validVars = true
						}
					}
				} else if len(condition.VarIds) != 0 {
					validVars = true
					for _, vId := range condition.VarIds {
						if value, ok := c.varCache[vId]; ok {
							if validVar, _ := condition.checkVar(vId, value); !validVar {
								validVars = false
								break
							}
						} else {
							validVars = false
							break
						}
					}
				} else {
					validVars = true
				}
			}

			if validVars {
				if switchId == condition.SwitchId {
					if valid, _ := condition.checkSwitch(switchId, value); valid {
						if condition.VarTrigger || (condition.VarId == 0 && len(condition.VarIds) == 0) {
							if !condition.TimeTrial {
								if c.checkConditionCoords(condition) {
									success, err := tryWritePlayerTag(c.session.uuid, condition.ConditionId)
									if err != nil {
										return err
									}
									if success {
										c.outbox <- buildMsg("b")
									}
								}
							} else if config.gameName == "2kki" {
								c.outbox <- buildMsg("ss", 1430, 0)
							}
						} else {
							varId := condition.VarId
							if len(condition.VarIds) != 0 {
								varId = condition.VarIds[0]
							}
							c.outbox <- buildMsg("sv", varId, 0)
						}
					}
				} else if len(condition.SwitchIds) != 0 {
					if valid, s := condition.checkSwitch(switchId, value); valid {
						if s == len(condition.SwitchIds)-1 {
							if condition.VarTrigger || (condition.VarId == 0 && len(condition.VarIds) == 0) {
								if !condition.TimeTrial {
									if c.checkConditionCoords(condition) {
										success, err := tryWritePlayerTag(c.session.uuid, condition.ConditionId)
										if err != nil {
											return err
										}
										if success {
											c.outbox <- buildMsg("b")
										}
									}
								} else if config.gameName == "2kki" {
									c.outbox <- buildMsg("ss", 1430, 0)
								}
							} else {
								varId := condition.VarId
								if len(condition.VarIds) != 0 {
									varId = condition.VarIds[0]
								}
								c.outbox <- buildMsg("sv", varId, 0)
							}
						} else {
							c.outbox <- buildMsg("ss", condition.SwitchIds[s+1], 0)
						}
					}
				}
			}
		}
	}

	return nil
}

func (c *RoomClient) handleSv(msg []string) error {
	if len(msg) != 3 {
		return errors.New("segment count mismatch")
	}

	varId, errconv := strconv.Atoi(msg[1])
	if errconv != nil {
		return errconv
	}
	value, errconv := strconv.Atoi(msg[2])
	if errconv != nil {
		return errconv
	}
	c.varCache[varId] = value

	conditions := append(globalConditions, c.room.conditions...)

	if varId == 88 && config.gameName == "2kki" {
		for _, condition := range conditions {
			if condition.TimeTrial && value < 3600 {
				if c.checkConditionCoords(condition) {
					success, err := tryWritePlayerTimeTrial(c.session.uuid, c.room.id, value)
					if err != nil {
						return err
					}
					if success {
						c.outbox <- buildMsg("b")
					}
				}
			}
		}
	} else {
		if len(c.room.minigames) != 0 {
			for m, minigame := range c.room.minigames {
				if minigame.Dev && c.session.rank < 1 {
					continue
				}
				if minigame.VarId == varId && c.minigameScores[m] < value {
					if minigame.SwitchId > 0 {
						c.outbox <- buildMsg("ss", minigame.SwitchId, 0)
					} else {
						tryWritePlayerMinigameScore(c.session.uuid, minigame.Id, value)
					}
				}
			}
		}

		for _, condition := range conditions {
			validSwitches := condition.VarTrigger
			if !condition.VarTrigger {
				if condition.SwitchId > 0 {
					if value, ok := c.switchCache[condition.SwitchId]; ok {
						if validSwitch, _ := condition.checkSwitch(condition.SwitchId, value); validSwitch {
							validSwitches = true
						}
					}
				} else if len(condition.SwitchIds) != 0 {
					validSwitches = true
					for _, sId := range condition.SwitchIds {
						if value, ok := c.switchCache[sId]; ok {
							if validSwitch, _ := condition.checkSwitch(sId, value); !validSwitch {
								validSwitches = false
								break
							}
						} else {
							validSwitches = false
							break
						}
					}
				} else {
					validSwitches = true
				}
			}

			if validSwitches {
				if varId == condition.VarId {
					if valid, _ := condition.checkVar(varId, value); valid {
						if !condition.VarTrigger || (condition.SwitchId == 0 && len(condition.SwitchIds) == 0) {
							if !condition.TimeTrial {
								if c.checkConditionCoords(condition) {
									success, err := tryWritePlayerTag(c.session.uuid, condition.ConditionId)
									if err != nil {
										return err
									}
									if success {
										c.outbox <- buildMsg("b")
									}
								}
							} else if config.gameName == "2kki" {
								c.outbox <- buildMsg("ss", 1430, 0)
							}
						} else {
							switchId := condition.SwitchId
							if len(condition.SwitchIds) != 0 {
								switchId = condition.SwitchIds[0]
							}
							c.outbox <- buildMsg("ss", switchId, 0)
						}
					}
				} else if len(condition.VarIds) != 0 {
					if valid, v := condition.checkVar(varId, value); valid {
						if v == len(condition.VarIds)-1 {
							if !condition.VarTrigger || (condition.SwitchId == 0 && len(condition.SwitchIds) == 0) {
								if !condition.TimeTrial {
									if c.checkConditionCoords(condition) {
										success, err := tryWritePlayerTag(c.session.uuid, condition.ConditionId)
										if err != nil {
											return err
										}
										if success {
											c.outbox <- buildMsg("b")
										}
									}
								} else if config.gameName == "2kki" {
									c.outbox <- buildMsg("ss", 1430, 0)
								}
							} else {
								switchId := condition.SwitchId
								if len(condition.SwitchIds) != 0 {
									switchId = condition.SwitchIds[0]
								}
								c.outbox <- buildMsg("ss", switchId, 0)
							}
						} else {
							c.outbox <- buildMsg("sv", condition.VarIds[v+1], 0)
						}
					}
				}
			}
		}
	}

	return nil
}

func (c *RoomClient) handleSev(msg []string) error {
	if len(msg) != 3 {
		return errors.New("segment count mismatch")
	}

	triggerType := "event"
	if msg[2] != "0" {
		triggerType = "eventAction"
	}
	c.checkRoomConditions(triggerType, msg[1])

	if c.room.id != currentEventVmMapId {
		return errors.New("event vm room id mismatch")
	}

	eventIdInt, err := strconv.Atoi(msg[1])
	if err != nil {
		return err
	}

	if currentEventVmEventId != eventIdInt {
		return errors.New("event vm id mismatch")
	}

	exp, err := tryCompleteEventVm(c.session.uuid, currentEventVmMapId, currentEventVmEventId)
	if err != nil {
		return err
	}
	if exp > -1 {
		c.session.outbox <- buildMsg("vm", exp)
	}

	return nil
}

// SESSION

func (c *SessionClient) handleI() error {
	badgeSlotRows, badgeSlotCols := getPlayerBadgeSlotCounts(c.name)
	screenshotLimit := getPlayerScreenshotLimit(c.name)
	playerInfoJson, err := json.Marshal(PlayerInfo{
		Uuid:            c.uuid,
		Name:            c.name,
		Rank:            c.rank,
		Badge:           c.badge,
		BadgeSlotRows:   badgeSlotRows,
		BadgeSlotCols:   badgeSlotCols,
		ScreenshotLimit: screenshotLimit,
		Medals:          getPlayerMedals(c.uuid),
	})
	if err != nil {
		return err
	}

	c.outbox <- buildMsg("i", playerInfoJson)

	return nil
}

func (c *SessionClient) handleName(msg []string) error {
	if len(msg) != 2 {
		return errors.New("segment count mismatch")
	}

	maxNameLength := 10
	if c.account {
		maxNameLength = 12
	}

	if c.name != "" || !isOkString(msg[1]) || len(msg[1]) > maxNameLength {
		return errors.New("invalid name")
	}

	c.name = msg[1]

	if c.roomC != nil {
		c.roomC.broadcast(buildMsg("name", c.id, c.name)) // broadcast name change to room if client is in one
	}

	return nil
}

func (c *SessionClient) handlePloc(msg []string) error {
	if c.roomC == nil {
		return errors.New("room client does not exist")
	}

	if len(msg) != 3 {
		return errors.New("segment count mismatch")
	}

	if len(msg[1]) != 4 {
		return errors.New("invalid prev map id")
	}

	c.roomC.prevMapId = msg[1]
	c.roomC.prevLocations = msg[2]

	c.roomC.checkRoomConditions("prevMap", c.roomC.prevMapId)

	return nil
}

func (c *SessionClient) handleLcol(msg []string) error {
	if c.roomC == nil {
		return errors.New("room client does not exist")
	}

	if len(msg) != 2 {
		return errors.New("segment count mismatch")
	}

	locationName := msg[1]

	if locationColors, ok := gameLocationColors[locationName]; ok {
		c.outbox <- buildMsg("lcol", locationColors[0], locationColors[1])
		return nil
	}

	c.outbox <- buildMsg("lcol", "", "")

	return nil
}

func (c *SessionClient) handleSay(msg []string) error {
	if c.roomC == nil {
		return errors.New("room client does not exist")
	}

	if c.muted {
		return errors.New("player is muted")
	}

	if len(msg) != 2 {
		return errors.New("segment count mismatch")
	}

	if c.name == "" || c.system == "" {
		return errors.New("no name or system graphic set")
	}

	msgContents := wordFilter.ReplaceAllString(strings.TrimSpace(msg[1]), ":2kkiSign:")
	if msgContents == "" || len(msgContents) > 150 {
		return errors.New("invalid message")
	}

	for _, client := range c.roomC.room.clients {
		if client.session == c {
			continue
		}

		if client.session.blockedUsers[c.uuid] || c.blockedUsers[client.session.uuid] {
			continue
		}

		if (client.session.private || c.private) && ((c.partyId == 0 || client.session.partyId != c.partyId) && !client.session.onlineFriends[c.uuid]) {
			continue
		}

		client.session.outbox <- buildMsg("say", c.uuid, msgContents)
	}

	// so local echo appears
	c.outbox <- buildMsg("say", c.uuid, msgContents)

	return nil
}

func (c *SessionClient) handleGPSay(msg []string) error {
	if c.muted {
		return errors.New("player is muted")
	}

	if len(msg) != 2 {
		return errors.New("segment count mismatch")
	}

	if c.name == "" {
		return errors.New("no name set")
	}

	msgContents := wordFilter.ReplaceAllString(strings.TrimSpace(msg[1]), ":2kkiSign:")
	if msgContents == "" || len(msgContents) > 150 {
		return errors.New("invalid message")
	}

	if msg[0] == "psay" && c.partyId == 0 {
		return errors.New("player not in a party")
	}

	mapId := "0000"
	prevMapId := "0000"
	prevLocations := ""
	x := -1
	y := -1

	if c.roomC != nil && !c.hideLocation {
		mapId = c.roomC.mapId
		prevMapId = c.roomC.prevMapId
		prevLocations = c.roomC.prevLocations
		x = c.roomC.x
		y = c.roomC.y
	}

	msgId := randString(12)

	if msg[0] == "gsay" {
		c.broadcast(buildMsg("p", c.uuid, c.name, c.system, c.rank, c.account, c.badge, c.medals[:]))
		c.broadcast(buildMsg("gsay", c.uuid, mapId, prevMapId, prevLocations, x, y, msgContents, msgId))

		err := writeGlobalChatMessage(msgId, c.uuid, mapId, prevMapId, prevLocations, x, y, msgContents)
		if err != nil {
			return err
		}

		if c.account && config.chatWebhook != "" {
			game := config.gameName
			if gameName, ok := gameIdToName[game]; ok {
				game = gameName
			}

			err = sendWebhookMessage(config.chatWebhook, fmt.Sprintf("%s (%s)", c.name, game), c.badge, msgContents, true)
			if err != nil {
				return err
			}
		}
	} else {
		for _, client := range clients.Get() {
			if client.partyId == c.partyId {
				client.outbox <- buildMsg("psay", c.uuid, msgContents, msgId)
			}
		}

		err := writePartyChatMessage(msgId, c.uuid, mapId, prevMapId, prevLocations, x, y, msgContents, c.partyId)
		if err != nil {
			return err
		}
	}

	return nil
}

func (c *SessionClient) handleL(msg []string) error {
	if c.roomC == nil {
		return errors.New("room client does not exist")
	}

	if len(c.roomC.locationIds) > 0 {
		c.roomC.locations = []string{}
		c.roomC.locationIds = []int{}
	}

	var locationIds []int

	for i, locationName := range msg {
		if i == 0 {
			continue
		}

		gameLocation, err := getGameLocationByName(locationName)
		if err != nil {
			writeLog(c.uuid, "sess", err.Error(), 200)
			continue
		}

		duplicateLocation := false

		for _, l := range c.roomC.locationIds {
			if l == gameLocation.Id {
				duplicateLocation = true
				break
			}
		}

		if duplicateLocation {
			continue
		}

		locationIds = append(locationIds, gameLocation.Id)
		c.roomC.locationIds = append(c.roomC.locationIds, gameLocation.Id)

		var matchedLocationMap bool

		for _, mapId := range gameLocation.MapIds {
			if mapId == c.roomC.mapId {
				matchedLocationMap = true
				break
			}
		}

		if matchedLocationMap {
			writePlayerGameLocation(c.uuid, locationName)
			c.roomC.locations = append(c.roomC.locations, locationName)
		}
	}

	c.outbox <- buildMsg("l", locationIds)

	return nil
}

func (c *SessionClient) handleNl(msg []string) error {
	if c.roomC == nil {
		return errors.New("room client does not exist")
	}

	if len(msg) != 2 {
		return errors.New("segment count mismatch")
	}

	destLocationId, err := strconv.Atoi(msg[1])
	if err != nil || destLocationId < 0 {
		return errors.New("invalid destination location id")
	}

	destLocationName, err := getLocationName(destLocationId)
	if err != nil {
		return fmt.Errorf("invalid destination location: %s", err)
	} else if destLocationName == "" {
		return errors.New("invalid destination location")
	}

	if len(c.roomC.locations) == 0 {
		return errors.New("player location unknown")
	}

	nextLocations, err := getNext2kkiLocations(c.roomC.locations[0], destLocationName)
	if err != nil {
		return fmt.Errorf("invalid next locations for %s -> %s: %s", c.roomC.locations[0], destLocationName, err)
	}

	nextLocationsJson, err := json.Marshal(nextLocations.Locations)
	if err != nil {
		return fmt.Errorf("error while marshaling: %s", err)
	}

	c.outbox <- buildMsg("nl", nextLocationsJson)

	return nil
}

func (c *SessionClient) handleLp() error {
	c.outbox <- buildMsg("lp", locationPlayerCountsPayload)

	return nil
}

func (c *SessionClient) handlePf() error {
	playerFriendData, err := getPlayerFriendData(c.uuid)
	if err != nil {
		return err
	}
	playerFriendDataJson, err := json.Marshal(playerFriendData)
	if err != nil {
		return err
	}

	c.outbox <- buildMsg("pf", playerFriendDataJson)

	return nil
}

func (c *SessionClient) handlePt() error {
	if c.partyId == 0 {
		return errors.New("player not in a party")
	}
	partyData, err := getPartyData(c.partyId)
	if err != nil {
		return err
	}
	partyDataJson, err := json.Marshal(partyData)
	if err != nil {
		return err
	}

	c.outbox <- buildMsg("pt", partyDataJson)

	return nil
}

func (c *SessionClient) handleEp() error {
	period, err := getCurrentEventPeriodData()
	if err != nil {
		return err
	}
	periodJson, err := json.Marshal(period)
	if err != nil {
		return err
	}

	c.outbox <- buildMsg("ep", periodJson)

	return nil
}

func (c *SessionClient) handleE() error {
	currentEventLocationsData, err := getCurrentPlayerEventLocationsData(c.uuid)
	if err != nil {
		return err
	}
	var hasIncompleteEvent bool
	for _, currentEventLocation := range currentEventLocationsData {
		if !currentEventLocation.Complete && currentEventLocation.Game == config.gameName {
			hasIncompleteEvent = true
			break
		}
	}
	if !hasIncompleteEvent {
		if config.gameName == "2kki" {
			addPlayer2kkiEventLocation(currentGameEventPeriodId, -1, freeEventLocationMinDepth, 0, 0, c.uuid)
		} else if len(freeEventLocationPool) > 0 {
			addPlayerEventLocation(config.gameName, -1, 0, freeEventLocationPool, c.uuid)
		}
		currentEventLocationsData, err = getCurrentPlayerEventLocationsData(c.uuid)
		if err != nil {
			return err
		}
	}

	currentEventVmsData, err := getCurrentPlayerEventVmsData(c.uuid)
	if err != nil {
		return err
	}

	eventsData := &EventsData{
		Locations: currentEventLocationsData,
		Vms:       currentEventVmsData,
	}

	eventsDataJson, err := json.Marshal(eventsData)
	if err != nil {
		return err
	}

	c.outbox <- buildMsg("e", eventsDataJson)

	return nil
}

func (c *SessionClient) handleEexp() error {
	if currentGameEventPeriodId <= 0 {
		return errors.New("events are disabled")
	}

	playerEventExpData, err := getPlayerEventExpData(c.uuid)
	if err != nil {
		return err
	}
	playerEventExpDataJson, err := json.Marshal(playerEventExpData)
	if err != nil {
		return err
	}

	c.outbox <- buildMsg("eexp", playerEventExpDataJson)

	return nil
}

func (c *SessionClient) handleEec(msg []string) error {
	if currentGameEventPeriodId <= 0 {
		c.outbox <- buildMsg("eec", 0, false)
		return errors.New("events are disabled")
	}

	if len(msg) < 3 {
		c.outbox <- buildMsg("eec", 0, false)
		return errors.New("segment count mismatch")
	}

	location := msg[1]
	if len(location) == 0 {
		c.outbox <- buildMsg("eec", 0, false)
		return errors.New("location not specified")
	}

	exp := -1
	if c.roomC != nil {
		if msg[2] != "1" { // not free expedition
			expV, err := tryCompleteEventLocation(c.uuid, location)
			if err != nil {
				c.outbox <- buildMsg("eec", 0, false)
				return err
			}
			if expV < 0 {
				c.outbox <- buildMsg("eec", 0, false)
				return errors.New("unexpected state")
			}
			exp = expV
		} else { // free expedition
			complete, err := tryCompletePlayerEventLocation(c.uuid, location)
			if err != nil {
				c.outbox <- buildMsg("eec", 0, false)
				return err
			}
			if complete {
				exp = 0
			}
		}
	}
	currentEventLocationsData, err := getCurrentPlayerEventLocationsData(c.uuid)
	if err != nil {
		c.outbox <- buildMsg("eec", 0, false)
		return err
	}
	var hasIncompleteEvent bool
	for _, currentEventLocation := range currentEventLocationsData {
		if !currentEventLocation.Complete && currentEventLocation.Game == config.gameName {
			hasIncompleteEvent = true
			break
		}
	}
	if !hasIncompleteEvent {
		if config.gameName == "2kki" {
			addPlayer2kkiEventLocation(currentGameEventPeriodId, -1, freeEventLocationMinDepth, 0, 0, c.uuid)
		} else if len(freeEventLocationPool) > 0 {
			addPlayerEventLocation(config.gameName, -1, 0, freeEventLocationPool, c.uuid)
		}
	}

	c.outbox <- buildMsg("eec", exp, true)

	return nil
}

func (c *SessionClient) handlePsi(uuid string, msg []string) error {
	if len(msg) != 3 {
		return errors.New("segment count mismatch")
	}

	screenshotInfo, err := getScreenshotInfo(uuid, msg[1], msg[2])
	if err != nil {
		return err
	}

	if screenshotInfo != nil {
		screenshotInfoJson, err := json.Marshal(screenshotInfo)
		if err != nil {
			return err
		}

		c.outbox <- buildMsg("psi", screenshotInfoJson)
	}

	return nil
}

func (c *SessionClient) handlePr(msg []string) error {
	if len(msg) != 2 {
		return errors.New("segment count mismatch")
	}

	c.private = msg[1] == "1"

	return nil
}

func (c *SessionClient) handleHl(msg []string) error {
	if len(msg) != 2 {
		return errors.New("segment count mismatch")
	}

	c.hideLocation = msg[1] == "1"

	return nil
}
