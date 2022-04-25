package server

import "strconv"

type ConfiguredBadge struct {
}

type Condition struct {
	Tag         string `json:"tag"`
	SwitchId    int    `json:"switchId"`
	SwitchVal   bool   `json:"switchVal"`
	SwitchDelay bool   `json:"switchDelay"`
	VarId       int    `json:"varId"`
	VarVal      int    `json:"varVal"`
	VarDelay    bool   `json:"varDelay"`
	Command     string `json:"command"`
	CommandVal  string `json:"commandVal"`
	Seconds     int    `json:"seconds"`
}

func getHubConditions(roomName string) (conditions []*Condition) {
	switch config.gameName {
	case "yume":
		switch roomName {
		case "6":
			conditions = append(conditions, &Condition{Tag: "kalimba", Command: "ap", CommandVal: "00000065"})
		case "54":
			conditions = append(conditions, &Condition{Tag: "a_r_m", Command: "ap", CommandVal: "01"})
		case "55":
			conditions = append(conditions, &Condition{Tag: "toriningen_party"})
		case "101":
			conditions = append(conditions, &Condition{Tag: "uboa"})
		case "179":
			conditions = append(conditions, &Condition{Tag: "witch_flight"})
		}
	case "2kki":
		switch roomName {
		case "243":
			conditions = append(conditions, &Condition{Tag: "hakoko", SwitchId: 3111, SwitchVal: true, VarDelay: true})
			conditions = append(conditions, &Condition{Seconds: 100, SwitchId: 3111, SwitchVal: true, VarDelay: true})
		case "274":
			conditions = append(conditions, &Condition{Tag: "amusement_park_hell"})
		case "458":
			conditions = append(conditions, &Condition{Tag: "gallery_of_me"})
		case "729":
			conditions = append(conditions, &Condition{Tag: "scrambled_egg_zone"})
		case "860":
			conditions = append(conditions, &Condition{Tag: "aooh", SwitchId: 2, SwitchVal: true})
		case "1073":
			conditions = append(conditions, &Condition{Tag: "vending_machine", SwitchId: 2, SwitchVal: true})
		case "1148":
			conditions = append(conditions, &Condition{Tag: "lavender_waters"})
			conditions = append(conditions, &Condition{Seconds: 720})
		case "1205":
			conditions = append(conditions, &Condition{Tag: "tomb_of_velleities"})
			conditions = append(conditions, &Condition{Seconds: 1740})
		case "1422":
			conditions = append(conditions, &Condition{Tag: "obentou_world"})
		case "1500":
			conditions = append(conditions, &Condition{Tag: "unknown_childs_room"})
		case "1673":
			conditions = append(conditions, &Condition{Tag: "magical_passage"})
			conditions = append(conditions, &Condition{Seconds: 510})
		case "1698":
			conditions = append(conditions, &Condition{Tag: "voxel_island", Command: "ploc", CommandVal: "1697"})
		}
	case "flow":
		switch roomName {
		case "154":
			conditions = append(conditions, &Condition{Tag: "cake", VarId: 135, VarVal: 20})
		}
	case "prayers":
		switch roomName {
		case "37":
			conditions = append(conditions, &Condition{Tag: "koraiyn"})
		case "57":
			conditions = append(conditions, &Condition{Tag: "missingno", Command: "ap", CommandVal: "BSOD1"})
		}
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
					switchSyncType := 2
					if c.SwitchDelay {
						switchSyncType = 1
					}
					client.send <- []byte("ss" + paramDelimStr + strconv.Itoa(c.SwitchId) + paramDelimStr + strconv.Itoa(switchSyncType))
				} else if c.VarId > 0 {
					varSyncType := 2
					if c.VarDelay {
						varSyncType = 1
					}
					client.send <- []byte("sv" + paramDelimStr + strconv.Itoa(c.VarId) + paramDelimStr + strconv.Itoa(varSyncType))
				} else {
					_, err := tryWritePlayerTag(client.uuid, c.Tag)
					if err != nil {
						writeErrLog(client.ip, h.roomName, err.Error())
					}
				}
			}
		} else if config.gameName == "2kki" {
			if c.SwitchId > 0 {
				switchSyncType := 2
				if c.SwitchDelay {
					switchSyncType = 1
				}
				client.send <- []byte("ss" + paramDelimStr + strconv.Itoa(c.SwitchId) + paramDelimStr + strconv.Itoa(switchSyncType))
			} else if c.VarId > 0 {
				varSyncType := 2
				if c.VarDelay {
					varSyncType = 1
				}
				client.send <- []byte("sv" + paramDelimStr + strconv.Itoa(c.VarId) + paramDelimStr + strconv.Itoa(varSyncType))
			} else {
				client.send <- []byte("sv" + paramDelimStr + "88" + paramDelimStr + "0")
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

	kalimbaBadge := &Badge{BadgeId: "kalimba", Game: "yume", MapId: 6}
	badges = append(badges, kalimbaBadge)

	armBadge := &Badge{BadgeId: "a_r_m", Game: "yume", MapId: 54}
	badges = append(badges, armBadge)

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
	badges = append(badges, hakokoBadge)

	hakokoPrimeBadge := &Badge{BadgeId: "hakoko_prime", Game: "2kki", MapId: 243, Seconds: 100}
	badges = append(badges, hakokoPrimeBadge)

	lesserLavenderBadge := &Badge{BadgeId: "lavender_lesser", Game: "2kki", MapId: 1148}
	badges = append(badges, lesserLavenderBadge)

	lavenderBadge := &Badge{BadgeId: "lavender", Game: "2kki", MapId: 1148, Seconds: 720}
	badges = append(badges, lavenderBadge)

	lesserButterflyBadge := &Badge{BadgeId: "butterfly_lesser", Game: "2kki", MapId: 1205}
	badges = append(badges, lesserButterflyBadge)

	butterflyBadge := &Badge{BadgeId: "butterfly", Game: "2kki", MapId: 1205, Seconds: 1740}
	badges = append(badges, butterflyBadge)

	lesserMagicalBadge := &Badge{BadgeId: "magical_lesser", Game: "2kki", MapId: 1673}
	badges = append(badges, lesserMagicalBadge)

	magicalBadge := &Badge{BadgeId: "magical", Game: "2kki", MapId: 1673, Seconds: 510}
	badges = append(badges, magicalBadge)

	voxelsBadge := &Badge{BadgeId: "voxels", Game: "2kki", MapId: 1698}
	badges = append(badges, voxelsBadge)

	vendingMachineBadge := &Badge{BadgeId: "vending_machine", Game: "2kki", MapId: 1073}
	badges = append(badges, vendingMachineBadge)

	cloverBadge := &Badge{BadgeId: "clover", Game: "2kki", MapId: 458}
	if playerRank == 2 {
		badges = append(badges, cloverBadge)
	}

	cakeBadge := &Badge{BadgeId: "cake", Game: "flow", MapId: 154}
	badges = append(badges, cakeBadge)

	koraiynBadge := &Badge{BadgeId: "koraiyn", Game: "prayers", MapId: 37}
	badges = append(badges, koraiynBadge)

	missingnoBadge := &Badge{BadgeId: "missingno", Game: "prayers", MapId: 57}
	badges = append(badges, missingnoBadge)

	for _, tag := range playerTags {
		switch tag {
		case "kalimba":
			kalimbaBadge.Unlocked = true
		case "a_r_m":
			armBadge.Unlocked = true
		case "toriningen_party":
			boomboxBadge.Unlocked = true
		case "uboa":
			uboaBadge.Unlocked = true
		case "witch_flight":
			blackCatBadge.Unlocked = true
		case "amusement_park_hell":
			crushedBadge.Unlocked = true
		case "obentou_world":
			obentouBadge.Unlocked = true
		case "unknown_childs_room":
			compass28Badge.Unlocked = true
		case "scrambled_egg_zone":
			blueOrbBadge.Unlocked = true
		case "aooh":
			aoohBadge.Unlocked = true
		case "hakoko":
			hakokoBadge.Unlocked = true
		case "lavender_waters":
			lesserLavenderBadge.Unlocked = true
		case "tomb_of_velleities":
			lesserButterflyBadge.Unlocked = true
		case "magical_passage":
			lesserMagicalBadge.Unlocked = true
		case "voxel_island":
			voxelsBadge.Unlocked = true
		case "vending_machine":
			vendingMachineBadge.Unlocked = true
		case "gallery_of_me":
			cloverBadge.Unlocked = true
		case "cake":
			cakeBadge.Unlocked = true
		case "koraiyn":
			koraiynBadge.Unlocked = true
		case "missingno":
			missingnoBadge.Unlocked = true
		}
	}

	for _, record := range timeTrialRecords {
		if record.MapId == hakokoPrimeBadge.MapId {
			hakokoPrimeBadge.Unlocked = record.Seconds < hakokoPrimeBadge.Seconds
		} else if record.MapId == butterflyBadge.MapId {
			butterflyBadge.Unlocked = record.Seconds <= butterflyBadge.Seconds
		} else if record.MapId == lavenderBadge.MapId {
			lavenderBadge.Unlocked = record.Seconds <= lavenderBadge.Seconds
		} else if record.MapId == magicalBadge.MapId {
			magicalBadge.Unlocked = record.Seconds <= magicalBadge.Seconds
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
