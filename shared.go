package main

func getPlayerInfo(conn *ConnInfo) (uuid string, name string, rank int, badge string, banned bool, muted bool, account bool) {
	if conn.Token != "" {
		uuid, name, rank, badge, banned, muted = readPlayerDataFromToken(conn.Token)
		if uuid != "" { //if we got a uuid back then we're logged in
			account = true
		}
	}

	if !account {
		uuid, rank, banned, muted = readPlayerData(conn.Ip)
	}

	if badge == "" {
		badge = "null"
	}

	return uuid, name, rank, badge, banned, muted, account
}
