package main

import (
	"github.com/golang/glog"
	"github.com/henyouqian/lwutil"
	"net/http"
	"time"
)

func battleGlog() {
	glog.Info("")
}

const (
	BATTLE_PACK_USER  = "uuid/9DA924BB-6327-4C8A-BA4D-B010765478CD"
	BATTLE_PACKID_SET = "BATTLE_PACKID_SET"
)

type BattleRoom struct {
	Name      string
	Title     string
	BetCoin   int
	PlayerNum int
}

var (
	BATTLE_ROOM_LIST = []BattleRoom{
		// {"free", "不毛之地", 0, 0},
		// {"coin1", "苏堤春晓", 1, 0},
		// {"coin2", "曲院风荷", 2, 0},
		// {"coin5", "平湖秋月", 5, 0},
		// {"coin10", "断桥残雪", 10, 0},
		// {"coin20", "柳浪闻莺", 20, 0},
		{"free", "无座", 0, 0},
		{"coin1", "硬座", 1, 0},
		{"coin2", "软座", 2, 0},
		{"coin5", "硬卧", 5, 0},
		{"coin10", "软卧", 10, 0},
		{"coin20", "高级软卧", 20, 0},
	}
)

func apiBattleRoomList(w http.ResponseWriter, r *http.Request) {
	lwutil.CheckMathod(r, "POST")

	//session
	_, err := findSession(w, r, nil)
	lwutil.CheckError(err, "err_auth")

	//out
	lwutil.WriteResponse(w, BATTLE_ROOM_LIST)
}

func updateBattleRoomPlayerNumTask() {
	for true {
		time.Sleep(1 * time.Minute)
		idxMap := make(map[string]int)
		for i, v := range BATTLE_ROOM_LIST {
			idxMap[v.Name] = i
		}
	}
}

func regBattle() {
	http.Handle("/battle/roomList", lwutil.ReqHandler(apiBattleRoomList))
	//go updateBattleRoomPlayerNumTask()
}
