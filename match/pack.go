package main

import (
	"./ssdb"
	"encoding/json"
	"fmt"
	"github.com/golang/glog"
	"github.com/henyouqian/lwutil"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	ADMIN_USERID    = uint64(0)
	H_PACK          = "H_PACK"          //key:packId, value:packData
	Z_USER_PACK_PRE = "Z_USER_PACK_PRE" //name:Z_USER_PACK_PRE/userId, key:packid, score:packid
	Z_TAG_PRE       = "Z_TAG_PRE"       //name:Z_TAG_PRE/tag, key:packid, score:packid
	Z_COMMENT       = "Z_COMMENT"       //name:Z_COMMENT/packid, key:commentId, score:commentId
	H_COMMENT       = "H_COMMENT"       //key:commentId, value:commentData
)

func makeZCommentName(packId int64) (name string) {
	return fmt.Sprintf("%s/%d", Z_COMMENT, packId)
}

type Image struct {
	File  string
	Key   string
	Title string
	Text  string
}

type Pack struct {
	Id        int64
	AuthorId  int64
	Time      string
	TimeUnix  int64
	Title     string
	Text      string
	Thumb     string
	Cover     string
	CoverBlur string
	Images    []Image
	Tags      []string
	EventIds  map[string]bool
}

func getPack(ssdb *ssdb.Client, packId int64) *Pack {
	resp, err := ssdb.Do("hget", H_PACK, packId)
	lwutil.CheckSsdbError(resp, err)

	pack := Pack{}
	err = json.Unmarshal([]byte(resp[1]), &pack)
	lwutil.CheckError(err, "")
	return &pack
}

func savePack(ssdb *ssdb.Client, pack *Pack) {
	js, err := json.Marshal(pack)
	lwutil.CheckError(err, "")
	resp, err := ssdb.Do("hset", H_PACK, pack.Id, js)
	lwutil.CheckSsdbError(resp, err)
}

func init() {
	glog.Info("init")
}

func makeTagName(tag string) (outName string) {
	s := Z_TAG_PRE + "/" + tag
	return strings.ToLower(s)
}

func apiNewPack(w http.ResponseWriter, r *http.Request) {
	var err error
	lwutil.CheckMathod(r, "POST")

	//session
	session, err := findSession(w, r, nil)
	lwutil.CheckError(err, "err_auth")

	//in
	var pack Pack
	err = lwutil.DecodeRequestBody(r, &pack)
	lwutil.CheckError(err, "err_decode_body")
	if isAdmin(session.Username) {
		pack.AuthorId = 0
	} else {
		pack.AuthorId = session.Userid
	}
	pack.EventIds = make(map[string]bool)

	now := time.Now()
	pack.Time = now.Format(time.RFC3339)
	pack.TimeUnix = now.Unix()
	if len(pack.Tags) > 8 {
		pack.Tags = pack.Tags[:8]
	}
	if pack.Tags == nil {
		pack.Tags = make([]string, 0)
	}

	//ssdb
	ssdb, err := ssdbPool.Get()
	lwutil.CheckError(err, "")
	defer ssdb.Close()

	//check pack exist if provide packId
	if pack.Id != 0 {
		resp, err := ssdb.Do("hexists", H_PACK, pack.Id)
		lwutil.CheckSsdbError(resp, err)
		if resp[1] == "1" {
			lwutil.SendError("err_exist", "pack already exist")
		}

		resp, err = ssdb.Do("hget", H_SERIAL, "userPack")
		lwutil.CheckSsdbError(resp, err)
		maxId, err := strconv.ParseInt(resp[1], 10, 64)
		lwutil.CheckError(err, "")
		if pack.Id > maxId {
			lwutil.SendError("err_packid", "packId > maxId, del or mod pack id from pack.js")
		}
	} else {
		//gen packid
		resp, err := ssdb.Do("hincr", H_SERIAL, "userPack", 1)
		lwutil.CheckSsdbError(resp, err)
		pack.Id, _ = strconv.ParseInt(resp[1], 10, 32)
	}

	//add to hash
	jsPack, _ := json.Marshal(&pack)
	resp, err := ssdb.Do("hset", H_PACK, pack.Id, jsPack)
	lwutil.CheckSsdbError(resp, err)

	//add to user pack zset
	name := fmt.Sprintf("%s/%d", Z_USER_PACK_PRE, pack.AuthorId)
	resp, err = ssdb.Do("zset", name, pack.Id, pack.Id)
	lwutil.CheckSsdbError(resp, err)

	//tags
	for _, v := range pack.Tags {
		tagName := makeTagName(v)
		resp, err = ssdb.Do("zset", tagName, pack.Id, pack.Id)
		lwutil.CheckSsdbError(resp, err)
	}

	//out
	lwutil.WriteResponse(w, pack)
}

func apiModPack(w http.ResponseWriter, r *http.Request) {
	var err error
	lwutil.CheckMathod(r, "POST")

	//session
	session, err := findSession(w, r, nil)
	lwutil.CheckError(err, "err_auth")

	//in
	var pack Pack
	err = lwutil.DecodeRequestBody(r, &pack)
	lwutil.CheckError(err, "err_decode_body")
	now := time.Now()
	pack.Time = now.Format(time.RFC3339)
	pack.TimeUnix = now.Unix()
	if len(pack.Tags) > 8 {
		pack.Tags = pack.Tags[:8]
	}

	//ssdb
	ssdb, err := ssdbPool.Get()
	lwutil.CheckError(err, "")
	defer ssdb.Close()

	//check owner
	if isAdmin(session.Username) {
		pack.AuthorId = 0
	} else {
		pack.AuthorId = session.Userid
	}
	name := fmt.Sprintf("%s/%d", Z_USER_PACK_PRE, pack.AuthorId)
	resp, err := ssdb.Do("zexists", name, pack.Id)
	lwutil.CheckSsdbError(resp, err)
	if resp[1] == "0" {
		lwutil.SendError("err_not_exist", fmt.Sprintf("pack not exist or not own the pack: userId=%d, packId=%d", session.Userid, pack.Id))
	}

	//tags
	///del from old tags list
	resp, err = ssdb.Do("hget", H_PACK, pack.Id)
	lwutil.CheckSsdbError(resp, err)
	var oldPack Pack
	err = json.Unmarshal([]byte(resp[1]), &oldPack)
	lwutil.CheckError(err, "")

	for _, v := range oldPack.Tags {
		resp, err = ssdb.Do("zdel", makeTagName(v), pack.Id)
		lwutil.CheckSsdbError(resp, err)
	}

	///add back
	for _, v := range pack.Tags {
		tagName := makeTagName(v)
		resp, err = ssdb.Do("zset", tagName, pack.Id, pack.Id)
		lwutil.CheckSsdbError(resp, err)
	}

	//lock images
	pack.Images = oldPack.Images

	//add to hash
	jsPack, _ := json.Marshal(&pack)
	resp, err = ssdb.Do("hset", H_PACK, pack.Id, jsPack)
	lwutil.CheckSsdbError(resp, err)

	//add to user pack zset
	resp, err = ssdb.Do("zset", name, pack.Id, pack.Id)
	lwutil.CheckSsdbError(resp, err)

	//update event which use this pack
	pack.EventIds = oldPack.EventIds
	for eventIdStr, _ := range pack.EventIds {
		eventId, err := strconv.ParseInt(eventIdStr, 10, 64)
		lwutil.CheckError(err, "")
		event := getEvent(ssdb, eventId)
		event.Thumb = pack.Thumb
		event.PackTimeUnix = pack.TimeUnix
		saveEvent(ssdb, event)
	}

	//out
	lwutil.WriteResponse(w, pack)
}

func apiDelPack(w http.ResponseWriter, r *http.Request) {
	var err error
	lwutil.CheckMathod(r, "POST")

	//session
	session, err := findSession(w, r, nil)
	lwutil.CheckError(err, "err_auth")

	//in
	var in struct {
		Id    uint64
		Force bool
	}
	err = lwutil.DecodeRequestBody(r, &in)
	lwutil.CheckError(err, "err_decode_body")

	//check force
	if in.Force == false {
		lwutil.SendError("err_force", "This operation is dangerous. Use <Force> param")
	}

	//ssdb
	ssdb, err := ssdbPool.Get()
	lwutil.CheckError(err, "")
	defer ssdb.Close()

	//check owner
	name := fmt.Sprintf("%s/%d", Z_USER_PACK_PRE, session.Userid)
	resp, err := ssdb.Do("zexists", name, in.Id)
	lwutil.CheckSsdbError(resp, err)
	if resp[1] == "0" {
		lwutil.SendError("err_not_exist", fmt.Sprintf("not own the pack: userId=%d, packId=%d", session.Userid, in.Id))
	}

	//del from tags list
	resp, err = ssdb.Do("hget", H_PACK, in.Id)
	lwutil.CheckSsdbError(resp, err)
	var pack Pack
	err = json.Unmarshal([]byte(resp[1]), &pack)
	lwutil.CheckError(err, "")

	for _, v := range pack.Tags {
		resp, err = ssdb.Do("zdel", makeTagName(v), in.Id)
		lwutil.CheckSsdbError(resp, err)
	}

	//del from owner list
	resp, err = ssdb.Do("zdel", name, in.Id)
	lwutil.CheckSsdbError(resp, err)

	//del pack data
	resp, err = ssdb.Do("hdel", H_PACK, in.Id)
	lwutil.CheckSsdbError(resp, err)

	//out
	lwutil.WriteResponse(w, in)
}

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
		if packs[i].Tags == nil {
			packs[i].Tags = make([]string, 0)
		}
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
		if packs[i].Tags == nil {
			packs[i].Tags = make([]string, 0)
		}
	}

	//out
	lwutil.WriteResponse(w, &packs)
}

func apiListPackByTag(w http.ResponseWriter, r *http.Request) {
	var err error
	lwutil.CheckMathod(r, "POST")

	//in
	var in struct {
		Tag     string
		StartId uint32
		Limit   uint32
	}
	err = lwutil.DecodeRequestBody(r, &in)
	lwutil.CheckError(err, "err_decode_body")

	//ssdb
	ssdb, err := ssdbPool.Get()
	lwutil.CheckError(err, "")
	defer ssdb.Close()

	//get keys
	name := makeTagName(in.Tag)
	var start interface{}
	if in.StartId == 0 {
		start = ""
	} else {
		start = in.StartId
	}

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
		if packs[i].Tags == nil {
			packs[i].Tags = make([]string, 0)
		}
	}

	//out
	lwutil.WriteResponse(w, &packs)
}

func apiGetPack(w http.ResponseWriter, r *http.Request) {
	var err error
	lwutil.CheckMathod(r, "POST")

	//in
	var in struct {
		Id uint64
	}
	err = lwutil.DecodeRequestBody(r, &in)
	lwutil.CheckError(err, "err_decode_body")

	//ssdb
	ssdb, err := ssdbPool.Get()
	lwutil.CheckError(err, "")
	defer ssdb.Close()

	//get packs
	resp, err := ssdb.Do("hget", H_PACK, in.Id)
	lwutil.CheckSsdbErrorDesc(resp, err, fmt.Sprintf("id:%d", in.Id))

	var pack Pack
	err = json.Unmarshal([]byte(resp[1]), &pack)
	lwutil.CheckError(err, "")
	if pack.Tags == nil {
		pack.Tags = make([]string, 0)
	}

	//out
	lwutil.WriteResponse(w, &pack)
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
	stringLimit(&in.Text, 200)

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
	if in.Limit > 20 {
		in.Limit = 20
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

	// 	comments := []Comment{
	// 		Comment{45323, 1, 1, "Ezra", "3", "北京", "The quick brown fox jumps over the lazy dog.The quick brown fox jumps over the lazy dog.The quick brown fox jumps over the lazy dog.The quick brown fox jumps over the lazy dog.The quick brown fox jumps over the lazy dog.The quick brown fox jumps over the lazy dog.The quick brown fox jumps over the lazy dog.The quick brown fox jumps over the lazy dog.The quick brown fox jumps over the lazy dog.The quick brown fox jumps over the lazy dog.The quick brown fox jumps over the lazy dog.The quick brown fox jumps over the lazy dog.The quick brown fox jumps over the lazy dog.The quick brown fox jumps over the lazy dog.The quick brown fox jumps over the lazy dog.The quick brown fox jumps over the lazy dog.\n\nThe quick brown fox jumps over the lazy dog.The quick brown fox jumps over the lazy dog.The quick brown fox jumps over the lazy dog.The quick brown fox jumps over the lazy dog.The quick brown fox jumps over the lazy dog."},
	// 		Comment{5424, 2, 1, "brian.clear", "4", "美国", `label.font = [UIFont fontWithName:@"HelveticaNeue-Light" size:17];`},
	// 		Comment{2452, 44, 1, "Alladinian", "6", "上海", "Try this!"},
	// 		Comment{4562435, 345, 1, "Proveme007", "7", "福建", "gogogo!"},
	// 		Comment{23452, 743, 1, "samfisher", "8", "山东", `implement scrollViewDidScroll: and check contentOffset in that for reaching the end`},
	// 		Comment{233452, 7523, 1, "很有钱", "9", "浙江", `赞同第一名的答案。痛经这个事情，每个人先天体质不一样没办法，但是其实很大一部分是坏的作息饮食习惯，和身体虚弱导致的。最管用的根治方法就是规律作息，多运动。我原来有朋友超级痛，走在路上会忽然痛到回不了家那种。后来去了军队，每天熄灯早起，天天搞体能，随意武装越野五公里，扳手腕偶尔能赢我。那会一点都不疼，经期生龙活虎嗷嗷叫。后来去文职机关了，老毛病又回来了。
	// 吃上面尽量少吃凉的，西瓜山竹什么的注意控制。能早睡就早睡。慢慢养成规律运动的习惯。坚持下来会显著改善的。那些贴的吃的涂得中药西药都不治本。止疼针止疼片实在没办法可以用，但是到了那一步了就真心要注意了。
	// 正能量的总结：多运动多早睡，生活更美好。实际观察爱运动身体好的女生痛的程度和概率远远小于水瓶盖扭不开八百米走完的女生。

	// —————————真答案分割线————————
	// 大家都知道运动有效，可是大多数时候是做不到的。谁愿意天天被人催着去坚持锻炼，去早睡，这也不吃那也不吃。能不能做到其实看女生自己，男朋友能起到作用有限。。。反正我能力不足，潜移默化常年失败，这事真那么容易那也不会有那么多整天分享各种郑多燕啦腹肌撕裂者啦实际从来不会认真去坚持的人啦。
	// 所以呢，碰见女朋友痛呢，悄悄叹口气，安静陪着，要热水给热水要荷包蛋做荷包蛋想吃啥去买啥要帮揉就揉不要就乖乖呆着不要烦她，

	// 然后努力忍住不要借机教育要规律生活多运动（更不能嘲笑说平时不努力现在徒伤悲！切记切记！）就好啦～`},
	// 	}

	//out
	lwutil.WriteResponse(w, &comments)
}

func regPack() {
	http.Handle("/pack/new", lwutil.ReqHandler(apiNewPack))
	http.Handle("/pack/mod", lwutil.ReqHandler(apiModPack))
	http.Handle("/pack/del", lwutil.ReqHandler(apiDelPack))
	http.Handle("/pack/list", lwutil.ReqHandler(apiListPack))
	http.Handle("/pack/listMatch", lwutil.ReqHandler(apiListMatchPack))
	http.Handle("/pack/listByTag", lwutil.ReqHandler(apiListPackByTag))
	http.Handle("/pack/get", lwutil.ReqHandler(apiGetPack))
	http.Handle("/pack/addComment", lwutil.ReqHandler(apiAddComment))
	http.Handle("/pack/getComments", lwutil.ReqHandler(apiGetComments))
}
