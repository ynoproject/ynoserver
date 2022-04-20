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

	badges = append(badges, &Badge{BadgeId: "mono", Unlocked: playerExp >= 40, Overlay: true})
	badges = append(badges, &Badge{BadgeId: "bronze", Unlocked: playerExp >= 100})
	badges = append(badges, &Badge{BadgeId: "silver", Unlocked: playerExp >= 250})
	badges = append(badges, &Badge{BadgeId: "gold", Unlocked: playerExp >= 500})
	badges = append(badges, &Badge{BadgeId: "platinum", Unlocked: playerExp >= 1000})
	badges = append(badges, &Badge{BadgeId: "diamond", Unlocked: playerExp >= 2000})
	badges = append(badges, &Badge{BadgeId: "compass", Unlocked: playerEventLocationCompletion >= 30})
	badges = append(badges, &Badge{BadgeId: "compass_bronze", Unlocked: playerEventLocationCompletion >= 50})
	badges = append(badges, &Badge{BadgeId: "compass_silver", Unlocked: playerEventLocationCompletion >= 70})
	badges = append(badges, &Badge{BadgeId: "compass_gold", Unlocked: playerEventLocationCompletion >= 80})
	badges = append(badges, &Badge{BadgeId: "compass_platinum", Unlocked: playerEventLocationCompletion >= 90})
	badges = append(badges, &Badge{BadgeId: "compass_diamond", Unlocked: playerEventLocationCompletion >= 95})

	compass28Badge := &Badge{BadgeId: "compass_28"}
	badges = append(badges, compass28Badge)

	for _, tag := range playerTags {
		if tag == "unknown_childs_room" {
			compass28Badge.Unlocked = true
		}
	}

	blueOrbBadge := &Badge{BadgeId: "blue_orb"}
	badges = append(badges, blueOrbBadge)

	for _, tag := range playerTags {
		if tag == "blue_orb_world" {
			blueOrbBadge.Unlocked = true
		}
	}

	playerUnlockedBadgeIds, err := readPlayerUnlockedBadgeIds(playerUuid)
	if err != nil {
		return badges, err
	}

	for _, badge := range badges {
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
				badge.NewUnlock = unlocked
			}
		}
	}

	return badges, nil
}
