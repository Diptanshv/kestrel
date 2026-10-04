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

  window.kestrel = { track: send };
  send();
})();