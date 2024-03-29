package main

import (
	"./ssdb"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"time"

	"github.com/golang/glog"
	"github.com/henyouqian/lwutil"
	qiniurs "github.com/qiniu/api/rs"
)

const (
	ADMIN_USERID    = uint64(0)
	H_PACK          = "H_PACK"          //subkey:packId value:packJson
	Z_USER_PACK_PRE = "Z_USER_PACK_PRE" //name:Z_USER_PACK_PRE/userId, key:packid, score:packid
	Z_COMMENT       = "Z_COMMENT"       //name:Z_COMMENT/packid, key:commentId, score:commentId
	H_COMMENT       = "H_COMMENT"       //key:commentId, value:commentData
)

func makeZCommentName(packId int64) (name string) {
	return fmt.Sprintf("%s/%d", Z_COMMENT, packId)
}

type Image struct {
	Key string
	Url string
	W   int
	H   int
}

type Pack struct {
	Id        int64
	MatchId   int64
	AuthorId  int64
	Time      string
	TimeUnix  int64
	Title     string
	Text      string
	Thumb     string
	Cover     string
	CoverBlur string
	Images    []Image
	Thumbs    []string
	SizeMb    float32
}

const (
	PACK_THUMB    = "Thumb"
	PACK_TIMEUNIX = "TimeUnix"
)

// func getPack(ssdb *ssdb.Client, packId int64) *Pack {
// 	resp, err := ssdb.Do("hget", H_PACK, packId)
// 	lwutil.CheckSsdbError(resp, err)

// 	pack := Pack{}
// 	err = json.Unmarshal([]byte(resp[1]), &pack)
// 	lwutil.CheckError(err, "")
// 	return &pack
// }

// func savePack(ssdb *ssdb.Client, pack *Pack) {
// 	js, err := json.Marshal(pack)
// 	lwutil.CheckError(err, "")
// 	resp, err := ssdb.Do("hset", H_PACK, pack.Id, js)
// 	lwutil.CheckSsdbError(resp, err)
// }

func getPack(ssdbc *ssdb.Client, packId int64) (*Pack, error) {
	var pack Pack
	resp, err := ssdbc.Do("hget", H_PACK, packId)
	if err != nil {
		return &pack, err
	}
	if len(resp) < 2 {
		return &pack, fmt.Errorf("not_found:packId=%d", packId)
	}

	err = json.Unmarshal([]byte(resp[1]), &pack)
	lwutil.CheckError(err, "")

	//pack url
	for i, image := range pack.Images {
		// if len(image.Url) > 0 {
		// 	pack.Images[i].Url = makeImagePrivateUrl(image.Key)
		// } else {
		// 	break
		// }
		pack.Images[i].Url = makeImagePrivateUrl(image.Key)
	}

	return &pack, err
}

func savePack(ssdbc *ssdb.Client, pack *Pack) error {
	js, err := json.Marshal(pack)
	if err != nil {
		return err
	}
	_, err = ssdbc.Do("hset", H_PACK, pack.Id, js)
	return err
}

func init() {
	glog.Info("init")
}

func newPack(ssdbc *ssdb.Client, pack *Pack, authorId int64, matchId int64) {
	if len(pack.Images) < 3 {
		lwutil.SendError("err_images", "len(pack.Images) < 3")
	}

	pack.MatchId = matchId
	pack.AuthorId = authorId

	now := time.Now()
	pack.Time = now.Format(time.RFC3339)
	pack.TimeUnix = now.Unix()

	//gen packid
	resp, err := ssdbc.Do("hincr", H_SERIAL, "userPack", 1)
	lwutil.CheckSsdbError(resp, err)
	pack.Id, err = strconv.ParseInt(resp[1], 10, 64)
	lwutil.CheckError(err, "")

	//add to hash
	jsPack, _ := json.Marshal(&pack)
	resp, err = ssdbc.Do("hset", H_PACK, pack.Id, jsPack)
	lwutil.CheckSsdbError(resp, err)
}

// func apiNewPack(w http.ResponseWriter, r *http.Request) {
// 	var err error
// 	lwutil.CheckMathod(r, "POST")

// 	//ssdb
// 	ssdb, err := ssdbPool.Get()
// 	lwutil.CheckError(err, "")
// 	defer ssdb.Close()

// 	//session
// 	session, err := findSession(w, r, nil)
// 	lwutil.CheckError(err, "err_auth")

// 	//in
// 	var pack Pack
// 	err = lwutil.DecodeRequestBody(r, &pack)
// 	lwutil.CheckError(err, "err_decode_body")
// 	authorId := int64(0)
// 	if isAdmin(session.Username) {
// 		authorId = 0
// 	} else {
// 		authorId = session.Userid
// 	}

// 	newPack(ssdb, &pack, authorId)

// 	//out
// 	lwutil.WriteResponse(w, pack)
// }

func apiListPack(w http.ResponseWriter, r *http.Request) {
	var err error
	lwutil.CheckMathod(r, "POST")

	//in
	var in struct {
		UserId  uint64
		StartId uint32
		Limit   uint32
	}
	err = lwutil.DecodeRequestBody(r, &in)
	lwutil.CheckError(err, "err_decode_body")
	if in.UserId == 0 {
		in.UserId = ADMIN_USERID
	}
	if in.Limit > 60 {
		in.Limit = 60
	}

	//ssdb
	ssdb, err := ssdbPool.Get()
	lwutil.CheckError(err, "")
	defer ssdb.Close()

	//get keys
	name := fmt.Sprintf("%s/%d", Z_USER_PACK_PRE, in.UserId)
	resp, err := ssdb.Do("zkeys", name, in.StartId, in.StartId, "", in.Limit)
	lwutil.CheckSsdbError(resp, err)

	if len(resp) == 1 {
		lwutil.SendError("err_not_found", "")
	}

	//get packs
	args := make([]interface{}, len(resp)+1)
	args[0] = "multi_hget"
	args[1] = H_PACK
	for i, _ := range args {
		if i >= 2 {
			args[i] = resp[i-1]
		}
	}
	resp, err = ssdb.Do(args...)
	lwutil.CheckSsdbError(resp, err)
	resp = resp[1:]

	packs := make([]Pack, len(resp)/2)
	for i, _ := range packs {
		packjs := resp[i*2+1]
		err = json.Unmarshal([]byte(packjs), &packs[i])
		lwutil.CheckError(err, "")
	}

	//out
	lwutil.WriteResponse(w, &packs)
}

func apiListMatchPack(w http.ResponseWriter, r *http.Request) {
	var err error
	lwutil.CheckMathod(r, "POST")

	//in
	var in struct {
		StartId uint32
		Limit   uint32
	}
	err = lwutil.DecodeRequestBody(r, &in)
	lwutil.CheckError(err, "err_decode_body")
	if in.Limit > 60 {
		in.Limit = 60
	}

	var start interface{}
	if in.StartId == 0 {
		start = ""
	} else {
		start = in.StartId
	}

	//ssdb
	ssdb, err := ssdbPool.Get()
	lwutil.CheckError(err, "")
	defer ssdb.Close()

	//get keys
	name := fmt.Sprintf("%s/%d", Z_USER_PACK_PRE, ADMIN_USERID)

	resp, err := ssdb.Do("zrscan", name, start, start, "", in.Limit)
	lwutil.CheckSsdbError(resp, err)
	if len(resp) == 1 {
		lwutil.SendError("err_not_found", fmt.Sprintf("in:%+v", in))
	}

	//get packs
	resp = resp[1:]
	packNum := (len(resp)) / 2
	args := make([]interface{}, packNum+2)
	args[0] = "multi_hget"
	args[1] = H_PACK
	for i, _ := range args {
		if i >= 2 {
			args[i] = resp[(i-2)*2]
		}
	}
	resp, err = ssdb.Do(args...)
	lwutil.CheckSsdbError(resp, err)
	resp = resp[1:]

	packs := make([]Pack, len(resp)/2)
	for i, _ := range packs {
		packjs := resp[i*2+1]
		err = json.Unmarshal([]byte(packjs), &packs[i])
		lwutil.CheckError(err, "")
	}

	//out
	lwutil.WriteResponse(w, &packs)
}

func makeImagePrivateUrl(key string) string {
	baseUrl := qiniurs.MakeBaseUrl(QINIU_PRIVATE_DOMAIN, key)
	policy := qiniurs.GetPolicy{}
	return policy.MakeRequest(baseUrl, nil)
}

func apiGetPack(w http.ResponseWriter, r *http.Request) {
	var err error
	lwutil.CheckMathod(r, "POST")

	//in
	var in struct {
		Id int64
	}
	err = lwutil.DecodeRequestBody(r, &in)
	lwutil.CheckError(err, "err_decode_body")

	//ssdb
	ssdb, err := ssdbPool.Get()
	lwutil.CheckError(err, "")
	defer ssdb.Close()

	pack, err := getPack(ssdb, in.Id)
	lwutil.CheckError(err, "err_get_pack")

	//get
	player, err := getPlayerInfo(ssdb, pack.AuthorId)
	lwutil.CheckError(err, "err_player")

	followed := false
	session, _ := findSession(w, r, nil)
	if session != nil {
		followed, err = isFollowed(ssdb, session.Userid, player.UserId)
		lwutil.CheckError(err, "err_follow")
	}

	type PlayerInfoPlus struct {
		*PlayerInfo
		Followed bool
	}
	author := PlayerInfoPlus{
		player,
		followed,
	}

	//pack url
	for i, image := range pack.Images {
		if len(image.Url) > 0 {
			pack.Images[i].Url = makeImagePrivateUrl(image.Key)
		} else {
			break
		}
	}

	out := struct {
		*Pack
		Author PlayerInfoPlus
	}{
		pack,
		author,
	}

	//out
	lwutil.WriteResponse(w, &out)
}

type Comment struct {
	Id              int64
	PackId          int64
	UserId          int64
	UserName        string
	GravatarKey     string
	CustomAvatarKey string
	Team            string
	Text            string
}

func apiAddComment(w http.ResponseWriter, r *http.Request) {
	var err error
	lwutil.CheckMathod(r, "POST")

	//in
	var in Comment
	err = lwutil.DecodeRequestBody(r, &in)
	lwutil.CheckError(err, "err_decode_body")

	if len(in.Text) == 0 {
		lwutil.SendError("err_empty_text", "")
	}
	stringLimit(&in.Text, 2000)

	//ssdb
	ssdb, err := ssdbPool.Get()
	lwutil.CheckError(err, "")
	defer ssdb.Close()

	//session
	session, err := findSession(w, r, nil)
	lwutil.CheckError(err, "err_auth")

	//player
	playerInfo, err := getPlayerInfo(ssdb, session.Userid)
	lwutil.CheckError(err, "")

	//check pack
	resp, err := ssdb.Do("hexists", H_PACK, in.PackId)
	lwutil.CheckSsdbError(resp, err)
	if resp[1] == "0" {
		lwutil.SendError("err_not_exist", "pack not exist")
	}

	//override in
	in.Id = GenSerial(ssdb, "comment")
	in.UserId = session.Userid
	in.UserName = playerInfo.NickName
	in.Team = playerInfo.TeamName
	in.GravatarKey = playerInfo.GravatarKey
	in.CustomAvatarKey = playerInfo.CustomAvatarKey

	//save
	zName := makeZCommentName(in.PackId)
	resp, err = ssdb.Do("zset", zName, in.Id, in.Id)
	lwutil.CheckSsdbError(resp, err)

	jsComment, _ := json.Marshal(in)
	resp, err = ssdb.Do("hset", H_COMMENT, in.Id, jsComment)
	lwutil.CheckSsdbError(resp, err)

	//out
	lwutil.WriteResponse(w, &in)
}

func apiGetComments(w http.ResponseWriter, r *http.Request) {
	var err error
	lwutil.CheckMathod(r, "POST")

	//in
	var in struct {
		PackId          int64
		BottomCommentId int64
		Limit           uint
	}
	err = lwutil.DecodeRequestBody(r, &in)
	lwutil.CheckError(err, "err_decode_body")

	if in.BottomCommentId == 0 {
		in.BottomCommentId = math.MaxInt64
	}
	if in.Limit > 50 {
		in.Limit = 50
	}

	//ssdb
	ssdb, err := ssdbPool.Get()
	lwutil.CheckError(err, "")
	defer ssdb.Close()

	//get zset
	name := makeZCommentName(in.PackId)
	resp, err := ssdb.Do("zrscan", name, in.BottomCommentId, in.BottomCommentId, "", in.Limit)
	lwutil.CheckSsdbError(resp, err)

	if len(resp) == 1 {
		//lwutil.SendError("err_not_found", "")
		comments := make([]Comment, 0)
		lwutil.WriteResponse(w, &comments)
		return
	}
	resp = resp[1:]

	//get comments
	cmds := make([]interface{}, len(resp)/2+2)
	cmds[0] = "multi_hget"
	cmds[1] = H_COMMENT
	for i, _ := range cmds {
		if i >= 2 {
			cmds[i] = resp[(i-2)*2]
		}
	}
	resp, err = ssdb.Do(cmds...)
	lwutil.CheckSsdbError(resp, err)
	resp = resp[1:]

	comments := make([]Comment, len(resp)/2)
	for i, _ := range comments {
		js := resp[i*2+1]
		err = json.Unmarshal([]byte(js), &comments[i])
		lwutil.CheckError(err, "")
	}

	//out
	lwutil.WriteResponse(w, &comments)
}

func regPack() {
	// http.Handle("/pack/new", lwutil.ReqHandler(apiNewPack))
	http.Handle("/pack/list", lwutil.ReqHandler(apiListPack))
	http.Handle("/pack/listMatch", lwutil.ReqHandler(apiListMatchPack))
	http.Handle("/pack/get", lwutil.ReqHandler(apiGetPack))
	http.Handle("/pack/addComment", lwutil.ReqHandler(apiAddComment))
	http.Handle("/pack/getComments", lwutil.ReqHandler(apiGetComments))
}
