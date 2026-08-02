import http from 'k6/http';
import { check, sleep } from 'k6';

const base = __ENV.BASE_URL || 'http://127.0.0.1:18088';

export const options = {
  scenarios: {
    steady: {
      executor: 'ramping-vus',
      stages: [
        { duration: __ENV.RAMP_UP || '30s', target: Number(__ENV.VUS || 50) },
        { duration: __ENV.HOLD || '2m', target: Number(__ENV.VUS || 50) },
        { duration: '20s', target: 0 },
      ],
    },
  },
  thresholds: {
    http_req_failed: ['rate<0.01'],
    http_req_duration: ['p(95)<500', 'p(99)<1200'],
    checks: ['rate>0.99'],
  },
};

export default function () {
  const responses = http.batch([
    ['GET', `${base}/healthz`],
    ['GET', `${base}/api/v3/system/info`],
    ['GET', `${base}/openapi.yaml`],
  ]);
  check(responses[0], { 'web healthy': (r) => r.status === 200 });
  check(responses[1], { 'API responds': (r) => r.status === 200 && r.json('service') === 'api' });
  check(responses[2], { 'OpenAPI responds': (r) => r.status === 200 });
  sleep(Math.random() * 0.5);
}
