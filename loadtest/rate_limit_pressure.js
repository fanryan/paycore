import http from 'k6/http';
import { check, sleep } from 'k6';

export const options = {
  vus: Number(__ENV.PAYCORE_LOADTEST_VUS || 10),
  duration: __ENV.PAYCORE_LOADTEST_DURATION || '20s',
  thresholds: {
    http_req_duration: ['p(95)<500'],
  },
};

const baseURL = __ENV.PAYCORE_BASE_URL || 'http://localhost:8080';
const clientIP = __ENV.PAYCORE_LOADTEST_CLIENT_IP || '203.0.113.10';

function uniqueID(prefix) {
  return `${prefix}-${__VU}-${__ITER}-${Date.now()}`;
}

function jsonHeaders(extra = {}) {
  return {
    headers: {
      'Content-Type': 'application/json',
      'X-Forwarded-For': clientIP,
      ...extra,
    },
  };
}

export function setup() {
  const suffix = Date.now();
  const merchantID = `rate-merchant-${suffix}`;
  const payerID = `rate-payer-${suffix}`;

  http.post(
    `${baseURL}/merchants`,
    JSON.stringify({
      id: merchantID,
      name: `Rate Limit Merchant ${suffix}`,
      settlement_currency: 'USD',
    }),
    jsonHeaders(),
  );

  http.post(
    `${baseURL}/payers`,
    JSON.stringify({
      id: payerID,
      available_balance_minor: 1000000000,
      currency: 'USD',
    }),
    jsonHeaders(),
  );

  return {
    merchantID,
    payerID,
  };
}

export default function (data) {
  const response = http.post(
    `${baseURL}/payments/authorize`,
    JSON.stringify({
      merchant_id: data.merchantID,
      payer_id: data.payerID,
      amount: 1,
      currency: 'USD',
    }),
    jsonHeaders({
      'Idempotency-Key': uniqueID('rate-key'),
    }),
  );

  check(response, {
    'rate-limit pressure returns allowed or rejected': (res) => res.status === 201 || res.status === 409 || res.status === 429,
    'rate-limit rejection uses stable error code': (res) => res.status !== 429 || res.json('error_code') === 'RATE_LIMIT_EXCEEDED',
  });

  sleep(0.1);
}
