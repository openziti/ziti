var webSocket;

function openWebSocket() {
    if (webSocket !== undefined && webSocket.readyState !== WebSocket.CLOSED) {
        return;
    }
    webSocket = new WebSocket("ws://localhost:8080/metrics");

    webSocket.onmessage = function (event) {
        let msg = $.parseJSON(event.data);
        processMessage(msg);
    };

    webSocket.onclose = function (event) {
        console.log("connection closed");
    };
}

$(window).on('beforeunload', function(){
    webSocket.close();
    console.log("closed websocket");
});

(function () {
    openWebSocket();
})();