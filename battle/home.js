$(function() {
    $("#pair").click(function() {
        var msg = '{"Type":"authPair", "Token":"5UeGteZ_TDd1BhiC8Ce_5A==", "RoomName":"free"}'
        conn.send(msg)
    })
    $("#emoji").click(function() {
        var msg = '{"Type":"talk", "Text":"😝"}'
        conn.send(msg)
    })
    $("#ready").click(function() {
        var msg = '{"Type":"ready"}'
        conn.send(msg)
    })
    $("#finish").click(function() {
        var msg = '{"Type":"finish", "Msec":1000}'
        conn.send(msg)
    })
})