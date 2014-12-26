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

    var matchId = parseInt(getUrlParam("id"))
    var matches = JSON.parse(localStorage.matches)
    var match = matches[matchId]
    console.log(match)

    var api = "pack/get"

    var url = HOST + api
    var limit = 6
    
    var data = {
        "Id": match.PackId
    }

    $.post(url, JSON.stringify(data), function(resp){
        var pack = resp
        // var 
        // for (var i in matches) {
        //     var match = matches[i]
        //     var thumbUrl = RES_HOST + match.Thumb
        //     $("#thumbRoot").append( '\
        //         <div class="col-xs-4 col-sm-3 col-md-2">\
        //             <a href="'+HTML5_HOST+'index.html?key=' + match.Id + '" class="thumbnail thumb">\
        //                 <img src="' + thumbUrl +'">\
        //             </a>\
        //         </div>\
        //         ' );
        //     lastScore = resp.LastScore
        //     lastMatchId = match.Id
        // }
    }, "json")

    $("#userRow").click(function() {
        alert("aaa")
    })

})();