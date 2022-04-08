package server

import (
	"net/http"

	"github.com/thanhpk/randstr"
	"golang.org/x/crypto/bcrypt"
)

func handleRegister(w http.ResponseWriter, r *http.Request) {
	//GET params user, password
	user, password := r.URL.Query()["user"], r.URL.Query()["password"]
	if len(user) < 1 || !isOkString(user[0]) || len(password) < 1 {
		handleError(w, r, "bad response")
		return
	}

	var userExists int
	db.QueryRow("SELECT COUNT(*) FROM accountdata WHERE user = ?", user[0]).Scan(&userExists)

	if userExists == 1 {
		handleError(w, r, "user exists")
		return
	}

	uuid, _, _ := readPlayerData(getIp(r)) //bind this account to existing uuid or make new one

	hashedPassword, _ := bcrypt.GenerateFromPassword([]byte(password[0]), bcrypt.DefaultCost)
	db.Exec("INSERT INTO accountdata (uuid, user, pass) VALUES (?, ?, ?)", uuid, user[0], hashedPassword)

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
	db.QueryRow("SELECT pass FROM accountdata WHERE user = ?", user[0]).Scan(&userPassHash)

	if userPassHash == "" || bcrypt.CompareHashAndPassword([]byte(userPassHash), []byte(password[0])) != nil {
		handleError(w, r, "bad login")
		return
	}

	sessionId := randstr.String(32)
	db.Exec("UPDATE accountdata SET session = ? WHERE user = ?", sessionId, user[0])

	w.Write([]byte(sessionId))
}

func readPlayerDataFromSession(session string) (uuid string, name string, rank int, banned bool) {
	db.QueryRow("SELECT accountdata.uuid, accountdata.name, playerdata.rank, playerdata.banned WHERE accountdata.uuid = ? AND playerdata.uuid = ?", session).Scan(&uuid, &name, &rank, &banned)

	return uuid, name, rank, banned
}
