package server

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	webpush "github.com/Appboy/webpush-go"
)

// Mirrors the `options` parameter of https://developer.mozilla.org/en-US/docs/Web/API/ServiceWorkerRegistration/showNotification
type Notification struct {
	Title string `json:"title"`
	Body  string `json:"body,omitempty"`

	Icon string `json:"icon,omitempty"`
	// the image to be used on a phone's status bar
	Badge string `json:"badge,omitempty"`
	Image string `json:"image,omitempty"`

	Data *json.RawMessage `json:"data,omitempty"`

	// Unix timestamp, in milliseconds
	Timestamp int64 `json:"timestamp,omitempty"`
}

func (n *Notification) SetDefaults() {
	if n.Timestamp == 0 {
		n.Timestamp = time.Now().UTC().UnixMilli()
	}
}

func handleRegisterSubscriber(w http.ResponseWriter, r *http.Request) {
	var sub webpush.Subscription
	var uuid string
	var banned bool

	if r.Method != "POST" {
		handleError(w, r, "unsupported HTTP method")
		return
	}

	token := r.Header.Get("Authorization")
	if token == "" {
		uuid, banned, _ = getOrCreatePlayerData(getIp(r))
	} else {
		uuid, _, _, _, banned, _ = getPlayerDataFromToken(token)
		if uuid == "" {
			handleError(w, r, "invalid token")
			return
		}
	}

	if banned {
		handleError(w, r, "player is banned")
		return
	}

	defer r.Body.Close()
	err := json.NewDecoder(r.Body).Decode(&sub)
	if err != nil {
		handleError(w, r, "invalid web notification subscription")
		return
	}

	_, err = db.Exec("INSERT IGNORE INTO pushSubscriptions (uuid, endpoint, p256dh, auth) VALUES (?, ?, ?, ?)", uuid, sub.Endpoint, sub.Keys.P256dh, sub.Keys.Auth)
	if err != nil {
		handleError(w, r, "error adding push subscription")
		return
	}

	err = sendPushNotification(&Notification{Title: "YNOproject Notification", Body: "This is how you will be notified of upcoming events."}, []string{uuid})
	if err != nil {
		log.Println("post-registration notification failed", err)
	}
}

func handleUnregisterSubscriber(w http.ResponseWriter, r *http.Request) {
	var uuid string
	var banned bool

	if r.Method != "POST" {
		handleError(w, r, "unsupported HTTP method")
		return
	}

	token := r.Header.Get("Authorization")
	if token == "" {
		uuid, banned, _ = getOrCreatePlayerData(getIp(r))
	} else {
		uuid, _, _, _, banned, _ = getPlayerDataFromToken(token)
		if uuid == "" {
			handleError(w, r, "invalid token")
			return
		}
	}

	if banned {
		handleError(w, r, "player is banned")
		return
	}

	var sub struct {
		Endpoint string `json:"endpoint"`
	}
	defer r.Body.Close()
	err := json.NewDecoder(r.Body).Decode(&sub)
	if err != nil {
		handleError(w, r, "invalid payload")
		return
	}

	_, err = db.Exec("DELETE FROM pushSubscriptions WHERE uuid = ? AND endpoint = ?", uuid, sub.Endpoint)
	if err != nil {
		handleError(w, r, "error removing push subscription")
		return
	}
}

func handleVapidPublicKeyRequest(w http.ResponseWriter, _ *http.Request) {
	w.Write([]byte(config.vapidKeys.public))
}

func sendPushNotification(notification *Notification, uuids []string) error {
	if len(uuids) < 1 {
		return errors.New("cannot handle empty uuids")
	}

	placeholder, uuidParams := getPlaceholders(uuids...)

	results, err := db.Query("SELECT endpoint, p256dh, auth FROM pushSubscriptions WHERE uuid IN ("+placeholder+")", uuidParams...)
	if err != nil {
		return err
	}

	notificationString, err := json.Marshal(notification)
	if err != nil {
		return err
	}

	notification.SetDefaults()

	defer results.Close()
	for results.Next() {
		var s webpush.Subscription
		err = results.Scan(&s.Endpoint, &s.Keys.P256dh, &s.Keys.Auth)
		if err != nil {
			return err
		}
		_, err = webpush.SendNotification(notificationString, &s, &webpush.Options{
			Subscriber:      "contact@ynoproject.net",
			VAPIDPublicKey:  config.vapidKeys.public,
			VAPIDPrivateKey: config.vapidKeys.private,
			TTL:             30, // seconds
		})
		if err != nil {
			return err
		}
	}

	return nil
}

func getPlaceholders(values ...string) (placeholders string, parameters []interface{}) {
	n := len(values)
	p := make([]string, n)
	parameters = make([]interface{}, n)
	for i := 0; i < n; i++ {
		p[i] = "?"
		parameters[i] = values[i]
	}
	placeholders = strings.Join(p, ",")
	return placeholders, parameters
}
