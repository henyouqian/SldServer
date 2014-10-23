(function(){
    var HOST = "http://sld.pintugame.com/"
    // var HOST = "http://localhost:9998/"
    var RES_HOST = "http://dn-pintuuserupload.qbox.me/"
    var HTML5_HOST = "http://pintuhtml5.qiniudn.com/"

    var url = HOST + "match/list"
    var data = {
        "StartId": 0,
        "BeginTime": 0,
        "Limit": 30
    }

    $.post(url, JSON.stringify(data), function(resp){
        console.log(resp)
        for (var i in resp) {
            var match = resp[i]
            var thumbUrl = RES_HOST + match.Thumb
            console.log(thumbUrl)

            // $("#thumbRoot").append( '\
            //     <div class="col-xs-4 col-sm-4">\
            //     <a href="index.html?key=' + match.Id + '" class="thumbnail">\
            //       <img src="' + thumbUrl +'">\
            //     </a>\
            //   </div>\
            //     ' );
            $("#thumbRoot").append( '\
                <a href="'+HTML5_HOST+'index.html?key=' + match.Id + '" class="thumbnail">\
                  <img src="' + thumbUrl +'">\
                </a>\
                ' );
        }
        

        // var nameString = ""
        // var scoreString = ""
        // for (var i in resp.Ranks) {
        //     var rank = parseInt(i) + 1
        //     var name = resp.Ranks[i].Name
        //     if (i == 9) {
        //         nameString += rank + ". " + name + "\n"
        //     } else {
        //         nameString += rank + ".   " + name + "\n"
        //     }

        //     var timeStr = msecToStr(resp.Ranks[i].Msec)
        //     if (name == data.UserName) {
        //         gMyScore = timeStr
        //         scoreString += "* "
        //         updateWeixin()
        //     }
        //     scoreString += timeStr + "\n"
        // }
        // self._nameLabel.setString(nameString)
        // self._scoreLabel.setString(scoreString)

    }, "json")
})();