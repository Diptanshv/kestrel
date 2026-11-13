(function () {
  var s = document.currentScript;
  if (!s) return;
  var domain = s.getAttribute('data-domain');
  if (!domain) return;

  var api;
  try {
    api = new URL(s.src).origin + '/api/event';
  } catch (e) {
    return;
  }

  function send(name, props) {
    try {
      var body = JSON.stringify({
        d: domain,
        n: name || 'pageview',
        u: location.href,
        r: document.referrer,
        w: window.innerWidth,
        p: props
      });
      // text/plain keeps this a CORS "simple request", so no preflight.
      var blob = new Blob([body], { type: 'text/plain' });
      if (!navigator.sendBeacon || !navigator.sendBeacon(api, blob)) {
        fetch(api, {
          method: 'POST',
          body: body,
          headers: { 'Content-Type': 'text/plain' },
          keepalive: true,
          mode: 'cors'
        }).catch(function () {});
      }
    } catch (e) {
      // Analytics must never break the host page.
    }
  }

  // SPA route changes
  var push = history.pushState;
  if (push) {
    history.pushState = function () {
      push.apply(this, arguments);
      send();
    };
  }
  addEventListener('popstate', function () { send(); });

  // Calls made before this script loaded are queued by the snippet stub
  // (window.kestrel = window.kestrel || {q: [], track: function () {
  //   this.q.push(arguments) }}) and replayed after the first pageview.
  var queued = (window.kestrel && window.kestrel.q) || [];
  window.kestrel = {
    track: function (name, props) {
      if (name) send(String(name), props);
    }
  };
  send();
  for (var i = 0; i < queued.length; i++) {
    window.kestrel.track(queued[i][0], queued[i][1]);
  }
})();