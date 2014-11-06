// Copyright 2013 The Gorilla WebSocket Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"flag"
	"log"
	"net/http"
	"text/template"

	"github.com/golang/glog"
	"github.com/henyouqian/ssdbgo"
)

const (
	SSDB_AUTH_PORT  = 9875
	SSDB_MATCH_PORT = 9876
)

var addr = flag.String("addr", ":8080", "http service address")
var homeTempl = template.Must(template.ParseFiles("home.html"))

func serveHome(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	if r.URL.Path != "/" {
		http.ServeFile(w, r, r.URL.Path[1:])
		return
	}
	if r.Method != "GET" {
		http.Error(w, "Method not allowed", 405)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	homeTempl.Execute(w, r.Host)
	// http.ServeFile(w, r, "home.html")
}

func main() {
	flag.Parse()
	glog.Info("Running----------")
	initSsdb()
	regBattle()
	go h.run()
	http.HandleFunc("/", serveHome)
	http.HandleFunc("/ws", serveWs)
	err := http.ListenAndServe(*addr, nil)
	if err != nil {
		log.Fatal("ListenAndServe: ", err)
	}
}

var (
	ssdbMatchPool *ssdbgo.Pool
	ssdbAuthPool  *ssdbgo.Pool
)

func initSsdb() {
	ssdbAuthPool = ssdbgo.NewPool("localhost", SSDB_AUTH_PORT, 10, 60)
	ssdbMatchPool = ssdbgo.NewPool("localhost", SSDB_MATCH_PORT, 10, 60)
}
