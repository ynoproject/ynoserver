package main

type AccessType int64

const (
	Muted        AccessType = 1
	ShadowBanned            = 2
	Banned                  = 4
)

func (accessType AccessType) isMuted() bool {
	return accessType&Muted > 0
}

func (accessType AccessType) isShadowBanned() bool {
	return accessType&ShadowBanned > 0
}

func (accessType AccessType) isBanned() bool {
	return accessType&Banned > 0
}

func getPlayerInfo(conn *ConnInfo) (uuid string, name string, rank int, badge string, accessType AccessType, account bool) {
	if conn.Token != "" {
		uuid, name, rank, badge, accessType = readPlayerDataFromToken(conn.Token)
		if uuid != "" { //if we got a uuid back then we're logged in
			account = true
		}
	}

	if !account {
		uuid, rank, accessType = readPlayerData(conn.Ip)
	}

	if badge == "" {
		badge = "null"
	}

	return uuid, name, rank, badge, accessType, account
}
