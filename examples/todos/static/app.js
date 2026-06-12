// Client-side glue: an EventSource for the live stats line and a plain
// WebSocket for the chat. textContent (never innerHTML) keeps chat XSS-safe.
new EventSource('/events').addEventListener('stats', function (e) {
  document.getElementById('live-stats').textContent = e.data;
});

var ws = new WebSocket('ws://' + location.host + '/chat');
ws.onmessage = function (e) {
  var li = document.createElement('li');
  li.textContent = e.data;
  var log = document.getElementById('chat-log');
  log.appendChild(li);
  log.scrollTop = log.scrollHeight;
};

document.getElementById('chat-form').addEventListener('submit', function (e) {
  e.preventDefault();
  var input = document.getElementById('chat-input');
  if (input.value) { ws.send(input.value); input.value = ''; }
});
