(function(){
    function getUrlParam(name) {
        var reg = new RegExp("(^|\\?|&)"+ name +"=([^&]*)(\\s|&|$)", "i");  
        if (reg.test(location.href)) return unescape(RegExp.$2.replace(/\+/g, " ")); return "";
    };


    var HOST = "http://sld.pintugame.com/"
    // var HOST = "http://localhost:9998/"
    var RES_HOST = "http://dn-pintuuserupload.qbox.me/"
    var HTML5_HOST = "http://pintuhtml5.qiniudn.com/"

    var lastMatchId
    var lastScore

    var api = "match/listUserWeb"

    var url = HOST + api
    var limit = 6
    var data = {
        "UserId": parseInt(getUrlParam("u")),
        "StartId": 0,
        "LastScore": 0,
        "Limit": limit
    }

    var onMatchList = function(resp){
        var matches = resp.Matches
        for (var i in matches) {
            var match = matches[i]
            var thumbUrl = RES_HOST + match.Thumb
            $("#thumbRoot").append( '\
                <div class="col-xs-4 col-sm-3 col-md-2">\
                    <a href="'+HTML5_HOST+'index.html?key=' + match.Id + '" class="thumbnail">\
                        <img src="' + thumbUrl +'">\
                    </a>\
                </div>\
                ' );
            lastScore = resp.LastScore
            lastMatchId = match.Id
        }
        if (matches.length < limit) {
            $("#loadMore").text("后面没有了")
            $("#loadMore").prop('class', "btn btn-default")
        } else {
            $("#loadMore").prop('disabled', false)
        }
    }

    $("#loadMore").prop('disabled', true)
    $.post(url, JSON.stringify(data), onMatchList, "json")
    
    $("#loadMore").click(function() {
        var url = HOST + api
        var data = {
            "UserId": parseInt(getUrlParam("u")),
            "StartId": lastMatchId,
            "LastScore": lastScore,
            "Limit": limit
        }

        $("#loadMore").prop('disabled', true)

        $.post(url, JSON.stringify(data), onMatchList, "json")
    });
})();