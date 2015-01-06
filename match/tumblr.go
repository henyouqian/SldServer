package main

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"strconv"

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
)

type TumblrBlog struct {
	Name        string
	Url         string
	Description string
	IsNswf      bool
	Avartar64   string
	Avartar128  string
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

	//hset
	js, err := json.Marshal(in.TumblrBlog)
	resp, err := ssdbc.Do("hset", H_TUMBLR_BLOG, in.Name, js)
	lwutil.CheckSsdbError(resp, err)

	//zset
	resp, err = ssdbc.Do("zset", Z_TUMBLR_BLOG, in.Name, lwutil.GetRedisTimeUnix())
	lwutil.CheckSsdbError(resp, err)

	resp, err = ssdbc.Do("hget", H_TUMBLR_BLOG_IMAGE_OFFSET, in.Name)
	lwutil.CheckSsdbError(resp, err)
	offset, err := strconv.Atoi(resp[1])
	lwutil.CheckError(err, "err_strconv")
	out := struct {
		ImageFetchOffset int
	}{
		offset,
	}
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
	}
	err = lwutil.DecodeRequestBody(r, &in)
	lwutil.CheckError(err, "err_decode_body")

	if !checkTumblrSecret(in.Secret) {
		lwutil.SendError("err_secret", "")
	}

	//hdel
	resp, err := ssdbc.Do("hdel", H_TUMBLR_BLOG, in.BlogName)
	lwutil.CheckSsdbError(resp, err)

	//zset
	resp, err = ssdbc.Do("zdel", Z_TUMBLR_BLOG, in.BlogName)
	lwutil.CheckSsdbError(resp, err)
}

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
		LastKey           string
		LastScore         int64
		Blogs             []*TumblrBlog
		ImageFetchOffsets map[string]int
	}{
		in.LastKey,
		in.LastScore,
		make([]*TumblrBlog, 0, 20),
		make(map[string]int),
	}

	resp = resp[1:]
	if len(resp) == 0 {
		lwutil.WriteResponse(w, out)
	}

	num := len(resp) / 2
	cmds := make([]interface{}, 2, num+2)
	cmds[0] = "multi_hget"
	cmds[1] = H_TUMBLR_BLOG
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
		blog := TumblrBlog{}
		err = json.Unmarshal([]byte(resp[i*2+1]), &blog)
		lwutil.CheckError(err, "json")
		out.Blogs = append(out.Blogs, &blog)
	}

	//offsets
	cmds = make([]interface{}, 2, num+2)
	cmds[0] = "multi_hget"
	cmds[1] = H_TUMBLR_BLOG_IMAGE_OFFSET
	for i := 0; i < num; i++ {
		cmds = append(cmds, out.Blogs[i].Name)
	}
	resp, err = ssdbc.Do(cmds...)
	lwutil.CheckSsdbError(resp, err)
	resp = resp[1:]
	num = len(resp) / 2
	for i := 0; i < num; i++ {
		offset, err := strconv.Atoi(resp[i*2+1])
		lwutil.CheckError(err, "err_strconv")
		out.ImageFetchOffsets[resp[i*2]] = offset
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
	}

	//incr blogImageFetchOffset
	resp, err := ssdbc.Do("hset", H_TUMBLR_BLOG_IMAGE_OFFSET, in.BlogName, in.ImageFetchOffset)
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

func regTumblr() {
	http.Handle("/tumblr/addBlog", lwutil.ReqHandler(apiTumblrAddBlog))
	http.Handle("/tumblr/delBlog", lwutil.ReqHandler(apiTumblrDelBlog))
	http.Handle("/tumblr/listBlog", lwutil.ReqHandler(apiTumblrListBlog))
	http.Handle("/tumblr/addImages", lwutil.ReqHandler(apiTumblrAddImages))
	http.Handle("/tumblr/listImage", lwutil.ReqHandler(apiTumblrListImage))

	//ssdb
	ssdbc, err := ssdbPool.Get()
	lwutil.CheckError(err, "")
	defer ssdbc.Close()

	type Person struct {
		Name string
		Age  int
	}
	s := Person{
		"liwei",
		33,
	}
	ssdbc.TableSetRow("H_TEST", "key1", s)
	var s2 Person
	ssdbc.TableGetRow("H_TEST", "key1", &s2)
	glog.Infof("%+v", s2)

	s2.Name = "wuhaili"
	s2.Age = 22
	ssdbc.TableSetRow("H_TEST", "key2", s2)
	var ps []Person
	ssdbc.TableGetRows("H_TEST", []string{"key1", "key2"}, &ps)
	glog.Infof("%+v", ps)
}
