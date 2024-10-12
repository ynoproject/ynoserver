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

	Metadata NotificationMetadata `json:"metadata"`

	Body string `json:"body,omitempty"`

	Icon string `json:"icon,omitempty"`
	// the image to be used on a phone's status bar
	Badge string `json:"badge,omitempty"`
	Image string `json:"image,omitempty"`

	Data *json.RawMessage `json:"data,omitempty"`

	// Unix timestamp, in milliseconds
	Timestamp int64 `json:"timestamp,omitempty"`
}

type NotificationMetadata struct {
	// Necessary for client-side muting of notifications.
	Category string `json:"category"`
	Type     string `json:"type"`
	// Specify an icon predefined by frontend.
	YnoIcon string `json:"ynoIcon,omitempty"`
	// If set, this notification should not be relayed to an active frontend client
	NoRelay bool `json:"noRelay"`
	// If not set, by default toasts disappear after some time.
	Persist bool `json:"persist"`
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

	err = sendPushNotification(&Notification{
		Title: "YNOproject",
		Body:  "This is how you will be notified of upcoming events.",
		Metadata: NotificationMetadata{
			Category: "system",
			Type:     "pushNotifications",
			YnoIcon:  "global",
			NoRelay:  true,
		},
	}, []string{uuid})
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

// If `uuids` is nil, sends the message to all users.
func sendPushNotification(notification *Notification, uuids []string) error {
	placeholder, uuidParams := getPlaceholders(uuids...)

	query := "SELECT endpoint, p256dh, auth FROM pushSubscriptions"
	if len(uuidParams) > 0 {
		query += " WHERE uuid IN (" + placeholder + ")"
	}
	results, err := db.Query(query, uuidParams...)
	if err != nil {
		return err
	}

	notificationString, err := json.Marshal(notification)
	if err != nil {
		return err
	}

	notification.SetDefaults()

	defer results.Close()
	var failures []error
	for results.Next() {
		var s webpush.Subscription
		err = results.Scan(&s.Endpoint, &s.Keys.P256dh, &s.Keys.Auth)
		if err != nil {
			return err
		}
		resp, err := webpush.SendNotification(notificationString, &s, &webpush.Options{
			Subscriber:      "contact@ynoproject.net",
			VAPIDPublicKey:  config.vapidKeys.public,
			VAPIDPrivateKey: config.vapidKeys.private,
			TTL:             30, // seconds,
		})
		if err != nil {
			log.Printf("error sending notifications: %s", err)
			failures = append(failures, err)
			continue
		}
		if resp != nil && resp.StatusCode >= 400 {
			log.Printf("webpush client responded with: %s", resp.Status)
			failures = append(failures, errors.New(s.Endpoint+" "+resp.Status))
		}
	}

	return errors.Join(failures...)
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
