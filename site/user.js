(function(){
    function getUrlParam(name) {
        var reg = new RegExp("(^|\\?|&)"+ name +"=([^&]*)(\\s|&|$)", "i");  
        if (reg.test(location.href)) return unescape(RegExp.$2.replace(/\+/g, " ")); return "";
    };


    var HOST = "http://sld.pintugame.com/"
    // var HOST = "http://localhost:9998/"
    var RES_HOST = "http://dn-pintuuserupload.qbox.me/"
    var HTML5_HOST = "http://pintuhtml5.qiniudn.com/"

    var lastMatchTime
    var lastMatchId

    var url = HOST + "match/listUser"
    var limit = 3
    var data = {
        "UserId": parseInt(getUrlParam("u")),
        "StartId": 0,
        "BeginTime": 0,
        "Limit": limit
    }

    

    $.post(url, JSON.stringify(data), function(resp){
        console.log(resp)
        for (var i in resp) {
            var match = resp[i]
            var thumbUrl = RES_HOST + match.Thumb
            $("#thumbRoot").append( '\
                <a href="'+HTML5_HOST+'index.html?key=' + match.Id + '" class="thumbnail">\
                  <img src="' + thumbUrl +'">\
                </a>\
                ' );
            lastMatchTime = match.BeginTime
            lastMatchId = match.Id
        }

    }, "json")
    
    $("#loadMore").click(function() {
        var url = HOST + "match/listUser"
        var data = {
            "UserId": parseInt(getUrlParam("u")),
            "StartId": lastMatchId,
            "BeginTime": lastMatchTime,
            "Limit": limit
        }

        $("#loadMore").prop('disabled', true)

        $.post(url, JSON.stringify(data), function(resp){
            console.log(resp)
            for (var i in resp) {
                var match = resp[i]
                var thumbUrl = RES_HOST + match.Thumb
                $("#thumbRoot").append( '\
                    <a href="'+HTML5_HOST+'index.html?key=' + match.Id + '" class="thumbnail">\
                      <img src="' + thumbUrl +'">\
                    </a>\
                    ' );
                lastMatchTime = match.BeginTime
                lastMatchId = match.Id
            }
            if (resp.length < limit) {
                $("#loadMore").text("没有更多了")
            } else {
                $("#loadMore").prop('disabled', false)
            }
        }, "json")
    });
})();