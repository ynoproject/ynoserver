package server

import "errors"

var (
	errBadReqSize    = errors.New("bad request size")
	errBadSignature  = errors.New("bad signature")
	errBadCounter    = errors.New("bad counter")
	errBcryptError   = errors.New("bcrypt error")
	errInvalidClient = errors.New("invalid client")
	errInvalidPrevMap = errors.New("invalid prev map ID")
	errInvalidMsg    = errors.New("invalid message")
	errInvalidUTF8   = errors.New("invalid UTF-8")
	errInsuffRank    = errors.New("insufficient rank")
	errLenMismatch   = errors.New("command length mismatch")
	errNoParty       = errors.New("player not in a party")
	errSelfBan       = errors.New("attempted self-ban")
	errSelfUnban     = errors.New("attempted self-unban")
	errSelfMute      = errors.New("attempted self-mute")
	errSelfUnmute    = errors.New("attempted self-unmute")
	errUnkMsgType    = errors.New("unknown message type")
	errUserNotFound  = errors.New("user not found")
)
