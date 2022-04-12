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

	ip := getIp(r)

	var uuid string
	db.QueryRow("SELECT uuid FROM playerdata WHERE ip = ?", ip).Scan(&uuid) //no row causes a non-fatal error, uuid is still unset so it doesn't matter

	if uuid != "" {
		db.Exec("UPDATE playerdata SET ip = NULL WHERE ip = ?", ip)
	} else {
		uuid, _, _ = readPlayerData(ip)
	}

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
