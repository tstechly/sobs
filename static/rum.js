/**
 * SOBS RUM – lightweight Real User Monitoring script.
 *
 * Usage:
 *   <script src="http://YOUR_SOBS_HOST/static/rum.js"></script>
 *   <script>
 *     SOBS.init({ endpoint: 'http://YOUR_SOBS_HOST/v1/rum', appName: 'my-app' });
 *   </script>
 *
 * Collects:
 *  - Page views
 *  - Web Vitals (LCP, FID/INP, CLS, TTFB, FCP) via PerformanceObserver
 *  - JS errors and unhandled promise rejections
 *  - Navigation / resource timing summaries
 */

(function (global) {
  'use strict';

  var SOBS = {};
  var _cfg = {};
  var _session = null;

  // ----- session id -----
  function _getSession() {
    try {
      var k = 'sobs_sid';
      var sid = sessionStorage.getItem(k);
      if (!sid) {
        sid = _uuid();
        sessionStorage.setItem(k, sid);
      }
      return sid;
    } catch (e) {
      return _uuid();
    }
  }

  function _uuid() {
    return 'xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx'.replace(/[xy]/g, function (c) {
      var r = (Math.random() * 16) | 0;
      return (c === 'x' ? r : (r & 0x3) | 0x8).toString(16);
    });
  }

  // ----- send -----
  function _send(events) {
    if (!_cfg.endpoint) return;
    var payload = Array.isArray(events) ? events : [events];
    payload = payload.map(function (e) {
      return Object.assign({ sessionId: _session, appName: _cfg.appName || '' }, e);
    });
    try {
      navigator.sendBeacon(_cfg.endpoint, JSON.stringify(payload));
    } catch (e) {
      // fallback
      var xhr = new XMLHttpRequest();
      xhr.open('POST', _cfg.endpoint, true);
      xhr.setRequestHeader('Content-Type', 'application/json');
      xhr.send(JSON.stringify(payload));
    }
  }

  function _ts() {
    return new Date().toISOString();
  }

  // ----- page view -----
  function _trackPageView() {
    _send({
      type: 'pageview',
      timestamp: _ts(),
      url: location.href,
      title: document.title,
      referrer: document.referrer,
    });
  }

  // ----- errors -----
  function _trackErrors() {
    global.addEventListener('error', function (evt) {
      _send({
        type: 'error',
        timestamp: _ts(),
        url: location.href,
        message: evt.message,
        errorType: (evt.error && evt.error.name) || 'Error',
        stack: (evt.error && evt.error.stack) || '',
        filename: evt.filename,
        lineno: evt.lineno,
        colno: evt.colno,
      });
    });
    global.addEventListener('unhandledrejection', function (evt) {
      var reason = evt.reason || {};
      _send({
        type: 'unhandledrejection',
        timestamp: _ts(),
        url: location.href,
        message: reason.message || String(reason),
        errorType: (reason.name) || 'UnhandledRejection',
        stack: reason.stack || '',
      });
    });
  }

  // ----- Web Vitals via PerformanceObserver -----
  function _reportWebVital(name, value, rating) {
    _send({
      type: 'web-vital',
      timestamp: _ts(),
      url: location.href,
      name: name,
      value: value,
      rating: rating || 'unknown',
    });
  }

  function _rating(name, value) {
    var thresholds = {
      LCP:  [2500, 4000],
      FID:  [100,  300],
      INP:  [200,  500],
      CLS:  [0.1,  0.25],
      TTFB: [800,  1800],
      FCP:  [1800, 3000],
    };
    var t = thresholds[name];
    if (!t) return 'unknown';
    if (value <= t[0]) return 'good';
    if (value <= t[1]) return 'needs-improvement';
    return 'poor';
  }

  function _trackWebVitals() {
    if (!global.PerformanceObserver) return;

    // LCP
    try {
      new PerformanceObserver(function (list) {
        var entries = list.getEntries();
        var last = entries[entries.length - 1];
        if (last) _reportWebVital('LCP', Math.round(last.startTime), _rating('LCP', last.startTime));
      }).observe({ type: 'largest-contentful-paint', buffered: true });
    } catch (e) {}

    // FID / INP
    try {
      new PerformanceObserver(function (list) {
        list.getEntries().forEach(function (entry) {
          var name = entry.interactionId ? 'INP' : 'FID';
          var val = entry.processingStart - entry.startTime;
          _reportWebVital(name, Math.round(val), _rating(name, val));
        });
      }).observe({ type: 'first-input', buffered: true });
    } catch (e) {}

    // CLS
    try {
      var clsVal = 0;
      new PerformanceObserver(function (list) {
        list.getEntries().forEach(function (entry) {
          if (!entry.hadRecentInput) clsVal += entry.value;
        });
      }).observe({ type: 'layout-shift', buffered: true });
      global.addEventListener('visibilitychange', function () {
        if (document.visibilityState === 'hidden')
          _reportWebVital('CLS', Math.round(clsVal * 1000) / 1000, _rating('CLS', clsVal));
      });
    } catch (e) {}

    // Navigation timing: TTFB + FCP
    try {
      new PerformanceObserver(function (list) {
        list.getEntries().forEach(function (entry) {
          if (entry.name === 'first-contentful-paint')
            _reportWebVital('FCP', Math.round(entry.startTime), _rating('FCP', entry.startTime));
        });
      }).observe({ type: 'paint', buffered: true });
    } catch (e) {}

    global.addEventListener('load', function () {
      try {
        var nav = performance.getEntriesByType('navigation')[0];
        if (nav && nav.responseStart) {
          var ttfb = nav.responseStart - nav.requestStart;
          _reportWebVital('TTFB', Math.round(ttfb), _rating('TTFB', ttfb));
        }
      } catch (e) {}
    });
  }

  // ----- SPA navigation -----
  function _trackSPANavigation() {
    var origPush = history.pushState;
    var origReplace = history.replaceState;
    function onNav() { setTimeout(_trackPageView, 0); }
    history.pushState = function () { origPush.apply(this, arguments); onNav(); };
    history.replaceState = function () { origReplace.apply(this, arguments); onNav(); };
    global.addEventListener('popstate', onNav);
  }

  // ----- public API -----
  SOBS.init = function (cfg) {
    _cfg = cfg || {};
    _session = _getSession();
    _trackPageView();
    _trackErrors();
    _trackWebVitals();
    if (_cfg.trackSPA !== false) _trackSPANavigation();
  };

  SOBS.track = function (eventType, data) {
    _send(Object.assign({ type: eventType, timestamp: _ts(), url: location.href }, data || {}));
  };

  global.SOBS = SOBS;
})(window);
