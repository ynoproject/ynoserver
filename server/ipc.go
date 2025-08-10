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
	"errors"
	"fmt"
	"log"
	"net"
	"net/rpc"
	"os"
	"time"
)

// "Methods" can be defined on this actor which then can be called by sibling processes.
type IPC struct{}

type Void struct{}

type TryBanArgs struct {
	TargetUuid                       string
	Disconnect, Temporary, Broadcast bool
}

func (*IPC) TryBan(args TryBanArgs, _ *Void) error {
	return banPlayerUnchecked(args.TargetUuid, false, args.Disconnect, args.Temporary, args.Broadcast)
}

type TryMuteArgs struct {
	TargetUuid           string
	Temporary, Broadcast bool
}

func (*IPC) TryMute(args TryMuteArgs, _ *Void) error {
	return mutePlayerUnchecked(args.TargetUuid, false, args.Temporary, args.Broadcast)
}

type SendReportLogArgs struct {
	Uuid, YnoMsgId, OriginalMsg, Game string
}

func (*IPC) SendReportLog(args SendReportLogArgs, _ *Void) error {
	return sendReportLogMainServer(args.Uuid, args.YnoMsgId, args.OriginalMsg, args.Game)
}

type ScheduleModActionReversalArgs struct {
	Uuid   string
	Action int
	Expiry time.Time
}

func (*IPC) ScheduleModActionReversal(args ScheduleModActionReversalArgs, _ *Void) error {
	return scheduleModActionReversalMainServer(args.Uuid, args.Action, args.Expiry, true)
}

func (*IPC) UpdateEventVmInfo(args Void, _ *Void) error {
	_, err := updateEventVmInfo()
	return err
}

func banPlayerInGameUnchecked(game, uuid string, disconnect, temporary, broadcast bool) error {
	if game == config.gameName {
		return banPlayerUnchecked(uuid, true, disconnect, temporary, broadcast)
	}
	client, err := rpc.Dial("unix", fmt.Sprintf("/tmp/yno/%s.sck", game))
	if err != nil {
		return errors.Join(errors.New("could not dial rpc socket"), err)
	}

	defer client.Close()
	call := client.Go("IPC.TryBan", TryBanArgs{uuid, disconnect, temporary, broadcast}, new(Void), make(chan *rpc.Call, 1))
	select {
	case <-call.Done:
		return call.Error
	case <-time.After(config.ipc.deadline):
		return errors.New("banPlayerInGameUnchecked: timed out")
	}
}

func mutePlayerInGameUnchecked(game, uuid string, temporary, broadcast bool) error {
	if game == config.gameName {
		return mutePlayerUnchecked(uuid, true, temporary, broadcast)
	}
	client, err := rpc.Dial("unix", fmt.Sprintf("/tmp/yno/%s.sck", game))
	if err != nil {
		return errors.Join(errors.New("could not dial rpc socket"), err)
	}

	defer client.Close()
	call := client.Go("IPC.TryMute", TryMuteArgs{uuid, temporary, broadcast}, new(Void), nil)
	select {
	case <-call.Done:
		return call.Error
	case <-time.After(config.ipc.deadline):
		return errors.New("mutePlayerInGameUnchecked: timed out")
	}
}

func sendReportLog(uuid, ynoMsgId, originalMsg string) error {
	if isMainServer {
		return sendReportLogMainServer(uuid, ynoMsgId, originalMsg, config.gameName)
	}
	client, err := rpc.Dial("unix", fmt.Sprintf("/tmp/yno/%s.sck", mainGameId))
	if err != nil {
		return errors.Join(errors.New("could not dial rpc socket"), err)
	}

	defer client.Close()
	call := client.Go("IPC.SendReportLog", SendReportLogArgs{uuid, ynoMsgId, originalMsg, config.gameName}, new(Void), make(chan *rpc.Call, 1))
	select {
	case <-call.Done:
		return call.Error
	case <-time.After(config.ipc.deadline):
		return errors.New("mutePlayerInGameUnchecked: timed out")
	}
}

func scheduleModActionReversal(uuid string, action int, expiry time.Time) error {
	if isMainServer {
		return scheduleModActionReversalMainServer(uuid, action, expiry, false)
	}
	client, err := rpc.Dial("unix", fmt.Sprintf("/tmp/yno/%s.sck", mainGameId))
	if err != nil {
		return errors.Join(errors.New("could not dial rpc socket"), err)
	}

	defer client.Close()
	call := client.Go("IPC.ScheduleModActionReversal", ScheduleModActionReversalArgs{uuid, action, expiry}, new(Void), make(chan *rpc.Call, 1))
	select {
	case <-call.Done:
		return call.Error
	case <-time.After(config.ipc.deadline):
		return errors.New("scheduleModActionReversal: timed out")
	}
}

func notifyVmUpdated(gameId string) {
	if !isMainServer {
		return
	}

	client, err := rpc.Dial("unix", fmt.Sprintf("/tmp/yno/%s.sck", gameId))
	if err != nil {
		eprintf("VM", "socket for %s not found", gameId)
		return
	}

	defer client.Close()
	call := client.Go("IPC.UpdateEventVmInfo", Void{}, new(Void), make(chan *rpc.Call, 1))
	select {
	case <-call.Done:
		if err := call.Error; err != nil {
			eprintf("VM", "error notifying %s: %s", gameId, err)
		}
	case <-time.After(config.ipc.deadline):
		eprintf("VM", "%s not responding to IPC", gameId)
	}
}

func initRpc() {
	var err error
	socketPath := fmt.Sprintf("/tmp/yno/%s.sck", config.gameName)

	os.MkdirAll("/tmp/yno", 0777)
	os.Remove(socketPath)

	socket, err := net.Listen("unix", socketPath)
	if err != nil {
		log.Fatal("initRpc(listen):", err)
	}

	if err := os.Chmod(socketPath, 0666); err != nil {
		log.Fatal("initRpc(chmod):", err)
	}

	ipc := new(IPC)
	rpc.Register(ipc)
	go rpc.Accept(socket)
}
