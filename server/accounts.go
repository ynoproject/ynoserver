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
	db.Exec("INSERT INTO playerSessions (sessionId, uuid, expiration) (SELECT ?, uuid, DATE_ADD(NOW(), INTERVAL 30 DAY) FROM accounts WHERE user = ?)", sessionId, user[0])
	db.Exec("UPDATE accounts SET timestampLoggedIn = CURRENT_TIMESTAMP() WHERE user = ?", user[0])

	w.Write([]byte(sessionId))
}

func handleLogout(w http.ResponseWriter, r *http.Request) {
	session := r.Header.Get("X-Session")

	if session == "" {
		handleError(w, r, "session token not specified")
		return
	}

	uuid, _, _, _, _ := readPlayerDataFromSession(session)

	if uuid == "" {
		handleError(w, r, "invalid session token")
		return
	}

	db.Exec("DELETE FROM playerSessions WHERE sessionId = ? AND uuid = ?", session, uuid)

	w.Write([]byte("ok"))
}
