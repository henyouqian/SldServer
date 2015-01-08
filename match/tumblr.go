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
)

func glogTumblr() {
	glog.Info("")
}

const (
	H_TUMBLR_BLOG              = "H_TUMBLR_BLOG"              //key:H_TUMBLR_BLOG subkey:blogName
	Z_TUMBLR_BLOG              = "Z_TUMBLR_BLOG"              //key:Z_TUMBLR_BLOG subkey:blogName score:addTime
	H_TUMBLR_IMAGE             = "H_TUMBLR_IMAGE"             //key:H_TUMBLR_IMAGE subkey:imageId
	Z_TUMBLR_BLOG_IMAGE        = "Z_TUMBLR_BLOG_IMAGE"        //key:Z_TUMBLR_BLOG_IMAGE/blogName subkey:imageKey score:postId
	H_TUMBLR_BLOG_IMAGE_OFFSET = "H_TUMBLR_BLOG_IMAGE_OFFSET" //key:H_TUMBLR_BLOG_IMAGE_OFFSET subkey:blogName value:offset
	H_TUMBLR_ACCONT            = "H_TUMBLR_ACCONT"            //key:H_TUMBLR_ACCONT subkey:blogName value:userId
)

type TumblrBlog struct {
	Name             string
	Url              string
	Description      string
	IsNswf           bool
	Avartar64        string
	Avartar128       string
	ImageFetchOffset int
	FetchFinish      bool
	MaxPostId        int64
}

type TumblrImage struct {
	PostId int64
	Sizes  []struct {
		Width  int
		Height int
		Url    string
	}
}

func makeTumblrImageKey(postId int64, imageIndex int) string {
	return fmt.Sprintf("%d/%d", postId, imageIndex)
}

func makeZTumblrBlogImageKey(blogName string) string {
	return fmt.Sprintf("%s/%s", Z_TUMBLR_BLOG_IMAGE, blogName)
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

	//checkExist
	resp, err := ssdbc.Do("hexists", H_TUMBLR_BLOG, in.Name)
	lwutil.CheckSsdbError(resp, err)
	if resp[0] == "1" {
		lwutil.SendError("err_exist", "")
	}

	//hset
	err = ssdbc.TableSetRow(H_TUMBLR_BLOG, in.Name, in.TumblrBlog)
	lwutil.CheckError(err, "err_table_set_row")

	// js, err := json.Marshal(in.TumblrBlog)
	// resp, err := ssdbc.Do("hset", H_TUMBLR_BLOG, in.Name, js)
	// lwutil.CheckSsdbError(resp, err)

	//zset
	resp, err = ssdbc.Do("zset", Z_TUMBLR_BLOG, in.Name, lwutil.GetRedisTimeUnix())
	lwutil.CheckSsdbError(resp, err)

	// resp, err = ssdbc.Do("hget", H_TUMBLR_BLOG_IMAGE_OFFSET, in.Name)
	// lwutil.CheckSsdbError(resp, err)
	// offset, err := strconv.Atoi(resp[1])
	// lwutil.CheckError(err, "err_strconv")
	// out := struct {
	// 	ImageFetchOffset int
	// }{
	// 	offset,
	// }
	// lwutil.WriteResponse(w, out)

	//add user
	resp, err = ssdbc.Do("hget", H_TUMBLR_ACCONT, in.Name)
	var userId int64
	if resp[0] == ssdb.NOT_FOUND {
		userId = GenSerial(ssdbc, ACCOUNT_SERIAL)

		resp, err = ssdbAuth.Do("hset", H_TUMBLR_ACCONT, in.Name, userId)
		lwutil.CheckSsdbError(resp, err)

		//set account
		var account Account
		account.RegisterTime = time.Now().Format(time.RFC3339)
		js, err := json.Marshal(account)
		lwutil.CheckError(err, "")
		_, err = ssdbAuth.Do("hset", H_ACCOUNT, userId, js)
		lwutil.CheckError(err, "")

		//set player
		playerKey := makePlayerInfoKey(userId)
		addPlayerGoldCoin(ssdbc, playerKey, 20)

		//
		var player PlayerInfo
		err = ssdbc.HSetStruct(playerKey, player)
		lwutil.CheckError(err, "")
		player.NickName = in.Name
		player.CustomAvatarKey = in.Avartar128
		player.Description = in.Description

		infoLite := makePlayerInfoLite(&player)
		js, err = json.Marshal(infoLite)
		lwutil.CheckError(err, "err_json")
		resp, err = ssdbc.Do("hset", H_PLAYER_INFO_LITE, infoLite.UserId, js)
		lwutil.CheckError(err, "")

	} else {
		lwutil.CheckSsdbError(resp, err)
		userId, err = strconv.ParseInt(resp[1], 10, 64)
	}
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
	}
	err = lwutil.DecodeRequestBody(r, &in)
	lwutil.CheckError(err, "err_decode_body")

	if !checkTumblrSecret(in.Secret) {
		lwutil.SendError("err_secret", "")
	}

	//hdel
	var blog TumblrBlog
	err = ssdbc.TableDelRow(H_TUMBLR_BLOG, in.BlogName, blog)
	lwutil.CheckError(err, "err_table_del_row")

	// resp, err := ssdbc.Do("hdel", H_TUMBLR_BLOG, in.BlogName)
	// lwutil.CheckSsdbError(resp, err)

	//zset
	resp, err := ssdbc.Do("zdel", Z_TUMBLR_BLOG, in.BlogName)
	lwutil.CheckSsdbError(resp, err)
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
	} else if in.Limit > 50 {
		in.Limit = 50
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
	zkey := makeZTumblrBlogImageKey(in.BlogName)
	for i, image := range in.Images {
		if len(image.Sizes) == 0 {
			glog.Error("tumblr image no sizes")
			continue
		}
		imageKey := makeTumblrImageKey(image.PostId, i)

		jsb, err := json.Marshal(image)
		lwutil.CheckError(err, "err_json")
		resp, err := ssdbc.Do("hset", H_TUMBLR_IMAGE, imageKey, jsb)
		lwutil.CheckSsdbError(resp, err)

		resp, err = ssdbc.Do("zset", zkey, imageKey, image.PostId)
		lwutil.CheckSsdbError(resp, err)

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

func apiTumblrListImage(w http.ResponseWriter, r *http.Request) {
	var err error
	lwutil.CheckMathod(r, "POST")

	//ssdb
	ssdbc, err := ssdbPool.Get()
	lwutil.CheckError(err, "")
	defer ssdbc.Close()

	//in
	var in struct {
		BlogName  string
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
	} else if in.Limit > 50 {
		in.Limit = 50
	}

	//zrscan
	zkey := makeZTumblrBlogImageKey(in.BlogName)
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

func regTumblr() {
	http.Handle("/tumblr/addBlog", lwutil.ReqHandler(apiTumblrAddBlog))
	http.Handle("/tumblr/delBlog", lwutil.ReqHandler(apiTumblrDelBlog))
	http.Handle("/tumblr/listBlog", lwutil.ReqHandler(apiTumblrListBlog))
	http.Handle("/tumblr/addImages", lwutil.ReqHandler(apiTumblrAddImages))
	http.Handle("/tumblr/listImage", lwutil.ReqHandler(apiTumblrListImage))
	http.Handle("/tumblr/setFetchFinish", lwutil.ReqHandler(apiTumblrSetFetchFinish))

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
