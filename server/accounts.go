package server

import (
	"net/http"
	"time"

	"github.com/thanhpk/randstr"
	"golang.org/x/crypto/bcrypt"
)

func handleRegister(w http.ResponseWriter, r *http.Request) {
	//GET params user, password
	user, password := r.URL.Query()["user"], r.URL.Query()["password"]
	if len(user) < 1 || len(user[0]) > 12 || !isOkString(user[0]) || len(password) < 1 {
		handleError(w, r, "bad response")
		return
	}

	var userExists int
	db.QueryRow("SELECT COUNT(*) FROM accounts WHERE user = ?", user[0]).Scan(&userExists)

	if userExists == 1 {
		handleError(w, r, "user exists")
		return
	}

	ip := getIp(r)

	ipIsVpn, _ := isVpn(ip)
	if ipIsVpn {
		handleError(w, r, "bad response")
	}

	var uuid string
	db.QueryRow("SELECT uuid FROM players WHERE ip = ?", ip).Scan(&uuid) //no row causes a non-fatal error, uuid is still unset so it doesn't matter

	if uuid != "" {
		db.Exec("UPDATE players SET ip = NULL WHERE ip = ?", ip)
	} else {
		uuid, _, _ = readPlayerData(ip)
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password[0]), bcrypt.DefaultCost)
	if err != nil {
		handleError(w, r, "bcrypt error")
		return
	}

	db.Exec("INSERT INTO accounts (ip, timestampRegistered, uuid, user, pass) VALUES (?, ?, ?, ?, ?)", ip, time.Now(), uuid, user[0], hashedPassword)

	w.Write([]byte("ok"))
}

func handleLogin(w http.ResponseWriter, r *http.Request) {
	//GET params user, password
	user, password := r.URL.Query()["user"], r.URL.Query()["password"]
	if len(user) < 1 || !isOkString(user[0]) || len(password) < 1 {
		handleError(w, r, "bad response")
		return
	}

	var userPassHash string
	db.QueryRow("SELECT pass FROM accounts WHERE user = ?", user[0]).Scan(&userPassHash)

	if userPassHash == "" || bcrypt.CompareHashAndPassword([]byte(userPassHash), []byte(password[0])) != nil {
		handleError(w, r, "bad login")
		return
	}

	sessionId := randstr.String(32)
	db.Exec("UPDATE accounts SET session = ? WHERE user = ?", sessionId, user[0])

	w.Write([]byte(sessionId))
}

func readPlayerBadgeData(playerUuid string) (badges []*Badge, err error) {
	playerExp, err := readPlayerTotalEventExp(playerUuid)
	if err != nil {
		return badges, err
	}
	playerEventLocationCompletion, err := readPlayerEventLocationCompletion(playerUuid)
	if err != nil {
		return badges, err
	}
	playerTags, err := readPlayerTags(playerUuid)
	if err != nil {
		return badges, err
	}
	timeTrialRecords, err := readPlayerTimeTrialRecords(playerUuid)
	if err != nil {
		return badges, err
	}

	uboaBadge := &Badge{BadgeId: "uboa", Game: "yume", MapId: 101}
	badges = append(badges, uboaBadge)

	badges = append(badges, &Badge{BadgeId: "mono", Game: "2kki", Unlocked: playerExp >= 40, Overlay: true})
	badges = append(badges, &Badge{BadgeId: "bronze", Game: "2kki", Unlocked: playerExp >= 100, Secret: true})
	badges = append(badges, &Badge{BadgeId: "silver", Game: "2kki", Unlocked: playerExp >= 250, Secret: true})
	badges = append(badges, &Badge{BadgeId: "gold", Game: "2kki", Unlocked: playerExp >= 500, Secret: true})
	badges = append(badges, &Badge{BadgeId: "platinum", Game: "2kki", Unlocked: playerExp >= 1000, Secret: true})
	badges = append(badges, &Badge{BadgeId: "diamond", Game: "2kki", Unlocked: playerExp >= 2000, Secret: true})
	badges = append(badges, &Badge{BadgeId: "compass", Game: "2kki", Unlocked: playerEventLocationCompletion >= 30})
	badges = append(badges, &Badge{BadgeId: "compass_bronze", Game: "2kki", Unlocked: playerEventLocationCompletion >= 50, Secret: true})
	badges = append(badges, &Badge{BadgeId: "compass_silver", Game: "2kki", Unlocked: playerEventLocationCompletion >= 70, Secret: true})
	badges = append(badges, &Badge{BadgeId: "compass_gold", Game: "2kki", Unlocked: playerEventLocationCompletion >= 80, Secret: true})
	badges = append(badges, &Badge{BadgeId: "compass_platinum", Game: "2kki", Unlocked: playerEventLocationCompletion >= 90, Secret: true})
	badges = append(badges, &Badge{BadgeId: "compass_diamond", Game: "2kki", Unlocked: playerEventLocationCompletion >= 95, Secret: true})

	crushedBadge := &Badge{BadgeId: "crushed", Game: "2kki", MapId: 274}
	badges = append(badges, crushedBadge)

	compass28Badge := &Badge{BadgeId: "compass_28", Game: "2kki", MapId: 1500}
	badges = append(badges, compass28Badge)

	blueOrbBadge := &Badge{BadgeId: "blue_orb", Game: "2kki", MapId: 729}
	badges = append(badges, blueOrbBadge)

	for _, tag := range playerTags {
		if tag == "amusement_park_hell" {
			crushedBadge.Unlocked = true
		} else if tag == "unknown_childs_room" {
			compass28Badge.Unlocked = true
		} else if tag == "scrambled_egg_zone" {
			blueOrbBadge.Unlocked = true
		} else if tag == "uboa" {
			uboaBadge.Unlocked = true
		}
	}

	butterflyBadge := &Badge{BadgeId: "butterfly", Game: "2kki", MapId: 1205}
	badges = append(badges, butterflyBadge)

	lavenderBadge := &Badge{BadgeId: "lavender", Game: "2kki", MapId: 1148}
	badges = append(badges, lavenderBadge)

	for _, record := range timeTrialRecords {
		if record.MapId == butterflyBadge.MapId {
			butterflyBadge.Unlocked = record.Seconds <= 1680
		} else if record.MapId == lavenderBadge.MapId {
			lavenderBadge.Unlocked = record.Seconds <= 750
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
		}
	}

	return badges, nil
}
