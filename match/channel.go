package main

import (
	"./ssdb"
	"encoding/json"
	"fmt"
	"github.com/golang/glog"
	"github.com/henyouqian/lwutil"
	"net/http"
	"strconv"
	"strings"
)

const (
	K_CHANNEL_LIST  = "K_CHANNEL_LIST"  //value:channel name array(json)
	Z_CHANNEL_USER  = "Z_CHANNEL_USER"  //key:Z_CHANNEL_USER/channelName subkey:userId score:time
	H_USER_CHANNEL  = "H_USER_CHANNEL"  //key:H_USER_CHANNEL/userId subkey:channelName value:1
	Z_CHANNEL_MATCH = "Z_CHANNEL_MATCH" //key:Z_CHANNEL_MATCH/channelName subkey:matchId score:time
	H_CHANNEL_THUMB = "H_CHANNEL_THUMB" //subkey:channelName value:thumb
)

func glogChannel() {
	glog.Info("")
}

func makeZChannelUserKey(channelName string) string {
	return fmt.Sprintf("%s/%s", Z_CHANNEL_USER, channelName)
}

func makeHUserChannelKey(userId int64) string {
	return fmt.Sprintf("%s/%s", H_USER_CHANNEL, userId)
}

func makeZChannelMatchKey(channelName string) string {
	return fmt.Sprintf("%s/%s", Z_CHANNEL_MATCH, channelName)
}

type Channel [2]string //[name, thumb]

func apiChannelSet(w http.ResponseWriter, r *http.Request) {
	lwutil.CheckMathod(r, "POST")
	var err error

	//ssdb
	ssdbc, err := ssdbPool.Get()
	lwutil.CheckError(err, "")
	defer ssdbc.Close()

	//session
	session, err := findSession(w, r, nil)
	lwutil.CheckError(err, "err_auth")

	checkAdmin(session)

	//in
	var in struct {
		Channels []Channel
	}
	err = lwutil.DecodeRequestBody(r, &in)
	lwutil.CheckError(err, "err_decode_body")

	//json
	bt, err := json.Marshal(in.Channels)
	lwutil.CheckError(err, "err_json")

	//set
	resp, err := ssdbc.Do("set", K_CHANNEL_LIST, bt)
	lwutil.CheckSsdbError(resp, err)

	//out
	lwutil.WriteResponse(w, in)
}

func apiChannelList(w http.ResponseWriter, r *http.Request) {
	lwutil.CheckMathod(r, "POST")
	var err error

	//ssdb
	ssdbc, err := ssdbPool.Get()
	lwutil.CheckError(err, "")
	defer ssdbc.Close()

	//session
	_, err = findSession(w, r, nil)
	lwutil.CheckError(err, "err_auth")

	//get
	resp, err := ssdbc.Do("get", K_CHANNEL_LIST)
	lwutil.CheckSsdbError(resp, err)

	var channels []Channel
	err = json.Unmarshal([]byte(resp[1]), &channels)

	//out
	out := struct {
		Channels []Channel
		ThumbMap map[string]string //[channelName]thumb
	}{
		channels,
		make(map[string]string),
	}

	//get thumbs
	num := len(out.Channels)
	if num > 0 {
		cmds := make([]interface{}, 2, num+2)
		cmds[0] = "multi_hget"
		cmds[1] = H_CHANNEL_THUMB
		for _, channel := range out.Channels {
			channelName := channel[0]
			cmds = append(cmds, channelName)
		}
		resp, err = ssdbc.Do(cmds...)
		lwutil.CheckSsdbError(resp, err)
		resp = resp[1:]
		num := len(resp) / 2
		for i := 0; i < num; i++ {
			out.ThumbMap[resp[i*2]] = resp[i*2+1]
		}
	}

	lwutil.WriteResponse(w, out)
}

func apiChannelListDetail(w http.ResponseWriter, r *http.Request) {
	lwutil.CheckMathod(r, "POST")
	var err error

	//ssdb
	ssdbc, err := ssdbPool.Get()
	lwutil.CheckError(err, "")
	defer ssdbc.Close()

	//session
	_, err = findSession(w, r, nil)
	lwutil.CheckError(err, "err_auth")

	//get
	resp, err := ssdbc.Do("get", K_CHANNEL_LIST)
	lwutil.CheckSsdbError(resp, err)

	var channels []Channel
	err = json.Unmarshal([]byte(resp[1]), &channels)

	//ChannelDetail

	type ChannelDetail struct {
		ChannelName string
		Players     []*PlayerInfoLite
	}

	//
	channelDetails := make([]ChannelDetail, 0, 10)

	for _, channel := range channels {
		var channelDetail ChannelDetail
		channelName := channel[0]
		channelDetail.ChannelName = channelName
		channelDetail.Players = make([]*PlayerInfoLite, 0, 10)

		key := makeZChannelUserKey(channelName)
		resp, _, _, err := ssdbc.ZScan(key, H_PLAYER_INFO_LITE, "", "", 100, false)
		lwutil.CheckError(err, "err_ssdb")

		//out
		num := len(resp) / 2
		for i := 0; i < num; i++ {
			var player PlayerInfoLite
			err := json.Unmarshal([]byte(resp[i*2+1]), &player)
			lwutil.CheckError(err, "err_json")
			channelDetail.Players = append(channelDetail.Players, &player)
		}
		channelDetails = append(channelDetails, channelDetail)
	}

	//out
	out := struct {
		ChannelDetails []ChannelDetail
	}{
		channelDetails,
	}

	lwutil.WriteResponse(w, out)
}

func apiChannelAddUser(w http.ResponseWriter, r *http.Request) {
	lwutil.CheckMathod(r, "POST")
	var err error

	//ssdb
	ssdbc, err := ssdbPool.Get()
	lwutil.CheckError(err, "")
	defer ssdbc.Close()

	//session
	session, err := findSession(w, r, nil)
	lwutil.CheckError(err, "err_auth")

	checkAdmin(session)

	//in
	var in struct {
		ChannelName string
		UserId      int64
	}
	err = lwutil.DecodeRequestBody(r, &in)
	lwutil.CheckError(err, "err_decode_body")

	//check channel name
	if len(in.ChannelName) == 0 {
		lwutil.SendError("err_channel_name", "")
	}

	//check user
	if !checkPlayerExist(ssdbc, in.UserId) {
		lwutil.SendError("err_user_id", "")
	}

	//zset
	t := lwutil.GetRedisTimeUnix()
	key := makeZChannelUserKey(in.ChannelName)
	resp, err := ssdbc.Do("zset", key, in.UserId, t)
	lwutil.CheckSsdbError(resp, err)

	//hset
	key = makeHUserChannelKey(in.UserId)
	resp, err = ssdbc.Do("hset", key, in.ChannelName, 1)
	lwutil.CheckSsdbError(resp, err)

	//
	channels := listUserChannel(ssdbc, in.UserId)

	//out
	out := struct {
		Channels []string
	}{
		channels,
	}
	lwutil.WriteResponse(w, out)
}

func apiChannelDelUser(w http.ResponseWriter, r *http.Request) {
	lwutil.CheckMathod(r, "POST")
	var err error

	//ssdb
	ssdbc, err := ssdbPool.Get()
	lwutil.CheckError(err, "")
	defer ssdbc.Close()

	//session
	session, err := findSession(w, r, nil)
	lwutil.CheckError(err, "err_auth")

	checkAdmin(session)

	//in
	var in struct {
		ChannelName string
		UserId      int64
	}
	err = lwutil.DecodeRequestBody(r, &in)
	lwutil.CheckError(err, "err_decode_body")

	//del channel user
	key := makeZChannelUserKey(in.ChannelName)
	resp, err := ssdbc.Do("zdel", key, in.UserId)
	lwutil.CheckSsdbError(resp, err)

	//del user channel
	key = makeHUserChannelKey(in.UserId)
	resp, err = ssdbc.Do("hdel", key, in.ChannelName)
	lwutil.CheckSsdbError(resp, err)

	//
	channels := listUserChannel(ssdbc, in.UserId)

	//out
	out := struct {
		Channels []string
	}{
		channels,
	}
	lwutil.WriteResponse(w, out)
}

func apiChannelListUser(w http.ResponseWriter, r *http.Request) {
	lwutil.CheckMathod(r, "POST")
	var err error

	//ssdb
	ssdbc, err := ssdbPool.Get()
	lwutil.CheckError(err, "")
	defer ssdbc.Close()

	//session
	_, err = findSession(w, r, nil)
	lwutil.CheckError(err, "err_auth")

	// checkAdmin(session)

	//in
	var in struct {
		ChannelName string
	}
	err = lwutil.DecodeRequestBody(r, &in)
	lwutil.CheckError(err, "err_decode_body")

	key := makeZChannelUserKey(in.ChannelName)
	// resp, err := ssdbc.Do("zkeys", key, "", "", "", 100)
	// lwutil.CheckSsdbError(resp, err)
	// resp = resp[1:]

	// userIds := make([]int64, 0, 10)
	// for _, v := range resp {
	// 	userId, err := strconv.ParseInt(v, 10, 64)
	// 	lwutil.CheckError(err, "err_strconv")
	// 	userIds = append(userIds, userId)
	// }

	//
	resp, _, _, err := ssdbc.ZScan(key, H_PLAYER_INFO_LITE, "", "", 100, false)

	//out
	num := len(resp) / 2
	out := struct {
		Players []*PlayerInfoLite
	}{
		make([]*PlayerInfoLite, 0, num),
	}
	for i := 0; i < num; i++ {
		var player PlayerInfoLite
		err := json.Unmarshal([]byte(resp[i*2+1]), &player)
		lwutil.CheckError(err, "err_json")
		out.Players = append(out.Players, &player)
	}
	lwutil.WriteResponse(w, out)

	//out
	// out := struct {
	// 	UserIds []int64
	// }{
	// 	userIds,
	// }
	// lwutil.WriteResponse(w, out)
}

func apiChannelListMatch(w http.ResponseWriter, r *http.Request) {
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
		ChannelName string
		Key         string
		Score       string
		Limit       int
	}
	err = lwutil.DecodeRequestBody(r, &in)
	lwutil.CheckError(err, "err_decode_body")

	if in.Limit <= 0 || in.Limit > 30 {
		in.Limit = 30
	}

	key := makeZChannelMatchKey(in.ChannelName)
	resp, lastKey, lastScore, err := ssdbc.ZScan(key, H_MATCH, in.Key, in.Score, in.Limit, true)
	lwutil.CheckError(err, "err_zscan")

	//out
	out := struct {
		Matches        []*Match
		LastKey        string
		LastScore      string
		PlayedMatchMap map[string]*PlayerMatchInfo
		OwnerMap       map[string]*PlayerInfoLite
		MatchExMap     map[string]*MatchExtra
	}{
		make([]*Match, 0, 30),
		lastKey,
		lastScore,
		make(map[string]*PlayerMatchInfo),
		make(map[string]*PlayerInfoLite),
		make(map[string]*MatchExtra),
	}

	num := len(resp) / 2
	for i := 0; i < num; i++ {
		matchJs := resp[i*2+1]
		var match Match
		err := json.Unmarshal([]byte(matchJs), &match)
		lwutil.CheckError(err, "err_json")
		out.Matches = append(out.Matches, &match)
	}

	//playedMap
	num = len(out.Matches)
	cmds := make([]interface{}, 2, num+2)
	cmds[0] = "multi_hget"
	cmds[1] = H_MATCH_PLAY
	ownerIds := make([]int64, 0, num)
	for _, match := range out.Matches {
		subkey := makeMatchPlaySubkey(match.Id, session.Userid)
		cmds = append(cmds, subkey)
		ownerIds = append(ownerIds, match.OwnerId)
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

	//ownerMap
	cmds = make([]interface{}, 2, len(ownerIds)+2)
	cmds[0] = "multi_hget"
	cmds[1] = H_PLAYER_INFO_LITE
	for _, ownerId := range ownerIds {
		cmds = append(cmds, ownerId)
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

	//match extra
	cmds = make([]interface{}, 2, len(out.Matches)*2+2)
	cmds[0] = "multi_hget"
	cmds[1] = H_MATCH_EXTRA
	for _, match := range out.Matches {
		playTimesKey := makeHMatchExtraSubkey(match.Id, MATCH_EXTRA_PLAY_TIMES)
		likeNumKey := makeHMatchExtraSubkey(match.Id, MATCH_EXTRA_LIKE_NUM)
		cmds = append(cmds, playTimesKey, likeNumKey)
	}
	resp, err = ssdbc.Do(cmds...)
	lwutil.CheckSsdbError(resp, err)
	resp = resp[1:]

	num = len(resp) / 2
	for i := 0; i < num; i++ {
		key := resp[i*2]
		var matchId int64
		var fieldKey string
		_, err = fmt.Sscanf(key, "%d/%s", &matchId, &fieldKey)
		lwutil.CheckError(err, fmt.Sprintf("key:%s", key))

		matchIdStr := fmt.Sprint(matchId)
		matchEx := out.MatchExMap[matchIdStr]
		if matchEx == nil {
			matchEx = new(MatchExtra)
		}

		if fieldKey == MATCH_EXTRA_PLAY_TIMES {
			matchEx.PlayTimes, err = strconv.Atoi(resp[i*2+1])
			lwutil.CheckError(err, "")
		} else if fieldKey == MATCH_EXTRA_LIKE_NUM {
			matchEx.LikeNum, err = strconv.Atoi(resp[i*2+1])
			lwutil.CheckError(err, "")
		}
		out.MatchExMap[matchIdStr] = matchEx
	}

	lwutil.WriteResponse(w, out)
}

func listUserChannel(ssdbc *ssdb.Client, userId int64) []string {
	key := makeHUserChannelKey(userId)
	resp, err := ssdbc.Do("hgetall", key)
	lwutil.CheckSsdbError(resp, err)
	resp = resp[1:]

	num := len(resp) / 2
	channels := make([]string, 0, num)
	for i := 0; i < num; i++ {
		channels = append(channels, resp[2*i])
	}
	return channels
}

func apiChannelListUserChannel(w http.ResponseWriter, r *http.Request) {
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
		UserId int64
	}
	err = lwutil.DecodeRequestBody(r, &in)
	lwutil.CheckError(err, "err_decode_body")

	channels := listUserChannel(ssdbc, in.UserId)

	//out
	out := struct {
		Channels []string
	}{
		channels,
	}
	lwutil.WriteResponse(w, out)
}

func regChannel() {
	http.Handle("/channel/set", lwutil.ReqHandler(apiChannelSet))
	http.Handle("/channel/list", lwutil.ReqHandler(apiChannelList))
	http.Handle("/channel/listDetail", lwutil.ReqHandler(apiChannelListDetail))
	http.Handle("/channel/addUser", lwutil.ReqHandler(apiChannelAddUser))
	http.Handle("/channel/delUser", lwutil.ReqHandler(apiChannelDelUser))
	http.Handle("/channel/listUser", lwutil.ReqHandler(apiChannelListUser))
	http.Handle("/channel/listMatch", lwutil.ReqHandler(apiChannelListMatch))
	http.Handle("/channel/listUserChannel", lwutil.ReqHandler(apiChannelListUserChannel))

	// //test
	// ssdbc, err := ssdbPool.Get()
	// lwutil.CheckError(err, "")
	// defer ssdbc.Close()

	// //batch test
	// cmds := [][]interface{}{
	// 	[]interface{}{"set", "a", "gg"},
	// 	[]interface{}{"set", "a", "bbb"},
	// }
	// resp, err := ssdbc.Batch(cmds)
	// glog.Info(resp)

	// resp1, err := ssdbc.Do("get", "a")
	// glog.Info(resp1)

	// key := "a"
	// ssdbc.Do("qclear", key)
	// ssdbc.Do("qpush", key, 1, 2, 3, 4, 3, 5, 6)
	// ssdbc.QDel(key, "3", 3, false)
	// resp, _ := ssdbc.Do("qrange", key, 0, 100)
	// glog.Info(resp)
}
