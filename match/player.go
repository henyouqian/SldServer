package main

import (
	"./ssdb"
	"encoding/json"
	"fmt"
	"github.com/golang/glog"
	"github.com/henyouqian/lwutil"
	"github.com/qiniu/api/rs"
	"math"
	"net/http"
	"strconv"
	"strings"
)

const (
	H_PLAYER_INFO              = "H_PLAYER_INFO"      //key:H_PLAYER_INFO/<userId> subkey:property
	H_PLAYER_INFO_LITE         = "H_PLAYER_INFO_LITE" //key:H_PLAYER_INFO_LITE subkey:userId value:playerInfoLite
	H_APP_PLAYER_RATE          = "H_APP_PLAYER_RATE"  //subkey:appName/userId value:1
	ADS_PERCENT_DEFAUT         = 0.5
	H_PLAYER_PRIZE_RECORD      = "H_PLAYER_PRIZE_RECORD" //key:H_PLAYER_PRIZE_RECORD subkey:prizeRecordId value prizeRecordJson
	Z_PLAYER_PRIZE_RECORD      = "Z_PLAYER_PRIZE_RECORD" //key:Z_PLAYER_PRIZE_RECORD/userId subkey:prizeRecordId score:timeUnix
	SEREAL_PLAYER_PRIZE_RECORD = "SEREAL_PLAYER_PRIZE_RECORD"
	BATTLE_HEART_TOTAL         = 10
	BATTLE_HEART_ADD_SEC       = 60 * 5
	Z_PLAYER_FAN               = "Z_PLAYER_FAN"    //key:Z_PLAYER_FAN/userId subkey:fanUserId score:time
	Z_PLAYER_FOLLOW            = "Z_PLAYER_FOLLOW" //key:Z_PLAYER_FOLLOW/userId subkey:userId score:time
	Z_PLAYER_SEARCH            = "Z_PLAYER_SEARCH" //key:Z_PLAYER_SEARCH/nickName subkey:userId score:time
	H_PLAYER_NAME              = "H_PLAYER_NAME"   //key:H_PLAYER_NAME subkey:playerName value:userId
)

type PlayerInfoLite struct {
	UserId          int64
	NickName        string
	TeamName        string
	Gender          int
	CustomAvatarKey string
	GravatarKey     string
	Text            string
}

type PlayerInfo struct {
	UserId              int64
	NickName            string
	TeamName            string
	Description         string
	Gender              int
	CustomAvatarKey     string
	GravatarKey         string
	Email               string
	GoldCoin            int
	Prize               int
	PrizeCache          int
	TotalPrize          int
	Secret              string
	BattlePoint         int
	BattleWinStreak     int
	BattleWinStreakMax  int
	BattleHeartZeroTime int64
	FanNum              int
	FollowNum           int
}

//player property
const (
	PLAYER_NICK_NAME              = "NickName"
	PLAYER_GOLD_COIN              = "GoldCoin"
	PLAYER_PRIZE                  = "Prize"
	PLAYER_PRIZE_CACHE            = "PrizeCache"
	PLAYER_TOTAL_PRIZE            = "TotalPrize"
	PLAYER_IAP_SECRET             = "IapSecret"
	PLAYER_BATTLE_POINT           = "BattlePoint"
	PLAYER_BATTLE_WIN_STREAK      = "BattleWinStreak"
	PLAYER_BATTLE_WIN_STREAK_MAX  = "BattleWinStreakMax"
	PLAYER_BATTLE_HEART_ZERO_TIME = "BattleHeartZeroTime"
	PLAYER_FAN_NUM                = "FanNum"
	PLAYER_FOLLOW_NUM             = "FollowNum"
)

type PlayerBattleLevel struct {
	Level      int
	Title      string
	StartPoint int
}

const (
	BATTLE_HELP_TEXT = `✦ 赢了加分加金币，输了不扣分扣金币。

✦ 赢了的话，连胜几场就加几分。

✦ 在使用金币的场次下（除了第一个免费场外），终结了对手连胜纪录的话，额外获得等于对手连胜场次的积分。

✦ 所以想要得高分就尽可能保持连胜，或者凭运气碰到高连胜对手并取得胜利（金币场次）。

✦ 第一个免费场不会赢也不会输掉金币，但每玩一次会消耗一颗心。

✦ 积分与等级的对应关系如下：
`
)

var (
	PLAYER_BATTLE_LEVELS = []PlayerBattleLevel{
		// {1, "🚶", 0},
		// {2, "🚣", 10},
		// {3, "🚲", 30},
		// {4, "🚜", 60},
		// {5, "🚛", 100},
		// {6, "🚚", 150},
		// {7, "🚗", 200},
		// {8, "🚙", 250},
		// {9, "🚌", 300},
		// {10, "🚃", 400},
		// {11, "🚤", 500},
		// {12, "🚈", 600},
		// {13, "🚄", 700},
		// {14, "🚁", 800},
		// {15, "✈️", 900},
		// {16, "🚀", 1000},
		// {17, "🐭", 1100},
		// {18, "🐮", 1200},
		// {19, "🐯", 1300},
		// {20, "🐰", 1400},
		// {21, "🐲", 1500},
		// {22, "🐍", 1600},
		// {23, "🐴", 1700},
		// {24, "🐑", 1800},
		// {25, "🐵", 1900},
		// {26, "🐔", 2000},
		// {27, "🐶", 2100},
		// {28, "🐷", 2200},
		{1, "🐭", 0},
		{2, "🐮", 10},
		{3, "🐯", 30},
		{4, "🐰", 60},
		{5, "🐲", 100},
		{6, "🐍", 150},
		{7, "🐴", 200},
		{8, "🐑", 250},
		{9, "🐵", 300},
		{10, "🐔", 350},
		{11, "🐶", 400},
		{12, "🐷", 450},
		{13, "🚶", 500},
		{14, "🚣", 600},
		{15, "🚲", 700},
		{16, "🚜", 800},
		{17, "🚛", 900},
		{18, "🚚", 1000},
		{19, "🚗", 1100},
		{20, "🚙", 1200},
		{21, "🚌", 1300},
		{22, "🚃", 1400},
		{23, "🚤", 1500},
		{24, "🚈", 1600},
		{25, "🚄", 1700},
		{26, "🚁", 1800},
		{27, "✈️", 1900},
		{28, "🚀", 2000},
	}
)

const (
	PRIZE_REASON_RANK  = "排名奖励"
	PRIZE_REASON_LUCK  = "幸运大奖"
	PRIZE_REASON_OWNER = "发布分成"
)

type PrizeRecord struct {
	Id      int64
	MatchId int64
	Thumb   string
	Reason  string
	Prize   int
	Rank    int
}

func makeZPlayerPrizeRecordKey(userId int64) string {
	return fmt.Sprintf("%s, %d", Z_PLAYER_PRIZE_RECORD, userId)
}

func addPrizeToCache(ssdbc *ssdb.Client, userId int64, matchId int64, matchThumb string, prize int, reason string, rank int) {
	if prize <= 0 {
		return
	}
	playerKey := makePlayerInfoKey(userId)
	addPrizeCache(ssdbc, playerKey, prize)

	var record PrizeRecord
	record.Id = GenSerial(ssdbc, SEREAL_PLAYER_PRIZE_RECORD)
	record.Thumb = matchThumb
	record.Reason = reason
	record.MatchId = matchId
	record.Prize = prize
	record.Rank = rank
	js, err := json.Marshal(record)
	lwutil.CheckError(err, "")

	resp, err := ssdbc.Do("hset", H_PLAYER_PRIZE_RECORD, record.Id, js)
	lwutil.CheckSsdbError(resp, err)

	zkey := makeZPlayerPrizeRecordKey(userId)
	resp, err = ssdbc.Do("zset", zkey, record.Id, record.Id)
	lwutil.CheckSsdbError(resp, err)
}

func init() {
	glog.Info("")

	//check player battle levels
	currLv := 1
	currPt := -1
	for _, v := range PLAYER_BATTLE_LEVELS {
		if v.Level != currLv {
			panic("v.Level != currLv")
		}
		if v.StartPoint <= currPt {
			glog.Error(v.StartPoint, currPt)
			panic("v.StartPoint <= currPt")
		}
		currLv++
		currPt = v.StartPoint
	}
}

func makePlayerInfoKey(userId int64) string {
	return fmt.Sprintf("%s/%d", H_PLAYER_INFO, userId)
}

func makeAppPlayerRateSubkey(appName string, userId int64) string {
	return fmt.Sprintf("%s/%d", appName, userId)
}

func makeZPlayerFanKey(userId int64) string {
	return fmt.Sprintf("%s/%d", Z_PLAYER_FAN, userId)
}

func makeZPlayerFollowKey(userId int64) string {
	return fmt.Sprintf("%s/%d", Z_PLAYER_FOLLOW, userId)
}

func getPlayerInfo(ssdb *ssdb.Client, userId int64) (*PlayerInfo, error) {
	key := makePlayerInfoKey(userId)

	var playerInfo PlayerInfo
	err := ssdb.HGetStruct(key, &playerInfo)
	if err != nil {
		return nil, err
	}

	playerInfo.UserId = userId

	return &playerInfo, err
}

func getPlayerFanFollowNum(ssdbc *ssdb.Client, userId int64) (fanNum, followNum int) {
	fanNum = 0
	followNum = 0
	key := makePlayerInfoKey(userId)
	resp, err := ssdbc.Do("multi_hget", key, PLAYER_FAN_NUM, PLAYER_FOLLOW_NUM)
	resp = resp[1:]
	if len(resp) > 1 {
		n := len(resp)
		for i := 0; i < n/2; i++ {
			k := resp[i*2]
			if k == PLAYER_FAN_NUM {
				fanNum, err = strconv.Atoi(resp[i*2+1])
				lwutil.CheckError(err, "err_strconv")
			} else if k == PLAYER_FOLLOW_NUM {
				followNum, err = strconv.Atoi(resp[i*2+1])
				lwutil.CheckError(err, "err_strconv")
			}
		}
	}

	return
}

func getPlayerInfoLite(ssdbc *ssdb.Client, userId int64, playerInfo *PlayerInfo) (*PlayerInfoLite, error) {
	resp, err := ssdbc.Do("hget", H_PLAYER_INFO_LITE, userId)
	var playerInfoLite PlayerInfoLite
	if resp[0] == ssdb.NOT_FOUND {
		if playerInfo == nil {
			playerInfo, err = getPlayerInfo(ssdbc, userId)
			if err != nil {
				return nil, err
			}
		}
		playerInfoLite.UserId = playerInfo.UserId
		playerInfoLite.NickName = playerInfo.NickName
		playerInfoLite.TeamName = playerInfo.TeamName
		playerInfoLite.Gender = playerInfo.Gender
		playerInfoLite.CustomAvatarKey = playerInfo.CustomAvatarKey
		playerInfoLite.GravatarKey = playerInfo.GravatarKey
		playerInfoLite.Text = ""

		js, err := json.Marshal(playerInfoLite)
		if err != nil {
			return nil, err
		}
		_, err = ssdbc.Do("hset", H_PLAYER_INFO_LITE, userId, js)
		if err != nil {
			return nil, err
		}
	} else {
		err = json.Unmarshal([]byte(resp[1]), &playerInfoLite)
		if err != nil {
			return nil, err
		}
	}
	return &playerInfoLite, nil
}

func addPlayerGoldCoin(ssc *ssdb.Client, playerKey string, addNum int) (rNum int) {
	resp, err := ssc.Do("hincr", playerKey, PLAYER_GOLD_COIN, addNum)
	lwutil.CheckSsdbError(resp, err)
	num, err := strconv.Atoi(resp[1])
	lwutil.CheckError(err, "")
	return num
}
func getPlayerGoldCoin(ssc *ssdb.Client, playerKey string) (rNum int) {
	resp, err := ssc.Do("hget", playerKey, PLAYER_GOLD_COIN)
	lwutil.CheckError(err, "")
	if resp[0] == ssdb.NOT_FOUND {
		return 0
	}

	num, err := strconv.Atoi(resp[1])
	lwutil.CheckError(err, "")
	return num
}

func getPrize(ssc *ssdb.Client, playerKey string) int {
	var prize int
	err := ssc.HGet(playerKey, PLAYER_PRIZE, &prize)
	lwutil.CheckError(err, "")
	return prize
}

func addPrize(ssc *ssdb.Client, playerKey string, prize int) (rPrize int) {
	resp, err := ssc.Do("hincr", playerKey, PLAYER_PRIZE, prize)
	lwutil.CheckSsdbError(resp, err)
	num, err := strconv.Atoi(resp[1])
	lwutil.CheckError(err, "")
	return num
}

func getPrizeCache(ssc *ssdb.Client, playerKey string) int {
	var prizeCache int
	err := ssc.HGet(playerKey, PLAYER_PRIZE_CACHE, &prizeCache)
	if err != nil {
		return 0
	}
	return prizeCache
}

func addPrizeCache(ssc *ssdb.Client, playerKey string, prize int) (rPrize int) {
	resp, err := ssc.Do("hincr", playerKey, PLAYER_PRIZE_CACHE, prize)
	lwutil.CheckSsdbError(resp, err)
	num, err := strconv.Atoi(resp[1])
	lwutil.CheckError(err, "")
	return num
}

func getPrizeTotal(ssc *ssdb.Client, playerKey string) int {
	var prize int
	err := ssc.HGet(playerKey, PLAYER_TOTAL_PRIZE, &prize)
	lwutil.CheckError(err, "")
	return prize
}

func addPrizeTotal(ssc *ssdb.Client, playerKey string, prize int) (rPrize int) {
	resp, err := ssc.Do("hincr", playerKey, PLAYER_TOTAL_PRIZE, prize)
	lwutil.CheckSsdbError(resp, err)
	num, err := strconv.Atoi(resp[1])
	lwutil.CheckError(err, "")
	return num
}

func isFollowed(ssdbc *ssdb.Client, fanId int64, followId int64) (bool, error) {
	key := makeZPlayerFollowKey(fanId)
	resp, err := ssdbc.Do("zexists", key, followId)
	if err != nil {
		return false, err
	}

	if resp[1] == "1" {
		return true, nil
	}
	return false, nil
}

func apiGetPlayerInfo(w http.ResponseWriter, r *http.Request) {
	var err error
	lwutil.CheckMathod(r, "POST")

	//session
	session, err := findSession(w, r, nil)
	lwutil.CheckError(err, "err_auth")

	//ssdb
	ssdb, err := ssdbPool.Get()
	lwutil.CheckError(err, "")
	defer ssdb.Close()

	//in
	var in struct {
		UserId int64
	}
	err = lwutil.DecodeRequestBody(r, &in)
	if in.UserId == 0 {
		in.UserId = session.Userid
	}

	//get info
	playerInfo, err := getPlayerInfo(ssdb, in.UserId)
	if err != nil {
		w.WriteHeader(500)
		lwutil.SendError("err_user_id", fmt.Sprintf("userId:%d", in.UserId))
		return
	}

	//followed?
	followed := false
	if session != nil && session.Userid != in.UserId {
		followed, err = isFollowed(ssdb, session.Userid, playerInfo.UserId)
		lwutil.CheckError(err, "err_follow")
	}

	//out
	out := struct {
		*PlayerInfo
		AdsConf              AdsConf
		ClientConf           map[string]string
		OwnerPrizeProportion float32
		BattleLevels         []PlayerBattleLevel
		BattleHelpText       string
		BattleHeartAddSec    int
		Followed             bool
	}{
		playerInfo,
		_adsConf,
		_clientConf,
		MATCH_OWNER_PRIZE_PROPORTION,
		PLAYER_BATTLE_LEVELS,
		BATTLE_HELP_TEXT,
		BATTLE_HEART_ADD_SEC,
		followed,
	}
	lwutil.WriteResponse(w, out)
}

func getBattleHeartNum(playerInfo *PlayerInfo) int {
	dt := lwutil.GetRedisTimeUnix() - playerInfo.BattleHeartZeroTime
	heartNum := int(dt) / BATTLE_HEART_ADD_SEC
	if heartNum > BATTLE_HEART_TOTAL {
		heartNum = BATTLE_HEART_TOTAL
	}
	return heartNum
}

func apiSetPlayerInfo(w http.ResponseWriter, r *http.Request) {
	var err error
	lwutil.CheckMathod(r, "POST")

	//session
	session, err := findSession(w, r, nil)
	lwutil.CheckError(err, "err_auth")

	//in
	var in struct {
		NickName        string
		GravatarKey     string
		CustomAvatarKey string
		TeamName        string
		Email           string
		Gender          int
	}
	err = lwutil.DecodeRequestBody(r, &in)
	lwutil.CheckError(err, "err_decode_body")
	if in.Gender > 2 {
		lwutil.SendError("err_gender", "")
	}

	stringLimit(&in.NickName, 40)
	stringLimit(&in.GravatarKey, 20)
	stringLimit(&in.CustomAvatarKey, 80)
	stringLimit(&in.TeamName, 40)
	stringLimit(&in.Email, 40)

	//check playerInfo
	if in.NickName == "" || in.TeamName == "" || (in.GravatarKey == "" && in.CustomAvatarKey == "") {
		lwutil.SendError("err_info_incomplete", "")
	}

	//ssdb
	ssdbc, err := ssdbPool.Get()
	lwutil.CheckError(err, "")
	defer ssdbc.Close()

	//check name
	resp, err := ssdbc.Do("hget", H_PLAYER_NAME, in.NickName)
	lwutil.CheckError(err, "err_ssdb")
	if resp[0] == SSDB_OK && len(resp) == 2 {
		uid, err := strconv.ParseInt(resp[1], 10, 64)
		lwutil.CheckError(err, "err_strconv")
		if uid != session.Userid {
			lwutil.SendError("err_name_taken", "")
		}
	}

	//
	playerKey := makePlayerInfoKey(session.Userid)

	//check playerInfo exist
	oldNickName := ""

	resp, err = ssdbc.Do("hget", playerKey, PLAYER_NICK_NAME)
	lwutil.CheckError(err, "err_hget")
	if resp[0] != SSDB_NOT_FOUND {
		oldNickName = resp[1]
	}

	//save
	err = ssdbc.HSetStruct(playerKey, in)
	lwutil.CheckError(err, "")

	//get player info
	playerInfo, err := getPlayerInfo(ssdbc, session.Userid)
	lwutil.CheckError(err, "")

	//set playerInfoLite
	infoLite := makePlayerInfoLite(playerInfo)

	js, err := json.Marshal(infoLite)
	lwutil.CheckError(err, "err_js")
	resp, err = ssdbc.Do("hset", H_PLAYER_INFO_LITE, infoLite.UserId, js)
	lwutil.CheckError(err, "")

	//updatePlayerSearchInfo
	updatePlayerSearchInfo(ssdbc, oldNickName, in.NickName, session.Userid)

	//out
	out := struct {
		PlayerInfo
	}{
		*playerInfo,
	}
	lwutil.WriteResponse(w, out)
}

func savePlayerInfo(ssdbc *ssdb.Client, userId int64, player interface{}) *PlayerInfo {
	playerKey := makePlayerInfoKey(userId)

	//check playerInfo exist
	oldNickName := ""

	resp, err := ssdbc.Do("hget", playerKey, PLAYER_NICK_NAME)
	lwutil.CheckError(err, "err_hget")
	if resp[0] != SSDB_NOT_FOUND {
		oldNickName = resp[1]
	}

	err = ssdbc.HSetStruct(playerKey, player)
	lwutil.CheckError(err, "")

	//get player info
	playerInfo, err := getPlayerInfo(ssdbc, userId)
	lwutil.CheckError(err, "")

	//set playerInfoLite
	infoLite := makePlayerInfoLite(playerInfo)

	js, err := json.Marshal(infoLite)
	lwutil.CheckError(err, "err_js")
	resp, err = ssdbc.Do("hset", H_PLAYER_INFO_LITE, infoLite.UserId, js)
	lwutil.CheckError(err, "")

	//updatePlayerSearchInfo
	updatePlayerSearchInfo(ssdbc, oldNickName, infoLite.NickName, userId)

	return playerInfo
}

func updatePlayerSearchInfo(ssdbc *ssdb.Client, oldName, newName string, userId int64) {
	if oldName == newName {
		return
	}

	resp, err := ssdbc.Do("hset", H_PLAYER_NAME, newName, userId)
	lwutil.CheckSsdbError(resp, err)
	if oldName != "" {
		resp, err := ssdbc.Do("hdel", H_PLAYER_NAME, oldName, userId)
		lwutil.CheckSsdbError(resp, err)
	}

	oldName = strings.ToLower(oldName)
	newName = strings.ToLower(newName)

	if oldName == newName {
		return
	}
	if oldName != "" {
		key := fmt.Sprintf("%s/%s", Z_PLAYER_SEARCH, oldName)
		resp, err := ssdbc.Do("zdel", key, userId)
		lwutil.CheckSsdbError(resp, err)
	}

	key := fmt.Sprintf("%s/%s", Z_PLAYER_SEARCH, newName)
	resp, err = ssdbc.Do("zset", key, userId, lwutil.GetRedisTimeUnix())
	lwutil.CheckSsdbError(resp, err)
}

func makePlayerInfoLite(playerInfo *PlayerInfo) *PlayerInfoLite {
	var infoLite PlayerInfoLite
	infoLite.UserId = playerInfo.UserId
	infoLite.NickName = playerInfo.NickName
	infoLite.TeamName = playerInfo.TeamName
	infoLite.Gender = playerInfo.Gender
	infoLite.CustomAvatarKey = playerInfo.CustomAvatarKey
	infoLite.GravatarKey = playerInfo.GravatarKey
	return &infoLite
}

func apiAddPrizeFromCache(w http.ResponseWriter, r *http.Request) {
	var err error
	lwutil.CheckMathod(r, "POST")

	//session
	session, err := findSession(w, r, nil)
	lwutil.CheckError(err, "err_auth")

	//ssdb
	ssdb, err := ssdbPool.Get()
	lwutil.CheckError(err, "")
	defer ssdb.Close()

	//
	key := makePlayerInfoKey(session.Userid)
	prizeCache := getPrizeCache(ssdb, key)
	lwutil.CheckError(err, "")
	if prizeCache > 0 {
		addPrize(ssdb, key, prizeCache)
		addPrizeTotal(ssdb, key, prizeCache)
		addPrizeCache(ssdb, key, -prizeCache)
	}

	playerInfo, err := getPlayerInfo(ssdb, session.Userid)
	lwutil.CheckError(err, "")

	//out
	out := map[string]interface{}{
		"Prize":      playerInfo.Prize,
		"TotalPrize": playerInfo.TotalPrize,
	}
	lwutil.WriteResponse(w, out)
}

func apiGetUptokens(w http.ResponseWriter, r *http.Request) {
	lwutil.CheckMathod(r, "POST")

	//in
	var in []string
	err := lwutil.DecodeRequestBody(r, &in)
	lwutil.CheckError(err, "err_decode_body")

	inLen := len(in)
	type outElem struct {
		Key   string
		Token string
	}
	out := make([]outElem, inLen, inLen)
	for i, v := range in {
		scope := fmt.Sprintf("%s:%s", USER_UPLOAD_BUCKET, v)
		putPolicy := rs.PutPolicy{
			Scope: scope,
		}
		out[i] = outElem{
			in[i],
			putPolicy.Token(nil),
		}
	}

	//out
	lwutil.WriteResponse(w, &out)
}

func apiGetUptoken(w http.ResponseWriter, r *http.Request) {
	lwutil.CheckMathod(r, "POST")

	//session
	_, err := findSession(w, r, nil)
	lwutil.CheckError(err, "err_auth")

	//
	scope := fmt.Sprintf("%s", USER_UPLOAD_BUCKET)
	putPolicy := rs.PutPolicy{
		Scope: scope,
	}

	//out
	out := struct {
		Token string
	}{
		putPolicy.Token(nil),
	}

	lwutil.WriteResponse(w, &out)
}

func apiGetPrivateUptoken(w http.ResponseWriter, r *http.Request) {
	lwutil.CheckMathod(r, "POST")

	//session
	_, err := findSession(w, r, nil)
	lwutil.CheckError(err, "err_auth")

	//
	scope := fmt.Sprintf("%s", USER_UPLOAD_BUCKET)
	putPolicy := rs.PutPolicy{
		Scope: scope,
	}
	token := putPolicy.Token(nil)

	scope = fmt.Sprintf("%s", USER_PRIVATE_UPLOAD_BUCKET)
	putPolicy = rs.PutPolicy{
		Scope: scope,
	}
	privateToken := putPolicy.Token(nil)

	//out
	out := struct {
		Token        string
		PrivateToken string
	}{
		token,
		privateToken,
	}

	lwutil.WriteResponse(w, &out)
}

func apiListMyEcard(w http.ResponseWriter, r *http.Request) {
	var err error
	lwutil.CheckMathod(r, "POST")

	//session
	session, err := findSession(w, r, nil)
	lwutil.CheckError(err, "err_auth")

	//ssdb
	ssdbc, err := ssdbPool.Get()
	lwutil.CheckError(err, "")
	defer ssdbc.Close()

	//in
	var in struct {
		StartId   int64
		LastScore int64
		Limit     int
	}
	err = lwutil.DecodeRequestBody(r, &in)
	lwutil.CheckError(err, "err_decode_body")

	if in.StartId <= 0 {
		in.StartId = math.MaxInt64
	}
	if in.LastScore <= 0 {
		in.LastScore = math.MaxInt64
	}

	if in.Limit > 50 {
		in.Limit = 50
	}

	//
	out := struct {
		Ecards    []OutEcard
		LastScore int64
	}{}
	out.Ecards = make([]OutEcard, 0)

	//
	playerEcardZKey := makePlayerEcardZsetKey(session.Userid)
	resp, err := ssdbc.Do("zrscan", playerEcardZKey, in.StartId, in.LastScore, "", in.Limit)
	lwutil.CheckSsdbError(resp, err)
	resp = resp[1:]

	itemNum := len(resp) / 2
	if itemNum == 0 {
		lwutil.WriteResponse(w, out)
		return
	}

	cmds := make([]interface{}, itemNum+2)
	cmds[0] = "multi_hget"
	cmds[1] = H_ECARD
	lastScore := int64(0)
	for i := 0; i < itemNum; i++ {
		cmds[i+2] = resp[i*2]
		lastScore, err = strconv.ParseInt(resp[i*2+1], 10, 64)
		lwutil.CheckError(err, "")
	}

	//
	resp, err = ssdbc.Do(cmds...)
	lwutil.CheckSsdbError(resp, err)
	resp = resp[1:]
	num := len(resp) / 2

	//
	out.LastScore = lastScore
	out.Ecards = make([]OutEcard, num)
	for i := 0; i < num; i++ {
		err = json.Unmarshal([]byte(resp[i*2+1]), &(out.Ecards[i].ECard))
		out.Ecards[i].Provider = ECARD_PROVIDERS[out.Ecards[i].ECard.Provider]
		lwutil.CheckError(err, "")
	}

	lwutil.WriteResponse(w, out)
}

func apiListMyPrize(w http.ResponseWriter, r *http.Request) {
	var err error
	lwutil.CheckMathod(r, "POST")

	//session
	session, err := findSession(w, r, nil)
	lwutil.CheckError(err, "err_auth")

	//ssdb
	ssdbc, err := ssdbPool.Get()
	lwutil.CheckError(err, "")
	defer ssdbc.Close()

	//in
	var in struct {
		StartId int64
		Limit   int
	}
	err = lwutil.DecodeRequestBody(r, &in)
	lwutil.CheckError(err, "err_decode_body")

	if in.StartId <= 0 {
		in.StartId = math.MaxInt32
	}

	if in.Limit > 50 {
		in.Limit = 50
	}

	//out
	out := []PrizeRecord{}

	//
	zkey := makeZPlayerPrizeRecordKey(session.Userid)
	resp, err := ssdbc.Do("zrscan", zkey, in.StartId, in.StartId, "", in.Limit)
	lwutil.CheckError(err, "")
	resp = resp[1:]
	if len(resp) == 0 {
		lwutil.WriteResponse(w, out)
		return
	}

	num := len(resp) / 2
	cmds := make([]interface{}, num+2)
	cmds[0] = "multi_hget"
	cmds[1] = H_PLAYER_PRIZE_RECORD
	for i := 0; i < num; i++ {
		cmds[i+2] = resp[i*2]
	}

	//
	resp, err = ssdbc.Do(cmds...)
	lwutil.CheckSsdbError(resp, err)
	resp = resp[1:]
	num = len(resp) / 2
	for i := 0; i < num; i++ {
		js := resp[i*2+1]
		record := PrizeRecord{}
		err = json.Unmarshal([]byte(js), &record)
		lwutil.CheckError(err, "")
		out = append(out, record)
	}

	lwutil.WriteResponse(w, out)
}

func apiGetPrizeCache(w http.ResponseWriter, r *http.Request) {
	var err error
	lwutil.CheckMathod(r, "POST")

	//session
	session, err := findSession(w, r, nil)
	lwutil.CheckError(err, "err_auth")

	//ssdb
	ssdbc, err := ssdbPool.Get()
	lwutil.CheckError(err, "")
	defer ssdbc.Close()

	//
	key := makePlayerInfoKey(session.Userid)
	prizeCache := getPrizeCache(ssdbc, key)

	//out
	out := struct {
		PrizeCache int
	}{
		prizeCache,
	}
	lwutil.WriteResponse(w, out)
}

func apiPlayerFanList(w http.ResponseWriter, r *http.Request) {
	var err error
	lwutil.CheckMathod(r, "POST")

	//session
	session, err := findSession(w, r, nil)
	lwutil.CheckError(err, "err_auth")

	//ssdb
	ssdbc, err := ssdbPool.Get()
	lwutil.CheckError(err, "")
	defer ssdbc.Close()

	//in
	var in struct {
		UserId    int64
		StartId   int64
		LastScore int64
		Limit     int
	}
	err = lwutil.DecodeRequestBody(r, &in)
	lwutil.CheckError(err, "err_decode_body")

	if in.UserId == 0 {
		in.UserId = session.Userid
	}

	if in.Limit <= 0 {
		in.Limit = 20
	} else if in.Limit > 50 {
		in.Limit = 50
	}

	if in.StartId == 0 {
		in.StartId = math.MaxInt64
	}
	if in.LastScore == 0 {
		in.LastScore = math.MaxInt64
	}

	//out
	out := struct {
		PlayerInfoLites []*PlayerInfoLite
		LastKey         int64
		LastScore       int64
	}{
		make([]*PlayerInfoLite, 0, 20),
		0,
		0,
	}

	//get userId list
	key := makeZPlayerFanKey(in.UserId)

	resp, err := ssdbc.Do("zrscan", key, in.StartId, in.LastScore, "", in.Limit)
	lwutil.CheckSsdbError(resp, err)

	resp = resp[1:]
	if len(resp) == 0 {
		lwutil.WriteResponse(w, out)
		return
	}

	//get infos
	num := len(resp) / 2
	args := make([]interface{}, 2, num+2)
	args[0] = "multi_hget"
	args[1] = H_PLAYER_INFO_LITE
	for i := 0; i < num; i++ {
		args = append(args, resp[i*2])
		if i == num-1 {
			out.LastKey, err = strconv.ParseInt(resp[i*2], 10, 64)
			lwutil.CheckError(err, "")
			out.LastScore, err = strconv.ParseInt(resp[i*2+1], 10, 64)
			lwutil.CheckError(err, "")
		}
	}
	resp, err = ssdbc.Do(args...)
	lwutil.CheckSsdbError(resp, err)
	resp = resp[1:]
	if len(resp) == 0 {
		lwutil.WriteResponse(w, out)
		return
	}

	num = len(resp) / 2
	for i := 0; i < num; i++ {
		js := resp[i*2+1]
		var infoLite PlayerInfoLite
		err = json.Unmarshal([]byte(js), &infoLite)
		lwutil.CheckError(err, "err_js")
		out.PlayerInfoLites = append(out.PlayerInfoLites, &infoLite)
	}
	lwutil.WriteResponse(w, out)
}

func apiPlayerFollowList(w http.ResponseWriter, r *http.Request) {
	var err error
	lwutil.CheckMathod(r, "POST")

	//session
	session, err := findSession(w, r, nil)
	lwutil.CheckError(err, "err_auth")

	//ssdb
	ssdbc, err := ssdbPool.Get()
	lwutil.CheckError(err, "")
	defer ssdbc.Close()

	//in
	var in struct {
		UserId    int64
		StartId   int64
		LastScore int64
		Limit     int
	}
	err = lwutil.DecodeRequestBody(r, &in)
	lwutil.CheckError(err, "err_decode_body")

	if in.UserId == 0 {
		in.UserId = session.Userid
	}

	if in.Limit <= 0 {
		in.Limit = 20
	} else if in.Limit > 50 {
		in.Limit = 50
	}

	if in.StartId == 0 {
		in.StartId = math.MaxInt64
	}
	if in.LastScore == 0 {
		in.LastScore = math.MaxInt64
	}

	//out
	out := struct {
		PlayerInfoLites []*PlayerInfoLite
		LastKey         int64
		LastScore       int64
	}{
		make([]*PlayerInfoLite, 0, 20),
		0,
		0,
	}

	//get userId list
	key := makeZPlayerFollowKey(in.UserId)

	resp, err := ssdbc.Do("zrscan", key, in.StartId, in.LastScore, "", in.Limit)
	lwutil.CheckSsdbError(resp, err)

	resp = resp[1:]
	if len(resp) == 0 {
		lwutil.WriteResponse(w, out)
		return
	}

	//get infos
	num := len(resp) / 2
	args := make([]interface{}, 2, num+2)
	args[0] = "multi_hget"
	args[1] = H_PLAYER_INFO_LITE
	for i := 0; i < num; i++ {
		args = append(args, resp[i*2])
		if i == num-1 {
			out.LastKey, err = strconv.ParseInt(resp[i*2], 10, 64)
			lwutil.CheckError(err, "")
			out.LastScore, err = strconv.ParseInt(resp[i*2+1], 10, 64)
			lwutil.CheckError(err, "")
		}
	}
	resp, err = ssdbc.Do(args...)
	lwutil.CheckSsdbError(resp, err)
	resp = resp[1:]
	if len(resp) == 0 {
		lwutil.WriteResponse(w, out)
		return
	}

	num = len(resp) / 2
	for i := 0; i < num; i++ {
		js := resp[i*2+1]
		var infoLite PlayerInfoLite
		err = json.Unmarshal([]byte(js), &infoLite)
		lwutil.CheckError(err, "err_js")
		out.PlayerInfoLites = append(out.PlayerInfoLites, &infoLite)
	}
	lwutil.WriteResponse(w, out)
}

func apiPlayerFollowListWeb(w http.ResponseWriter, r *http.Request) {
	var err error
	lwutil.CheckMathod(r, "POST")

	//ssdb
	ssdbc, err := ssdbPool.Get()
	lwutil.CheckError(err, "")
	defer ssdbc.Close()

	//in
	var in struct {
		Type      int
		UserId    int64
		LastKey   int64
		LastScore int64
		Limit     int
	}
	err = lwutil.DecodeRequestBody(r, &in)
	lwutil.CheckError(err, "err_decode_body")

	if in.UserId == 0 {
		//session
		session, err := findSession(w, r, nil)
		lwutil.CheckError(err, "err_auth")

		in.UserId = session.Userid
	}

	if in.Limit <= 0 {
		in.Limit = 20
	} else if in.Limit > 50 {
		in.Limit = 50
	}

	if in.LastKey == 0 {
		in.LastKey = math.MaxInt64
	}
	if in.LastScore == 0 {
		in.LastScore = math.MaxInt64
	}

	//out
	out := struct {
		PlayerInfoLites []*PlayerInfoLite
		LastKey         int64
		LastScore       int64
	}{
		make([]*PlayerInfoLite, 0, 20),
		0,
		0,
	}

	//get userId list
	var key string
	if in.Type == 0 {
		key = makeZPlayerFollowKey(in.UserId)
	} else {
		key = makeZPlayerFanKey(in.UserId)
	}

	resp, err := ssdbc.Do("zrscan", key, in.LastKey, in.LastScore, "", in.Limit)
	lwutil.CheckSsdbError(resp, err)

	resp = resp[1:]
	if len(resp) == 0 {
		lwutil.WriteResponse(w, out)
		return
	}

	//get infos
	num := len(resp) / 2
	args := make([]interface{}, 2, num+2)
	args[0] = "multi_hget"
	args[1] = H_PLAYER_INFO_LITE
	for i := 0; i < num; i++ {
		args = append(args, resp[i*2])
		if i == num-1 {
			out.LastKey, err = strconv.ParseInt(resp[i*2], 10, 64)
			lwutil.CheckError(err, "")
			out.LastScore, err = strconv.ParseInt(resp[i*2+1], 10, 64)
			lwutil.CheckError(err, "")
		}
	}
	resp, err = ssdbc.Do(args...)
	lwutil.CheckSsdbError(resp, err)
	resp = resp[1:]
	if len(resp) == 0 {
		lwutil.WriteResponse(w, out)
		return
	}

	num = len(resp) / 2
	for i := 0; i < num; i++ {
		js := resp[i*2+1]
		var infoLite PlayerInfoLite
		err = json.Unmarshal([]byte(js), &infoLite)
		lwutil.CheckError(err, "err_js")
		out.PlayerInfoLites = append(out.PlayerInfoLites, &infoLite)
	}
	lwutil.WriteResponse(w, out)
}

func apiPlayerFollow(w http.ResponseWriter, r *http.Request) {
	var err error
	lwutil.CheckMathod(r, "POST")

	//session
	session, err := findSession(w, r, nil)
	lwutil.CheckError(err, "err_auth")

	//ssdb
	ssdbc, err := ssdbPool.Get()
	lwutil.CheckError(err, "")
	defer ssdbc.Close()

	//in
	var in struct {
		UserId int64
	}
	err = lwutil.DecodeRequestBody(r, &in)
	lwutil.CheckError(err, "err_decode_body")

	//self check
	if in.UserId == session.Userid {
		lwutil.SendError("err_self", "self follow")
	}

	//exist userid
	if !checkPlayerExist(ssdbc, in.UserId) {
		lwutil.SendError("err_user", "user not exist")
	}

	//check follow already
	followed, err := isFollowed(ssdbc, session.Userid, in.UserId)
	lwutil.CheckError(err, "err_follow")
	if followed {
		lwutil.SendError("err_followed", "fallowed already")
	}

	//
	score := lwutil.GetRedisTimeUnix()

	followKey := makeZPlayerFollowKey(session.Userid)
	resp, err := ssdbc.Do("zset", followKey, in.UserId, score)
	lwutil.CheckSsdbError(resp, err)

	fanKey := makeZPlayerFanKey(in.UserId)
	resp, err = ssdbc.Do("zset", fanKey, session.Userid, score)
	lwutil.CheckSsdbError(resp, err)

	//getPlayerInfoLite
	playerInfoLite, err := getPlayerInfoLite(ssdbc, in.UserId, nil)
	lwutil.CheckError(err, "err_player_info_lite")

	//update nums
	resp, err = ssdbc.Do("zsize", followKey)
	lwutil.CheckSsdbError(resp, err)
	followNum, err := strconv.Atoi(resp[1])
	lwutil.CheckError(err, "err_strconv")

	resp, err = ssdbc.Do("zsize", fanKey)
	lwutil.CheckSsdbError(resp, err)
	fanNum, err := strconv.Atoi(resp[1])
	lwutil.CheckError(err, "err_strconv")

	//update num
	key := makePlayerInfoKey(session.Userid)
	ssdbc.HSet(key, PLAYER_FOLLOW_NUM, followNum)
	key = makePlayerInfoKey(in.UserId)
	ssdbc.HSet(key, PLAYER_FAN_NUM, fanNum)

	//out
	out := struct {
		Follow         bool
		FollowNum      int //myfollowNum
		FanNum         int
		PlayerInfoLite *PlayerInfoLite
	}{
		true,
		followNum,
		fanNum,
		playerInfoLite,
	}
	lwutil.WriteResponse(w, out)
}

func apiPlayerUnfollow(w http.ResponseWriter, r *http.Request) {
	var err error
	lwutil.CheckMathod(r, "POST")

	//session
	session, err := findSession(w, r, nil)
	lwutil.CheckError(err, "err_auth")

	//ssdb
	ssdbc, err := ssdbPool.Get()
	lwutil.CheckError(err, "")
	defer ssdbc.Close()

	//in
	var in struct {
		UserId int64
	}
	err = lwutil.DecodeRequestBody(r, &in)
	lwutil.CheckError(err, "err_decode_body")

	//
	followKey := makeZPlayerFollowKey(session.Userid)
	resp, err := ssdbc.Do("zdel", followKey, in.UserId)
	lwutil.CheckSsdbError(resp, err)

	fanKey := makeZPlayerFanKey(in.UserId)
	resp, err = ssdbc.Do("zdel", fanKey, session.Userid)
	lwutil.CheckSsdbError(resp, err)

	//update nums
	resp, err = ssdbc.Do("zsize", followKey)
	lwutil.CheckSsdbError(resp, err)
	followNum, err := strconv.Atoi(resp[1])
	lwutil.CheckError(err, "err_strconv")

	resp, err = ssdbc.Do("zsize", fanKey)
	lwutil.CheckSsdbError(resp, err)
	fanNum, err := strconv.Atoi(resp[1])
	lwutil.CheckError(err, "err_strconv")

	//update num
	key := makePlayerInfoKey(session.Userid)
	ssdbc.HSet(key, PLAYER_FOLLOW_NUM, followNum)
	key = makePlayerInfoKey(in.UserId)
	ssdbc.HSet(key, PLAYER_FAN_NUM, fanNum)

	//out
	out := struct {
		Follow    bool
		FollowNum int //myFollowNum
		FanNum    int
	}{
		false,
		followNum,
		fanNum,
	}
	lwutil.WriteResponse(w, out)
}

func checkPlayerExist(ssdbc *ssdb.Client, userId int64) bool {
	key := makePlayerInfoKey(userId)
	resp, err := ssdbc.Do("hexists", key, PLAYER_NICK_NAME)
	lwutil.CheckSsdbError(resp, err)
	if resp[1] != "1" {
		return false
	}

	return true
}

func apiPlayerGetPlayerInfoWeb(w http.ResponseWriter, r *http.Request) {
	var err error
	lwutil.CheckMathod(r, "POST")

	//ssdb
	ssdbc, err := ssdbPool.Get()
	lwutil.CheckError(err, "")
	defer ssdbc.Close()

	//session
	session, err := findSession(w, r, nil)
	lwutil.CheckError(err, "err_auth")

	//in
	var in struct {
		UserId int64
	}
	err = lwutil.DecodeRequestBody(r, &in)

	if in.UserId == 0 {
		in.UserId = session.Userid
	}

	//get info
	playerInfo, err := getPlayerInfo(ssdbc, in.UserId)
	if err != nil {
		lwutil.SendError("err_get_player_info", "")
	}

	//get packNum
	key := makeQLikeMatchKey(in.UserId)
	resp, err := ssdbc.Do("qsize", key)
	lwutil.CheckSsdbError(resp, err)
	matchNum, err := strconv.Atoi(resp[1])
	lwutil.CheckError(err, "err_strconv")

	//followed?
	followed := false
	if session != nil && session.Userid != in.UserId {
		followed, err = isFollowed(ssdbc, session.Userid, playerInfo.UserId)
		lwutil.CheckError(err, "err_follow")
	}

	//out
	out := struct {
		*PlayerInfo
		MatchNum int
		Followed bool
	}{
		playerInfo,
		matchNum,
		followed,
	}
	lwutil.WriteResponse(w, out)
}

func apiPlayerSearchUser(w http.ResponseWriter, r *http.Request) {
	var err error
	lwutil.CheckMathod(r, "POST")

	//ssdb
	ssdbc, err := ssdbPool.Get()
	lwutil.CheckError(err, "")
	defer ssdbc.Close()

	//in
	var in struct {
		UserName  string
		LastKey   string
		LastScore int64
	}
	err = lwutil.DecodeRequestBody(r, &in)

	//userName
	limit := 20
	userName := strings.ToLower(in.UserName)
	zkey := fmt.Sprintf("%s/%s", Z_PLAYER_SEARCH, userName)
	hkey := H_PLAYER_INFO_LITE
	resp, lastKey, lastScore, err := ssdbc.ZScan(zkey, hkey, in.LastKey, in.LastScore, limit, false)
	lwutil.CheckError(err, "err_zmultiget")

	//userId
	userId, _ := strconv.ParseInt(userName, 10, 64)

	//out
	num := len(resp) / 2
	out := struct {
		Players   []*PlayerInfoLite
		Limit     int
		LastKey   string
		LastScore string
	}{
		make([]*PlayerInfoLite, 0, num+1),
		limit,
		lastKey,
		lastScore,
	}
	if userId != 0 {
		player, err := getPlayerInfoLite(ssdbc, userId, nil)
		if err == nil {
			out.Players = append(out.Players, player)
		}
	}
	for i := 0; i < num; i++ {
		var player PlayerInfoLite
		err = json.Unmarshal([]byte(resp[i*2+1]), &player)
		lwutil.CheckError(err, "")
		out.Players = append(out.Players, &player)
	}
	lwutil.WriteResponse(w, out)
}

func regPlayer() {
	http.Handle("/player/getInfo", lwutil.ReqHandler(apiGetPlayerInfo))
	http.Handle("/player/setInfo", lwutil.ReqHandler(apiSetPlayerInfo))
	http.Handle("/player/addPrizeFromCache", lwutil.ReqHandler(apiAddPrizeFromCache))
	http.Handle("/player/getUptokens", lwutil.ReqHandler(apiGetUptokens))
	http.Handle("/player/getUptoken", lwutil.ReqHandler(apiGetUptoken))
	http.Handle("/player/getPrivateUptoken", lwutil.ReqHandler(apiGetPrivateUptoken))
	http.Handle("/player/listMyEcard", lwutil.ReqHandler(apiListMyEcard))
	http.Handle("/player/listMyPrize", lwutil.ReqHandler(apiListMyPrize))
	http.Handle("/player/getPrizeCache", lwutil.ReqHandler(apiGetPrizeCache))
	http.Handle("/player/fanList", lwutil.ReqHandler(apiPlayerFanList))
	http.Handle("/player/followList", lwutil.ReqHandler(apiPlayerFollowList))
	http.Handle("/player/follow", lwutil.ReqHandler(apiPlayerFollow))
	http.Handle("/player/unfollow", lwutil.ReqHandler(apiPlayerUnfollow))
	http.Handle("/player/searchUser", lwutil.ReqHandler(apiPlayerSearchUser))

	http.Handle("/player/web/getInfo", lwutil.ReqHandler(apiPlayerGetPlayerInfoWeb))
	http.Handle("/player/web/followList", lwutil.ReqHandler(apiPlayerFollowListWeb))
}
