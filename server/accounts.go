package server

import (
	"net/http"

	"github.com/thanhpk/randstr"
	"golang.org/x/crypto/bcrypt"
)

func handleRegister(w http.ResponseWriter, r *http.Request) {
	//GET params user, password
	user, password := r.URL.Query()["user"], r.URL.Query()["password"]
	if len(user) < 1 || !isOkString.MatchString(user[0]) || len(password) < 1 {
		handleError(w, r, "bad response")
		return
	}

	var userExists int
	db.handle.QueryRow("SELECT COUNT(*) FROM accounts WHERE user = ?", user[0]).Scan(&userExists)

	if userExists == 1 {
		handleError(w, r, "user exists")
		return
	}

	hashedPassword, _ := bcrypt.GenerateFromPassword([]byte(password[0]), bcrypt.DefaultCost)
	db.handle.Exec("INSERT INTO accounts (user, password) VALUES (?, ?)", user[0], hashedPassword)

	w.Write([]byte("ok"))
}

func handleLogin(w http.ResponseWriter, r *http.Request) {
	//GET params user, password
	user, password := r.URL.Query()["user"], r.URL.Query()["password"]
	if len(user) < 1 || !isOkString.MatchString(user[0]) || len(password) < 1 {
		handleError(w, r, "bad response")
		return
	}

	var userPassHash string
	db.handle.QueryRow("SELECT password FROM accounts WHERE user = ?", user[0]).Scan(&userPassHash)

	if userPassHash == "" || bcrypt.CompareHashAndPassword([]byte(userPassHash), []byte(password[0])) != nil {
		handleError(w, r, "bad login")
		return
	}

	sessionId := randstr.String(32)
	db.handle.Exec("UPDATE accounts SET session = ? WHERE user = ?", sessionId, user[0])

	w.Write([]byte(sessionId))
}

func getInfoFromSession(sessionId string) (username string, uuid string) {
	db.handle.QueryRow("SELECT username, uuid FROM accounts WHERE session = ?", sessionId).Scan(&username, &uuid)

	return username, uuid
}
