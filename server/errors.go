/*
	Copyright (C) 2021-2022  The YNOproject Developers

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

import "errors"

var (
	errBadReqSize     = errors.New("bad request size")
	errBadSignature   = errors.New("bad signature")
	errBadCounter     = errors.New("bad counter")
	errBcryptError    = errors.New("bcrypt error")
	errInvalidClient  = errors.New("invalid client")
	errInvalidPrevMap = errors.New("invalid prev map ID")
	errInvalidMsg     = errors.New("invalid message")
	errInvalidUTF8    = errors.New("invalid UTF-8")
	errInsuffRank     = errors.New("insufficient rank")
	errLenMismatch    = errors.New("command length mismatch")
	errNoParty        = errors.New("player not in a party")
	errSelfBan        = errors.New("attempted self-ban")
	errSelfUnban      = errors.New("attempted self-unban")
	errSelfMute       = errors.New("attempted self-mute")
	errSelfUnmute     = errors.New("attempted self-unmute")
	errUnkMsgType     = errors.New("unknown message type")
	errUserNotFound   = errors.New("user not found")
)
