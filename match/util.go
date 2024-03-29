package main

import (
	"./ssdb"
	"errors"
	"fmt"
	"math"
	"net/http"
	"os/user"
	"runtime"
	"strconv"
	"unicode/utf8"

	"github.com/golang/glog"
	"github.com/henyouqian/lwutil"
)

const (
	SSDB_OK        = "ok"
	SSDB_NOT_FOUND = "not_found"
)

func stringLimit(str *string, limit uint) {
	if uint(len(*str)) > limit {
		*str = (*str)[:limit]
		for len(*str) > 0 {
			if utf8.ValidString(*str) {
				return
			}
			*str = (*str)[:len(*str)-1]
		}
	}
}

func ssdbCheckExists(resp []string) bool {
	return resp[1] == "1"
}

func checkError(err error) {
	if err != nil {
		_, file, line, _ := runtime.Caller(1)
		e := fmt.Sprintf("[%s:%d]%v", file, line, err)
		panic(e)
	}
}

func checkSsdbError(resp []string, err error) {
	if resp[0] != "ok" {
		err = errors.New(fmt.Sprintf("ssdb error: %s", resp[0]))
	}
	if err != nil {
		_, file, line, _ := runtime.Caller(1)
		e := fmt.Sprintf("[%s:%d]%v", file, line, err)
		panic(e)
	}
}

func handleError() {
	if r := recover(); r != nil {
		_, file, line, _ := runtime.Caller(2)
		glog.Error(r, file, line)

		buf := make([]byte, 2048)
		runtime.Stack(buf, false)
		glog.Errorf("%s", buf)
	}
}

func sendErrorNoLog(w http.ResponseWriter, errType string, errStr string) {
	_, file, line, _ := runtime.Caller(1)
	errStr = fmt.Sprintf("%s\n\t%s : %d", errStr, file, line)
	out := struct {
		Error       string
		ErrorString string
	}{
		errType,
		errStr,
	}
	w.WriteHeader(http.StatusBadRequest)
	lwutil.WriteResponse(w, out)
}

func zrscanGet(ssdbc *ssdb.Client, zkey string, zSubkeyStart, zScoreStart interface{}, limit int, hkey string) (out []string, err error) {
	out = make([]string, 0)

	//zrscan
	if zSubkeyStart == 0 {
		zSubkeyStart = math.MaxInt64
	}
	resp, err := ssdbc.Do("zrscan", zkey, zSubkeyStart, zScoreStart, "", limit)
	if err != nil {
		return
	}

	resp = resp[1:]
	if len(resp) == 0 {
		return
	}

	//multi_hget
	num := len(resp) / 2
	cmds := make([]interface{}, 2)
	cmds[0] = "multi_hget"
	cmds[1] = hkey

	for i := 0; i < num; i++ {
		cmds = append(cmds, resp[2*i])
	}

	resp, err = ssdbc.Do(cmds...)
	if err != nil {
		return
	}
	out = resp[1:]

	return
}

func isReleaseServer() bool {
	u, _ := user.Current()
	return u.Username != "liwei"
}

func formateMsec(msec int) string {
	sec := msec / 1000
	m := sec / 60
	s := sec % 60
	ms := msec % 1000
	return fmt.Sprintf("%d:%02d.%03d", m, s, ms)
}

func parseInt64(str string) (int64, error) {
	return strconv.ParseInt(str, 10, 64)
}
