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

import (
	"encoding/json"
	"errors"
	"strconv"
	"strings"
)

func (c *RoomClient) handleSr(msg []string) (err error) {
	if len(msg) != 2 {
		return errors.New("command length mismatch")
	}

	roomId, errconv := strconv.Atoi(msg[1])
	if errconv != nil {
		return errconv
	}

	room, ok := rooms[roomId]
	if !ok {
		return err
	}

	c.leaveRoom()
	c.joinRoom(room)

	return nil
}

func (c *RoomClient) handleM(msg []string) (err error) {
	if len(msg) != 3 {
		return errors.New("command length mismatch")
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

	if msg[0] == "m" && !(c.y-y < 1 || c.x-x > 1 || c.y-y > 1 || c.x-x < 1) {
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

	c.broadcast(buildMsg("m", c.sClient.id, msg[1:])) // user %id% moved to x y

	return nil
}

func (c *RoomClient) handleJmp(msg []string) (err error) {
	if len(msg) != 3 {
		return errors.New("command length mismatch")
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

	c.x = x
	c.y = y

	if c.syncCoords {
		c.checkRoomConditions("coords", "")
	}

	c.broadcast(buildMsg("jmp", c.sClient.id, msg[1:])) // user %id% jumped to x y

	return nil
}

func (c *RoomClient) handleF(msg []string) (err error) {
	if len(msg) != 2 {
		return errors.New("command length mismatch")
	}

	// check if direction is valid
	facing, errconv := strconv.Atoi(msg[1])
	if errconv != nil || facing < 0 || facing > 3 {
		return errconv
	}

	c.facing = facing

	c.broadcast(buildMsg("f", c.sClient.id, msg[1])) // user %id% facing changed to f

	return nil
}

func (c *RoomClient) handleSpd(msg []string) (err error) {
	if len(msg) != 2 {
		return errors.New("command length mismatch")
	}

	spd, errconv := strconv.Atoi(msg[1])
	if errconv != nil || spd < 0 || spd > 10 {
		return errconv
	}

	c.spd = spd

	c.broadcast(buildMsg("spd", c.sClient.id, msg[1]))

	return nil
}

func (c *RoomClient) handleSpr(msg []string) (err error) {
	if len(msg) != 3 {
		return errors.New("command length mismatch")
	}

	if !gameAssets.IsValidSprite(msg[1]) {
		return err
	}

	if serverConfig.GameName == "2kki" && !gameAssets.IsValid2kkiSprite(msg[1], c.room.id) {
		return err
	}

	index, errconv := strconv.Atoi(msg[2])
	if errconv != nil || index < 0 {
		return errconv
	}

	c.sClient.spriteName = msg[1]
	c.sClient.spriteIndex = index

	c.broadcast(buildMsg("spr", c.sClient.id, msg[1:]))

	return nil
}

func (c *RoomClient) handleFl(msg []string) (err error) {
	if len(msg) != 6 {
		return errors.New("command length mismatch")
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

	c.broadcast(buildMsg(msg[0], c.sClient.id, msg[1:]))

	return nil
}

func (c *RoomClient) handleRrfl() (err error) {
	c.repeatingFlash = false

	for i := 0; i < 5; i++ {
		c.flash[i] = 0
	}

	c.broadcast(buildMsg("rrfl", c.sClient.id))

	return nil
}

func (c *RoomClient) handleH(msg []string) (err error) {
	if len(msg) != 2 {
		return errors.New("command length mismatch")
	}

	hiddenBin, errconv := strconv.Atoi(msg[1])
	if errconv != nil || hiddenBin < 0 || hiddenBin > 1 {
		return errconv
	}

	c.hidden = hiddenBin == 1

	c.broadcast(buildMsg(msg[0], c.sClient.id, msg[1]))

	return nil
}

func (c *RoomClient) handleSys(msg []string) (err error) {
	if len(msg) != 2 {
		return errors.New("command length mismatch")
	}

	if !gameAssets.IsValidSystem(msg[1], false) {
		return err
	}

	c.sClient.systemName = msg[1]

	c.broadcast(buildMsg("sys", c.sClient.id, msg[1]))

	return nil
}

func (c *RoomClient) handleSe(msg []string) (err error) {
	if len(msg) != 5 {
		return errors.New("command length mismatch")
	}

	if !gameAssets.IsValidSound(msg[1]) {
		return err
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

	c.broadcast(buildMsg("se", c.sClient.id, msg[1:]))

	return nil
}

func (c *RoomClient) handleP(msg []string) (err error) {
	isShow := msg[0] == "ap"
	msgLength := 18
	if isShow {
		msgLength = 20
	}
	if len(msg) != msgLength {
		return errors.New("command length mismatch")
	}

	if isShow {
		c.checkRoomConditions("picture", msg[17])
		if !gameAssets.IsValidPicture(msg[17]) {
			return err
		}
	}

	picId, errconv := strconv.Atoi(msg[1])
	if errconv != nil || picId == 0 {
		return errconv
	}

	positionX, errconv := strconv.Atoi(msg[2])
	if errconv != nil {
		return errconv
	}
	positionY, errconv := strconv.Atoi(msg[3])
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
			return err
		}

		useTransparentColorBin, errconv := strconv.Atoi(msg[18])
		if errconv != nil || useTransparentColorBin < 0 || useTransparentColorBin > 1 {
			return errconv
		}

		fixedToMapBin, errconv := strconv.Atoi(msg[19])
		if errconv != nil || fixedToMapBin < 0 || fixedToMapBin > 1 {
			return errconv
		}

		pic = &Picture{
			name:                picName,
			useTransparentColor: useTransparentColorBin == 1,
			fixedToMap:          fixedToMapBin == 1,
		}

		if _, found := c.pictures[picId]; found {
			rpErr := c.processMsg("rp" + delim + msg[1])
			if rpErr != nil {
				return rpErr
			}
		}
	} else {
		if _, found := c.pictures[picId]; found {
			duration, errconv := strconv.Atoi(msg[17])
			if errconv != nil || duration < 0 {
				return errconv
			}

			pic = c.pictures[picId]
		} else {
			return nil
		}
	}

	pic.positionX = positionX
	pic.positionY = positionY
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

	c.pictures[picId] = pic

	c.broadcast(buildMsg(msg[0], c.sClient.id, msg[1:]))

	return nil
}

func (c *RoomClient) handleRp(msg []string) (err error) {
	if len(msg) != 2 {
		return errors.New("command length mismatch")
	}

	picId, errconv := strconv.Atoi(msg[1])
	if errconv != nil || picId == 0 {
		return errconv
	}

	delete(c.pictures, picId)

	c.broadcast(buildMsg("rp", c.sClient.id, msg[1]))

	return nil
}

func (c *RoomClient) handleBa(msg []string) (err error) {
	if len(msg) != 2 {
		return errors.New("command length mismatch")
	}

	id, errconv := strconv.Atoi(msg[1])
	if errconv != nil {
		return errconv
	}

	if !gameAssets.BattleAnimIds[id] {
		return err
	}

	c.broadcast(buildMsg("ba", c.sClient.id, msg[1]))

	return nil
}

func (c *RoomClient) handleSay(msg []string) (err error) {
	if c.sClient.muted {
		return nil
	}

	if len(msg) != 2 {
		return errors.New("command length mismatch")
	}

	if c.sClient.name == "" || c.sClient.systemName == "" {
		return errors.New("no name or system graphic set")
	}

	msgContents := strings.TrimSpace(msg[1])
	if msgContents == "" || len(msgContents) > 150 {
		return errors.New("invalid message")
	}

	c.broadcast(buildMsg("say", c.sClient.id, msgContents))

	return nil
}

func (c *RoomClient) handleSs(msg []string) (err error) {
	if len(msg) != 3 {
		return errors.New("command length mismatch")
	}

	switchId, errconv := strconv.Atoi(msg[1])
	if errconv != nil {
		return errconv
	}
	valueBin, errconv := strconv.Atoi(msg[2])
	if errconv != nil || valueBin < 0 || valueBin > 1 {
		return errconv
	}

	var value bool
	if valueBin == 1 {
		value = true
	}
	c.switchCache[switchId] = value
	if switchId == 1430 && serverConfig.GameName == "2kki" { // time trial mode
		if value {
			c.send <- buildMsg("sv", "88", "0") // time elapsed
		}
	} else {
		if len(c.room.minigames) != 0 {
			for m, minigame := range c.room.minigames {
				if minigame.Dev && c.sClient.rank < 1 {
					continue
				}
				if minigame.SwitchId == switchId && minigame.SwitchValue == value && c.minigameScores[m] < c.varCache[minigame.VarId] {
					tryWritePlayerMinigameScore(c.sClient.uuid, minigame.Id, c.varCache[minigame.VarId])
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
									success, err := tryWritePlayerTag(c.sClient.uuid, condition.ConditionId)
									if err != nil {
										return err
									}
									if success {
										c.send <- buildMsg("b")
									}
								}
							} else if serverConfig.GameName == "2kki" {
								c.send <- buildMsg("ss", "1430", "0")
							}
						} else {
							varId := condition.VarId
							if len(condition.VarIds) != 0 {
								varId = condition.VarIds[0]
							}
							c.send <- buildMsg("sv", varId, "0")
						}
					}
				} else if len(condition.SwitchIds) != 0 {
					if valid, s := condition.checkSwitch(switchId, value); valid {
						if s == len(condition.SwitchIds)-1 {
							if condition.VarTrigger || (condition.VarId == 0 && len(condition.VarIds) == 0) {
								if !condition.TimeTrial {
									if c.checkConditionCoords(condition) {
										success, err := tryWritePlayerTag(c.sClient.uuid, condition.ConditionId)
										if err != nil {
											return err
										}
										if success {
											c.send <- buildMsg("b")
										}
									}
								} else if serverConfig.GameName == "2kki" {
									c.send <- buildMsg("ss", "1430", "0")
								}
							} else {
								varId := condition.VarId
								if len(condition.VarIds) != 0 {
									varId = condition.VarIds[0]
								}
								c.send <- buildMsg("sv", varId, "0")
							}
						} else {
							c.send <- buildMsg("ss", condition.SwitchIds[s+1], "0")
						}
					}
				}
			}
		}
	}

	return nil
}

func (c *RoomClient) handleSv(msg []string) (err error) {
	if len(msg) != 3 {
		return errors.New("command length mismatch")
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

	if varId == 88 && serverConfig.GameName == "2kki" {
		for _, condition := range conditions {
			if condition.TimeTrial && value < 3600 {
				if c.checkConditionCoords(condition) {
					success, err := tryWritePlayerTimeTrial(c.sClient.uuid, c.room.id, value)
					if err != nil {
						return err
					}
					if success {
						c.send <- buildMsg("b")
					}
				}
			}
		}
	} else {
		if len(c.room.minigames) != 0 {
			for m, minigame := range c.room.minigames {
				if minigame.Dev && c.sClient.rank < 1 {
					continue
				}
				if minigame.VarId == varId && c.minigameScores[m] < value {
					if minigame.SwitchId > 0 {
						c.send <- buildMsg("ss", minigame.SwitchId, "0")
					} else {
						tryWritePlayerMinigameScore(c.sClient.uuid, minigame.Id, value)
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
									success, err := tryWritePlayerTag(c.sClient.uuid, condition.ConditionId)
									if err != nil {
										return err
									}
									if success {
										c.send <- buildMsg("b")
									}
								}
							} else if serverConfig.GameName == "2kki" {
								c.send <- buildMsg("ss", "1430", "0")
							}
						} else {
							switchId := condition.SwitchId
							if len(condition.SwitchIds) != 0 {
								switchId = condition.SwitchIds[0]
							}
							c.send <- buildMsg("ss", switchId, "0")
						}
					}
				} else if len(condition.VarIds) != 0 {
					if valid, v := condition.checkVar(varId, value); valid {
						if v == len(condition.VarIds)-1 {
							if !condition.VarTrigger || (condition.SwitchId == 0 && len(condition.SwitchIds) == 0) {
								if !condition.TimeTrial {
									if c.checkConditionCoords(condition) {
										success, err := tryWritePlayerTag(c.sClient.uuid, condition.ConditionId)
										if err != nil {
											return err
										}
										if success {
											c.send <- buildMsg("b")
										}
									}
								} else if serverConfig.GameName == "2kki" {
									c.send <- buildMsg("ss", "1430", "0")
								}
							} else {
								switchId := condition.SwitchId
								if len(condition.SwitchIds) != 0 {
									switchId = condition.SwitchIds[0]
								}
								c.send <- buildMsg("ss", switchId, "0")
							}
						} else {
							c.send <- buildMsg("sv", condition.VarIds[v+1], "0")
						}
					}
				}
			}
		}
	}

	return nil
}

func (c *RoomClient) handleSev(msg []string) (err error) {
	if len(msg) != 3 {
		return errors.New("command length mismatch")
	}

	actionBin, errconv := strconv.Atoi(msg[2])
	if errconv != nil || actionBin < 0 || actionBin > 1 {
		return errconv
	}

	triggerType := "event"
	if actionBin == 1 {
		triggerType = "eventAction"
	}
	c.checkRoomConditions(triggerType, msg[1])

	if c.room.id != currentEventVmMapId {
		return err
	}

	eventIdInt, err := strconv.Atoi(msg[1])
	if err != nil {
		return err
	}

	if currentEventVmEventId != eventIdInt {
		return err
	}

	exp, err := tryCompleteEventVm(c.sClient.uuid, currentEventVmMapId, currentEventVmEventId)
	if err != nil {
		return err
	}
	if exp > -1 {
		c.sClient.send <- buildMsg("vm", exp)
	}

	return nil
}

// SESSION

func (c *SessionClient) handleI() (err error) {
	badgeSlotRows, badgeSlotCols := getPlayerBadgeSlotCounts(c.name)
	playerInfoJson, err := json.Marshal(PlayerInfo{
		Uuid:          c.uuid,
		Name:          c.name,
		Rank:          c.rank,
		Badge:         c.badge,
		BadgeSlotRows: badgeSlotRows,
		BadgeSlotCols: badgeSlotCols,
		Medals:        getPlayerMedals(c.uuid),
	})
	if err != nil {
		return err
	}

	c.send <- buildMsg("i", playerInfoJson)

	return nil
}

func (c *SessionClient) handleName(msg []string) (err error) {
	if len(msg) != 2 {
		return errors.New("command length mismatch")
	}

	maxNameLength := 10
	if c.account {
		maxNameLength = 12
	}

	if c.name != "" || !isOkString(msg[1]) || len(msg[1]) > maxNameLength {
		return err
	}

	c.name = msg[1]

	if c.rClient != nil {
		c.rClient.broadcast(buildMsg("name", c.id, c.name)) // broadcast name change to room if client is in one
	}

	return nil
}

func (c *SessionClient) handlePloc(msg []string) (err error) {
	if c.rClient == nil {
		return err
	}

	if len(msg) != 3 {
		return errors.New("command length mismatch")
	}

	if len(msg[1]) != 4 {
		return errors.New("invalid prev map id")
	}

	c.rClient.prevMapId = msg[1]
	c.rClient.prevLocations = msg[2]

	c.rClient.checkRoomConditions("prevMap", c.rClient.prevMapId)

	return nil
}

func (c *SessionClient) handleGSay(msg []string) (err error) {
	if c.muted {
		return errors.New("player is muted")
	}

	if len(msg) != 3 {
		return errors.New("command length mismatch")
	}

	if c.name == "" || c.systemName == "" {
		return errors.New("no name or system graphic set")
	}

	msgContents := strings.TrimSpace(msg[1])
	if msgContents == "" || len(msgContents) > 150 {
		return errors.New("invalid message")
	}

	enableLocBin, errconv := strconv.Atoi(msg[2])
	if errconv != nil || enableLocBin < 0 || enableLocBin > 1 {
		return errconv
	}

	mapId := "0000"
	prevMapId := "0000"
	prevLocations := ""
	x := -1
	y := -1

	if c.rClient != nil && enableLocBin == 1 {
		mapId = c.rClient.mapId
		prevMapId = c.rClient.prevMapId
		prevLocations = c.rClient.prevLocations
		x = c.rClient.x
		y = c.rClient.y
	}

	msgId := randString(8)

	c.broadcast(buildMsg("p", c.uuid, c.name, c.systemName, c.rank, c.account, c.badge, c.medals[:]))
	c.broadcast(buildMsg("gsay", c.uuid, mapId, prevMapId, prevLocations, x, y, msgContents, msgId))

	err = writeGlobalChatMessage(msgId, c.uuid, mapId, prevMapId, prevLocations, x, y, msgContents)
	if err != nil {
		return err
	}

	return nil
}

func (c *SessionClient) handlePSay(msg []string) (err error) {
	if c.muted {
		return errors.New("player is muted")
	}

	if len(msg) != 2 {
		return errors.New("command length mismatch")
	}

	if c.name == "" || c.systemName == "" {
		return errors.New("no name or system graphic set")
	}

	msgContents := strings.TrimSpace(msg[1])
	if msgContents == "" || len(msgContents) > 150 {
		return errors.New("invalid message")
	}

	partyId, err := getPlayerPartyId(c.uuid)
	if err != nil {
		return err
	}
	if partyId == 0 {
		return errors.New("player not in a party")
	}
	partyMemberUuids, err := getPartyMemberUuids(partyId)
	if err != nil {
		return err
	}

	msgId := randString(8)

	for _, uuid := range partyMemberUuids {
		if client, ok := clients.Load(uuid); ok {
			client.(*SessionClient).send <- buildMsg("psay", c.uuid, msgContents, msgId)
		}
	}

	mapId := "0000"
	prevMapId := "0000"
	prevLocations := ""
	x := -1
	y := -1

	if c.rClient != nil {
		mapId = c.rClient.mapId
		prevMapId = c.rClient.prevMapId
		prevLocations = c.rClient.prevLocations
		x = c.rClient.x
		y = c.rClient.y
	}

	err = writePartyChatMessage(msgId, c.uuid, mapId, prevMapId, prevLocations, x, y, msgContents, partyId)
	if err != nil {
		return err
	}

	return nil
}

func (c *SessionClient) handlePt() (err error) {
	partyId, err := getPlayerPartyId(c.uuid)
	if err != nil {
		return err
	}
	if partyId == 0 {
		return errors.New("player not in a party")
	}
	partyData, err := getPartyData(c.uuid)
	if err != nil {
		return err
	}
	if c.uuid != partyData.OwnerUuid {
		partyData.Pass = ""
	}
	partyDataJson, err := json.Marshal(partyData)
	if err != nil {
		return err
	}

	c.send <- buildMsg("pt", partyDataJson)

	return nil
}

func (c *SessionClient) handleEp() (err error) {
	period, err := getCurrentEventPeriodData()
	if err != nil {
		return err
	}
	periodJson, err := json.Marshal(period)
	if err != nil {
		return err
	}

	c.send <- buildMsg("ep", periodJson)

	return nil
}

func (c *SessionClient) handleE() (err error) {
	currentEventLocationsData, err := getCurrentPlayerEventLocationsData(c.uuid)
	if err != nil {
		return err
	}
	var hasIncompleteEvent bool
	for _, currentEventLocation := range currentEventLocationsData {
		if !currentEventLocation.Complete && currentEventLocation.Game == serverConfig.GameName {
			hasIncompleteEvent = true
			break
		}
	}
	if !hasIncompleteEvent {
		if serverConfig.GameName == "2kki" {
			addPlayer2kkiEventLocation(-1, freeEventLocationMinDepth, 0, 0, c.uuid)
		} else if len(freeEventLocationPool) > 0 {
			addPlayerEventLocation(serverConfig.GameName, -1, 0, freeEventLocationPool, c.uuid)
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

	c.send <- buildMsg("e", eventsDataJson)

	return nil
}

func (c *SessionClient) handleEexp() (err error) {
	if currentGameEventPeriodId <= 0 {
		return err
	}

	playerEventExpData, err := getPlayerEventExpData(c.uuid)
	if err != nil {
		return err
	}
	playerEventExpDataJson, err := json.Marshal(playerEventExpData)
	if err != nil {
		return err
	}

	c.send <- buildMsg("eexp", playerEventExpDataJson)

	return nil
}

func (c *SessionClient) handleEec(msg []string) (err error) {
	if currentGameEventPeriodId <= 0 {
		c.send <- buildMsg("eec", 0, false)
		return err
	}

	if len(msg) < 3 {
		c.send <- buildMsg("eec", 0, false)
		return errors.New("command length mismatch")
	}

	location := msg[1]
	if len(location) == 0 {
		c.send <- buildMsg("eec", 0, false)
		return err // location not specified
	}

	ret := -1
	if c.rClient != nil {
		if msg[2] != "1" { // not free expedition
			exp, err := tryCompleteEventLocation(c.uuid, location)
			if err != nil {
				c.send <- buildMsg("eec", 0, false)
				return err
			}
			if exp < 0 {
				c.send <- buildMsg("eec", 0, false)
				return err // unexpected state
			}
			ret = exp
		} else { // free expedition
			complete, err := tryCompletePlayerEventLocation(c.uuid, location)
			if err != nil {
				c.send <- buildMsg("eec", 0, false)
				return err
			}
			if complete {
				ret = 0
			}
		}
	}
	currentEventLocationsData, err := getCurrentPlayerEventLocationsData(c.uuid)
	if err != nil {
		c.send <- buildMsg("eec", 0, false)
		return err
	}
	var hasIncompleteEvent bool
	for _, currentEventLocation := range currentEventLocationsData {
		if !currentEventLocation.Complete {
			hasIncompleteEvent = true
			break
		}
	}
	if !hasIncompleteEvent && serverConfig.GameName == "2kki" {
		addPlayer2kkiEventLocation(-1, 2, 0, 0, c.uuid)
	}

	c.send <- buildMsg("eec", ret, true)

	return nil
}
