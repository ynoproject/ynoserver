package server

import "strconv"

type Condition struct {
	Name       string `json:"name"`
	SwitchId   int    `json:"switchId"`
	SwitchVal  bool   `json:"switchVal"`
	VarId      int    `json:"varId"`
	VarValue   int    `json:"varVal"`
	Command    string `json:"command"`
	CommandVal string `json:"commandVal"`
	Seconds    int    `json:"seconds"`
}

func getHubConditions(roomName string) (conditions []*Condition) {
	switch config.gameName {
	case "yume":
		switch roomName {
		case "6":
			conditions = append(conditions, &Condition{Name: "kalimba", Command: "ap", CommandVal: "00000065"})
		case "55":
			conditions = append(conditions, &Condition{Name: "toriningen_party"})
		case "101":
			conditions = append(conditions, &Condition{Name: "uboa"})
		case "179":
			conditions = append(conditions, &Condition{Name: "witch_flight"})
		}
	case "2kki":
		switch roomName {
		case "243":
			conditions = append(conditions, &Condition{Name: "hakoko"})
			conditions = append(conditions, &Condition{Seconds: 100})
		case "274":
			conditions = append(conditions, &Condition{Name: "amusement_park_hell"})
		case "458":
			conditions = append(conditions, &Condition{Name: "gallery_of_me"})
		case "729":
			conditions = append(conditions, &Condition{Name: "scrambled_egg_zone"})
		case "860":
			conditions = append(conditions, &Condition{Name: "aooh", SwitchId: 2, SwitchVal: true})
		case "1073":
			conditions = append(conditions, &Condition{Name: "vending_machine", SwitchId: 2, SwitchVal: true})
		case "1148":
			conditions = append(conditions, &Condition{Name: "lavender_waters"})
			conditions = append(conditions, &Condition{Seconds: 720})
		case "1205":
			conditions = append(conditions, &Condition{Name: "tomb_of_velleities"})
			conditions = append(conditions, &Condition{Seconds: 1740})
		case "1422":
			conditions = append(conditions, &Condition{Name: "obentou_world"})
		case "1500":
			conditions = append(conditions, &Condition{Name: "unknown_childs_room"})
		case "1673":
			conditions = append(conditions, &Condition{Name: "magical_passage"})
			conditions = append(conditions, &Condition{Seconds: 510})
		case "1698":
			conditions = append(conditions, &Condition{Name: "voxel_island", Command: "ploc", CommandVal: "1697"})
		}
	case "prayers":
	case "37":
		conditions = append(conditions, &Condition{Name: "koraiyn"})
	case "57":
		conditions = append(conditions, &Condition{Name: "missingno", Command: "ap", CommandVal: "BSOD1"})
	}
	return conditions
}

func checkHubConditions(h *Hub, client *Client, command string, commandVal string) {
	if !client.account {
		return
	}
	for _, c := range h.conditions {
		if c.Seconds == 0 {
			if c.Command == command && (command == "" || commandVal == c.CommandVal) {
				if c.SwitchId > 0 {
					client.send <- []byte("ss" + paramDelimStr + strconv.Itoa(c.SwitchId) + paramDelimStr + "1")
				} else if c.VarId > 0 {
					client.send <- []byte("sv" + paramDelimStr + strconv.Itoa(c.VarId) + paramDelimStr + "1")
				} else {
					_, err := tryWritePlayerTag(client.uuid, c.Name)
					if err != nil {
						writeErrLog(client.ip, h.roomName, err.Error())
					}
				}
			}
		} else {
			if config.gameName == "2kki" {
				if c.SwitchId > 0 {
					client.send <- []byte("ss" + paramDelimStr + strconv.Itoa(c.SwitchId) + paramDelimStr + "1")
				} else if c.VarId > 0 {
					client.send <- []byte("sv" + paramDelimStr + strconv.Itoa(c.VarId) + paramDelimStr + "1")
				} else {
					client.send <- []byte("sv" + paramDelimStr + "88" + paramDelimStr + "0")
				}
			}
		}
	}
}

func readPlayerBadgeData(playerUuid string, playerRank int, playerTags []string) (badges []*Badge, err error) {
	playerExp, err := readPlayerTotalEventExp(playerUuid)
	if err != nil {
		return badges, err
	}
	playerEventLocationCompletion, err := readPlayerEventLocationCompletion(playerUuid)
	if err != nil {
		return badges, err
	}
	timeTrialRecords, err := readPlayerTimeTrialRecords(playerUuid)
	if err != nil {
		return badges, err
	}

	boomboxBadge := &Badge{BadgeId: "boombox", Game: "yume", MapId: 55}
	badges = append(badges, boomboxBadge)

	uboaBadge := &Badge{BadgeId: "uboa", Game: "yume", MapId: 101}
	badges = append(badges, uboaBadge)

	blackCatBadge := &Badge{BadgeId: "blackcat", Game: "yume", MapId: 179}
	badges = append(badges, blackCatBadge)

	badges = append(badges, &Badge{BadgeId: "mono", Game: "2kki", Unlocked: playerExp >= 40, Overlay: true})
	badges = append(badges, &Badge{BadgeId: "bronze", Game: "2kki", Unlocked: playerExp >= 100, Secret: playerExp < 40})
	badges = append(badges, &Badge{BadgeId: "silver", Game: "2kki", Unlocked: playerExp >= 250, Secret: playerExp < 100})
	badges = append(badges, &Badge{BadgeId: "gold", Game: "2kki", Unlocked: playerExp >= 500, Secret: playerExp < 250})
	badges = append(badges, &Badge{BadgeId: "platinum", Game: "2kki", Unlocked: playerExp >= 1000, Secret: playerExp < 500})
	badges = append(badges, &Badge{BadgeId: "diamond", Game: "2kki", Unlocked: playerExp >= 2000, Secret: playerExp < 1000})
	badges = append(badges, &Badge{BadgeId: "compass", Game: "2kki", Unlocked: playerEventLocationCompletion >= 30})
	badges = append(badges, &Badge{BadgeId: "compass_bronze", Game: "2kki", Unlocked: playerEventLocationCompletion >= 50, Secret: playerEventLocationCompletion < 30})
	badges = append(badges, &Badge{BadgeId: "compass_silver", Game: "2kki", Unlocked: playerEventLocationCompletion >= 70, Secret: playerEventLocationCompletion < 50})
	badges = append(badges, &Badge{BadgeId: "compass_gold", Game: "2kki", Unlocked: playerEventLocationCompletion >= 80, Secret: playerEventLocationCompletion < 70})
	badges = append(badges, &Badge{BadgeId: "compass_platinum", Game: "2kki", Unlocked: playerEventLocationCompletion >= 90, Secret: playerEventLocationCompletion < 80})
	badges = append(badges, &Badge{BadgeId: "compass_diamond", Game: "2kki", Unlocked: playerEventLocationCompletion >= 95, Secret: playerEventLocationCompletion < 90})

	crushedBadge := &Badge{BadgeId: "crushed", Game: "2kki", MapId: 274}
	badges = append(badges, crushedBadge)

	obentouBadge := &Badge{BadgeId: "obentou", Game: "2kki", MapId: 1422}
	badges = append(badges, obentouBadge)

	compass28Badge := &Badge{BadgeId: "compass_28", Game: "2kki", MapId: 1500}
	badges = append(badges, compass28Badge)

	blueOrbBadge := &Badge{BadgeId: "blue_orb", Game: "2kki", MapId: 729}
	badges = append(badges, blueOrbBadge)

	aoohBadge := &Badge{BadgeId: "aooh", Game: "2kki"}
	badges = append(badges, aoohBadge)

	hakokoBadge := &Badge{BadgeId: "hakoko", Game: "2kki", MapId: 243}
	if playerRank == 2 {
		badges = append(badges, hakokoBadge)
	}

	hakokoPrimeBadge := &Badge{BadgeId: "hakoko_prime", Game: "2kki", MapId: 243}
	if playerRank == 2 {
		badges = append(badges, hakokoPrimeBadge)
	}

	lesserLavenderBadge := &Badge{BadgeId: "lavender_lesser", Game: "2kki", MapId: 1148}
	badges = append(badges, lesserLavenderBadge)

	lavenderBadge := &Badge{BadgeId: "lavender", Game: "2kki", MapId: 1148}
	badges = append(badges, lavenderBadge)

	lesserButterflyBadge := &Badge{BadgeId: "butterfly_lesser", Game: "2kki", MapId: 1205}
	badges = append(badges, lesserButterflyBadge)

	butterflyBadge := &Badge{BadgeId: "butterfly", Game: "2kki", MapId: 1205}
	badges = append(badges, butterflyBadge)

	lesserMagicalBadge := &Badge{BadgeId: "magical_lesser", Game: "2kki", MapId: 1673}
	badges = append(badges, lesserMagicalBadge)

	magicalBadge := &Badge{BadgeId: "magical", Game: "2kki", MapId: 1673}
	badges = append(badges, magicalBadge)

	voxelsBadge := &Badge{BadgeId: "voxels", Game: "2kki", MapId: 1698}
	badges = append(badges, voxelsBadge)

	vendingMachineBadge := &Badge{BadgeId: "vending_machine", Game: "2kki", MapId: 1073}
	badges = append(badges, vendingMachineBadge)

	cloverBadge := &Badge{BadgeId: "clover", Game: "2kki", MapId: 458}
	if playerRank == 2 {
		badges = append(badges, cloverBadge)
	}

	koraiynBadge := &Badge{BadgeId: "koraiyn", Game: "prayers", MapId: 37}
	badges = append(badges, koraiynBadge)

	missingnoBadge := &Badge{BadgeId: "missingno", Game: "prayers", MapId: 57}
	badges = append(badges, missingnoBadge)

	for _, tag := range playerTags {
		if tag == "toriningen_party" {
			boomboxBadge.Unlocked = true
		} else if tag == "uboa" {
			uboaBadge.Unlocked = true
		} else if tag == "witch_flight" {
			blackCatBadge.Unlocked = true
		} else if tag == "amusement_park_hell" {
			crushedBadge.Unlocked = true
		} else if tag == "obentou_world" {
			obentouBadge.Unlocked = true
		} else if tag == "unknown_childs_room" {
			compass28Badge.Unlocked = true
		} else if tag == "scrambled_egg_zone" {
			blueOrbBadge.Unlocked = true
		} else if tag == "aooh" {
			aoohBadge.Unlocked = true
		} else if tag == "hakoko" {
			hakokoBadge.Unlocked = true
		} else if tag == "lavender_waters" {
			lesserLavenderBadge.Unlocked = true
		} else if tag == "tomb_of_velleities" {
			lesserButterflyBadge.Unlocked = true
		} else if tag == "magical_passage" {
			lesserMagicalBadge.Unlocked = true
		} else if tag == "voxel_island" {
			voxelsBadge.Unlocked = true
		} else if tag == "vending_machine" {
			vendingMachineBadge.Unlocked = true
		} else if tag == "gallery_of_me" {
			cloverBadge.Unlocked = true
		} else if tag == "koraiyn" {
			koraiynBadge.Unlocked = true
		} else if tag == "missingno" {
			missingnoBadge.Unlocked = true
		}
	}

	for _, record := range timeTrialRecords {
		if record.MapId == hakokoPrimeBadge.MapId && record.Seconds <= 100 {
			hakokoPrimeBadge.Unlocked = true
		} else if record.MapId == butterflyBadge.MapId {
			butterflyBadge.Unlocked = record.Seconds <= 1740
		} else if record.MapId == lavenderBadge.MapId {
			lavenderBadge.Unlocked = record.Seconds <= 720
		} else if record.MapId == magicalBadge.MapId {
			magicalBadge.Unlocked = record.Seconds <= 510
		}
	}

	playerUnlockedBadgeIds, err := readPlayerUnlockedBadgeIds(playerUuid)
	if err != nil {
		return badges, err
	}

	unlockPercentages, err := readBadgeUnlockPercentages()
	if err != nil {
		return badges, err
	}

	for _, badge := range badges {
		for _, badgePercentUnlocked := range unlockPercentages {
			if badge.BadgeId == badgePercentUnlocked.BadgeId {
				badge.Percent = badgePercentUnlocked.Percent
				break
			}
		}

		if badge.Unlocked {
			unlocked := false
			for _, unlockedBadgeId := range playerUnlockedBadgeIds {
				if badge.BadgeId == unlockedBadgeId {
					unlocked = true
					break
				}
			}
			if !unlocked {
				err := unlockPlayerBadge(playerUuid, badge.BadgeId)
				if err != nil {
					return badges, err
				}
				badge.NewUnlock = true
			}
		} else {
			for _, unlockedBadgeId := range playerUnlockedBadgeIds {
				if badge.BadgeId == unlockedBadgeId {
					badge.Unlocked = true
					break
				}
			}
		}
	}

	return badges, nil
}
