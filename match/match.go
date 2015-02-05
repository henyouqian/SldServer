package main

import (
	"./ssdb"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/garyburd/redigo/redis"
	"github.com/golang/glog"
	"github.com/henyouqian/fastimage"
	"github.com/henyouqian/lwutil"
)

const (
	MATCH_SERIAL            = "MATCH_SERIAL"
	H_MATCH                 = "H_MATCH"                 //subkey:matchId value:matchJson
	H_MATCH_EXTRA           = "H_MATCH_EXTRA"           //key:H_MATCH_EXTRA subkey:matchId/fieldKey value:fieldValue
	H_MATCH_PLAY            = "H_MATCH_PLAY"            //key:H_MATCH_PLAY subkey:matchId/userId value:match json
	H_MATCH_RANK            = "H_MATCH_RANK"            //key:H_MATCH_RANK/matchId subkey:rank value:userId
	Z_MATCH                 = "Z_MATCH"                 //subkey:matchId score:beginTime
	Z_PENDING_MATCH         = "Z_PENDING_MATCH"         //subkey:matchId score:beginTime
	Z_PLAYER_MATCH          = "Z_PLAYER_MATCH"          //original key:Z_PLAYER_MATCH/userId subkey:matchId score:beginTime
	Z_LIKE_MATCH            = "Z_LIKE_MATCH"            //key:Z_LIKE_MATCH/userId subkey:matchId score:likeTime
	Q_PLAYER_MATCH          = "Q_PLAYER_MATCH"          //original key:Q_PLAYER_MATCH/userId value:matchId
	Q_LIKE_MATCH            = "Q_LIKE_MATCH"            //key:Q_LIKE_MATCH/userId value:matchId
	Z_PLAYED_MATCH          = "Z_PLAYED_MATCH"          //key:Z_PLAYED_MATCH/userId subkey:matchId score:lastPlayTime
	Z_PLAYED_ALL            = "Z_PLAYED_ALL"            //key:Z_PLAYED_ALL/userId subkey:matchId score:lastPlayTime
	Z_OPEN_MATCH            = "Z_OPEN_MATCH"            //subkey:matchId score:endTime
	Z_HOT_MATCH             = "Z_HOT_MATCH"             //subkey:matchId score:totalPrize
	RDS_Z_MATCH_LEADERBOARD = "RDS_Z_MATCH_LEADERBOARD" //key:RDS_Z_MATCH_LEADERBOARD/matchId
	Z_MATCH_LIKER           = "Z_MATCH_LIKER"           //key:Z_MATCH_LIKER/matchId subkey:userId score:time
	Q_MATCH_ACTIVITY        = "Q_MATCH_ACTIVITY"        //key:Q_MATCH_ACTIVITY/matchId subkey:userId score:time
	Q_MATCH_DEL_LIMIT       = 50

	PRIZE_NUM_PER_COIN         = 100
	MIN_PRIZE                  = PRIZE_NUM_PER_COIN
	FREE_TRY_NUM               = 3
	MATCH_TRY_EXPIRE_SECONDS   = 600
	MATCH_CLOSE_BEFORE_END_SEC = 60
	MATCH_TIME_SEC             = 60 * 60 * 24
	// MATCH_TIME_SEC = 60 * 3
	MATCH_ACTIVITY_Q_LIMIT = 50
)

type Match struct {
	Id                   int64
	PackId               int64
	ImageNum             int
	OwnerId              int64
	OwnerName            string
	SliderNum            int
	Thumb                string
	Thumbs               []string
	Title                string
	Prize                int
	BeginTime            int64
	BeginTimeStr         string
	EndTime              int64
	HasResult            bool
	RankPrizeProportions []float32
	LuckyPrizeProportion float32
	MinPrizeProportion   float32
	OwnerPrizeProportion float32
	PromoUrl             string
	PromoImage           string
	Private              bool
	Deleted              bool
	Source               string
}

type MatchExtra struct {
	PlayTimes  int
	ExtraPrize int
	LikeNum    int
}

const (
	MATCH_EXTRA_PLAY_TIMES = "PlayTimes"
	MATCH_EXTRA_PRIZE      = "ExtraPrize"
	MATCH_EXTRA_LIKE_NUM   = "LikeNum"
)

type MatchPlay struct {
	PlayerName       string
	GravatarKey      string
	CustomAvartarKey string
	HighScore        int
	HighScoreTime    int64
	FinalRank        int
	FreeTries        int
	Tries            int
	Team             string
	Secret           string
	SecretExpire     int64
	LuckyNums        []int64
	Prize            int
	Like             bool
	Played           bool
	PrivateLike      bool
}

type MatchActivity struct {
	Player *PlayerInfoLite
	Text   string
}

type PlayerMatchInfo struct {
	Played       bool
	Liked        bool
	PrivateLiked bool
}

const (
	MATCH_LUCKY_PRIZE_PROPORTION = float32(0.00)
	MATCH_MIN_PRIZE_PROPORTION   = float32(0.05)
	MATCH_OWNER_PRIZE_PROPORTION = float32(0.1)
)

var (
	MATCH_RANK_PRIZE_PROPORTIONS = []float32{
		0.15, 0.10, 0.09, 0.08, 0.07, 0.06, 0.05, 0.04, 0.03, 0.02,
		0.01, 0.01, 0.01, 0.01, 0.01, 0.01, 0.01, 0.01, 0.01, 0.01,
	}
)

func matchGlog() {
	glog.Info("")
}

func makeHMatchExtraSubkey(matchId int64, fieldKey string) string {
	return fmt.Sprintf("%d/%s", matchId, fieldKey)
}

func makeZPlayerMatchKey(userId int64) string {
	return fmt.Sprintf("%s/%d", Z_PLAYER_MATCH, userId)
}

func makeQPlayerMatchKey(userId int64) string {
	return fmt.Sprintf("%s/%d", Q_PLAYER_MATCH, userId)
}

func makeZLikeMatchKey(userId int64) string {
	return fmt.Sprintf("%s/%d", Z_LIKE_MATCH, userId)
}

func makeQLikeMatchKey(userId int64) string {
	return fmt.Sprintf("%s/%d", Q_LIKE_MATCH, userId)
}

func makeZPlayedMatchKey(userId int64) string {
	return fmt.Sprintf("%s/%d", Z_PLAYED_MATCH, userId)
}

func makeZPlayedAllKey(userId int64) string {
	return fmt.Sprintf("%s/%d", Z_PLAYED_ALL, userId)
}

func makeMatchPlaySubkey(matchId int64, userId int64) string {
	return fmt.Sprintf("%d/%d", matchId, userId)
}

func makeMatchLeaderboardRdsKey(matchId int64) string {
	return fmt.Sprintf("%s/%d", RDS_Z_MATCH_LEADERBOARD, matchId)
}

func makeHMatchRankKey(matchId int64) string {
	return fmt.Sprintf("%s/%d", H_MATCH_RANK, matchId)
}

func makeSecretKey(secret string) string {
	return fmt.Sprintf("%s/%s", "MATCH_SECRET", secret)
}

func makeZMatchLikerKey(matchId int64) string {
	return fmt.Sprintf("%s/%d", Z_MATCH_LIKER, matchId)
}

func makeQMatchActivityKey(matchId int64) string {
	return fmt.Sprintf("%s/%d", Q_MATCH_ACTIVITY, matchId)
}

func makePlayerMatchInfo(matchPlay *MatchPlay) *PlayerMatchInfo {
	if matchPlay == nil {
		return nil
	}
	var playerMatchInfo PlayerMatchInfo
	playerMatchInfo.Played = matchPlay.Played
	playerMatchInfo.Liked = matchPlay.Like
	playerMatchInfo.PrivateLiked = matchPlay.PrivateLike
	return &playerMatchInfo
}

func getMatch(ssdbc *ssdb.Client, matchId int64) *Match {
	resp, err := ssdbc.Do("hget", H_MATCH, matchId)
	lwutil.CheckSsdbError(resp, err)
	var match Match
	err = json.Unmarshal([]byte(resp[1]), &match)
	lwutil.CheckError(err, "")
	return &match
}

func saveMatch(ssdbc *ssdb.Client, match *Match) {
	js, err := json.Marshal(match)
	lwutil.CheckError(err, "")

	//add to hash
	resp, err := ssdbc.Do("hset", H_MATCH, match.Id, js)
	lwutil.CheckSsdbError(resp, err)
}

func getMatchPlay(ssdbc *ssdb.Client, matchId int64, userId int64) (*MatchPlay, error) {
	var play MatchPlay
	subkey := makeMatchPlaySubkey(matchId, userId)

	resp, err := ssdbc.Do("hget", H_MATCH_PLAY, subkey)
	lwutil.CheckError(err, "")
	if resp[0] == ssdb.NOT_FOUND {
		play.FreeTries = FREE_TRY_NUM

		playerInfo, err := getPlayerInfo(ssdbc, userId)
		if err != nil {
			return nil, fmt.Errorf("no playerInfo:userId=%d", userId)
		}

		play.Team = playerInfo.TeamName
		play.PlayerName = playerInfo.NickName
		play.GravatarKey = playerInfo.GravatarKey
		play.CustomAvartarKey = playerInfo.CustomAvatarKey

		//save
		js, err := json.Marshal(play)
		lwutil.CheckError(err, "")
		resp, err = ssdbc.Do("hset", H_MATCH_PLAY, subkey, js)
		lwutil.CheckSsdbError(resp, err)
	} else {
		err := json.Unmarshal([]byte(resp[1]), &play)
		lwutil.CheckError(err, "")
	}
	return &play, nil
}

func saveMatchPlay(ssdbc *ssdb.Client, matchId int64, userId int64, play *MatchPlay) {
	js, err := json.Marshal(play)
	lwutil.CheckError(err, "")
	subkey := makeMatchPlaySubkey(matchId, userId)
	resp, err := ssdbc.Do("hset", H_MATCH_PLAY, subkey, js)
	lwutil.CheckSsdbError(resp, err)
}

func init() {
	//check reward
	sum := float32(0)
	for _, v := range MATCH_RANK_PRIZE_PROPORTIONS {
		sum += v
	}
	sum += MATCH_LUCKY_PRIZE_PROPORTION
	sum += MATCH_MIN_PRIZE_PROPORTION
	sum += MATCH_OWNER_PRIZE_PROPORTION
	if sum > 1.0 {
		panic("reward sum > 1.0")
	}
}

func apiMatchNew(w http.ResponseWriter, r *http.Request) {
	lwutil.CheckMathod(r, "POST")
	var err error

	//ssdb
	ssdbc, err := ssdbPool.Get()
	lwutil.CheckError(err, "")
	defer ssdbc.Close()

	//session
	session, err := findSession(w, r, nil)
	lwutil.CheckError(err, "err_auth")

	//in
	var in struct {
		Pack
		BeginTimeStr     string
		SliderNum        int
		GoldCoinForPrize int
		PromoUrl         string
		PromoImage       string
		Private          bool
		Tags             []string
	}
	err = lwutil.DecodeRequestBody(r, &in)
	lwutil.CheckError(err, "err_decode_body")

	if in.SliderNum < 3 {
		in.SliderNum = 3
	} else if in.SliderNum > 9 {
		in.SliderNum = 9
	}
	if len(in.Tags) > 8 {
		in.Tags = in.Tags[:8]
	}

	stringLimit(&in.Title, 100)
	stringLimit(&in.Text, 1000)

	//check gold coin
	playerKey := makePlayerInfoKey(session.Userid)
	goldNum := getPlayerGoldCoin(ssdbc, playerKey)
	if goldNum < in.GoldCoinForPrize {
		lwutil.SendError("err_gold_coin", "goldNum < in.GoldCoinForPrize")
	}

	player, err := getPlayerInfo(ssdbc, session.Userid)
	lwutil.CheckError(err, "")

	//check repeat
	key := makeQPlayerMatchKey(session.Userid)
	resp, err := ssdbc.Do("qback", key)
	if err == nil && len(resp) == 2 && resp[0] == "ok" {
		lastMatchId, err := strconv.ParseInt(resp[1], 10, 64)
		lwutil.CheckError(err, "err_strconv")
		match := getMatch(ssdbc, lastMatchId)
		if match.Thumb == in.Pack.Thumb {
			lwutil.SendError("err_match_repeat", "是否重复发送？")
		}
	}

	//
	matchId := GenSerial(ssdbc, MATCH_SERIAL)

	//new pack
	newPack(ssdbc, &in.Pack, session.Userid, matchId)

	//new match
	beginTimeUnix := int64(0)
	beginTimeStr := in.BeginTimeStr
	endTimeUnix := int64(0)
	beginTime := lwutil.GetRedisTime()
	beginTimeUnix = beginTime.Unix()
	beginTimeStr = beginTime.Format("2006-01-02T15:04:05")
	endTimeUnix = beginTime.Add(MATCH_TIME_SEC * time.Second).Unix()

	match := Match{
		Id:                   matchId,
		PackId:               in.Pack.Id,
		ImageNum:             len(in.Pack.Images),
		OwnerId:              session.Userid,
		OwnerName:            player.NickName,
		SliderNum:            in.SliderNum,
		Thumb:                in.Pack.Thumb,
		Thumbs:               in.Pack.Thumbs,
		Title:                in.Title,
		Prize:                in.GoldCoinForPrize * PRIZE_NUM_PER_COIN,
		BeginTime:            beginTimeUnix,
		BeginTimeStr:         beginTimeStr,
		EndTime:              endTimeUnix,
		HasResult:            false,
		RankPrizeProportions: MATCH_RANK_PRIZE_PROPORTIONS,
		LuckyPrizeProportion: MATCH_LUCKY_PRIZE_PROPORTION,
		MinPrizeProportion:   MATCH_MIN_PRIZE_PROPORTION,
		OwnerPrizeProportion: MATCH_OWNER_PRIZE_PROPORTION,
		PromoUrl:             in.PromoUrl,
		PromoImage:           in.PromoImage,
		Private:              in.Private,
	}

	//json
	js, err := json.Marshal(match)
	lwutil.CheckError(err, "")

	//add to hash
	resp, err = ssdbc.Do("hset", H_MATCH, matchId, js)
	lwutil.CheckSsdbError(resp, err)

	if !in.Private {
		//add to Z_MATCH
		resp, err = ssdbc.Do("zset", Z_MATCH, matchId, beginTimeUnix)
		lwutil.CheckSsdbError(resp, err)

		//Z_HOT_MATCH
		resp, err = ssdbc.Do("zset", Z_HOT_MATCH, matchId, in.GoldCoinForPrize*PRIZE_NUM_PER_COIN)
		lwutil.CheckSsdbError(resp, err)
	}

	//Z_LIKE_MATCH
	key = makeZLikeMatchKey(session.Userid)
	resp, err = ssdbc.Do("zset", key, matchId, beginTimeUnix)
	lwutil.CheckSsdbError(resp, err)

	//Z_PLAYER_MATCH
	key = makeZPlayerMatchKey(session.Userid)
	resp, err = ssdbc.Do("zset", key, matchId, beginTimeUnix)
	lwutil.CheckSsdbError(resp, err)

	//Q_LIKE_MATCH
	key = makeQLikeMatchKey(session.Userid)
	resp, err = ssdbc.Do("qpush_back", key, matchId)
	lwutil.CheckSsdbError(resp, err)

	//Q_PLAYER_MATCH
	key = makeQPlayerMatchKey(session.Userid)
	resp, err = ssdbc.Do("qpush_back", key, matchId)
	lwutil.CheckSsdbError(resp, err)

	//Z_OPEN_MATCH
	resp, err = ssdbc.Do("zset", Z_OPEN_MATCH, matchId, endTimeUnix)
	lwutil.CheckSsdbError(resp, err)

	// //channel
	// key = makeHUserChannelKey(session.Userid)
	// resp, err = ssdbc.Do("hgetall", key)
	// lwutil.CheckSsdbError(resp, err)
	// resp = resp[1:]

	// for _, channelName := range resp {
	// 	key := makeZChannelMatchKey(channelName)
	// 	resp, err := ssdbc.Do("zset", key, matchId, beginTimeUnix)
	// 	lwutil.CheckSsdbError(resp, err)
	// }

	//decrease gold coin
	if in.GoldCoinForPrize != 0 {
		addPlayerGoldCoin(ssdbc, playerKey, -in.GoldCoinForPrize)
	}

	//out
	lwutil.WriteResponse(w, match)
}

func apiMatchDel(w http.ResponseWriter, r *http.Request) {
	lwutil.CheckMathod(r, "POST")
	var err error

	//ssdb
	ssdbc, err := ssdbPool.Get()
	lwutil.CheckError(err, "")
	defer ssdbc.Close()

	//session
	session, err := findSession(w, r, nil)
	lwutil.CheckError(err, "err_auth")

	//in
	var in struct {
		MatchId int64
	}
	err = lwutil.DecodeRequestBody(r, &in)
	lwutil.CheckError(err, "err_decode_body")

	//get match
	match := getMatch(ssdbc, in.MatchId)

	//check owner
	if match.OwnerId != session.Userid {
		lwutil.SendError("err_owner", "not the pack's owner")
	}

	//del
	resp, err := ssdbc.Do("zdel", Z_MATCH, in.MatchId)
	lwutil.CheckSsdbError(resp, err)

	resp, err = ssdbc.Do("zdel", Z_HOT_MATCH, in.MatchId)
	lwutil.CheckSsdbError(resp, err)

	key := makeZPlayerMatchKey(session.Userid)
	resp, err = ssdbc.Do("zdel", key, in.MatchId)
	lwutil.CheckSsdbError(resp, err)

	key = makeZLikeMatchKey(session.Userid)
	resp, err = ssdbc.Do("zdel", key, in.MatchId)
	lwutil.CheckSsdbError(resp, err)

	key = makeQLikeMatchKey(session.Userid)
	matchIdStr := fmt.Sprintf("%d", in.MatchId)
	err = ssdbc.QDel(key, matchIdStr, Q_MATCH_DEL_LIMIT, true)
	lwutil.CheckError(err, "err_ssdb_qdel")

	key = makeQPlayerMatchKey(session.Userid)
	err = ssdbc.QDel(key, matchIdStr, Q_MATCH_DEL_LIMIT, true)
	lwutil.CheckError(err, "err_ssdb_qdel")

	match.Deleted = true
	saveMatch(ssdbc, match)
}

func apiMatchMod(w http.ResponseWriter, r *http.Request) {
	lwutil.CheckMathod(r, "POST")
	var err error

	//ssdb
	ssdbc, err := ssdbPool.Get()
	lwutil.CheckError(err, "")
	defer ssdbc.Close()

	//session
	session, err := findSession(w, r, nil)
	lwutil.CheckError(err, "err_auth")

	//in
	var in struct {
		MatchId    int64
		Title      string
		PromoUrl   string
		PromoImage string
		Private    bool
	}
	err = lwutil.DecodeRequestBody(r, &in)
	lwutil.CheckError(err, "err_decode_body")

	//get match
	match := getMatch(ssdbc, in.MatchId)

	//check owner
	if match.OwnerId != session.Userid {
		lwutil.SendError("err_owner", "not the pack's owner")
	}

	//check time
	// now := lwutil.GetRedisTimeUnix()
	// if now > match.EndTime {
	// 	lwutil.WriteResponse(w, match)
	// 	return
	// }

	//private
	if match.Private != in.Private {
		match.Private = in.Private
		if match.Private {
			resp, err := ssdbc.Do("zdel", Z_MATCH, in.MatchId)
			lwutil.CheckSsdbError(resp, err)

			resp, err = ssdbc.Do("zdel", Z_HOT_MATCH, in.MatchId)
			lwutil.CheckSsdbError(resp, err)
		} else {
			if !match.Deleted {
				//add to Z_MATCH
				resp, err := ssdbc.Do("zset", Z_MATCH, match.Id, match.BeginTime)
				lwutil.CheckSsdbError(resp, err)

				if !match.HasResult {
					//Z_HOT_MATCH
					prizeKey := makeHMatchExtraSubkey(in.MatchId, MATCH_EXTRA_PRIZE)
					resp, err = ssdbc.Do("hget", H_MATCH_EXTRA, prizeKey)
					extraPrize := 0
					if resp[0] == "ok" {
						extraPrize, err = strconv.Atoi(resp[1])
						lwutil.CheckError(err, "")
					}

					resp, err = ssdbc.Do("zset", Z_HOT_MATCH, in.MatchId, match.Prize+extraPrize)
					lwutil.CheckSsdbError(resp, err)
				}
			}
		}
	}

	//update match
	match.Title = in.Title
	match.PromoImage = in.PromoImage
	match.PromoUrl = in.PromoUrl

	//save
	js, err := json.Marshal(match)
	lwutil.CheckError(err, "")
	resp, err := ssdbc.Do("hset", H_MATCH, in.MatchId, js)
	lwutil.CheckSsdbError(resp, err)

	//out
	lwutil.WriteResponse(w, match)
}

func apiMatchGet(w http.ResponseWriter, r *http.Request) {
	lwutil.CheckMathod(r, "POST")
	var err error

	//ssdb
	ssdbc, err := ssdbPool.Get()
	lwutil.CheckError(err, "")
	defer ssdbc.Close()

	//in
	var in struct {
		MatchId int64
	}
	err = lwutil.DecodeRequestBody(r, &in)
	lwutil.CheckError(err, "err_decode_body")

	//get match
	match := getMatch(ssdbc, in.MatchId)
	pack, _ := getPack(ssdbc, match.PackId)

	//out
	out := struct {
		*Match
		Pack *Pack
	}{
		match,
		pack,
	}

	//out
	lwutil.WriteResponse(w, out)
}

func apiMatchList(w http.ResponseWriter, r *http.Request) {
	lwutil.CheckMathod(r, "POST")
	var err error

	//ssdb
	ssdbc, err := ssdbPool.Get()
	lwutil.CheckError(err, "")
	defer ssdbc.Close()

	// //session
	// _, err = findSession(w, r, nil)
	// lwutil.CheckError(err, "err_auth")

	//in
	var in struct {
		StartId   int64
		BeginTime int64
		Limit     int
	}
	err = lwutil.DecodeRequestBody(r, &in)
	lwutil.CheckError(err, "err_decode_body")

	if in.Limit <= 0 {
		in.Limit = 20
	} else if in.Limit > 50 {
		in.Limit = 50
	}

	if in.StartId == 0 {
		in.StartId = math.MaxInt64
	}

	if in.BeginTime == 0 {
		in.BeginTime = math.MaxInt64
	}

	//get keys
	resp, err := ssdbc.Do("zrscan", Z_MATCH, in.StartId, in.BeginTime, "", in.Limit)
	lwutil.CheckSsdbError(resp, err)

	if len(resp) == 1 {
		w.Write([]byte("[]"))
		return
	}
	resp = resp[1:]

	//get matches
	num := len(resp) / 2
	args := make([]interface{}, 2, num+2)
	args[0] = "multi_hget"
	args[1] = H_MATCH
	for i := 0; i < num; i++ {
		args = append(args, resp[i*2])
	}
	resp, err = ssdbc.Do(args...)
	lwutil.CheckSsdbError(resp, err)
	resp = resp[1:]

	type OutMatch struct {
		Match
		MatchExtra
	}

	matches := make([]OutMatch, len(resp)/2)
	m := make(map[int64]int) //key:matchId, value:index
	for i, _ := range matches {
		js := resp[i*2+1]
		err = json.Unmarshal([]byte(js), &matches[i])
		lwutil.CheckError(err, "")
		m[matches[i].Id] = i
	}

	//match extra
	if len(matches) > 0 {
		args = make([]interface{}, 2, len(matches)*2+2)
		args[0] = "multi_hget"
		args[1] = H_MATCH_EXTRA
		for _, v := range matches {
			playTimesKey := makeHMatchExtraSubkey(v.Id, MATCH_EXTRA_PLAY_TIMES)
			prizeKey := makeHMatchExtraSubkey(v.Id, MATCH_EXTRA_PRIZE)
			likeNumKey := makeHMatchExtraSubkey(v.Id, MATCH_EXTRA_LIKE_NUM)
			args = append(args, playTimesKey, prizeKey, likeNumKey)
		}
		resp, err = ssdbc.Do(args...)
		lwutil.CheckSsdbError(resp, err)
		resp = resp[1:]

		num := len(resp) / 2
		for i := 0; i < num; i++ {
			key := resp[i*2]
			var matchId int64
			var fieldKey string
			_, err = fmt.Sscanf(key, "%d/%s", &matchId, &fieldKey)
			lwutil.CheckError(err, "")

			idx := m[matchId]
			if fieldKey == MATCH_EXTRA_PLAY_TIMES {
				playTimes, err := strconv.Atoi(resp[i*2+1])
				lwutil.CheckError(err, "")
				matches[idx].PlayTimes = playTimes
			} else if fieldKey == MATCH_EXTRA_PRIZE {
				ExtraPrize, err := strconv.Atoi(resp[i*2+1])
				lwutil.CheckError(err, "")
				matches[idx].ExtraPrize = ExtraPrize
			} else if fieldKey == MATCH_EXTRA_LIKE_NUM {
				likeNum, err := strconv.Atoi(resp[i*2+1])
				lwutil.CheckError(err, "")
				matches[idx].LikeNum = likeNum
			}
		}
	}

	//out
	lwutil.WriteResponse(w, matches)
}

func apiMatchListMine(w http.ResponseWriter, r *http.Request) {
	lwutil.CheckMathod(r, "POST")
	var err error

	//ssdb
	ssdbc, err := ssdbPool.Get()
	lwutil.CheckError(err, "")
	defer ssdbc.Close()

	//session
	session, err := findSession(w, r, nil)
	lwutil.CheckError(err, "err_auth")

	//in
	var in struct {
		StartId   int64
		BeginTime int64
		Limit     int
	}
	err = lwutil.DecodeRequestBody(r, &in)
	lwutil.CheckError(err, "err_decode_body")

	if in.Limit <= 0 {
		in.Limit = 20
	} else if in.Limit > 50 {
		in.Limit = 50
	}

	if in.StartId == 0 {
		in.StartId = math.MaxInt64
	}
	if in.BeginTime == 0 {
		in.BeginTime = math.MaxInt64
	}

	//get keys
	keyMatchMine := makeZPlayerMatchKey(session.Userid)
	resp, err := ssdbc.Do("zrscan", keyMatchMine, in.StartId, in.BeginTime, "", in.Limit)
	lwutil.CheckSsdbError(resp, err)

	if len(resp) == 1 {
		w.Write([]byte("[]"))
		return
	}
	resp = resp[1:]

	//get matches
	num := len(resp) / 2
	args := make([]interface{}, 2, num+2)
	args[0] = "multi_hget"
	args[1] = H_MATCH
	for i := 0; i < num; i++ {
		args = append(args, resp[i*2])
	}
	resp, err = ssdbc.Do(args...)
	lwutil.CheckSsdbError(resp, err)
	resp = resp[1:]

	type OutMatch struct {
		Match
		MatchExtra
	}

	matches := make([]OutMatch, len(resp)/2)
	m := make(map[int64]int) //key:matchId, value:index
	for i, _ := range matches {
		packjs := resp[i*2+1]
		err = json.Unmarshal([]byte(packjs), &matches[i])
		lwutil.CheckError(err, "")
		m[matches[i].Id] = i
	}

	//match extra
	if len(matches) > 0 {
		args = make([]interface{}, 2, len(matches)*2+2)
		args[0] = "multi_hget"
		args[1] = H_MATCH_EXTRA
		for _, v := range matches {
			playTimesKey := makeHMatchExtraSubkey(v.Id, MATCH_EXTRA_PLAY_TIMES)
			prizeKey := makeHMatchExtraSubkey(v.Id, MATCH_EXTRA_PRIZE)
			likeNumKey := makeHMatchExtraSubkey(v.Id, MATCH_EXTRA_LIKE_NUM)
			args = append(args, playTimesKey, prizeKey, likeNumKey)
		}
		resp, err = ssdbc.Do(args...)
		lwutil.CheckSsdbError(resp, err)
		resp = resp[1:]

		num := len(resp) / 2
		for i := 0; i < num; i++ {
			key := resp[i*2]
			var matchId int64
			var fieldKey string
			_, err = fmt.Sscanf(key, "%d/%s", &matchId, &fieldKey)
			lwutil.CheckError(err, "")

			idx := m[matchId]
			if fieldKey == MATCH_EXTRA_PLAY_TIMES {
				playTimes, err := strconv.Atoi(resp[i*2+1])
				lwutil.CheckError(err, "")
				matches[idx].PlayTimes = playTimes
			} else if fieldKey == MATCH_EXTRA_PRIZE {
				extraPrize, err := strconv.Atoi(resp[i*2+1])
				lwutil.CheckError(err, "")
				matches[idx].ExtraPrize = extraPrize
			} else if fieldKey == MATCH_EXTRA_LIKE_NUM {
				likeNum, err := strconv.Atoi(resp[i*2+1])
				lwutil.CheckError(err, "")
				matches[idx].LikeNum = likeNum
			}
		}
	}

	//out
	lwutil.WriteResponse(w, matches)
}

func apiMatchListOriginal(w http.ResponseWriter, r *http.Request) {
	lwutil.CheckMathod(r, "POST")
	var err error

	//ssdb
	ssdbc, err := ssdbPool.Get()
	lwutil.CheckError(err, "")
	defer ssdbc.Close()

	//session
	session, err := findSession(w, r, nil)
	lwutil.CheckError(err, "err_auth")

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

	//out struct
	type OutMatch struct {
		Match
		MatchExtra
	}

	type Out struct {
		Matches   []OutMatch
		LastScore int64
		FanNum    int
		FollowNum int
	}
	out := Out{
		[]OutMatch{},
		0,
		0,
		0,
	}

	//get keys
	key := makeZPlayerMatchKey(in.UserId)
	resp, err := ssdbc.Do("zrscan", key, in.StartId, in.LastScore, "", in.Limit)
	lwutil.CheckSsdbError(resp, err)

	if len(resp) == 1 {
		lwutil.WriteResponse(w, out)
		return
	}
	resp = resp[1:]

	//get matches
	num := len(resp) / 2
	args := make([]interface{}, 2, num+2)
	args[0] = "multi_hget"
	args[1] = H_MATCH
	for i := 0; i < num; i++ {
		args = append(args, resp[i*2])
		if i == num-1 {
			out.LastScore, err = strconv.ParseInt(resp[i*2+1], 10, 64)
			lwutil.CheckError(err, "")
		}
	}
	resp, err = ssdbc.Do(args...)
	lwutil.CheckSsdbError(resp, err)
	resp = resp[1:]

	matches := make([]OutMatch, len(resp)/2)
	m := make(map[int64]int) //key:matchId, value:index
	for i, _ := range matches {
		packjs := resp[i*2+1]
		err = json.Unmarshal([]byte(packjs), &matches[i])
		lwutil.CheckError(err, "")
		m[matches[i].Id] = i
	}

	//match extra
	if len(matches) > 0 {
		args = make([]interface{}, 2, len(matches)*2+2)
		args[0] = "multi_hget"
		args[1] = H_MATCH_EXTRA
		for _, v := range matches {
			playTimesKey := makeHMatchExtraSubkey(v.Id, MATCH_EXTRA_PLAY_TIMES)
			prizeKey := makeHMatchExtraSubkey(v.Id, MATCH_EXTRA_PRIZE)
			likeNumKey := makeHMatchExtraSubkey(v.Id, MATCH_EXTRA_LIKE_NUM)
			args = append(args, playTimesKey, prizeKey, likeNumKey)
		}
		resp, err = ssdbc.Do(args...)
		lwutil.CheckSsdbError(resp, err)
		resp = resp[1:]

		num := len(resp) / 2
		for i := 0; i < num; i++ {
			key := resp[i*2]
			var matchId int64
			var fieldKey string
			_, err = fmt.Sscanf(key, "%d/%s", &matchId, &fieldKey)
			lwutil.CheckError(err, "")

			idx := m[matchId]
			if fieldKey == MATCH_EXTRA_PLAY_TIMES {
				playTimes, err := strconv.Atoi(resp[i*2+1])
				lwutil.CheckError(err, "")
				matches[idx].PlayTimes = playTimes
			} else if fieldKey == MATCH_EXTRA_PRIZE {
				extraPrize, err := strconv.Atoi(resp[i*2+1])
				lwutil.CheckError(err, "")
				matches[idx].ExtraPrize = extraPrize
			} else if fieldKey == MATCH_EXTRA_LIKE_NUM {
				likeNum, err := strconv.Atoi(resp[i*2+1])
				lwutil.CheckError(err, "")
				matches[idx].LikeNum = likeNum
			}
		}
	}

	//out matches
	out.Matches = matches

	//
	out.FanNum, out.FollowNum = getPlayerFanFollowNum(ssdbc, in.UserId)

	lwutil.WriteResponse(w, out)
}

func apiMatchListPlayedMatch(w http.ResponseWriter, r *http.Request) {
	lwutil.CheckMathod(r, "POST")
	var err error

	//ssdb
	ssdbc, err := ssdbPool.Get()
	lwutil.CheckError(err, "")
	defer ssdbc.Close()

	//session
	session, err := findSession(w, r, nil)
	lwutil.CheckError(err, "err_auth")

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

	//out struct
	type OutMatch struct {
		Match
		MatchExtra
	}

	type Out struct {
		Matches   []OutMatch
		LastScore int64
		FanNum    int
		FollowNum int
	}
	out := Out{
		[]OutMatch{},
		0,
		0,
		0,
	}

	//get keys
	key := makeZPlayedMatchKey(in.UserId)
	resp, err := ssdbc.Do("zrscan", key, in.StartId, in.LastScore, "", in.Limit)
	lwutil.CheckSsdbError(resp, err)

	if len(resp) == 1 {
		lwutil.WriteResponse(w, out)
		return
	}
	resp = resp[1:]

	//get matches
	num := len(resp) / 2
	args := make([]interface{}, 2, num+2)
	args[0] = "multi_hget"
	args[1] = H_MATCH
	for i := 0; i < num; i++ {
		args = append(args, resp[i*2])
		if i == num-1 {
			out.LastScore, err = strconv.ParseInt(resp[i*2+1], 10, 64)
			lwutil.CheckError(err, "")
		}
	}
	resp, err = ssdbc.Do(args...)
	lwutil.CheckSsdbError(resp, err)
	resp = resp[1:]

	matches := make([]OutMatch, len(resp)/2)
	m := make(map[int64]int) //key:matchId, value:index
	for i, _ := range matches {
		packjs := resp[i*2+1]
		err = json.Unmarshal([]byte(packjs), &matches[i])
		lwutil.CheckError(err, "")
		m[matches[i].Id] = i
	}

	//match extra
	if len(matches) > 0 {
		args = make([]interface{}, 2, len(matches)*2+2)
		args[0] = "multi_hget"
		args[1] = H_MATCH_EXTRA
		for _, v := range matches {
			playTimesKey := makeHMatchExtraSubkey(v.Id, MATCH_EXTRA_PLAY_TIMES)
			prizeKey := makeHMatchExtraSubkey(v.Id, MATCH_EXTRA_PRIZE)
			likeNumKey := makeHMatchExtraSubkey(v.Id, MATCH_EXTRA_LIKE_NUM)
			args = append(args, playTimesKey, prizeKey, likeNumKey)
		}
		resp, err = ssdbc.Do(args...)
		lwutil.CheckSsdbError(resp, err)
		resp = resp[1:]

		num := len(resp) / 2
		for i := 0; i < num; i++ {
			key := resp[i*2]
			var matchId int64
			var fieldKey string
			_, err = fmt.Sscanf(key, "%d/%s", &matchId, &fieldKey)
			lwutil.CheckError(err, "")

			idx := m[matchId]
			if fieldKey == MATCH_EXTRA_PLAY_TIMES {
				playTimes, err := strconv.Atoi(resp[i*2+1])
				lwutil.CheckError(err, "")
				matches[idx].PlayTimes = playTimes
			} else if fieldKey == MATCH_EXTRA_PRIZE {
				extraPrize, err := strconv.Atoi(resp[i*2+1])
				lwutil.CheckError(err, "")
				matches[idx].ExtraPrize = extraPrize
			} else if fieldKey == MATCH_EXTRA_LIKE_NUM {
				likeNum, err := strconv.Atoi(resp[i*2+1])
				lwutil.CheckError(err, "")
				matches[idx].LikeNum = likeNum
			}
		}
	}

	//out
	out.Matches = matches
	out.FanNum, out.FollowNum = getPlayerFanFollowNum(ssdbc, in.UserId)

	lwutil.WriteResponse(w, out)
}

func apiMatchListPlayedAll(w http.ResponseWriter, r *http.Request) {
	lwutil.CheckMathod(r, "POST")
	var err error

	//ssdb
	ssdbc, err := ssdbPool.Get()
	lwutil.CheckError(err, "")
	defer ssdbc.Close()

	//session
	session, err := findSession(w, r, nil)
	lwutil.CheckError(err, "err_auth")

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

	//out struct
	type OutMatch struct {
		Match
		MatchExtra
	}

	type Out struct {
		Matches   []OutMatch
		LastScore int64
		FanNum    int
		FollowNum int
	}
	out := Out{
		[]OutMatch{},
		0,
		0,
		0,
	}

	//get keys
	key := makeZPlayedAllKey(in.UserId)
	resp, err := ssdbc.Do("zrscan", key, in.StartId, in.LastScore, "", in.Limit)
	lwutil.CheckSsdbError(resp, err)

	if len(resp) == 1 {
		lwutil.WriteResponse(w, out)
		return
	}
	resp = resp[1:]

	//get matches
	num := len(resp) / 2
	args := make([]interface{}, 2, num+2)
	args[0] = "multi_hget"
	args[1] = H_MATCH
	for i := 0; i < num; i++ {
		args = append(args, resp[i*2])
		if i == num-1 {
			out.LastScore, err = strconv.ParseInt(resp[i*2+1], 10, 64)
			lwutil.CheckError(err, "")
		}
	}
	resp, err = ssdbc.Do(args...)
	lwutil.CheckSsdbError(resp, err)
	resp = resp[1:]

	matches := make([]OutMatch, len(resp)/2)
	m := make(map[int64]int) //key:matchId, value:index
	for i, _ := range matches {
		packjs := resp[i*2+1]
		err = json.Unmarshal([]byte(packjs), &matches[i])
		lwutil.CheckError(err, "")
		m[matches[i].Id] = i
	}

	//match extra
	if len(matches) > 0 {
		args = make([]interface{}, 2, len(matches)*2+2)
		args[0] = "multi_hget"
		args[1] = H_MATCH_EXTRA
		for _, v := range matches {
			playTimesKey := makeHMatchExtraSubkey(v.Id, MATCH_EXTRA_PLAY_TIMES)
			prizeKey := makeHMatchExtraSubkey(v.Id, MATCH_EXTRA_PRIZE)
			likeNumKey := makeHMatchExtraSubkey(v.Id, MATCH_EXTRA_LIKE_NUM)
			args = append(args, playTimesKey, prizeKey, likeNumKey)
		}
		resp, err = ssdbc.Do(args...)
		lwutil.CheckSsdbError(resp, err)
		resp = resp[1:]

		num := len(resp) / 2
		for i := 0; i < num; i++ {
			key := resp[i*2]
			var matchId int64
			var fieldKey string
			_, err = fmt.Sscanf(key, "%d/%s", &matchId, &fieldKey)
			lwutil.CheckError(err, "")

			idx := m[matchId]
			if fieldKey == MATCH_EXTRA_PLAY_TIMES {
				playTimes, err := strconv.Atoi(resp[i*2+1])
				lwutil.CheckError(err, "")
				matches[idx].PlayTimes = playTimes
			} else if fieldKey == MATCH_EXTRA_PRIZE {
				extraPrize, err := strconv.Atoi(resp[i*2+1])
				lwutil.CheckError(err, "")
				matches[idx].ExtraPrize = extraPrize
			} else if fieldKey == MATCH_EXTRA_LIKE_NUM {
				likeNum, err := strconv.Atoi(resp[i*2+1])
				lwutil.CheckError(err, "")
				matches[idx].LikeNum = likeNum
			}
		}
	}

	//out
	out.Matches = matches
	out.FanNum, out.FollowNum = getPlayerFanFollowNum(ssdbc, in.UserId)

	lwutil.WriteResponse(w, out)
}

func apiMatchListMyPlayed(w http.ResponseWriter, r *http.Request) {
	lwutil.CheckMathod(r, "POST")
	var err error

	//
	lastPlayedTime := int64(0)

	//ssdb
	ssdbc, err := ssdbPool.Get()
	lwutil.CheckError(err, "")
	defer ssdbc.Close()

	//session
	session, err := findSession(w, r, nil)
	lwutil.CheckError(err, "err_auth")

	//in
	var in struct {
		StartId    int64
		PlayedTime int64
		Limit      int
	}
	err = lwutil.DecodeRequestBody(r, &in)
	lwutil.CheckError(err, "err_decode_body")

	if in.Limit <= 0 {
		in.Limit = 20
	} else if in.Limit > 50 {
		in.Limit = 50
	}

	if in.StartId == 0 {
		in.StartId = math.MaxInt64
	}
	if in.PlayedTime == 0 {
		in.PlayedTime = math.MaxInt64
	}

	//out struct
	type OutMatch struct {
		Match
		MatchExtra
	}

	type Out struct {
		Matches        []OutMatch
		LastPlayedTime int64
	}
	out := Out{
		[]OutMatch{},
		0,
	}

	//get keys
	key := makeZPlayedMatchKey(session.Userid)
	resp, err := ssdbc.Do("zrscan", key, in.StartId, in.PlayedTime, "", in.Limit)
	lwutil.CheckSsdbError(resp, err)

	if len(resp) == 1 {
		lwutil.WriteResponse(w, out)
		return
	}
	resp = resp[1:]

	//get matches
	num := len(resp) / 2
	args := make([]interface{}, 2, num+2)
	args[0] = "multi_hget"
	args[1] = H_MATCH
	for i := 0; i < num; i++ {
		args = append(args, resp[i*2])
		lastPlayedTime, err = strconv.ParseInt(resp[i*2+1], 10, 64)
		lwutil.CheckError(err, "")
	}
	resp, err = ssdbc.Do(args...)
	lwutil.CheckSsdbError(resp, err)
	resp = resp[1:]

	matches := make([]OutMatch, len(resp)/2)
	m := make(map[int64]int) //key:matchId, value:index
	for i, _ := range matches {
		packjs := resp[i*2+1]
		err = json.Unmarshal([]byte(packjs), &matches[i])
		lwutil.CheckError(err, "")
		m[matches[i].Id] = i
	}

	//match extra
	if len(matches) > 0 {
		args = make([]interface{}, 2, len(matches)*2+2)
		args[0] = "multi_hget"
		args[1] = H_MATCH_EXTRA
		for _, v := range matches {
			playTimesKey := makeHMatchExtraSubkey(v.Id, MATCH_EXTRA_PLAY_TIMES)
			prizeKey := makeHMatchExtraSubkey(v.Id, MATCH_EXTRA_PRIZE)
			likeNumKey := makeHMatchExtraSubkey(v.Id, MATCH_EXTRA_PRIZE)
			args = append(args, playTimesKey, prizeKey, likeNumKey)
		}
		resp, err = ssdbc.Do(args...)
		lwutil.CheckSsdbError(resp, err)
		resp = resp[1:]

		num := len(resp) / 2
		for i := 0; i < num; i++ {
			key := resp[i*2]
			var matchId int64
			var fieldKey string
			_, err = fmt.Sscanf(key, "%d/%s", &matchId, &fieldKey)
			lwutil.CheckError(err, "")

			idx := m[matchId]
			if fieldKey == MATCH_EXTRA_PLAY_TIMES {
				playTimes, err := strconv.Atoi(resp[i*2+1])
				lwutil.CheckError(err, "")
				matches[idx].PlayTimes = playTimes
			} else if fieldKey == MATCH_EXTRA_PRIZE {
				extraPrize, err := strconv.Atoi(resp[i*2+1])
				lwutil.CheckError(err, "")
				matches[idx].ExtraPrize = extraPrize
			} else if fieldKey == MATCH_EXTRA_LIKE_NUM {
				likeNum, err := strconv.Atoi(resp[i*2+1])
				lwutil.CheckError(err, "")
				matches[idx].LikeNum = likeNum
			}
		}
	}

	//out
	out = Out{
		matches,
		lastPlayedTime,
	}

	lwutil.WriteResponse(w, out)
}

func apiMatchListHot(w http.ResponseWriter, r *http.Request) {
	lwutil.CheckMathod(r, "POST")
	var err error

	//ssdb
	ssdbc, err := ssdbPool.Get()
	lwutil.CheckError(err, "")
	defer ssdbc.Close()

	//session
	_, err = findSession(w, r, nil)
	lwutil.CheckError(err, "err_auth")

	//in
	var in struct {
		StartId  int64
		PrizeSum int64
		Limit    int
	}
	err = lwutil.DecodeRequestBody(r, &in)
	lwutil.CheckError(err, "err_decode_body")

	if in.Limit <= 0 {
		in.Limit = 20
	} else if in.Limit > 50 {
		in.Limit = 50
	}

	if in.StartId == 0 {
		in.StartId = math.MaxInt64
	}
	if in.PrizeSum == -1 {
		in.PrizeSum = math.MaxInt64
	}

	//out struct
	type OutMatch struct {
		Match
		MatchExtra
	}

	type Out struct {
		Matches []OutMatch
	}
	out := Out{
		[]OutMatch{},
	}

	//get keys
	resp, err := ssdbc.Do("zrscan", Z_HOT_MATCH, in.StartId, in.PrizeSum, "", in.Limit)
	lwutil.CheckSsdbError(resp, err)

	if len(resp) == 1 {
		lwutil.WriteResponse(w, out)
		return
	}
	resp = resp[1:]

	//get matches
	num := len(resp) / 2
	args := make([]interface{}, 2, num+2)
	args[0] = "multi_hget"
	args[1] = H_MATCH
	for i := 0; i < num; i++ {
		args = append(args, resp[i*2])
		lwutil.CheckError(err, "")
	}
	resp, err = ssdbc.Do(args...)
	lwutil.CheckSsdbError(resp, err)
	resp = resp[1:]

	matches := make([]OutMatch, len(resp)/2)
	m := make(map[int64]int) //key:matchId, value:index
	for i, _ := range matches {
		packjs := resp[i*2+1]
		err = json.Unmarshal([]byte(packjs), &matches[i])
		lwutil.CheckError(err, "")
		m[matches[i].Id] = i
	}

	//match extra
	if len(matches) > 0 {
		args = make([]interface{}, 2, len(matches)*2+2)
		args[0] = "multi_hget"
		args[1] = H_MATCH_EXTRA
		for _, v := range matches {
			playTimesKey := makeHMatchExtraSubkey(v.Id, MATCH_EXTRA_PLAY_TIMES)
			prizeKey := makeHMatchExtraSubkey(v.Id, MATCH_EXTRA_PRIZE)
			likeNumKey := makeHMatchExtraSubkey(v.Id, MATCH_EXTRA_LIKE_NUM)
			args = append(args, playTimesKey, prizeKey, likeNumKey)
		}
		resp, err = ssdbc.Do(args...)
		lwutil.CheckSsdbError(resp, err)
		resp = resp[1:]

		num := len(resp) / 2
		for i := 0; i < num; i++ {
			key := resp[i*2]
			var matchId int64
			var fieldKey string
			_, err = fmt.Sscanf(key, "%d/%s", &matchId, &fieldKey)
			lwutil.CheckError(err, "")

			idx := m[matchId]
			if fieldKey == MATCH_EXTRA_PLAY_TIMES {
				playTimes, err := strconv.Atoi(resp[i*2+1])
				lwutil.CheckError(err, "")
				matches[idx].PlayTimes = playTimes
			} else if fieldKey == MATCH_EXTRA_PRIZE {
				extarPrize, err := strconv.Atoi(resp[i*2+1])
				lwutil.CheckError(err, "")
				matches[idx].ExtraPrize = extarPrize
			} else if fieldKey == MATCH_EXTRA_LIKE_NUM {
				likeNum, err := strconv.Atoi(resp[i*2+1])
				lwutil.CheckError(err, "")
				matches[idx].LikeNum = likeNum
			}
		}
	}

	//out
	out = Out{
		matches,
	}

	lwutil.WriteResponse(w, out)
}

func apiMatchListLike(w http.ResponseWriter, r *http.Request) {
	lwutil.CheckMathod(r, "POST")
	var err error

	//ssdb
	ssdbc, err := ssdbPool.Get()
	lwutil.CheckError(err, "")
	defer ssdbc.Close()

	//session
	session, err := findSession(w, r, nil)
	lwutil.CheckError(err, "err_auth")

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

	//out struct
	type OutMatch struct {
		Match
		MatchExtra
	}

	type Out struct {
		Matches   []*OutMatch
		LastScore int64
		FanNum    int
		FollowNum int
	}
	out := Out{
		[]*OutMatch{},
		0,
		0,
		0,
	}

	//get keys
	key := makeZLikeMatchKey(in.UserId)
	resp, err := ssdbc.Do("zrscan", key, in.StartId, in.LastScore, "", in.Limit)
	lwutil.CheckSsdbError(resp, err)

	if len(resp) == 1 {
		lwutil.WriteResponse(w, out)
		return
	}
	resp = resp[1:]

	//get matches
	num := len(resp) / 2
	args := make([]interface{}, 2, num+2)
	args[0] = "multi_hget"
	args[1] = H_MATCH
	for i := 0; i < num; i++ {
		args = append(args, resp[i*2])
		if i == num-1 {
			out.LastScore, err = strconv.ParseInt(resp[i*2+1], 10, 64)
			lwutil.CheckError(err, "")
		}
	}
	resp, err = ssdbc.Do(args...)
	lwutil.CheckSsdbError(resp, err)
	resp = resp[1:]

	matches := make([]*OutMatch, len(resp)/2)
	m := make(map[int64]int) //key:matchId, value:index
	for i, _ := range matches {
		packjs := resp[i*2+1]
		err = json.Unmarshal([]byte(packjs), &matches[i])
		lwutil.CheckError(err, "")
		m[matches[i].Id] = i
	}

	//match extra
	if len(matches) > 0 {
		args = make([]interface{}, 2, len(matches)*2+2)
		args[0] = "multi_hget"
		args[1] = H_MATCH_EXTRA
		for _, v := range matches {
			playTimesKey := makeHMatchExtraSubkey(v.Id, MATCH_EXTRA_PLAY_TIMES)
			prizeKey := makeHMatchExtraSubkey(v.Id, MATCH_EXTRA_PRIZE)
			likeNumKey := makeHMatchExtraSubkey(v.Id, MATCH_EXTRA_LIKE_NUM)
			args = append(args, playTimesKey, prizeKey, likeNumKey)
		}
		resp, err = ssdbc.Do(args...)
		lwutil.CheckSsdbError(resp, err)
		resp = resp[1:]

		num := len(resp) / 2
		for i := 0; i < num; i++ {
			key := resp[i*2]
			var matchId int64
			var fieldKey string
			_, err = fmt.Sscanf(key, "%d/%s", &matchId, &fieldKey)
			lwutil.CheckError(err, "")

			idx := m[matchId]
			if fieldKey == MATCH_EXTRA_PLAY_TIMES {
				playTimes, err := strconv.Atoi(resp[i*2+1])
				lwutil.CheckError(err, "")
				matches[idx].PlayTimes = playTimes
			} else if fieldKey == MATCH_EXTRA_PRIZE {
				extraPrize, err := strconv.Atoi(resp[i*2+1])
				lwutil.CheckError(err, "")
				matches[idx].ExtraPrize = extraPrize
			} else if fieldKey == MATCH_EXTRA_LIKE_NUM {
				likeNum, err := strconv.Atoi(resp[i*2+1])
				lwutil.CheckError(err, "")
				matches[idx].LikeNum = likeNum
			}
		}
	}

	//del deleted match
	outMatches := make([]*OutMatch, 0, len(matches))
	delMatcheIds := make([]int64, 0, 4)
	for _, match := range matches {
		if match.Deleted && match.OwnerId == session.Userid {
			delMatcheIds = append(delMatcheIds, match.Id)
		} else {
			outMatches = append(outMatches, match)
		}
	}

	if len(delMatcheIds) > 0 {
		args := make([]interface{}, 2, len(delMatcheIds)+2)
		args[0] = "multi_zdel"
		key := makeZLikeMatchKey(in.UserId)
		args[1] = key
		for _, v := range delMatcheIds {
			args = append(args, v)
		}

		resp, err = ssdbc.Do(args...)
		lwutil.CheckSsdbError(resp, err)
	}

	//out
	out.Matches = outMatches
	out.FanNum, out.FollowNum = getPlayerFanFollowNum(ssdbc, in.UserId)

	lwutil.WriteResponse(w, out)
}

func apiMatchListUserWeb(w http.ResponseWriter, r *http.Request) {
	lwutil.CheckMathod(r, "POST")
	var err error

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

	type Out struct {
		Matches   []*Match
		LastScore int64
	}
	out := Out{
		[]*Match{},
		0,
	}

	//get keys
	key := makeZLikeMatchKey(in.UserId)
	resp, err := ssdbc.Do("zrscan", key, in.StartId, in.LastScore, "", in.Limit)
	lwutil.CheckSsdbError(resp, err)

	if len(resp) == 1 {
		lwutil.WriteResponse(w, out)
		return
	}
	resp = resp[1:]

	//get matches
	num := len(resp) / 2
	args := make([]interface{}, 2, num+2)
	args[0] = "multi_hget"
	args[1] = H_MATCH
	for i := 0; i < num; i++ {
		args = append(args, resp[i*2])
		if i == num-1 {
			out.LastScore, err = strconv.ParseInt(resp[i*2+1], 10, 64)
			lwutil.CheckError(err, "")
		}
	}
	resp, err = ssdbc.Do(args...)
	lwutil.CheckSsdbError(resp, err)
	resp = resp[1:]

	matches := make([]*Match, len(resp)/2)
	for i, _ := range matches {
		packjs := resp[i*2+1]
		err = json.Unmarshal([]byte(packjs), &matches[i])
		lwutil.CheckError(err, "")
	}

	//out
	out.Matches = matches

	lwutil.WriteResponse(w, out)
}

func apiMatchListUserWebQ(w http.ResponseWriter, r *http.Request) {
	lwutil.CheckMathod(r, "POST")
	var err error

	//ssdb
	ssdbc, err := ssdbPool.Get()
	lwutil.CheckError(err, "")
	defer ssdbc.Close()

	//session
	session, err := findSession(w, r, nil)
	lwutil.CheckError(err, "err_auth")

	//in
	var in struct {
		Type   int //0:all(like) 1:original(player) 2:privateLike
		UserId int64
		Offset int
		Limit  int
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

	type Out struct {
		Matches        []*Match
		MatchNum       int
		PlayedMatchMap map[string]*PlayerMatchInfo
		OwnerMap       map[string]*PlayerInfoLite
	}
	out := Out{
		[]*Match{},
		0,
		make(map[string]*PlayerMatchInfo),
		make(map[string]*PlayerInfoLite),
	}

	//matchNum
	key := ""
	if in.Type == 0 {
		key = makeQLikeMatchKey(in.UserId)
	} else if in.Type == 1 {
		key = makeQPlayerMatchKey(in.UserId)
	} else {
		lwutil.SendError("err_type", "")
	}

	resp, err := ssdbc.Do("qsize", key)
	lwutil.CheckSsdbError(resp, err)
	out.MatchNum, err = strconv.Atoi(resp[1])
	lwutil.CheckError(err, "err_strconv")

	//adjust offset
	if in.Offset < 1 {
		in.Offset = -in.Limit
		if in.Limit > out.MatchNum {
			in.Offset = 0
		}
	}

	//qrange
	resp, err = ssdbc.Do("qrange", key, in.Offset, in.Limit)
	lwutil.CheckSsdbError(resp, err)

	if len(resp) == 1 {
		lwutil.WriteResponse(w, out)
		return
	}
	resp = resp[1:]

	//get matches
	num := len(resp)
	args := make([]interface{}, 2, num+2)
	args[0] = "multi_hget"
	args[1] = H_MATCH
	for i := 0; i < num; i++ {
		args = append(args, resp[i])
	}
	resp, err = ssdbc.Do(args...)
	lwutil.CheckSsdbError(resp, err)
	resp = resp[1:]

	matches := make([]*Match, len(resp)/2)
	for i, _ := range matches {
		packjs := resp[i*2+1]
		err = json.Unmarshal([]byte(packjs), &matches[i])
		lwutil.CheckError(err, "")
	}
	out.Matches = matches

	//playedMap
	num = len(matches)
	cmds := make([]interface{}, 2, num+2)
	cmds[0] = "multi_hget"
	cmds[1] = H_MATCH_PLAY
	for _, match := range matches {
		subkey := makeMatchPlaySubkey(match.Id, session.Userid)
		cmds = append(cmds, subkey)
	}
	resp, err = ssdbc.Do(cmds...)
	lwutil.CheckSsdbError(resp, err)
	resp = resp[1:]

	num = len(resp) / 2
	for i := 0; i < num; i++ {
		matchIdStr := strings.Split(resp[i*2], "/")[0]
		var matchPlay MatchPlay
		err := json.Unmarshal([]byte(resp[i*2+1]), &matchPlay)
		lwutil.CheckError(err, "err_json")
		playerMatchInfo := makePlayerMatchInfo(&matchPlay)
		out.PlayedMatchMap[matchIdStr] = playerMatchInfo
	}

	//match thumbs fix
	for _, match := range matches {
		if match.Thumbs == nil {
			pack, err := getPack(ssdbc, match.PackId)
			lwutil.CheckError(err, "err_get_pack")
			match.Thumbs = pack.Thumbs
			saveMatch(ssdbc, match)
		}
	}

	//ownerMap
	cmds = make([]interface{}, 2, len(matches)+2)
	cmds[0] = "multi_hget"
	cmds[1] = H_PLAYER_INFO_LITE
	for _, match := range matches {
		cmds = append(cmds, match.OwnerId)
	}

	resp, err = ssdbc.Do(cmds...)
	lwutil.CheckSsdbError(resp, err)
	resp = resp[1:]
	num = len(resp) / 2
	for i := 0; i < num; i++ {
		var owner PlayerInfoLite
		err = json.Unmarshal([]byte(resp[i*2+1]), &owner)
		lwutil.CheckError(err, "err_json")
		out.OwnerMap[resp[i*2]] = &owner
	}

	//out
	lwutil.WriteResponse(w, out)
}

func apiMatchGetDynamicData(w http.ResponseWriter, r *http.Request) {
	lwutil.CheckMathod(r, "POST")
	var err error

	//ssdb
	ssdbc, err := ssdbPool.Get()
	lwutil.CheckError(err, "")
	defer ssdbc.Close()

	//session
	session, err := findSession(w, r, nil)
	lwutil.CheckError(err, "err_auth")

	//in
	var in struct {
		MatchId int64
	}
	err = lwutil.DecodeRequestBody(r, &in)
	lwutil.CheckError(err, "err_decode_body")

	//
	args := make([]interface{}, 2, 4)
	args[0] = "multi_hget"
	args[1] = H_MATCH_EXTRA
	playTimesKey := makeHMatchExtraSubkey(in.MatchId, MATCH_EXTRA_PLAY_TIMES)
	prizeKey := makeHMatchExtraSubkey(in.MatchId, MATCH_EXTRA_PRIZE)
	likeNumKey := makeHMatchExtraSubkey(in.MatchId, MATCH_EXTRA_LIKE_NUM)
	args = append(args, playTimesKey, prizeKey, likeNumKey)
	resp, err := ssdbc.Do(args...)
	lwutil.CheckSsdbError(resp, err)
	resp = resp[1:]

	var playTimes int
	var extraPrize int
	var likeNum int
	num := len(resp) / 2
	for i := 0; i < num; i++ {
		if resp[i*2] == playTimesKey {
			playTimes, err = strconv.Atoi(resp[i*2+1])
			lwutil.CheckError(err, "")
		} else if resp[i*2] == prizeKey {
			extraPrize, err = strconv.Atoi(resp[i*2+1])
			lwutil.CheckError(err, "")
		} else if resp[i*2] == likeNumKey {
			likeNum, err = strconv.Atoi(resp[i*2+1])
			lwutil.CheckError(err, "")
		}
	}

	//get match play
	play, err := getMatchPlay(ssdbc, in.MatchId, session.Userid)
	lwutil.CheckError(err, "err_get_match_play")

	//get match
	match := getMatch(ssdbc, in.MatchId)

	//get rank
	myRank := 0
	rankNum := 0
	if match.HasResult {
		myRank = play.FinalRank

		//rankNum
		hRankKey := makeHMatchRankKey(in.MatchId)
		resp, err = ssdbc.Do("hsize", hRankKey)
		lwutil.CheckSsdbError(resp, err)
		rankNum, err = strconv.Atoi(resp[1])
		lwutil.CheckError(err, "")
	} else {
		//redis
		rc := redisPool.Get()
		defer rc.Close()

		lbKey := makeMatchLeaderboardRdsKey(in.MatchId)

		//get my rank and rank count
		rc.Send("ZREVRANK", lbKey, session.Userid)
		rc.Send("ZCARD", lbKey)
		err = rc.Flush()
		lwutil.CheckError(err, "")
		myRank, err = redis.Int(rc.Receive())
		if err == nil {
			myRank += 1
		} else {
			myRank = 0
		}
		rankNum, err = redis.Int(rc.Receive())
		if err != nil {
			rankNum = 0
		}
	}

	//out
	out := struct {
		PlayTimes  int
		ExtraPrize int
		LikeNum    int
		MyRank     int
		RankNum    int
		MatchPlay
	}{
		playTimes,
		extraPrize,
		likeNum,
		myRank,
		rankNum,
		*play,
	}
	lwutil.WriteResponse(w, out)
}

func apiMatchPlayBegin(w http.ResponseWriter, r *http.Request) {
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
		MatchId int64
	}
	err = lwutil.DecodeRequestBody(r, &in)
	lwutil.CheckError(err, "err_decode_body")

	//get match
	resp, err := ssdbc.Do("hget", H_MATCH, in.MatchId)
	lwutil.CheckSsdbError(resp, err)
	var match Match
	err = json.Unmarshal([]byte(resp[1]), &match)
	lwutil.CheckError(err, "")
	now := lwutil.GetRedisTimeUnix()

	if now < match.BeginTime || now >= match.EndTime || match.HasResult {
		lwutil.SendError("err_time", "match out of time")
	}

	if now > match.EndTime-MATCH_CLOSE_BEFORE_END_SEC {
		lwutil.SendError("err_end_soon", "match end soon")
	}

	if match.Deleted {
		lwutil.SendError("err_deleted", "match deleted")
	}

	//get matchPlay
	play, err := getMatchPlay(ssdbc, in.MatchId, session.Userid)
	lwutil.CheckError(err, "err_get_match_play")

	//free try or use goldCoin
	genLuckyNum := false
	if play.FreeTries == FREE_TRY_NUM || play.FreeTries == 0 {
		genLuckyNum = true
	}

	playerKey := makePlayerInfoKey(session.Userid)
	goldCoin := getPlayerGoldCoin(ssdbc, playerKey)
	autoPaging := false
	if play.FreeTries > 0 {
		play.FreeTries--
	} else {
		if goldCoin > 0 {
			addPlayerGoldCoin(ssdbc, playerKey, -1)
			goldCoin--
			autoPaging = true
			err = addEcoRecord(ssdbc, session.Userid, 1, ECO_FORWHAT_MATCHBEGIN)
			lwutil.CheckError(err, "")

			prizeKey := makeHMatchExtraSubkey(in.MatchId, MATCH_EXTRA_PRIZE)
			resp, err = ssdbc.Do("hincr", H_MATCH_EXTRA, prizeKey, PRIZE_NUM_PER_COIN)
			lwutil.CheckSsdbError(resp, err)
			extraPrize, err := strconv.Atoi(resp[1])
			lwutil.CheckError(err, "")

			resp, err = ssdbc.Do("zset", Z_HOT_MATCH, in.MatchId, match.Prize+extraPrize)
			lwutil.CheckSsdbError(resp, err)
		} else {
			lwutil.SendError("err_gold_coin", "no coin")
		}
	}
	play.Tries++

	playTimesKey := makeHMatchExtraSubkey(in.MatchId, MATCH_EXTRA_PLAY_TIMES)
	resp, err = ssdbc.Do("hincr", H_MATCH_EXTRA, playTimesKey, 1)

	//gen lucky number
	luckyNum := int64(0)
	if genLuckyNum {
		luckyNum = GenSerial(ssdbc, fmt.Sprintf("MATCH_LUCKY_NUM/%d", in.MatchId))
		play.LuckyNums = append(play.LuckyNums, luckyNum)
	}

	//gen secret
	play.Secret = lwutil.GenUUID()
	play.SecretExpire = lwutil.GetRedisTimeUnix() + MATCH_TRY_EXPIRE_SECONDS

	secretKey := makeSecretKey(play.Secret)
	resp, err = ssdbc.Do("setx", secretKey, in.MatchId, MATCH_TRY_EXPIRE_SECONDS)
	lwutil.CheckSsdbError(resp, err)

	//update play
	js, err := json.Marshal(play)
	lwutil.CheckError(err, "")
	subkey := makeMatchPlaySubkey(in.MatchId, session.Userid)
	resp, err = ssdbc.Do("hset", H_MATCH_PLAY, subkey, js)
	lwutil.CheckSsdbError(resp, err)

	//update Z_PLAYED_MATCH
	key := makeZPlayedMatchKey(session.Userid)
	resp, err = ssdbc.Do("zset", key, in.MatchId, match.BeginTime)
	lwutil.CheckSsdbError(resp, err)

	key = makeZPlayedAllKey(session.Userid)
	resp, err = ssdbc.Do("zset", key, in.MatchId, now)
	lwutil.CheckSsdbError(resp, err)

	//out
	out := map[string]interface{}{
		"Secret":       play.Secret,
		"SecretExpire": play.SecretExpire,
		"LuckyNum":     luckyNum,
		"GoldCoin":     goldCoin,
		"FreeTries":    play.FreeTries,
		"AutoPaging":   autoPaging,
		"SliderNum":    match.SliderNum,
	}
	lwutil.WriteResponse(w, out)
}

func apiMatchPlayEnd(w http.ResponseWriter, r *http.Request) {
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
		MatchId  int64
		Secret   string
		Score    int
		Checksum string
	}
	err = lwutil.DecodeRequestBody(r, &in)
	lwutil.CheckError(err, "err_decode_body")

	//check score
	if in.Score > -2000 {
		glog.Errorf("invalid score:%d, userId:%d, userName:%s", in.Score, session.Userid, session.Username)
		return
	}

	//secret
	secretKey := makeSecretKey(in.Secret)
	resp, err := ssdbc.Do("get", secretKey)
	lwutil.CheckSsdbError(resp, err)
	in.MatchId, err = strconv.ParseInt(resp[1], 10, 64)
	lwutil.CheckError(err, "")

	//checksum
	checksum := fmt.Sprintf("%s+%d9d7a", in.Secret, in.Score+8703)
	hasher := sha1.New()
	hasher.Write([]byte(checksum))
	checksum = hex.EncodeToString(hasher.Sum(nil))
	if in.Checksum != checksum {
		//fixme: cheater
		lwutil.SendError("err_checksum", "")
	}

	//check match play
	now := lwutil.GetRedisTimeUnix()
	matchPlayKey := makeMatchPlaySubkey(in.MatchId, session.Userid)
	resp, err = ssdbc.Do("hget", H_MATCH_PLAY, matchPlayKey)
	lwutil.CheckError(err, "")
	if resp[0] != "ok" {
		lwutil.SendError("err_not_found", "match play not found")
	}

	matchPlay := MatchPlay{}
	err = json.Unmarshal([]byte(resp[1]), &matchPlay)
	lwutil.CheckError(err, "")
	if matchPlay.Secret != in.Secret {
		lwutil.SendError("err_not_match", "Secret not match")
	}
	if now > matchPlay.SecretExpire {
		lwutil.SendError("err_expired", "secret expired")
	}

	//clear secret
	matchPlay.SecretExpire = 0

	//update score
	scoreUpdate := false
	if matchPlay.HighScore == 0 {
		matchPlay.HighScore = in.Score
		matchPlay.HighScoreTime = now
		scoreUpdate = true
	} else {
		if in.Score > matchPlay.HighScore {
			matchPlay.HighScore = in.Score
			scoreUpdate = true
		}
	}
	matchPlay.Played = true

	//save match play
	js, err := json.Marshal(matchPlay)
	resp, err = ssdbc.Do("hset", H_MATCH_PLAY, matchPlayKey, js)
	lwutil.CheckSsdbError(resp, err)

	//redis
	rc := redisPool.Get()
	defer rc.Close()

	//match leaderboard
	lbKey := makeMatchLeaderboardRdsKey(in.MatchId)
	if scoreUpdate {
		_, err = rc.Do("ZADD", lbKey, matchPlay.HighScore, session.Userid)
		lwutil.CheckError(err, "")
	}

	//get rank
	rc.Send("ZREVRANK", lbKey, session.Userid)
	rc.Send("ZCARD", lbKey)
	err = rc.Flush()
	lwutil.CheckError(err, "")
	rank, err := redis.Int(rc.Receive())
	lwutil.CheckError(err, "")
	rankNum, err := redis.Int(rc.Receive())
	lwutil.CheckError(err, "")

	// //recaculate team score
	// if scoreUpdate && rank <= TEAM_SCORE_RANK_MAX {
	// 	recaculateTeamScore(ssdbc, rc, in.EventId)
	// }

	//activity
	msec := -in.Score
	t := formateMsec(msec)
	text := fmt.Sprintf("进行了一场比赛，用时%s", t)
	addMatchActivity(ssdbc, in.MatchId, session.Userid, text)

	//out
	out := struct {
		MyRank  uint32
		RankNum uint32
	}{
		uint32(rank + 1),
		uint32(rankNum),
	}

	//out
	lwutil.WriteResponse(w, out)
}

func addMatchActivity(ssdbc *ssdb.Client, matchId int64, userId int64, text string) {
	//get playerInfoLite
	playerL, err := getPlayerInfoLite(ssdbc, userId, nil)
	lwutil.CheckError(err, "err_player")

	//add to qMatchActivity
	key := makeQMatchActivityKey(matchId)
	var activity MatchActivity
	activity.Player = playerL
	activity.Text = text
	js, err := json.Marshal(activity)

	resp, err := ssdbc.Do("qpush_front", key, js)
	lwutil.CheckSsdbError(resp, err)

	resp, err = ssdbc.Do("zsize", key)
	lwutil.CheckSsdbError(resp, err)
	num, err := strconv.Atoi(resp[1])
	lwutil.CheckError(err, "err_strconv")
	if num > MATCH_ACTIVITY_Q_LIMIT {
		resp, err = ssdbc.Do("qtrim_back", key, num-MATCH_ACTIVITY_Q_LIMIT)
		lwutil.CheckError(err, "err_ssdb_trim")
	}
}

func apiMatchFreePlay(w http.ResponseWriter, r *http.Request) {
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
		MatchId int64
		Score   int
	}
	err = lwutil.DecodeRequestBody(r, &in)
	lwutil.CheckError(err, "err_decode_body")

	//check score
	if in.Score > -2000 {
		lwutil.SendError("err_score", "invalid score")
	}

	//check match play
	play, err := getMatchPlay(ssdbc, in.MatchId, session.Userid)
	lwutil.CheckError(err, "err_get_match_play")

	if play.Played == false {
		play.Played = true

		//save match play
		matchPlayKey := makeMatchPlaySubkey(in.MatchId, session.Userid)
		js, err := json.Marshal(play)
		resp, err := ssdbc.Do("hset", H_MATCH_PLAY, matchPlayKey, js)
		lwutil.CheckSsdbError(resp, err)
	}

	playNumKey := makeHMatchExtraSubkey(in.MatchId, MATCH_EXTRA_PLAY_TIMES)
	resp, err := ssdbc.Do("hincr", H_MATCH_EXTRA, playNumKey, 1)
	lwutil.CheckSsdbError(resp, err)

	//activity
	msec := -in.Score
	t := formateMsec(msec)
	text := fmt.Sprintf("玩了一盘，用时%s", t)

	addMatchActivity(ssdbc, in.MatchId, session.Userid, text)

	//
	now := lwutil.GetRedisTimeUnix()
	key := makeZPlayedAllKey(session.Userid)
	resp, err = ssdbc.Do("zset", key, in.MatchId, now)
	lwutil.CheckSsdbError(resp, err)

	//out
	lwutil.WriteResponse(w, in)
}

func apiMatchListActivity(w http.ResponseWriter, r *http.Request) {
	var err error
	lwutil.CheckMathod(r, "POST")

	//ssdb
	ssdbc, err := ssdbPool.Get()
	lwutil.CheckError(err, "")
	defer ssdbc.Close()

	//in
	var in struct {
		MatchId int64
	}
	err = lwutil.DecodeRequestBody(r, &in)
	lwutil.CheckError(err, "err_decode_body")

	//
	key := makeQMatchActivityKey(in.MatchId)
	resp, err := ssdbc.Do("qrange", key, 0, MATCH_ACTIVITY_Q_LIMIT)
	lwutil.CheckSsdbError(resp, err)

	resp = resp[1:]
	activities := make([]*MatchActivity, 0, 10)
	for _, v := range resp {
		var activity MatchActivity
		err := json.Unmarshal([]byte(v), &activity)
		lwutil.CheckError(err, "err_json")
		activities = append(activities, &activity)
	}

	//
	lwutil.WriteResponse(w, activities)
}

func apiMatchGetRanks(w http.ResponseWriter, r *http.Request) {
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
		MatchId int64
		Offset  int
		Limit   int
	}
	err = lwutil.DecodeRequestBody(r, &in)
	lwutil.CheckError(err, "err_decode_body")

	if in.Limit > 50 {
		in.Limit = 50
	}

	//check match id
	resp, err := ssdbc.Do("hget", H_MATCH, in.MatchId)
	lwutil.CheckSsdbError(resp, err)
	var match Match
	err = json.Unmarshal([]byte(resp[1]), &match)
	lwutil.CheckError(err, "")

	//RankInfo
	type RankInfo struct {
		Rank            int
		UserId          int64
		NickName        string
		TeamName        string
		GravatarKey     string
		CustomAvatarKey string
		Score           int
		Time            int64
		Tries           int
	}

	type Out struct {
		MatchId int64
		MyRank  int
		Ranks   []RankInfo
		RankNum int
	}

	//get ranks
	var ranks []RankInfo
	myRank := 0
	rankNum := 0

	if match.HasResult {
		cmds := make([]interface{}, in.Limit+2)
		cmds[0] = "multi_hget"
		cmds[1] = makeHMatchRankKey(match.Id)
		hRankKey := cmds[1]
		for i := 0; i < in.Limit; i++ {
			rank := i + in.Offset + 1
			cmds[i+2] = rank
		}

		resp, err := ssdbc.Do(cmds...)
		lwutil.CheckSsdbError(resp, err)
		resp = resp[1:]

		num := len(resp) / 2
		ranks = make([]RankInfo, num)

		for i := 0; i < num; i++ {
			ranks[i].Rank, err = strconv.Atoi(resp[i*2])
			lwutil.CheckError(err, "")
			ranks[i].UserId, err = strconv.ParseInt(resp[i*2+1], 10, 64)
			lwutil.CheckError(err, "")
		}

		//my rank
		play, err := getMatchPlay(ssdbc, in.MatchId, session.Userid)
		lwutil.CheckError(err, "err_get_match_play")

		myRank = play.FinalRank

		//rankNum
		resp, err = ssdbc.Do("hsize", hRankKey)
		lwutil.CheckSsdbError(resp, err)
		rankNum, err = strconv.Atoi(resp[1])
		lwutil.CheckError(err, "")
	} else {
		//redis
		rc := redisPool.Get()
		defer rc.Close()

		lbKey := makeMatchLeaderboardRdsKey(in.MatchId)

		//get ranks from redis
		values, err := redis.Values(rc.Do("ZREVRANGE", lbKey, in.Offset, in.Offset+in.Limit-1))
		lwutil.CheckError(err, "")

		num := len(values)
		if num > 0 {
			ranks = make([]RankInfo, num)

			currRank := in.Offset + 1
			for i := 0; i < num; i++ {
				ranks[i].Rank = currRank
				currRank++
				ranks[i].UserId, err = redisInt64(values[i], nil)
				lwutil.CheckError(err, "")
			}
		}

		//get my rank and rank count
		rc.Send("ZREVRANK", lbKey, session.Userid)
		rc.Send("ZCARD", lbKey)
		err = rc.Flush()
		lwutil.CheckError(err, "")
		myRank, err = redis.Int(rc.Receive())
		if err == nil {
			myRank += 1
		} else {
			myRank = 0
		}
		rankNum, err = redis.Int(rc.Receive())
		if err != nil {
			rankNum = 0
		}
	}

	num := len(ranks)
	if num == 0 {
		out := Out{
			in.MatchId,
			myRank,
			[]RankInfo{},
			rankNum,
		}
		lwutil.WriteResponse(w, out)
		return
	}

	//get match plays
	cmds := make([]interface{}, 0, num+2)
	cmds = append(cmds, "multi_hget")
	cmds = append(cmds, H_MATCH_PLAY)
	for _, rank := range ranks {
		subkey := makeMatchPlaySubkey(in.MatchId, rank.UserId)
		cmds = append(cmds, subkey)
	}
	resp, err = ssdbc.Do(cmds...)
	lwutil.CheckSsdbError(resp, err)
	resp = resp[1:]

	if num*2 != len(resp) {
		lwutil.SendError("err_data_missing", "")
	}
	var play MatchPlay
	for i := range ranks {
		err = json.Unmarshal([]byte(resp[i*2+1]), &play)
		lwutil.CheckError(err, "")
		ranks[i].Score = play.HighScore
		ranks[i].NickName = play.PlayerName
		ranks[i].Time = play.HighScoreTime
		ranks[i].Tries = play.Tries
		ranks[i].TeamName = play.Team
		ranks[i].GravatarKey = play.GravatarKey
		ranks[i].CustomAvatarKey = play.CustomAvartarKey
	}

	//out
	out := Out{
		in.MatchId,
		myRank,
		ranks,
		rankNum,
	}

	lwutil.WriteResponse(w, out)
}

func apiMatchReport(w http.ResponseWriter, r *http.Request) {
	var err error
	lwutil.CheckMathod(r, "POST")

	//in
	var in struct {
		MatchId int64
	}
	err = lwutil.DecodeRequestBody(r, &in)
	lwutil.CheckError(err, "err_decode_body")

	//ssdb
	ssdbc, err := ssdbPool.Get()
	lwutil.CheckError(err, "")
	defer ssdbc.Close()

	//checkMatchExist
	resp, err := ssdbc.Do("hexists", H_MATCH, in.MatchId)
	lwutil.CheckError(err, "")
	if !ssdbCheckExists(resp) {
		lwutil.SendError("err_match_id", "Can't find match")
	}

	//
	resp, err = ssdbc.Do("zset", Z_REPORT, in.MatchId, lwutil.GetRedisTimeUnix())
	lwutil.CheckSsdbError(resp, err)

	//out
	lwutil.WriteResponse(w, &in)
}

func apiMatchLike(w http.ResponseWriter, r *http.Request) {
	lwutil.CheckMathod(r, "POST")
	var err error

	//ssdb
	ssdbc, err := ssdbPool.Get()
	lwutil.CheckError(err, "")
	defer ssdbc.Close()

	//session
	session, err := findSession(w, r, nil)
	lwutil.CheckError(err, "err_auth")

	//in
	var in struct {
		MatchId int64
	}
	err = lwutil.DecodeRequestBody(r, &in)
	lwutil.CheckError(err, "err_decode_body")

	//check match exist
	resp, err := ssdbc.Do("hexists", H_MATCH, in.MatchId)
	lwutil.CheckError(err, "")
	if !ssdbCheckExists(resp) {
		lwutil.SendError("err_match_id", "Can't find match")
	}

	//zset
	key := makeZLikeMatchKey(session.Userid)
	resp, err = ssdbc.Do("zexists", key, in.MatchId)
	lwutil.CheckError(err, "")
	if ssdbCheckExists(resp) {
		lwutil.WriteResponse(w, in)
		return
	}

	score := lwutil.GetRedisTimeUnix()
	resp, err = ssdbc.Do("zset", key, in.MatchId, score)
	lwutil.CheckSsdbError(resp, err)

	//queue
	key = makeQLikeMatchKey(session.Userid)
	resp, err = ssdbc.Do("qpush_back", key, in.MatchId)
	lwutil.CheckSsdbError(resp, err)

	//update matchLiker list
	key = makeZMatchLikerKey(in.MatchId)
	resp, err = ssdbc.Do("zset", key, session.Userid, score)
	lwutil.CheckSsdbError(resp, err)

	resp, err = ssdbc.Do("zsize", key)
	lwutil.CheckSsdbError(resp, err)
	likeNum, err := strconv.Atoi(resp[1])
	lwutil.CheckError(err, "err_strconv")

	matchLikeKey := makeHMatchExtraSubkey(in.MatchId, MATCH_EXTRA_LIKE_NUM)
	resp, err = ssdbc.Do("hset", H_MATCH_EXTRA, matchLikeKey, likeNum)

	//match play
	play, err := getMatchPlay(ssdbc, in.MatchId, session.Userid)
	lwutil.CheckError(err, "err_get_match_play")
	play.Like = true
	saveMatchPlay(ssdbc, in.MatchId, session.Userid, play)

	//activity
	text := "❤️喜欢了这组拼图"
	addMatchActivity(ssdbc, in.MatchId, session.Userid, text)

	//out
	lwutil.WriteResponse(w, in)

	//battle
	if session.Username == BATTLE_PACK_USER {
		match := getMatch(ssdbc, in.MatchId)

		rc := redisPool.Get()
		defer rc.Close()

		_, err = rc.Do("SADD", BATTLE_PACKID_SET, match.PackId)
		lwutil.CheckError(err, "")
	}
}

func apiMatchUnlike(w http.ResponseWriter, r *http.Request) {
	lwutil.CheckMathod(r, "POST")
	var err error

	//ssdb
	ssdbc, err := ssdbPool.Get()
	lwutil.CheckError(err, "")
	defer ssdbc.Close()

	//session
	session, err := findSession(w, r, nil)
	lwutil.CheckError(err, "err_auth")

	//in
	var in struct {
		MatchId int64
	}
	err = lwutil.DecodeRequestBody(r, &in)
	lwutil.CheckError(err, "err_decode_body")

	//check match exist
	match := getMatch(ssdbc, in.MatchId)
	if match.OwnerId == session.Userid {
		lwutil.SendError("err_owner", "")
	}

	//zset
	key := makeZLikeMatchKey(session.Userid)
	resp, err := ssdbc.Do("zdel", key, in.MatchId)
	lwutil.CheckSsdbError(resp, err)

	//queue
	key = makeQLikeMatchKey(session.Userid)
	err = ssdbc.QDel(key, fmt.Sprintf("%d", in.MatchId), Q_MATCH_DEL_LIMIT, true)
	lwutil.CheckError(err, "err_ssdb_qdel")

	//update matchLike
	score := lwutil.GetRedisTimeUnix()
	key = makeZMatchLikerKey(in.MatchId)
	resp, err = ssdbc.Do("zset", key, session.Userid, score)
	lwutil.CheckSsdbError(resp, err)

	resp, err = ssdbc.Do("zsize", key)
	lwutil.CheckSsdbError(resp, err)
	likeNum, err := strconv.Atoi(resp[1])
	lwutil.CheckError(err, "err_strconv")

	matchLikeKey := makeHMatchExtraSubkey(in.MatchId, MATCH_EXTRA_LIKE_NUM)
	resp, err = ssdbc.Do("hset", H_MATCH_EXTRA, matchLikeKey, likeNum)

	//match play
	play, err := getMatchPlay(ssdbc, in.MatchId, session.Userid)
	lwutil.CheckError(err, "err_get_match_play")
	play.Like = false
	saveMatchPlay(ssdbc, in.MatchId, session.Userid, play)

	lwutil.WriteResponse(w, in)

	//battle
	if session.Username == BATTLE_PACK_USER {
		match := getMatch(ssdbc, in.MatchId)

		rc := redisPool.Get()
		defer rc.Close()

		_, err = rc.Do("SREM", BATTLE_PACKID_SET, match.PackId)
		lwutil.CheckError(err, "")
	}
}

func apiMatchWebGet(w http.ResponseWriter, r *http.Request) {
	lwutil.CheckMathod(r, "POST")
	var err error

	//ssdb
	ssdbc, err := ssdbPool.Get()
	lwutil.CheckError(err, "")
	defer ssdbc.Close()

	//session
	session, err := findSession(w, r, nil)
	lwutil.CheckError(err, "err_auth")

	//in
	var in struct {
		MatchId int64
	}
	err = lwutil.DecodeRequestBody(r, &in)
	lwutil.CheckError(err, "err_decode_body")

	//get match
	match := getMatch(ssdbc, in.MatchId)
	pack, _ := getPack(ssdbc, match.PackId)

	//get playerInfo
	player, err := getPlayerInfo(ssdbc, match.OwnerId)
	lwutil.CheckError(err, "err_player_info")

	//gen image size
	needSave := false
	for i := range pack.Images {
		image := pack.Images[i]
		if image.W != 0 {
			continue
		}
		url := image.Url
		if len(url) == 0 {
			url = fmt.Sprintf("%s/%s", QINIU_USERUPLOAD_URL, image.Key)
		}
		_, size, err := fastimage.DetectImageType(url)
		lwutil.CheckError(err, "err_fastimage")
		pack.Images[i].W = int(size.Width)
		pack.Images[i].H = int(size.Height)
		needSave = true
	}
	if needSave {
		savePack(ssdbc, pack)
	}

	//PlayerMatchInfo
	matchPlay, err := getMatchPlay(ssdbc, in.MatchId, session.Userid)
	lwutil.CheckError(err, "err_get_match_play")
	playerMatchInfo := makePlayerMatchInfo(matchPlay)

	//out
	out := struct {
		Match           *Match
		Pack            *Pack
		Player          *PlayerInfo
		PlayerMatchInfo *PlayerMatchInfo
	}{
		match,
		pack,
		player,
		playerMatchInfo,
	}

	//out
	lwutil.WriteResponse(w, out)
}

func apiMatchTestSize(w http.ResponseWriter, r *http.Request) {
	lwutil.CheckMathod(r, "POST")
	var err error

	//in
	var in struct {
		Url string
	}
	err = lwutil.DecodeRequestBody(r, &in)
	lwutil.CheckError(err, "err_decode_body")

	_, size, err := fastimage.DetectImageType(in.Url)
	lwutil.CheckError(err, "err_fastimage")

	//
	out := struct {
		W int
		H int
	}{
		int(size.Width),
		int(size.Height),
	}

	lwutil.WriteResponse(w, out)
}

func regMatch() {
	http.Handle("/match/new", lwutil.ReqHandler(apiMatchNew))
	http.Handle("/match/del", lwutil.ReqHandler(apiMatchDel))
	http.Handle("/match/mod", lwutil.ReqHandler(apiMatchMod))
	http.Handle("/match/get", lwutil.ReqHandler(apiMatchGet))

	http.Handle("/match/list", lwutil.ReqHandler(apiMatchList))
	http.Handle("/match/listMine", lwutil.ReqHandler(apiMatchListMine))
	http.Handle("/match/listMyPlayed", lwutil.ReqHandler(apiMatchListMyPlayed))
	http.Handle("/match/listHot", lwutil.ReqHandler(apiMatchListHot))
	http.Handle("/match/listLike", lwutil.ReqHandler(apiMatchListLike))
	http.Handle("/match/listOriginal", lwutil.ReqHandler(apiMatchListOriginal))
	http.Handle("/match/listPlayedMatch", lwutil.ReqHandler(apiMatchListPlayedMatch))
	http.Handle("/match/listPlayedAll", lwutil.ReqHandler(apiMatchListPlayedAll))

	http.Handle("/match/playBegin", lwutil.ReqHandler(apiMatchPlayBegin))
	http.Handle("/match/playEnd", lwutil.ReqHandler(apiMatchPlayEnd))
	http.Handle("/match/freePlay", lwutil.ReqHandler(apiMatchFreePlay))
	http.Handle("/match/listActivity", lwutil.ReqHandler(apiMatchListActivity))

	http.Handle("/match/getDynamicData", lwutil.ReqHandler(apiMatchGetDynamicData))
	http.Handle("/match/getRanks", lwutil.ReqHandler(apiMatchGetRanks))

	http.Handle("/match/report", lwutil.ReqHandler(apiMatchReport))

	http.Handle("/match/like", lwutil.ReqHandler(apiMatchLike))
	http.Handle("/match/unlike", lwutil.ReqHandler(apiMatchUnlike))

	http.Handle("/match/web/listUser", lwutil.ReqHandler(apiMatchListUserWeb))
	http.Handle("/match/web/listUserQ", lwutil.ReqHandler(apiMatchListUserWebQ))

	http.Handle("/match/web/get", lwutil.ReqHandler(apiMatchWebGet))

	http.Handle("/test/size", lwutil.ReqHandler(apiMatchTestSize))
}
