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
	// uuid -> ynoMsgId -> discordMsgId
	reportLog     map[string]map[string]string
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

	reportLog = make(map[string]map[string]string)

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
		resp := discordgo.InteractionResponse{}
		defer func() {
			if err := bot.InteractionRespond(action.Interaction, &resp); err != nil {
				log.Printf("bot/respond: %s", err)
			}
		}()

		if action.Type != discordgo.InteractionMessageComponent {
			return
		}
		data := action.MessageComponentData()
		cmd, uuid, ok := strings.Cut(data.CustomID, ":")
		if !ok {
			return
		}

		ynoMsgId := parseMsgIdFromComponent(action.Interaction.Message)

		switch cmd {
		case "ban":
			targetName := getNameFromUuid(uuid)
			for game := range gameIdToName {
				banPlayerInGameUnchecked(game, uuid, false)
			}

			content := fmt.Sprintf("*%s has been **banned** by %s*", targetName, action.Member.DisplayName())

			resp.Type = discordgo.InteractionResponseUpdateMessage
			resp.Data = &discordgo.InteractionResponseData{Content: content, Embeds: action.Message.Embeds}
			if len(resp.Data.Embeds) >= 1 {
				if desc := resp.Data.Embeds[0].Description; desc != "" {
					if unquoted, ok := strings.CutPrefix(desc, "> "); ok {
						resp.Data.Embeds[0].Description = fmt.Sprintf("> ||%s||", unquoted)
					}
				}
			}
			delete(reportLog[uuid], ynoMsgId)
			markAsResolved(uuid)
		case "mute":
			targetName := getNameFromUuid(uuid)
			for game := range gameIdToName {
				mutePlayerInGameUnchecked(game, uuid)
			}

			content := fmt.Sprintf("*%s has been muted by %s*", targetName, action.Member.DisplayName())

			resp.Type = discordgo.InteractionResponseUpdateMessage
			resp.Data = &discordgo.InteractionResponseData{Content: content, Embeds: action.Message.Embeds}
			if len(resp.Data.Embeds) >= 1 {
				if desc := resp.Data.Embeds[0].Description; desc != "" {
					if unquoted, ok := strings.CutPrefix(desc, "> "); ok {
						resp.Data.Embeds[0].Description = fmt.Sprintf("> ||%s||", unquoted)
					}
				}
			}
			delete(reportLog[uuid], ynoMsgId)
			markAsResolved(uuid)
		case "ack":
			targetName := getNameFromUuid(uuid)
			content := fmt.Sprintf("*Report on %s acknowledged by %s*", targetName, action.Member.DisplayName())

			resp.Type = discordgo.InteractionResponseUpdateMessage
			resp.Data = &discordgo.InteractionResponseData{Content: content, Embeds: action.Message.Embeds}
			if len(resp.Data.Embeds) >= 1 {
				if desc := resp.Data.Embeds[0].Description; desc != "" {
					if unquoted, ok := strings.CutPrefix(desc, "> "); ok {
						resp.Data.Embeds[0].Description = fmt.Sprintf("> ||%s||", unquoted)
					}
				}
			}
			delete(reportLog[uuid], ynoMsgId)
			markAsResolved(uuid)
		case "cmd":
			if len(data.Values) != 1 {
				return
			}
			switch data.Values[0] {
			case "reveal":
				reports, err := getReportersForPlayer(uuid, ynoMsgId)
				if err != nil {
					log.Printf("getReportersForPlayer: %s", err)
					return
				}
				reportsContent := ""
				for reporter, reason := range reports {
					reportsContent += fmt.Sprintf("%s: `%s`  \n", reporter, getReadableReportReason(reason))
				}
				resp.Type = discordgo.InteractionResponseChannelMessageWithSource
				resp.Data = &discordgo.InteractionResponseData{
					Embeds: []*discordgo.MessageEmbed{
						{
							Title:       fmt.Sprintf("Reporters for `msgid=%s`", ynoMsgId),
							Description: reportsContent,
						},
					},
				}
			case "dban":
				targetName := getNameFromUuid(uuid)
				for game := range gameIdToName {
					banPlayerInGameUnchecked(game, uuid, true)
				}

				content := fmt.Sprintf("*%s has been **banned** by %s*", targetName, action.Member.DisplayName())

				resp.Type = discordgo.InteractionResponseUpdateMessage
				resp.Data = &discordgo.InteractionResponseData{Content: content, Embeds: action.Message.Embeds}
				if len(resp.Data.Embeds) >= 1 {
					if desc := resp.Data.Embeds[0].Description; desc != "" {
						if unquoted, ok := strings.CutPrefix(desc, "> "); ok {
							resp.Data.Embeds[0].Description = fmt.Sprintf("> ||%s||", unquoted)
						}
					}
				}
				delete(reportLog[uuid], ynoMsgId)
				markAsResolved(uuid)
			}

			// reset the selection
			edit := discordgo.NewMessageEdit(config.moderation.channelId, action.Interaction.Message.ID)
			edit.Components = &action.Interaction.Message.Components
			if _, err = bot.ChannelMessageEditComplex(edit); err != nil {
				log.Printf("bot/cmd/edit: %s", err)
			}
		}
	})

	if err = bot.Open(); err != nil {
		log.Printf("bot/open: %s", err)
		return
	}
}

func parseMsgIdFromComponent(msgObj *discordgo.Message) string {
	if msgObj == nil || len(msgObj.Embeds) < 1 || len(msgObj.Embeds[0].Fields) < 3 {
		log.Printf("bot/cmd: message interaction absent")
		return ""
	}
	metadataField := msgObj.Embeds[0].Fields[2]
	msgIdMatch := msgIdPattern.FindSubmatch([]byte(metadataField.Value))
	if msgIdMatch == nil {
		return ""
	}
	ynoMsgId := string(msgIdMatch[1])
	return ynoMsgId
}

// obj should be an outpointer to one of the compatible message types
func formatReportLog(obj interface{}, targetUuid, ynoMsgId, originalMsg string, reasons map[string]int) {
	targetName := getNameFromUuid(targetUuid)
	if originalMsg != "" {
		originalMsg = fmt.Sprintf("> *%s*", originalMsg)
	}
	reasonsString := ""
	for reason, count := range reasons {
		reasonsString += fmt.Sprintf("- `%s`: %d\n", getReadableReportReason(reason), count)
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
	}
	if ynoMsgId != "" {
		components = append(components, discordgo.ActionsRow{
			Components: []discordgo.MessageComponent{
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
						{
							Label: "Banish",
							Value: "dban",
						},
					},
				},
			},
		})
	}

	content := fmt.Sprintf("<@&%s>", config.moderation.modRoleId)
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

	msgid, originalMsg, err := createReport(uuid, req.Uuid, req.Reason, req.MsgId, req.OriginalMsg)
	if err != nil {
		writeErrLog(uuid, r.URL.Path, "createReport failed: "+err.Error())
		handleError(w, r, "Could not create report")
		return
	}

	err = sendReportLog(req.Uuid, msgid, originalMsg)
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
WHERE targetUuid = ? AND NOT actionTaken
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
	if discordMsgId, ok := reportLog[uuid][ynoMsgId]; ok {
		payload := discordgo.NewMessageEdit(config.moderation.channelId, discordMsgId)
		formatReportLog(payload, uuid, ynoMsgId, originalMsg, reasons)
		msg, err = bot.ChannelMessageEditComplex(payload)
	} else {
		payload := &discordgo.MessageSend{}
		formatReportLog(payload, uuid, ynoMsgId, originalMsg, reasons)
		msg, err = bot.ChannelMessageSendComplex(config.moderation.channelId, payload)
	}

	if msg != nil && err == nil {
		var forUuid map[string]string
		if map_, ok := reportLog[uuid]; ok {
			forUuid = map_
		} else {
			forUuid = make(map[string]string)
			reportLog[uuid] = forUuid
		}
		forUuid[ynoMsgId] = msg.ID
	}

	return err
}

func createReport(uuid, targetUuid, reason, msgId, originalMsg string) (string, string, error) {
	var err error
	row := db.QueryRow("SELECT contents FROM chatMessages WHERE msgId = ? AND uuid = ? AND game = ?", msgId, targetUuid, config.gameName)
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

	var msgIdLink *string
	if msgId != "" {
		msgIdLink = &msgId
	}

	_, err = db.Exec(`
REPLACE INTO playerReports
	(uuid, targetUuid, msgId, game, reason, originalMsg, timestampReported, actionTaken)
VALUES
	(?, ?, ?, ?, ?, ?, NOW(), 0)`,
		uuid, targetUuid, msgIdLink, config.gameName, urlReplacer.Replace(reason), originalMsg)
	return msgId, originalMsg, err
}

func markAsResolved(targetUuid string) {
	_, err := db.Exec(`UPDATE playerReports SET actionTaken = 1 WHERE targetUuid = ?`, targetUuid)
	if err != nil {
		log.Printf("markAsResolved: %s", err)
	}
}
