package main

import (
	"./ssdb"
	"encoding/json"
	"fmt"
	"github.com/golang/glog"
	"github.com/henyouqian/lwutil"
	"net/http"
	"sort"
	"strconv"
)

const (
	H_SOCIAL_PACK      = "H_SOCIAL_PACK"  //subkey:userId/packId/sliderNum value:socialPack
	Z_SOCIAL_PACK      = "Z_SOCIAL_PACK"  //key: Z_SOCIAL_PACK/userId subkey: socialPackKey score:serial
	H_SOCIAL_RANKS     = "H_SOCIAL_RANKS" //key:H_SOCIAL_RANK subkey:matchId value:ranks
	SERIAL_SOCIAL_PACK = "SERIAL_SOCIAL_PACK"
)

type SocialRank struct {
	Name string
	Msec int
}

type ByRank []SocialRank

func (a ByRank) Len() int           { return len(a) }
func (a ByRank) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a ByRank) Less(i, j int) bool { return a[i].Msec < a[j].Msec }

type SocialPack struct {
	UserId    int64
	PackId    int64
	SliderNum int
	PlayTimes int
	IsOwner   bool
	Ranks     []SocialRank
}

func _glogSocial() {
	glog.Info("social")
}

func makeHSocialPackSubKey(userId int64, packId int64, sliderNum int) string {
	key := fmt.Sprintf("%d/%d/%d", userId, packId, sliderNum)
	key = lwutil.Sha224(key)
	return key
}

func makeZSocialPackKey(userId int64) string {
	key := fmt.Sprintf("%s/%d", Z_SOCIAL_PACK, userId)
	return key
}

// func apiSocialNewPack(w http.ResponseWriter, r *http.Request) {
// 	lwutil.CheckMathod(r, "POST")
// 	var err error

// 	//ssdb
// 	ssdbc, err := ssdbPool.Get()
// 	lwutil.CheckError(err, "")
// 	defer ssdbc.Close()

// 	//session
// 	session, err := findSession(w, r, nil)
// 	lwutil.CheckError(err, "err_auth")

// 	//in
// 	var in struct {
// 		PackId    int64
// 		SliderNum int
// 		Msec      int
// 	}
// 	err = lwutil.DecodeRequestBody(r, &in)
// 	lwutil.CheckError(err, "err_decode_body")

// 	if in.SliderNum < 3 || in.SliderNum > 8 {
// 		lwutil.SendError("err_slider_num", "in.SliderNum < 3 || SliderNum > 8")
// 	}

// 	//get player
// 	player, err := getPlayerInfo(ssdbc, session.Userid)
// 	lwutil.CheckError(err, "")

// 	//out
// 	out := struct {
// 		Key string
// 	}{}

// 	//
// 	subkey := makeHSocialPackSubKey(session.Userid, in.PackId, in.SliderNum)

// 	//check exist
// 	resp, err := ssdbc.Do("hget", H_SOCIAL_PACK, subkey)
// 	lwutil.CheckError(err, "")
// 	if resp[0] == ssdb.OK {
// 		var socialPack SocialPack
// 		err = json.Unmarshal([]byte(resp[1]), &socialPack)
// 		lwutil.CheckError(err, "")

// 		if in.Msec > 0 {
// 			if socialPack.Ranks == nil {
// 				socialPack.Ranks = []SocialRank{
// 					{Name: player.NickName, Msec: in.Msec},
// 				}
// 			} else {
// 				found := false
// 				for i, v := range socialPack.Ranks {
// 					if v.Name == player.NickName {
// 						found = true
// 						if in.Msec < v.Msec {
// 							socialPack.Ranks[i].Msec = in.Msec
// 						}
// 						break
// 					}
// 				}
// 				if !found {
// 					socialPack.Ranks = append(socialPack.Ranks, SocialRank{player.NickName, in.Msec})
// 				}

// 				//sort
// 				sort.Sort(ByRank(socialPack.Ranks))

// 				//
// 				if len(socialPack.Ranks) > 10 {
// 					socialPack.Ranks = socialPack.Ranks[:10]
// 				}
// 			}
// 		}

// 		out.Key = subkey
// 		lwutil.WriteResponse(w, out)
// 		return
// 	}

// 	//get pack
// 	pack, err := getPack(ssdbc, in.PackId)
// 	lwutil.CheckError(err, "")

// 	isOwner := pack.AuthorId == session.Userid
// 	if isAdmin(session.Username) && pack.AuthorId == 0 {
// 		isOwner = true
// 	}

// 	ranks := []SocialRank{
// 		{Name: player.NickName, Msec: in.Msec},
// 	}
// 	if in.Msec <= 0 {
// 		ranks = []SocialRank{}
// 	}

// 	socialPack := SocialPack{
// 		UserId:    session.Userid,
// 		PackId:    in.PackId,
// 		SliderNum: in.SliderNum,
// 		PlayTimes: 0,
// 		IsOwner:   isOwner,
// 		Ranks:     ranks,
// 	}

// 	//json
// 	js, err := json.Marshal(socialPack)
// 	lwutil.CheckError(err, "")

// 	//add to hash
// 	resp, err = ssdbc.Do("hset", H_SOCIAL_PACK, subkey, js)
// 	lwutil.CheckSsdbError(resp, err)

// 	// //add to zset
// 	// zkey := makeZSocialPackKey(session.Userid)
// 	// score := GenSerial(ssdbc, SERIAL_SOCIAL_PACK)
// 	// resp, err = ssdbc.Do("zset", zkey, subkey, score)
// 	// lwutil.CheckSsdbError(resp, err)

// 	//out
// 	out.Key = subkey
// 	lwutil.WriteResponse(w, out)
// }

func apiSocialNewPack(w http.ResponseWriter, r *http.Request) {
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
		PackId int64
	}
	err = lwutil.DecodeRequestBody(r, &in)
	lwutil.CheckError(err, "err_decode_body")

	//out
	out := struct {
		Key string
	}{
		fmt.Sprintf("%d", in.PackId),
	}

	lwutil.WriteResponse(w, out)
}

func apiSocialGetPack(w http.ResponseWriter, r *http.Request) {
	lwutil.CheckMathod(r, "POST")
	var err error

	//ssdb
	ssdbc, err := ssdbPool.Get()
	lwutil.CheckError(err, "")
	defer ssdbc.Close()

	//in
	var in struct {
		Key string
	}
	err = lwutil.DecodeRequestBody(r, &in)
	lwutil.CheckError(err, "err_decode_body")

	socialPack := SocialPack{}

	id, err := strconv.ParseInt(in.Key, 10, 64)
	if err != nil {
		resp, err := ssdbc.Do("hget", H_SOCIAL_PACK, in.Key)
		lwutil.CheckSsdbError(resp, err)

		err = json.Unmarshal([]byte(resp[1]), &socialPack)
		lwutil.CheckError(err, "")
	} else {
		resp, err := ssdbc.Do("hget", H_SOCIAL_PACK, id)
		lwutil.CheckError(err, "")
		if resp[0] == ssdb.NOT_FOUND {
			match := getMatch(ssdbc, id)
			ranks := []SocialRank{}

			socialPack = SocialPack{
				UserId:    match.OwnerId,
				PackId:    match.PackId,
				SliderNum: match.SliderNum,
				PlayTimes: 0,
				IsOwner:   true,
				Ranks:     ranks,
			}

			//save
			js, err := json.Marshal(socialPack)
			lwutil.CheckError(err, "")
			resp, err = ssdbc.Do("hset", H_SOCIAL_PACK, id, js)
			lwutil.CheckSsdbError(resp, err)
		} else {
			err = json.Unmarshal([]byte(resp[1]), &socialPack)
			lwutil.CheckError(err, "")
		}
	}

	//get pack
	pack, err := getPack(ssdbc, socialPack.PackId)
	lwutil.CheckError(err, "")

	//out
	out := struct {
		SocialPack
		Pack *Pack
	}{
		socialPack,
		pack,
	}

	lwutil.WriteResponse(w, out)
}

func apiSocialGetMatch(w http.ResponseWriter, r *http.Request) {
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
		Match *Match
		Pack  *Pack
		Ranks []SocialRank
	}{
		match,
		pack,
		[]SocialRank{},
	}

	//out
	lwutil.WriteResponse(w, out)
}

func apiSocialPlay(w http.ResponseWriter, r *http.Request) {
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
		Key      string
		MatchId  int64
		Checksum string
		UserName string
		Msec     int
	}
	err = lwutil.DecodeRequestBody(r, &in)
	lwutil.CheckError(err, "err_decode_body")

	//
	if len(in.Key) > 0 {
		resp, err := ssdbc.Do("hget", H_SOCIAL_PACK, in.Key)
		lwutil.CheckSsdbError(resp, err)

		socialPack := SocialPack{}
		err = json.Unmarshal([]byte(resp[1]), &socialPack)
		lwutil.CheckError(err, "")

		//update score
		found := false
		for i, v := range socialPack.Ranks {
			if v.Name == in.UserName {
				found = true
				if in.Msec < v.Msec {
					socialPack.Ranks[i].Msec = in.Msec
				}
				break
			}
		}
		if !found {
			socialPack.Ranks = append(socialPack.Ranks, SocialRank{in.UserName, in.Msec})
		}

		//sort
		sort.Sort(ByRank(socialPack.Ranks))

		//
		if len(socialPack.Ranks) > 10 {
			socialPack.Ranks = socialPack.Ranks[:10]
		}

		//json
		js, err := json.Marshal(socialPack)
		lwutil.CheckError(err, "")

		//add to hash
		resp, err = ssdbc.Do("hset", H_SOCIAL_PACK, in.Key, js)
		lwutil.CheckSsdbError(resp, err)

		//out
		lwutil.WriteResponse(w, socialPack)
	}

	//match played
	if in.MatchId != 0 {
		play, err := getMatchPlay(ssdbc, in.MatchId, session.Userid)
		lwutil.CheckError(err, "err_get_match_play")
		play.Played = true
		saveMatchPlay(ssdbc, in.MatchId, session.Userid, play)
		playerMatchInfo := makePlayerMatchInfo(play)

		//ranks
		ranks := []SocialRank{}

		resp, err := ssdbc.Do("hget", H_SOCIAL_RANKS, in.MatchId)
		lwutil.CheckError(err, "err_ssdb")
		if resp[0] == SSDB_OK {
			err = json.Unmarshal([]byte(resp[1]), &ranks)
			lwutil.CheckError(err, "")
		}

		//update score
		found := false
		for i, v := range ranks {
			if v.Name == play.PlayerName {
				found = true
				if in.Msec < v.Msec {
					ranks[i].Msec = in.Msec
				}
				break
			}
		}
		if !found {
			ranks = append(ranks, SocialRank{play.PlayerName, in.Msec})
		}

		//sort
		sort.Sort(ByRank(ranks))

		//
		if len(ranks) > 10 {
			ranks = ranks[:10]
		}

		//json
		js, err := json.Marshal(ranks)
		lwutil.CheckError(err, "")

		//add to hash
		resp, err = ssdbc.Do("hset", H_SOCIAL_RANKS, in.MatchId, js)
		lwutil.CheckSsdbError(resp, err)

		//out
		out := struct {
			PlayerMatchInfo *PlayerMatchInfo
			Ranks           []SocialRank
		}{
			playerMatchInfo,
			ranks,
		}
		lwutil.WriteResponse(w, out)
	}

}

func regSocial() {
	http.Handle("/social/newPack", lwutil.ReqHandler(apiSocialNewPack))
	http.Handle("/social/getPack", lwutil.ReqHandler(apiSocialGetPack)) //deprecated
	http.Handle("/social/getMatch", lwutil.ReqHandler(apiSocialGetMatch))
	http.Handle("/social/play", lwutil.ReqHandler(apiSocialPlay))
}
