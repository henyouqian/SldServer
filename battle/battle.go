// Copyright 2013 The Gorilla WebSocket Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"encoding/json"
	"strconv"
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

const (
	WAIT_TIME_SEC = 5
)

type Battle struct {
	state      BattleState
	readyConn  *Connection
	secret     string
	createTime time.Time
	room       BattleRoom
}

type BattleRoom struct {
	Name    string
	BetCoin int
}

type BattleResult struct {
	Type       string
	Result     string //win, lose, draw
	MyMsec     int
	FoeMsec    int
	RewardCoin int
	TotalCoin  int
}

var (
	BATTLE_ROOM_MAP = map[string]BattleRoom{
		"free":   {"free", 0},
		"coin1":  {"coin1", 1},
		"coin2":  {"coin2", 2},
		"coin5":  {"coin5", 5},
		"coin10": {"coin10", 10},
		"coin20": {"coin20", 20},
	}
)

func makeBattle() *Battle {
	battle := new(Battle)
	battle.state = PREPARE
	battle.readyConn = nil
	battle.secret = genUUID()
	battle.createTime = time.Now()
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
		dt := time.Now().Sub(battle.createTime)
		waitTime := WAIT_TIME_SEC * time.Second
		startMatch := func() {
			battle.state = MATCHING
			msg := []byte(`{"Type":"start"}`)
			conn.send <- msg
			conn.foe.send <- msg
		}
		if dt < waitTime {
			go func() {
				time.Sleep(waitTime - dt)
				startMatch()
			}()
		} else {
			startMatch()
		}
	}
}

func battleProgress(conn *Connection, msg []byte) {
	if conn.battle == nil {
		conn.sendErr("need pair")
		return
	}
	if conn.battle.state != MATCHING && conn.battle.state != FINISH {
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
		errstr := makeBattleResult(conn, false)
		if errstr != "" {
			conn.sendErr(errstr)
		}
	} else {
		out := struct {
			Type string
			Msec int
		}{
			"foeFinish",
			in.Msec,
		}
		conn.foe.sendMsg(out)

		go func() {
			time.Sleep(5 * time.Second)
			if conn.battle != nil && conn.battle.state != FINISH {
				errstr := makeBattleResult(conn, false)
				if errstr != "" {
					conn.sendErr(errstr)
				}
			}
		}()
	}
}

func makeBattleResult(conn *Connection, isDisconnect bool) string {
	if conn.battle == nil {
		return "err_no_battle"
	}
	if conn.foe == nil {
		return "err_no_foe"
	}
	if conn.battle.state == FINISH {
		return "err_already_finish"
	}
	conn.battle.state = FINISH

	//ssdb
	ssdbc, err := ssdbMatchPool.Get()
	if err != nil {
		return "err_ssdb_pool"
	}
	defer ssdbc.Close()

	//
	myMsec := conn.result
	foeMsec := conn.foe.result

	betCoin := conn.battle.room.BetCoin

	result := "lose"

	if isDisconnect { //lose
		result = "lose"
	} else {
		if myMsec == 0 {
			result = "lose"
		} else if foeMsec == 0 || myMsec < foeMsec {
			result = "win"
		} else if myMsec != 0 && foeMsec != 0 && myMsec == foeMsec {
			result = "draw"
		}
	}

	getCoin := 0
	if result == "win" {
		getCoin = betCoin
	} else if result == "draw" {
		getCoin = 0
	} else {
		getCoin = -betCoin
	}

	out := BattleResult{
		"result",
		result,
		myMsec,
		foeMsec,
		getCoin,
		0,
	}

	//add coin to ssdb
	key := makePlayerInfoKey(conn.playerInfo.UserId)
	resp, err := ssdbc.Do("hincr", key, PLAYER_GOLD_COIN, out.RewardCoin)
	if err != nil || resp[0] != "ok" {
		return "err_ssdb"
	}

	coinNum, err := strconv.Atoi(resp[1])
	if err != nil {
		return "err_strconv"
	}
	out.TotalCoin = coinNum

	//send to me
	conn.sendMsg(out)

	//foe
	if result == "win" {
		out.Result = "lose"
		out.RewardCoin = -betCoin
	} else if result == "lose" {
		out.Result = "win"
		out.RewardCoin = betCoin
	} else if result == "draw" {
		out.RewardCoin = 0
	}
	out.FoeMsec = myMsec
	out.MyMsec = foeMsec

	//ssdb
	key = makePlayerInfoKey(conn.foe.playerInfo.UserId)
	resp, err = ssdbc.Do("hincr", key, PLAYER_GOLD_COIN, out.RewardCoin)
	if err != nil || resp[0] != "ok" {
		return "err_ssdb"
	}

	coinNum, err = strconv.Atoi(resp[1])
	if err != nil {
		return "err_strconv"
	}
	out.TotalCoin = coinNum

	//send to foe
	conn.foe.sendMsg(out)

	return ""
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
