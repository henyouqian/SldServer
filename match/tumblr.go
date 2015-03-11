package main

import (
	"./ssdb"
	"encoding/json"
	"fmt"
	"math"
	// "math/rand"
	"net/http"
	"strconv"
	"time"

	"github.com/golang/glog"
	"github.com/henyouqian/lwutil"
	qiniurs "github.com/qiniu/api/rs"
)

func glogTumblr() {
	glog.Info("")
}

const (
	H_TUMBLR_BLOG              = "H_TUMBLR_BLOG"              //key:H_TUMBLR_BLOG subkey:blogName
	Z_TUMBLR_BLOG              = "Z_TUMBLR_BLOG"              //key:Z_TUMBLR_BLOG subkey:blogName score:addTime
	H_TUMBLR_IMAGE             = "H_TUMBLR_IMAGE"             //key:H_TUMBLR_IMAGE subkey:imageKey value:TumblrImage Json
	Z_TUMBLR_BLOG_IMAGE        = "Z_TUMBLR_BLOG_IMAGE"        //key:Z_TUMBLR_BLOG_IMAGE/blogName subkey:imageKey score:postId
	H_TUMBLR_BLOG_IMAGE_OFFSET = "H_TUMBLR_BLOG_IMAGE_OFFSET" //key:H_TUMBLR_BLOG_IMAGE_OFFSET subkey:blogName value:offset
	H_TUMBLR_ACCONT            = "H_TUMBLR_ACCONT"            //key:H_TUMBLR_ACCONT subkey:blogName value:userId
	Z_TUMBLR_DELETED_IMAGE     = "Z_TUMBLR_DELETED_IMAGE"     //key:Z_TUMBLR_DELETED_IMAGE/blogName subkey:imageKey score:time
	Z_TUMBLR_UPLOADED_IMAGE    = "Z_TUMBLR_UPLOADED_IMAGE"    //key:Z_TUMBLR_UPLOADED_IMAGE/blogName/p|l subkey:imageKey score:time
	Z_TUMBLR_USER_PUBLISH      = "Z_TUMBLR_USER_PUBLISH"      //key:Z_TUMBLR_USER_PUBLISH/userId subkey:matchId score:addTime
	Z_TUMBLR_AUTO_PUBLISH      = "Z_TUMBLR_AUTO_PUBLISH"      //key:Z_TUMBLR_AUTO_PUBLISH subkey:matchId score:addTime
)

type TumblrBlog struct {
	UserId              int64
	Name                string
	Url                 string
	Description         string
	IsNswf              bool
	Avartar64           string
	Avartar128          string
	ImageFetchOffset    int
	FetchFinish         bool
	MaxPostId           int64
	UploadedImageOffset int
}

type TumblrImage struct {
	Key         string
	PostId      int64
	IndexInPost int
	Sizes       []struct {
		Width  int
		Height int
		Url    string
	}
}

type TumblrUploadedImage struct {
	QiniuKey string
}

func makeTumblrImageKey(postId int64, imageIndex int) string {
	return fmt.Sprintf("%d/%d", postId, imageIndex)
}

func makeZTumblrBlogImageKey(blogName string, isPortrait bool) string {
	if isPortrait {
		return fmt.Sprintf("%s/%s/p", Z_TUMBLR_BLOG_IMAGE, blogName)
	} else {
		return fmt.Sprintf("%s/%s/l", Z_TUMBLR_BLOG_IMAGE, blogName)
	}
}

func makeZTumblrUploadedImageKey(blogName string, isPortrait bool) string {
	if isPortrait {
		return fmt.Sprintf("%s/%s/p", Z_TUMBLR_UPLOADED_IMAGE, blogName)
	} else {
		return fmt.Sprintf("%s/%s/l", Z_TUMBLR_UPLOADED_IMAGE, blogName)
	}
}

func makeZTumblrDeletedImageKey(blogName string) string {
	return fmt.Sprintf("%s/%s", Z_TUMBLR_DELETED_IMAGE, blogName)
}

func makeZTumblrUserPublishKey(userId int64) string {
	return fmt.Sprintf("%s/%d", Z_TUMBLR_USER_PUBLISH, userId)
}

func checkTumblrSecret(secret string) bool {
	return secret == "isjdifj242i0o;a;lidf"
}

func apiTumblrAddBlog(w http.ResponseWriter, r *http.Request) {
	var err error
	lwutil.CheckMathod(r, "POST")

	//ssdb
	ssdbc, err := ssdbPool.Get()
	lwutil.CheckError(err, "")
	defer ssdbc.Close()

	ssdbAuth, err := ssdbAuthPool.Get()
	lwutil.CheckError(err, "")
	defer ssdbAuth.Close()

	//in
	var in struct {
		TumblrBlog
		Secret string
	}
	err = lwutil.DecodeRequestBody(r, &in)
	lwutil.CheckError(err, "err_decode_body")

	if !checkTumblrSecret(in.Secret) {
		lwutil.SendError("err_secret", "")
	}

	//add user
	resp, err := ssdbc.Do("hget", H_TUMBLR_ACCONT, in.Name)
	var userId int64
	if resp[0] == ssdb.NOT_FOUND {
		userId = GenSerial(ssdbAuth, ACCOUNT_SERIAL)

		resp, err = ssdbc.Do("hset", H_TUMBLR_ACCONT, in.Name, userId)
		lwutil.CheckSsdbError(resp, err)

		//set account
		var account Account
		account.RegisterTime = time.Now().Format(time.RFC3339)
		js, err := json.Marshal(account)
		lwutil.CheckError(err, "")
		_, err = ssdbAuth.Do("hset", H_ACCOUNT, userId, js)
		lwutil.CheckError(err, "")
	} else {
		lwutil.CheckSsdbError(resp, err)
		userId, err = strconv.ParseInt(resp[1], 10, 64)
	}

	//set player
	playerKey := makePlayerInfoKey(userId)

	var player PlayerInfo
	player.UserId = userId
	player.NickName = in.Name
	player.CustomAvatarKey = in.Avartar128
	player.Description = in.Description
	err = ssdbc.HSetStruct(playerKey, player)
	lwutil.CheckError(err, "")

	infoLite := makePlayerInfoLite(&player)
	js, err := json.Marshal(infoLite)
	lwutil.CheckError(err, "err_json")
	resp, err = ssdbc.Do("hset", H_PLAYER_INFO_LITE, infoLite.UserId, js)
	lwutil.CheckError(err, "")

	//
	in.TumblrBlog.UserId = userId

	//
	updatePlayerSearchInfo(ssdbc, "", in.Name, userId)

	//checkExist
	resp, err = ssdbc.Do("hexists", H_TUMBLR_BLOG, in.Name)
	lwutil.CheckSsdbError(resp, err)
	if resp[1] != "1" {
		//hset
		err = ssdbc.TableSetRow(H_TUMBLR_BLOG, in.Name, in.TumblrBlog)
		lwutil.CheckError(err, "err_table_set_row")
	}

	//zset
	resp, err = ssdbc.Do("zset", Z_TUMBLR_BLOG, in.Name, lwutil.GetRedisTimeUnix())
	lwutil.CheckSsdbError(resp, err)

	//out
	out := struct {
		Blog TumblrBlog
	}{}
	ssdbc.TableGetRow(H_TUMBLR_BLOG, in.Name, &out.Blog)
	lwutil.WriteResponse(w, out)
}

func apiTumblrDelBlog(w http.ResponseWriter, r *http.Request) {
	var err error
	lwutil.CheckMathod(r, "POST")

	//ssdb
	ssdbc, err := ssdbPool.Get()
	lwutil.CheckError(err, "")
	defer ssdbc.Close()

	//in
	var in struct {
		BlogName string
		Secret   string
		Clear    bool
	}
	err = lwutil.DecodeRequestBody(r, &in)
	lwutil.CheckError(err, "err_decode_body")

	if !checkTumblrSecret(in.Secret) {
		lwutil.SendError("err_secret", "")
	}

	//zdel
	resp, err := ssdbc.Do("zdel", Z_TUMBLR_BLOG, in.BlogName)
	lwutil.CheckSsdbError(resp, err)

	//clear
	if in.Clear {
		//hdel
		var blog TumblrBlog
		err = ssdbc.TableDelRow(H_TUMBLR_BLOG, in.BlogName, blog)
		lwutil.CheckError(err, "err_table_del_row")

		//zclear
		key := makeZTumblrBlogImageKey(in.BlogName, true)
		resp, err = ssdbc.Do("zclear", key)
		lwutil.CheckSsdbError(resp, err)

		key = makeZTumblrBlogImageKey(in.BlogName, false)
		resp, err = ssdbc.Do("zclear", key)
		lwutil.CheckSsdbError(resp, err)
	}
}

// func apiTumblrListBlog(w http.ResponseWriter, r *http.Request) {
// 	var err error
// 	lwutil.CheckMathod(r, "POST")

// 	//ssdb
// 	ssdbc, err := ssdbPool.Get()
// 	lwutil.CheckError(err, "")
// 	defer ssdbc.Close()

// 	//in
// 	var in struct {
// 		LastKey   string
// 		LastScore int64
// 		Limit     int
// 	}
// 	err = lwutil.DecodeRequestBody(r, &in)
// 	lwutil.CheckError(err, "err_decode_body")

// 	if in.LastScore <= 0 {
// 		in.LastScore = math.MaxInt64
// 	}
// 	if in.Limit <= 0 {
// 		in.Limit = 20
// 	} else if in.Limit > 50 {
// 		in.Limit = 50
// 	}

// 	//zrscan
// 	resp, err := ssdbc.Do("zrscan", Z_TUMBLR_BLOG, in.LastKey, in.LastScore, "", in.Limit)
// 	lwutil.CheckSsdbError(resp, err)

// 	//out
// 	out := struct {
// 		LastKey           string
// 		LastScore         int64
// 		Blogs             []*TumblrBlog
// 		ImageFetchOffsets map[string]int
// 	}{
// 		in.LastKey,
// 		in.LastScore,
// 		make([]*TumblrBlog, 0, 20),
// 		make(map[string]int),
// 	}

// 	resp = resp[1:]
// 	if len(resp) == 0 {
// 		lwutil.WriteResponse(w, out)
// 	}

// 	num := len(resp) / 2
// 	cmds := make([]interface{}, 2, num+2)
// 	cmds[0] = "multi_hget"
// 	cmds[1] = H_TUMBLR_BLOG
// 	for i := 0; i < num; i++ {
// 		cmds = append(cmds, resp[i*2])
// 		if i == num-1 {
// 			out.LastKey = resp[i*2]
// 			out.LastScore, err = strconv.ParseInt(resp[i*2+1], 10, 64)
// 			lwutil.CheckError(err, "strconv")
// 		}
// 	}
// 	resp, err = ssdbc.Do(cmds...)
// 	lwutil.CheckSsdbError(resp, err)
// 	resp = resp[1:]
// 	num = len(resp) / 2
// 	for i := 0; i < num; i++ {
// 		blog := TumblrBlog{}
// 		err = json.Unmarshal([]byte(resp[i*2+1]), &blog)
// 		lwutil.CheckError(err, "json")
// 		out.Blogs = append(out.Blogs, &blog)
// 	}

// 	//offsets
// 	cmds = make([]interface{}, 2, num+2)
// 	cmds[0] = "multi_hget"
// 	cmds[1] = H_TUMBLR_BLOG_IMAGE_OFFSET
// 	for i := 0; i < num; i++ {
// 		cmds = append(cmds, out.Blogs[i].Name)
// 	}
// 	resp, err = ssdbc.Do(cmds...)
// 	lwutil.CheckSsdbError(resp, err)
// 	resp = resp[1:]
// 	num = len(resp) / 2
// 	for i := 0; i < num; i++ {
// 		offset, err := strconv.Atoi(resp[i*2+1])
// 		lwutil.CheckError(err, "err_strconv")
// 		out.ImageFetchOffsets[resp[i*2]] = offset
// 	}

// 	lwutil.WriteResponse(w, out)

// }

func apiTumblrListBlog(w http.ResponseWriter, r *http.Request) {
	var err error
	lwutil.CheckMathod(r, "POST")

	//ssdb
	ssdbc, err := ssdbPool.Get()
	lwutil.CheckError(err, "")
	defer ssdbc.Close()

	//in
	var in struct {
		LastKey   string
		LastScore int64
		Limit     int
	}
	err = lwutil.DecodeRequestBody(r, &in)
	lwutil.CheckError(err, "err_decode_body")

	if in.LastScore <= 0 {
		in.LastScore = math.MaxInt64
	}
	if in.Limit <= 0 {
		in.Limit = 20
	} else if in.Limit > 100 {
		in.Limit = 100
	}

	//zrscan
	resp, err := ssdbc.Do("zrscan", Z_TUMBLR_BLOG, in.LastKey, in.LastScore, "", in.Limit)
	lwutil.CheckSsdbError(resp, err)

	//out
	out := struct {
		LastKey   string
		LastScore int64
		Blogs     []TumblrBlog
	}{
		in.LastKey,
		in.LastScore,
		make([]TumblrBlog, 0, 20),
	}

	resp = resp[1:]
	if len(resp) == 0 {
		lwutil.WriteResponse(w, out)
	}

	num := len(resp) / 2
	keys := make([]string, 0, num)
	for i := 0; i < num; i++ {
		keys = append(keys, resp[i*2])
		if i == num-1 {
			out.LastKey = resp[i*2]
			out.LastScore, err = strconv.ParseInt(resp[i*2+1], 10, 64)
			lwutil.CheckError(err, "strconv")
		}
	}

	ssdbc.TableGetRows(H_TUMBLR_BLOG, keys, &out.Blogs)

	lwutil.WriteResponse(w, out)
}

func apiTumblrGetBlog(w http.ResponseWriter, r *http.Request) {
	var err error
	lwutil.CheckMathod(r, "POST")

	//ssdb
	ssdbc, err := ssdbPool.Get()
	lwutil.CheckError(err, "")
	defer ssdbc.Close()

	//in
	var in struct {
		BlogName string
	}
	err = lwutil.DecodeRequestBody(r, &in)
	lwutil.CheckError(err, "err_decode_body")

	var blog TumblrBlog
	ssdbc.TableGetRow(H_TUMBLR_BLOG, in.BlogName, &blog)

	//out
	out := struct {
		Blog *TumblrBlog
	}{
		&blog,
	}

	lwutil.WriteResponse(w, out)
}

func apiTumblrAddImages(w http.ResponseWriter, r *http.Request) {
	var err error
	lwutil.CheckMathod(r, "POST")

	//ssdb
	ssdbc, err := ssdbPool.Get()
	lwutil.CheckError(err, "")
	defer ssdbc.Close()

	//in
	var in struct {
		BlogName         string
		Images           []TumblrImage
		ImageFetchOffset int
		Secret           string
	}
	err = lwutil.DecodeRequestBody(r, &in)
	lwutil.CheckError(err, "err_decode_body")

	if !checkTumblrSecret(in.Secret) {
		lwutil.SendError("err_secret", "")
	}

	//get blog
	var blog TumblrBlog
	err = ssdbc.TableGetRow(H_TUMBLR_BLOG, in.BlogName, &blog)
	lwutil.CheckError(err, "err_ssdb")

	//
	zPortraitKey := makeZTumblrBlogImageKey(in.BlogName, true)
	zLandscapeKey := makeZTumblrBlogImageKey(in.BlogName, false)
	for _, image := range in.Images {
		if len(image.Sizes) == 0 {
			glog.Error("tumblr image no sizes")
			continue
		}
		imageKey := makeTumblrImageKey(image.PostId, image.IndexInPost)
		image.Key = imageKey

		resp, err := ssdbc.Do("hexists", H_TUMBLR_IMAGE, imageKey)
		lwutil.CheckSsdbError(resp, err)
		if resp[0] == "1" {
			continue
		}

		jsb, err := json.Marshal(image)
		lwutil.CheckError(err, "err_json")
		resp, err = ssdbc.Do("hset", H_TUMBLR_IMAGE, imageKey, jsb)
		lwutil.CheckSsdbError(resp, err)

		if image.Sizes[0].Width <= image.Sizes[0].Height {
			resp, err = ssdbc.Do("zset", zPortraitKey, imageKey, image.PostId)
			lwutil.CheckSsdbError(resp, err)
		} else {
			resp, err = ssdbc.Do("zset", zLandscapeKey, imageKey, image.PostId)
			lwutil.CheckSsdbError(resp, err)
		}

		if image.PostId > blog.MaxPostId {
			mp := map[string]interface{}{
				"MaxPostId": image.PostId,
			}
			err = ssdbc.TableSetRowWithMap(H_TUMBLR_BLOG, in.BlogName, mp)
			lwutil.CheckError(err, "err_table_set_row")
		}
	}

	//set blogImageFetchOffset
	row := struct {
		ImageFetchOffset int
	}{
		in.ImageFetchOffset,
	}
	err = ssdbc.TableSetRow(H_TUMBLR_BLOG, in.BlogName, row)
	lwutil.CheckError(err, "err_table_set_row")
}

func apiTumblrDelImage(w http.ResponseWriter, r *http.Request) {
	var err error
	lwutil.CheckMathod(r, "POST")

	//ssdb
	ssdbc, err := ssdbPool.Get()
	lwutil.CheckError(err, "")
	defer ssdbc.Close()

	//in
	var in struct {
		BlogName string
		Key      string
		Secret   string
	}
	err = lwutil.DecodeRequestBody(r, &in)
	lwutil.CheckError(err, "err_decode_body")

	if !checkTumblrSecret(in.Secret) {
		lwutil.SendError("err_secret", "")
	}

	//get image
	resp, err := ssdbc.Do("hget", H_TUMBLR_IMAGE, in.Key)
	lwutil.CheckSsdbError(resp, err)
	var image TumblrImage
	err = json.Unmarshal([]byte(resp[1]), &image)
	lwutil.CheckError(err, "err_json")

	//
	zkey := makeZTumblrBlogImageKey(in.BlogName, true)
	if image.Sizes[0].Width > image.Sizes[0].Height {
		zkey = makeZTumblrBlogImageKey(in.BlogName, false)
	}
	resp, err = ssdbc.Do("zdel", zkey, in.Key)
	lwutil.CheckSsdbError(resp, err)
	zDeletedKey := makeZTumblrDeletedImageKey(in.BlogName)
	resp, err = ssdbc.Do("zset", zDeletedKey, in.Key, lwutil.GetRedisTimeUnix())
	lwutil.CheckSsdbError(resp, err)
}

func apiTumblrListImage(w http.ResponseWriter, r *http.Request) {
	var err error
	lwutil.CheckMathod(r, "POST")

	//ssdb
	ssdbc, err := ssdbPool.Get()
	lwutil.CheckError(err, "")
	defer ssdbc.Close()

	//in
	var in struct {
		BlogName   string
		LastKey    string
		LastScore  int64
		Limit      int
		IsPortrait bool
	}
	err = lwutil.DecodeRequestBody(r, &in)
	lwutil.CheckError(err, "err_decode_body")

	if in.LastScore <= 0 {
		in.LastScore = math.MaxInt64
	}
	if in.Limit <= 0 {
		in.Limit = 20
	} else if in.Limit > 50 {
		in.Limit = 50
	}

	//zrscan
	zkey := makeZTumblrBlogImageKey(in.BlogName, in.IsPortrait)
	resp, err := ssdbc.Do("zrscan", zkey, in.LastKey, in.LastScore, "", in.Limit)
	lwutil.CheckSsdbError(resp, err)

	//out
	out := struct {
		LastKey   string
		LastScore int64
		Images    []*TumblrImage
	}{
		in.LastKey,
		in.LastScore,
		make([]*TumblrImage, 0, 20),
	}

	resp = resp[1:]
	if len(resp) == 0 {
		lwutil.WriteResponse(w, out)
		return
	}

	num := len(resp) / 2
	cmds := make([]interface{}, 2, num+2)
	cmds[0] = "multi_hget"
	cmds[1] = H_TUMBLR_IMAGE
	for i := 0; i < num; i++ {
		cmds = append(cmds, resp[i*2])
		if i == num-1 {
			out.LastKey = resp[i*2]
			out.LastScore, err = strconv.ParseInt(resp[i*2+1], 10, 64)
			lwutil.CheckError(err, "strconv")
		}
	}
	resp, err = ssdbc.Do(cmds...)
	lwutil.CheckSsdbError(resp, err)
	resp = resp[1:]
	num = len(resp) / 2
	for i := 0; i < num; i++ {
		image := TumblrImage{}
		err = json.Unmarshal([]byte(resp[i*2+1]), &image)
		lwutil.CheckError(err, "json")
		out.Images = append(out.Images, &image)
	}

	lwutil.WriteResponse(w, out)
}

func apiTumblrSetFetchFinish(w http.ResponseWriter, r *http.Request) {
	var err error
	lwutil.CheckMathod(r, "POST")

	//ssdb
	ssdbc, err := ssdbPool.Get()
	lwutil.CheckError(err, "")
	defer ssdbc.Close()

	//in
	var in struct {
		BlogName string
	}
	err = lwutil.DecodeRequestBody(r, &in)
	lwutil.CheckError(err, "err_decode_body")

	//set
	mp := map[string]interface{}{
		"FetchFinish": true,
	}
	err = ssdbc.TableSetRowWithMap(H_TUMBLR_BLOG, in.BlogName, mp)
	lwutil.CheckError(err, "err_table_set_row")
}

func apiTumblrPublish(w http.ResponseWriter, r *http.Request) {
	var err error
	lwutil.CheckMathod(r, "POST")

	//ssdb
	ssdbc, err := ssdbPool.Get()
	lwutil.CheckError(err, "")
	defer ssdbc.Close()

	//in
	var in struct {
		Pack
		SliderNum        int
		GoldCoinForPrize int
		Private          bool
		Tags             []string
		BlogName         string
		ImageKeys        []string
		Secret           string
	}
	err = lwutil.DecodeRequestBody(r, &in)
	lwutil.CheckError(err, "err_decode_body")

	if !checkTumblrSecret(in.Secret) {
		lwutil.SendError("err_secret", "")
	}

	if len(in.ImageKeys) != len(in.Images) {
		lwutil.SendError("err_len_match", "len(in.ImageKeys) != len(in.Images)")
	}

	if in.SliderNum < 3 {
		in.SliderNum = 3
	} else if in.SliderNum > 8 {
		in.SliderNum = 8
	}
	stringLimit(&in.Title, 100)
	stringLimit(&in.Text, 1000)

	//
	if in.GoldCoinForPrize != 0 {
		lwutil.SendError("err_gold_coin", "")
	}

	//get userId
	resp, err := ssdbc.Do("hget", H_TUMBLR_ACCONT, in.BlogName)
	lwutil.CheckSsdbError(resp, err)
	userId, err := strconv.ParseInt(resp[1], 10, 64)

	//
	playerKey := makePlayerInfoKey(userId)

	//new match
	matchId := GenSerial(ssdbc, MATCH_SERIAL)

	newPack(ssdbc, &in.Pack, userId, matchId)

	beginTime := lwutil.GetRedisTime()
	beginTimeUnix := beginTime.Unix()
	beginTimeStr := beginTime.Format("2006-01-02T15:04:05")
	endTimeUnix := beginTime.Add(MATCH_TIME_SEC * time.Second).Unix()

	match := Match{
		Id:                   matchId,
		PackId:               in.Pack.Id,
		ImageNum:             len(in.Pack.Images),
		OwnerId:              userId,
		OwnerName:            in.BlogName,
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
		PromoUrl:             "",
		PromoImage:           "",
		Private:              in.Private,
	}

	js, err := json.Marshal(match)
	lwutil.CheckError(err, "")

	//add to hash
	resp, err = ssdbc.Do("hset", H_MATCH, matchId, js)
	lwutil.CheckSsdbError(resp, err)

	//
	if !in.Private {
		//add to Z_MATCH
		resp, err = ssdbc.Do("zset", Z_MATCH, matchId, beginTimeUnix)
		lwutil.CheckSsdbError(resp, err)

		// //Z_HOT_MATCH
		// resp, err = ssdbc.Do("zset", Z_HOT_MATCH, matchId, in.GoldCoinForPrize*PRIZE_NUM_PER_COIN)
		// lwutil.CheckSsdbError(resp, err)
	}

	// //Z_OPEN_MATCH
	// resp, err = ssdbc.Do("zset", Z_OPEN_MATCH, matchId, endTimeUnix)
	// lwutil.CheckSsdbError(resp, err)

	//Z_LIKE_MATCH
	key := makeZLikeMatchKey(userId)
	resp, err = ssdbc.Do("zset", key, matchId, beginTimeUnix)
	lwutil.CheckSsdbError(resp, err)

	//Q_LIKE_MATCH
	key = makeQLikeMatchKey(userId)
	resp, err = ssdbc.Do("qpush_back", key, matchId)
	lwutil.CheckSsdbError(resp, err)

	//Z_PLAYER_MATCH
	zPlayerMatchKey := makeZPlayerMatchKey(userId)
	resp, err = ssdbc.Do("zset", zPlayerMatchKey, matchId, beginTimeUnix)
	lwutil.CheckSsdbError(resp, err)

	//Q_PLAYER_MATCH
	qPlayerMatchKey := makeQPlayerMatchKey(userId)
	resp, err = ssdbc.Do("qpush_back", qPlayerMatchKey, matchId)
	lwutil.CheckSsdbError(resp, err)

	//channel
	key = makeHUserChannelKey(userId)
	resp, err = ssdbc.Do("hgetall", key)
	lwutil.CheckSsdbError(resp, err)
	resp = resp[1:]
	num := len(resp) / 2
	for i := 0; i < num; i++ {
		channelName := resp[i*2]
		key := makeZChannelMatchKey(channelName)
		resp, err := ssdbc.Do("zset", key, matchId, beginTimeUnix)
		lwutil.CheckSsdbError(resp, err)

		//set channel thumb
		resp, err = ssdbc.Do("hset", H_CHANNEL_THUMB, channelName, match.Thumb)
		lwutil.CheckSsdbError(resp, err)
	}

	//decrease gold coin
	if in.GoldCoinForPrize != 0 {
		addPlayerGoldCoin(ssdbc, playerKey, -in.GoldCoinForPrize)
	}

	//delete from list
	zkey := makeZTumblrBlogImageKey(in.BlogName, true)
	cmds := make([]interface{}, 2, len(in.ImageKeys)+2)
	cmds[0] = "multi_zdel"
	cmds[1] = zkey
	for _, v := range in.ImageKeys {
		cmds = append(cmds, v)
	}
	resp, err = ssdbc.Do(cmds...)
	lwutil.CheckSsdbError(resp, err)

	zkey = makeZTumblrBlogImageKey(in.BlogName, false)
	cmds = make([]interface{}, 2, len(in.ImageKeys)+2)
	cmds[0] = "multi_zdel"
	cmds[1] = zkey
	for _, v := range in.ImageKeys {
		cmds = append(cmds, v)
	}
	resp, err = ssdbc.Do(cmds...)
	lwutil.CheckSsdbError(resp, err)

	//fanout
	go fanout(&match)

	//out
	lwutil.WriteResponse(w, match)
}

func apiTumblrPublishQueue(w http.ResponseWriter, r *http.Request) {
	var err error
	lwutil.CheckMathod(r, "POST")

	//ssdb
	ssdbc, err := ssdbPool.Get()
	lwutil.CheckError(err, "")
	defer ssdbc.Close()

	//in
	var in struct {
		Pack
		Private   bool
		Tags      []string
		BlogName  string
		ImageKeys []string
		SliderNum int
		Secret    string
	}
	err = lwutil.DecodeRequestBody(r, &in)
	lwutil.CheckError(err, "err_decode_body")

	if !checkTumblrSecret(in.Secret) {
		lwutil.SendError("err_secret", "")
	}

	if len(in.ImageKeys) != len(in.Images) {
		lwutil.SendError("err_len_match", "len(in.ImageKeys) != len(in.Images)")
	}

	if in.SliderNum < 3 {
		in.SliderNum = 3
	} else if in.SliderNum > 8 {
		in.SliderNum = 8
	}
	stringLimit(&in.Title, 100)
	stringLimit(&in.Text, 1000)

	//get userId
	resp, err := ssdbc.Do("hget", H_TUMBLR_ACCONT, in.BlogName)
	lwutil.CheckSsdbError(resp, err)
	userId, err := strconv.ParseInt(resp[1], 10, 64)

	//new match
	matchId := GenSerial(ssdbc, MATCH_SERIAL)

	newPack(ssdbc, &in.Pack, userId, matchId)

	match := Match{
		Id:         matchId,
		PackId:     in.Pack.Id,
		ImageNum:   len(in.Pack.Images),
		OwnerId:    userId,
		OwnerName:  in.BlogName,
		SliderNum:  in.SliderNum,
		Thumb:      in.Pack.Thumb,
		Thumbs:     in.Pack.Thumbs,
		Title:      in.Title,
		HasResult:  false,
		PromoUrl:   "",
		PromoImage: "",
		Private:    in.Private,
	}

	js, err := json.Marshal(match)
	lwutil.CheckError(err, "")

	//add to hash
	resp, err = ssdbc.Do("hset", H_MATCH, matchId, js)
	lwutil.CheckSsdbError(resp, err)

	//add to publish queue
	now := lwutil.GetRedisTimeUnix()
	key := makeZTumblrUserPublishKey(userId)
	resp, err = ssdbc.Do("zset", key, matchId, now)
	lwutil.CheckSsdbError(resp, err)

	//add to auto publish zset
	resp, err = ssdbc.Do("zset", Z_TUMBLR_AUTO_PUBLISH, matchId, now)
	lwutil.CheckSsdbError(resp, err)

	//delete from list
	zkey := makeZTumblrBlogImageKey(in.BlogName, true)
	cmds := make([]interface{}, 2, len(in.ImageKeys)+2)
	cmds[0] = "multi_zdel"
	cmds[1] = zkey
	for _, v := range in.ImageKeys {
		cmds = append(cmds, v)
	}
	resp, err = ssdbc.Do(cmds...)
	lwutil.CheckSsdbError(resp, err)

	zkey = makeZTumblrBlogImageKey(in.BlogName, false)
	cmds = make([]interface{}, 2, len(in.ImageKeys)+2)
	cmds[0] = "multi_zdel"
	cmds[1] = zkey
	for _, v := range in.ImageKeys {
		cmds = append(cmds, v)
	}
	resp, err = ssdbc.Do(cmds...)
	lwutil.CheckSsdbError(resp, err)

	//out
	lwutil.WriteResponse(w, match)
}

func apiTumblrPublishFromQueue(w http.ResponseWriter, r *http.Request) {
	var err error
	lwutil.CheckMathod(r, "POST")

	//ssdb
	ssdbc, err := ssdbPool.Get()
	lwutil.CheckError(err, "")
	defer ssdbc.Close()

	//in
	var in struct {
		MatchId   int64
		Private   bool
		SliderNum int
		Secret    string
	}
	err = lwutil.DecodeRequestBody(r, &in)
	lwutil.CheckError(err, "err_decode_body")

	if !checkTumblrSecret(in.Secret) {
		lwutil.SendError("err_secret", "")
	}

	if in.SliderNum < 3 {
		in.SliderNum = 3
	} else if in.SliderNum > 8 {
		in.SliderNum = 8
	}

	//get match
	match := getMatch(ssdbc, in.MatchId)
	matchId := match.Id

	//get userId
	userId := match.OwnerId

	//check publish queue
	key := makeZTumblrUserPublishKey(userId)
	resp, err := ssdbc.Do("zexists", key, matchId)
	lwutil.CheckSsdbError(resp, err)
	if !ssdbCheckExists(resp) {
		lwutil.SendError("err_not_queueing", "")
	}
	queueKey := key

	//check publish already
	zPlayerMatchKey := makeZPlayerMatchKey(userId)
	resp, err = ssdbc.Do("zexists", zPlayerMatchKey, matchId)
	if ssdbCheckExists(resp) {
		lwutil.SendError("err_already_publish", "")
	}

	//
	beginTime := lwutil.GetRedisTime()
	beginTimeUnix := beginTime.Unix()
	beginTimeStr := beginTime.Format("2006-01-02T15:04:05")
	endTimeUnix := beginTime.Add(MATCH_TIME_SEC * time.Second).Unix()

	match.BeginTime = beginTimeUnix
	match.BeginTimeStr = beginTimeStr
	match.EndTime = endTimeUnix

	//
	if !in.Private {
		//add to Z_MATCH
		resp, err = ssdbc.Do("zset", Z_MATCH, matchId, beginTimeUnix)
		lwutil.CheckSsdbError(resp, err)

		// //Z_HOT_MATCH
		// resp, err = ssdbc.Do("zset", Z_HOT_MATCH, matchId, in.GoldCoinForPrize*PRIZE_NUM_PER_COIN)
		// lwutil.CheckSsdbError(resp, err)
	}

	// //Z_OPEN_MATCH
	// resp, err = ssdbc.Do("zset", Z_OPEN_MATCH, matchId, endTimeUnix)
	// lwutil.CheckSsdbError(resp, err)

	//Z_LIKE_MATCH
	key = makeZLikeMatchKey(userId)
	resp, err = ssdbc.Do("zset", key, matchId, beginTimeUnix)
	lwutil.CheckSsdbError(resp, err)

	//Q_LIKE_MATCH
	key = makeQLikeMatchKey(userId)
	resp, err = ssdbc.Do("qpush_back", key, matchId)
	lwutil.CheckSsdbError(resp, err)

	//Z_PLAYER_MATCH
	resp, err = ssdbc.Do("zset", zPlayerMatchKey, matchId, beginTimeUnix)
	lwutil.CheckSsdbError(resp, err)

	//Q_PLAYER_MATCH
	qPlayerMatchKey := makeQPlayerMatchKey(userId)
	resp, err = ssdbc.Do("qpush_back", qPlayerMatchKey, matchId)
	lwutil.CheckSsdbError(resp, err)

	//channel
	key = makeHUserChannelKey(userId)
	resp, err = ssdbc.Do("hgetall", key)
	lwutil.CheckSsdbError(resp, err)
	resp = resp[1:]
	num := len(resp) / 2
	for i := 0; i < num; i++ {
		channelName := resp[i*2]
		key := makeZChannelMatchKey(channelName)
		resp, err := ssdbc.Do("zset", key, matchId, beginTimeUnix)
		lwutil.CheckSsdbError(resp, err)

		//set channel thumb
		resp, err = ssdbc.Do("hset", H_CHANNEL_THUMB, channelName, match.Thumb)
		lwutil.CheckSsdbError(resp, err)
	}

	//del from queue
	resp, err = ssdbc.Do("zdel", queueKey, matchId)
	lwutil.CheckSsdbError(resp, err)

	//del from auto zset
	resp, err = ssdbc.Do("zdel", Z_TUMBLR_AUTO_PUBLISH, matchId)
	lwutil.CheckSsdbError(resp, err)

	//fanout
	go fanout(match)

	//out
	lwutil.WriteResponse(w, match)
}

func apiTumblrGetUptoken(w http.ResponseWriter, r *http.Request) {
	lwutil.CheckMathod(r, "POST")

	//in
	var in struct {
		Secret string
	}
	err := lwutil.DecodeRequestBody(r, &in)
	lwutil.CheckError(err, "err_decode_body")

	if !checkTumblrSecret(in.Secret) {
		lwutil.SendError("err_secret", "")
	}

	//
	scope := fmt.Sprintf("%s", USER_UPLOAD_BUCKET)
	putPolicy := qiniurs.PutPolicy{
		Scope: scope,
	}
	token := putPolicy.Token(nil)

	scope = fmt.Sprintf("%s", USER_PRIVATE_UPLOAD_BUCKET)
	putPolicy = qiniurs.PutPolicy{
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

func apiTumblrListPublishQueue(w http.ResponseWriter, r *http.Request) {
	lwutil.CheckMathod(r, "POST")
	var err error

	//session
	session, err := findSession(w, r, nil)
	lwutil.CheckError(err, "err_auth")
	checkAdmin(session)

	//ssdb
	ssdbc, err := ssdbPool.Get()
	lwutil.CheckError(err, "")
	defer ssdbc.Close()

	//in
	var in struct {
		UserId int64
		Key    string
		Score  string
		Limit  int
	}
	err = lwutil.DecodeRequestBody(r, &in)
	lwutil.CheckError(err, "err_decode_body")

	if in.Limit <= 0 || in.Limit > 30 {
		in.Limit = 30
	}

	key := makeZTumblrUserPublishKey(in.UserId)
	resp, lastKey, lastScore, err := ssdbc.ZScan(key, H_MATCH, in.Key, in.Score, in.Limit, false)
	lwutil.CheckError(err, "err_zscan")

	num := len(resp) / 2
	matches := make([]*Match, 0, num)
	for i := 0; i < num; i++ {
		matchJs := resp[i*2+1]
		var match Match
		err := json.Unmarshal([]byte(matchJs), &match)
		lwutil.CheckError(err, "err_json")
		matches = append(matches, &match)
	}

	//out
	out := struct {
		Matches   []*Match
		LastKey   string
		LastScore string
	}{
		matches,
		lastKey,
		lastScore,
	}

	lwutil.WriteResponse(w, out)
}

func tumblrAutoPublish() {
	// for true {
	// 	now := lwutil.GetRedisTime()
	// 	hour := now.Hour()
	// 	if hour >= 2 && hour <= 8 {
	// 		time.Sleep(10 * time.Minute)
	// 		continue
	// 	}

	// 	//
	// 	resp, err := ssdbc.Do("zkeys", Z_TUMBLR_AUTO_PUBLISH, "", "", "", 100)
	// 	lwutil.CheckSsdbError(resp, err)
	// 	resp = resp[1:]

	// 	if len(resp) > 0 {
	// 		key := resp[rand.Intn(len(resp))]
	// 	}

	// 	//del
	// 	resp, err = ssdbc.Do("zdel", Z_TUMBLR_AUTO_PUBLISH, matchId)
	// 	lwutil.CheckSsdbError(resp, err)

	// 	sleepMinute := 30 + rand.Intn(30)
	// 	time.Sleep(sleepMinute * time.Minute)
	// }
}

func regTumblr() {
	http.Handle("/tumblr/addBlog", lwutil.ReqHandler(apiTumblrAddBlog))
	http.Handle("/tumblr/delBlog", lwutil.ReqHandler(apiTumblrDelBlog))
	http.Handle("/tumblr/listBlog", lwutil.ReqHandler(apiTumblrListBlog))
	http.Handle("/tumblr/getBlog", lwutil.ReqHandler(apiTumblrGetBlog))
	http.Handle("/tumblr/addImages", lwutil.ReqHandler(apiTumblrAddImages))
	http.Handle("/tumblr/delImage", lwutil.ReqHandler(apiTumblrDelImage))
	http.Handle("/tumblr/listImage", lwutil.ReqHandler(apiTumblrListImage))
	http.Handle("/tumblr/publish", lwutil.ReqHandler(apiTumblrPublish))
	http.Handle("/tumblr/publishQueue", lwutil.ReqHandler(apiTumblrPublishQueue))
	http.Handle("/tumblr/publishFromQueue", lwutil.ReqHandler(apiTumblrPublishFromQueue))
	http.Handle("/tumblr/setFetchFinish", lwutil.ReqHandler(apiTumblrSetFetchFinish))
	http.Handle("/tumblr/getUptoken", lwutil.ReqHandler(apiTumblrGetUptoken))

	http.Handle("/tumblr/listPublishQueue", lwutil.ReqHandler(apiTumblrListPublishQueue))

	// http.Handle("/tumblr/addUploadedImages", lwutil.ReqHandler(apiTumblrAddUploadedImages))

	// //ssdb
	// ssdbc, err := ssdbPool.Get()
	// lwutil.CheckError(err, "")
	// defer ssdbc.Close()

	// type Person struct {
	// 	Name     string
	// 	Age      int
	// 	Sex      int
	// 	NickName string
	// }
	// s := Person{
	// 	"liwei",
	// 	33,
	// 	112,
	// 	"henyouqian",
	// }
	// ssdbc.TableSetRow("H_TEST", "key1", s)

	// s2 := struct {
	// 	Name string
	// 	Age  int
	// }{
	// 	"wuhaili",
	// 	22,
	// }
	// ssdbc.TableSetRow("H_TEST", "key2", s2)

	// var ps []Person
	// ssdbc.TableGetRows("H_TEST", []string{"key1", "key2"}, &ps)
	// glog.Infof("%+v", ps)
}
