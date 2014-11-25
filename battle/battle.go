// Copyright 2013 The Gorilla WebSocket Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"encoding/json"
	"time"

	"github.com/golang/glog"
)

func _battlelog() {
	glog.Info("")
}

type BattleState int

const (
	PREPARE BattleState = iota
	MATCHING
	ONELEFT
	FINISH
)

type Battle struct {
	state     BattleState
	readyConn *Connection
	secret    string
}

func makeBattle() *Battle {
	battle := new(Battle)
	battle.state = PREPARE
	battle.readyConn = nil
	battle.secret = genUUID()
	return battle
}

func battleReady(conn *Connection, msg []byte) {
	battle := conn.battle
	if battle == nil || battle.state != PREPARE {
		conn.sendErr("err_state")
		return
	}
	if battle.readyConn == nil {
		battle.readyConn = conn
		conn.sendType("ready")
	} else if battle.readyConn == conn.foe {
		battle.state = MATCHING
		msg := []byte(`{"Type":"start"}`)
		conn.send <- msg
		conn.foe.send <- msg
	}
}

func battleProgress(conn *Connection, msg []byte) {
	if conn.battle == nil {
		conn.sendErr("need pair")
		return
	}
	if conn.battle.state != MATCHING || conn.battle.state != FINISH {
		conn.sendErr("battle state error")
		return
	}

	var in struct {
		CompleteNum int
	}

	err := json.Unmarshal(msg, &in)
	if err != nil {
		conn.sendErr("json error")
		return
	}

	if in.CompleteNum <= 0 {
		conn.sendErr("in.CompleteNum")
		return
	}

	conn.foe.send <- msg
}

func battleFinish(conn *Connection, msg []byte) {
	if conn.battle == nil {
		conn.sendErr("need pair")
		return
	}
	if conn.battle.state == FINISH {
		return
	}
	if conn.battle.state != MATCHING {
		conn.sendErr("battle state error")
		return
	}

	//in
	var in struct {
		Msec int
	}

	err := json.Unmarshal(msg, &in)
	if err != nil {
		conn.sendErr("json error")
		return
	}

	//check exist
	if conn.result != 0 {
		conn.sendErr("result exist")
		return
	}
	conn.result = in.Msec

	//end
	if conn.foe.result != 0 {
		conn.battle.state = FINISH
		myMsec := conn.result
		foeMsec := conn.foe.result

		out := struct {
			Type    string
			MyMsec  int
			FoeMsec int
		}{
			"result",
			myMsec,
			foeMsec,
		}

		conn.sendMsg(out)

		//foe
		out.FoeMsec = myMsec
		out.MyMsec = foeMsec
		conn.foe.sendMsg(out)
	} else {
		out := struct {
			Type string
			Msec int
		}{
			"foeFinish",
			in.Msec,
		}
		conn.foe.sendMsg(out)

		time.Sleep(5 * time.Second)
		if conn.battle.state != FINISH {
			conn.battle.state = FINISH

			out := struct {
				Type    string
				MyMsec  int
				FoeMsec int
			}{
				"result",
				in.Msec,
				-1,
			}
			conn.sendMsg(out)

			//foe
			out.FoeMsec = in.Msec
			out.MyMsec = -1
			conn.foe.sendMsg(out)
		}
	}
}

func talk(conn *Connection, msg []byte) {
	var in struct {
		Text string
	}

	err := json.Unmarshal(msg, &in)
	if err != nil {
		conn.sendErr("json error")
		return
	}
	if len(in.Text) > 0 && conn.foe != nil {
		conn.foe.send <- msg
	}
}

func regBattle() {
	regHandler("ready", MsgHandler(battleReady))
	regHandler("progress", MsgHandler(battleProgress))
	regHandler("finish", MsgHandler(battleFinish))
	regHandler("talk", MsgHandler(talk))
}
