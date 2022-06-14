package main

import (
	"errors"
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

	if isVpn(ip) {
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

	token := randstr.String(32)
	db.Exec("INSERT INTO playerSessions (sessionId, uuid, expiration) (SELECT ?, uuid, DATE_ADD(NOW(), INTERVAL 30 DAY) FROM accounts WHERE user = ?)", token, user[0])
	db.Exec("UPDATE accounts SET timestampLoggedIn = CURRENT_TIMESTAMP() WHERE user = ?", user[0])

	w.Write([]byte(token))
}

func handleLogout(w http.ResponseWriter, r *http.Request) {
	token := r.Header.Get("X-Session")

	if token == "" {
		handleError(w, r, "token not specified")
		return
	}

	uuid, _, _, _, _ := readPlayerDataFromToken(token)

	if uuid == "" {
		handleError(w, r, "invalid token")
		return
	}

	db.Exec("DELETE FROM playerSessions WHERE sessionId = ? AND uuid = ?", token, uuid)

	w.Write([]byte("ok"))
}

func handleChangePw(w http.ResponseWriter, r *http.Request) {
	//GET params user, old password, new password
	user, password, newPassword := r.URL.Query()["user"], r.URL.Query()["password"], r.URL.Query()["newPassword"]
	if len(user) < 1 || !isOkString(user[0]) || len(password) < 1 || len(newPassword) < 1 {
		handleError(w, r, "bad response")
		return
	}

	var userPassHash string
	db.QueryRow("SELECT pass FROM accounts WHERE user = ?", user[0]).Scan(&userPassHash)

	if userPassHash == "" || bcrypt.CompareHashAndPassword([]byte(userPassHash), []byte(password[0])) != nil {
		handleError(w, r, "bad login")
		return
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(newPassword[0]), bcrypt.DefaultCost)
	if err != nil {
		handleError(w, r, "bcrypt error")
		return
	}

	db.Exec("UPDATE accounts SET password = ? WHERE user = ?", hashedPassword, user[0])

	w.Write([]byte("ok"))
}

func setRandomPw(uuid string) (newPassword string, err error) {
	var userCount int
	db.QueryRow("SELECT COUNT(*) FROM accounts WHERE user = ?", uuid).Scan(&userCount)

	if userCount == 0 {
		return "", errors.New("user not found")
	}

	newPassword = randstr.String(8)

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}

	db.Exec("UPDATE accounts SET password = ? WHERE uuid = ?", hashedPassword, uuid)

	return newPassword, nil
}
