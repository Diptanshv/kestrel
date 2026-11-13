// k6 load test for POST /api/event.
//
//   make loadtest                                  # against localhost:8080
//   BASE_URL=https://api.example.com k6 run loadtest/ingest.js
//
// The server must know DOMAIN as a site (`make seed` creates demo.kestrel.dev)
// and have its rate limits raised: k6 sends everything from one IP to one
// site, which the default limits are designed to stop.
import http from 'k6/http'
import { check } from 'k6'
import { Counter } from 'k6/metrics'

const BASE = __ENV.BASE_URL || 'http://localhost:8080'
const DOMAIN = __ENV.DOMAIN || 'demo.kestrel.dev'
const PEAK = Number(__ENV.PEAK_RPS || 3000)

const rejected = new Counter('rejected_events')

export const options = {
  discardResponseBodies: true,
  scenarios: {
    ramp: {
      executor: 'ramping-arrival-rate',
      startRate: 100,
      timeUnit: '1s',
      preAllocatedVUs: 200,
      maxVUs: 2000,
      stages: [
        { duration: '30s', target: PEAK }, // find the knee
        { duration: '60s', target: PEAK }, // sustain it
        { duration: '10s', target: 0 },
      ],
    },
  },
  thresholds: {
    // Design requirement N1: ingest answers in under 10 ms at p99.
    http_req_duration: ['p(99)<10'],
    http_req_failed: ['rate<0.001'],
  },
  summaryTrendStats: ['avg', 'med', 'p(90)', 'p(95)', 'p(99)', 'max'],
}

const pages = ['/', '/pricing', '/docs/getting-started', '/blog/hyperloglog-in-go', '/about']
const uas = [
  'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Safari/537.36',
  'Mozilla/5.0 (Macintosh; Intel Mac OS X 14_5) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.5 Safari/605.1.15',
  'Mozilla/5.0 (X11; Linux x86_64; rv:131.0) Gecko/20100101 Firefox/131.0',
  'Mozilla/5.0 (iPhone; CPU iPhone OS 17_5 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.5 Mobile/15E148 Safari/604.1',
]

export default function () {
  const body = JSON.stringify({
    d: DOMAIN,
    n: 'pageview',
    u: `https://${DOMAIN}${pages[Math.floor(Math.random() * pages.length)]}`,
    r: Math.random() < 0.4 ? 'https://news.ycombinator.com/' : '',
    w: 1440,
  })
  const res = http.post(`${BASE}/api/event`, body, {
    headers: {
      'Content-Type': 'text/plain',
      Origin: `https://${DOMAIN}`,
      // A unique UA suffix per request makes every beacon a distinct visitor,
      // so hashing and session tracking do real work.
      'User-Agent': `${uas[__ITER % uas.length]} k6/${__VU}-${__ITER}`,
    },
  })
  if (!check(res, { accepted: r => r.status === 202 })) rejected.add(1)
}
