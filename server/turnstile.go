package server

import (
	"bytes"
	"encoding/json"
	"net/http"
)

type TurnstileRequest struct {
	Secret   string `json:"secret"`
	Response string `json:"response"`
	Remote   string `json:"remote"`
}

type TurnstileResponse struct {
	// this is all we care about
	Success bool `json:"success"`
}

func verifyTurnstile(r *http.Request) (bool, error) {
	buf := new(bytes.Buffer)
	err := json.NewEncoder(buf).Encode(TurnstileRequest{
		Secret:   config.turnstileKey,
		Response: r.FormValue("cf-turnstile-response"),
		Remote:   getIp(r),
	})
	if err != nil {
		return false, err
	}

	resp, err := http.Post("https://challenges.cloudflare.com/turnstile/v0/siteverify", "application/json", buf)
	if err != nil {
		return false, err
	}

	defer resp.Body.Close()

	var tr TurnstileResponse
	err = json.NewDecoder(resp.Body).Decode(&tr)
	if err != nil {
		return false, err
	}

	return tr.Success, nil
}
