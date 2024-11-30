/*
	Copyright (C) 2021-2024  The YNOproject Developers

	This program is free software: you can redistribute it and/or modify
	it under the terms of the GNU Affero General Public License as published by
	the Free Software Foundation, either version 3 of the License, or
	(at your option) any later version.

	This program is distributed in the hope that it will be useful,
	but WITHOUT ANY WARRANTY; without even the implied warranty of
	MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
	GNU Affero General Public License for more details.

	You should have received a copy of the GNU Affero General Public License
	along with this program.  If not, see <http://www.gnu.org/licenses/>.
*/

package server

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"strings"

	"github.com/bwmarrin/discordgo"
)

var (
	bot *discordgo.Session
	// uuid -> discord message ID
	reportLog     map[string]string
	reportReasons = map[string]string{
		":1": "Slurs, harmful or inappropriate language",
		":2": "Harassment, bullying, stalking",
		":3": "Inappropriate name",
		":4": "Ban evasion",
		":5": "Cheating, abusing exploits",
		":6": "Underage player",
		":7": "Spam",
	}
	msgIdPattern = regexp.MustCompile("msgid=(\\S*)$")
)

func getReadableReportReason(reason string) string {
	if desc, ok := reportReasons[reason]; ok {
		return desc
	}
	return reason
}

func initReports() {
	if !isMainServer {
		return
	}

	reportLog = make(map[string]string)

	var err error
	bot, err = discordgo.New("Bot " + config.moderation.botToken)
	if err != nil {
		if config.moderation.botToken != "" {
			log.Fatalf("initReports(bot): %s", err)
		}
		log.Printf("no bot token defined, not launching bot thread. (err=%s)", err)
		return
	}

	bot.AddHandler(func(s *discordgo.Session, action *discordgo.InteractionCreate) {
		if action.Interaction.Type != discordgo.InteractionMessageComponent {
			return
		}
		data := action.MessageComponentData()
		cmd, uuid, ok := strings.Cut(data.CustomID, ":")
		if !ok {
			return
		}
		switch cmd {
		case "ban":
			for game := range gameIdToName {
				banPlayerInGameUnchecked(game, uuid)
			}
			if msgId, ok := reportLog[uuid]; ok {
				targetName := getNameFromUuid(uuid)
				bot.ChannelMessageEdit(config.moderation.channelId, msgId, fmt.Sprintf("*%s has been **banned** by %s.*", targetName, action.User.Username))
				delete(reportLog, uuid)
			}
			markAsResolved(uuid)
		case "mute":
			for game := range gameIdToName {
				mutePlayerInGameUnchecked(game, uuid)
			}
			if msgId, ok := reportLog[uuid]; ok {
				targetName := getNameFromUuid(uuid)
				bot.ChannelMessageEdit(config.moderation.channelId, msgId, fmt.Sprintf("*%s has been muted by %s.*", targetName, action.User.Username))
				delete(reportLog, uuid)
			}
			markAsResolved(uuid)
		case "ack":
			if msgId, ok := reportLog[uuid]; ok {
				targetName := getNameFromUuid(uuid)
				bot.ChannelMessageEdit(config.moderation.channelId, msgId, fmt.Sprintf("*Report on %s acknowledged by %s*", targetName, action.User.Username))
				delete(reportLog, uuid)
			}
		case "cmd":
			if len(data.Values) != 1 {
				return
			}
			if discordMsgId, ok := reportLog[uuid]; ok {
				// get the full message object, since we're interacting thru a Select Menu
				msgObj, err := bot.ChannelMessage(config.moderation.channelId, discordMsgId)
				if err != nil {
					log.Printf("bot/handler/cmd: %s", err)
					return
				}
				metadataField := msgObj.Embeds[0].Fields[2]
				msgIdMatch := msgIdPattern.FindSubmatch([]byte(metadataField.Value))
				if msgIdMatch == nil {
					return
				}
				ynoMsgId := string(msgIdMatch[1])

				switch data.Values[0] {
				case "reveal":
					reports, err := getReportersForPlayer(uuid, ynoMsgId)
					if err != nil {
						log.Printf("getReportersForPlayer: %s", err)
						return
					}
					reportsContent := "```\n"
					for reporter, reason := range reports {
						reportsContent += fmt.Sprintf("%s: %s\n", reporter, getReadableReportReason(reason))
					}
					reportsContent += "```"
					_, err = bot.ChannelMessageSendEmbed(config.moderation.channelId, &discordgo.MessageEmbed{
						Title:       fmt.Sprintf("Reporters for `msgid`=%s", ynoMsgId),
						Description: reportsContent,
					})
					if err != nil {
						log.Printf("bot/handler/cmd/reveal: %s", err)
					}
				}

				// reset components
				edit := discordgo.NewMessageEdit(config.moderation.channelId, discordMsgId)
				edit.Components = &msgObj.Components
				msgObj, err = bot.ChannelMessageEditComplex(edit)
				if err == nil && msgObj != nil {
					reportLog[uuid] = msgObj.ID
				}
			}
		}
	})

	bot.AddHandler(func(s *discordgo.Session, msg *discordgo.MessageDelete) {
		if msg.BeforeDelete.ChannelID == config.moderation.channelId {
			for uuid := range reportLog {
				if reportLog[uuid] == msg.BeforeDelete.ID {
					delete(reportLog, uuid)
					break
				}
			}

		}
	})
}

// obj should be an outpointer to one of the compatible message types
func formatReportLog(obj interface{}, targetUuid, ynoMsgId, originalMsg string, reasons map[string]int) {
	targetName := getNameFromUuid(targetUuid)
	if originalMsg != "" {
		originalMsg = fmt.Sprintf("> *%s*", originalMsg)
	}
	reasonsString := ""
	for reason, count := range reasons {
		reasonsString += fmt.Sprintf("- %s: %d\n", reason, count)
	}

	verifiedString := "false"
	if ynoMsgId != "" {
		verifiedString = "true"
	}

	metadataString := fmt.Sprintf(
		`-# uid=%s
-# msgid=%s`, targetUuid, ynoMsgId)

	embed := &discordgo.MessageEmbed{
		Title:       fmt.Sprintf("Report received for **%s**", targetName),
		Description: originalMsg,
		Fields: []*discordgo.MessageEmbedField{
			{
				Name:  "Reasons",
				Value: reasonsString,
			},
			{
				Name:   "Verified",
				Value:  verifiedString,
				Inline: true,
			},
			{
				Name:   "Metadata",
				Value:  metadataString,
				Inline: true,
			},
		},
	}

	components := []discordgo.MessageComponent{
		discordgo.ActionsRow{
			Components: []discordgo.MessageComponent{
				discordgo.Button{
					Label:    "Ban",
					CustomID: "ban:" + targetUuid,
					Style:    discordgo.DangerButton,
				},
				discordgo.Button{
					Label:    "Mute",
					CustomID: "mute:" + targetUuid,
					Style:    discordgo.PrimaryButton,
				},
				discordgo.Button{
					Label:    "Acknowledge",
					CustomID: "ack:" + targetUuid,
					Style:    discordgo.SecondaryButton,
				},
			},
		},
		discordgo.SelectMenu{
			Placeholder: "More Actions",
			MaxValues:   1,
			MenuType:    discordgo.StringSelectMenu,
			CustomID:    "cmd:" + targetUuid,
			Options: []discordgo.SelectMenuOption{
				{
					Label: "Reveal Reporters",
					Value: "reveal",
				},
			},
		},
	}

	content := fmt.Sprintf("<@&%s> Received report:", config.moderation.modRoleId)
	allowedMentions := &discordgo.MessageAllowedMentions{
		Roles: []string{config.moderation.modRoleId},
	}
	switch msg := obj.(type) {
	case *discordgo.MessageSend:
		msg.Content = content
		msg.AllowedMentions = allowedMentions
		msg.Embeds = []*discordgo.MessageEmbed{embed}
		msg.Components = components
	case *discordgo.MessageEdit:
		msg.Content = &content
		msg.AllowedMentions = allowedMentions
		msg.Embeds = &[]*discordgo.MessageEmbed{embed}
		msg.Components = &components
	default:
		log.Fatalf("formatReportLog: Unrecognized outpointer type: %T", obj)
	}
}

func handleReport(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		handleError(w, r, "Invalid request")
		return
	}

	var uuid string
	var banned bool

	token := r.Header.Get("Authorization")
	if token == "" {
		handleError(w, r, "token not specified")
		return
	}

	uuid, _, _, _, banned, _ = getPlayerDataFromToken(token)
	if uuid == "" {
		handleError(w, r, "invalid token")
		return
	}

	if banned {
		handleError(w, r, "player is banned")
		return
	}

	defer r.Body.Close()
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()

	var req struct {
		Uuid        string `json:"uuid"`
		Reason      string `json:"reason"`
		OriginalMsg string `json:"original_msg"`
		MsgId       string `json:"msg_id"`
	}
	err := dec.Decode(&req)
	if err != nil {
		handleError(w, r, "Invalid request")
		return
	}

	req.MsgId, req.OriginalMsg, err = createReport(uuid, req.Uuid, req.Reason, req.MsgId, req.OriginalMsg)
	if err != nil {
		writeErrLog(uuid, r.URL.Path, "createReport failed: "+err.Error())
		handleError(w, r, "Could not create report")
		return
	}

	err = sendReportLog(req.Uuid, req.MsgId, req.OriginalMsg)
	if err != nil {
		writeErrLog(uuid, r.URL.Path, "sendReportMessage failed: "+err.Error())
	}

	w.WriteHeader(200)
}

func sendReportLogMainServer(uuid, ynoMsgId, originalMsg string) error {
	if !isMainServer {
		return errors.New("cannot call sendReportMessage from non-main server")
	}

	rows, err := db.Query(`
SELECT reason, COUNT(*) FROM playerReports
WHERE targetUuid = ? AND actionTaken = 0
GROUP BY reason`, uuid)

	var reasons map[string]int
	if err == nil {
		reasons = make(map[string]int)
		defer rows.Close()
		for rows.Next() {
			var reason string
			var count int
			err := rows.Scan(&reason, &count)
			if err != nil {
				return err
			}

			reasons[reason] = count
		}
	}

	var msg *discordgo.Message

	// Don't override a verified message with an unverified one
	if discordMsgId, ok := reportLog[uuid]; ok && ynoMsgId != "" {
		payload := discordgo.NewMessageEdit(config.moderation.channelId, discordMsgId)
		formatReportLog(payload, uuid, ynoMsgId, originalMsg, reasons)
		msg, err = bot.ChannelMessageEditComplex(payload)
	} else {
		payload := &discordgo.MessageSend{}
		formatReportLog(payload, uuid, ynoMsgId, originalMsg, reasons)
		msg, err = bot.ChannelMessageSendComplex(config.moderation.channelId, payload)
	}

	if msg != nil && err == nil {
		reportLog[uuid] = msg.ID
	}

	return err
}

func createReport(uuid, targetUuid, reason, msgId, originalMsg string) (string, string, error) {
	var err error
	row := db.QueryRow("SELECT contents FROM chatMessages WHERE msgId = ? AND uuid = ?", msgId, targetUuid)
	var contentsFromDb string
	err = row.Scan(&contentsFromDb)
	if err == nil {
		originalMsg = contentsFromDb
	} else if err != sql.ErrNoRows {
		return msgId, originalMsg, err
	} else {
		// we could not corroborate this with data from the server
		msgId = ""
	}

	_, err = db.Exec(`
REPLACE INTO playerReports
	(uuid, targetUuid, msgId, game, reason, originalMsg, timestampReported)
VALUES
	(?, ?, ?, ?, ?, NOW())`,
		uuid, targetUuid, msgId, config.gameName, urlReplacer.Replace(reason), originalMsg)
	return msgId, originalMsg, err
}

func markAsResolved(targetUuid string) {
	_, err := db.Exec(`UPDATE playerReports SET actionTaken = 1 WHERE targetUuid = ?`, targetUuid)
	if err != nil {
		log.Printf("markAsResolved: %s", err)
	}
}
