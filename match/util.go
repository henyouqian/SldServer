package main

import (
	"errors"
	"fmt"
	"net/http"
	"runtime"
	"unicode/utf8"

	"github.com/golang/glog"
	"github.com/henyouqian/lwutil"
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
