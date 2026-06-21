import http from 'k6/http';
import { check, sleep } from 'k6';

export const options = {
  vus: Number(__ENV.PAYCORE_LOADTEST_VUS || 20),
  duration: __ENV.PAYCORE_LOADTEST_DURATION || '20s',
  thresholds: {
    http_req_duration: ['p(95)<750'],
  },
};

const baseURL = __ENV.PAYCORE_BASE_URL || 'http://localhost:8080';

function uniqueID(prefix) {
  return `${prefix}-${__VU}-${__ITER}-${Date.now()}`;
}

function jsonHeaders(extra = {}) {
  return {
    headers: {
      'Content-Type': 'application/json',
      'X-Forwarded-For': '203.0.113.20',
      ...extra,
    },
  };
}

export function setup() {
  const suffix = Date.now();
  const merchantID = `contention-merchant-${suffix}`;
  const payerID = `contention-payer-${suffix}`;

  const merchantResponse = http.post(
    `${baseURL}/merchants`,
    JSON.stringify({
      id: merchantID,
      name: `Contention Merchant ${suffix}`,
      settlement_currency: 'USD',
    }),
    jsonHeaders(),
  );

  const payerResponse = http.post(
    `${baseURL}/payers`,
    JSON.stringify({
      id: payerID,
      available_balance_minor: 1000000000,
      currency: 'USD',
    }),
    jsonHeaders(),
  );

  check(merchantResponse, {
    'contention merchant created': (response) => response.status === 201,
  });

  check(payerResponse, {
    'contention payer created': (response) => response.status === 201,
  });

  const data = {
    merchantID,
    payerID,
  };
  return data;
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
      'Idempotency-Key': uniqueID('contention-key'),
    }),
  );

  check(response, {
    'contention authorize succeeds or conflicts': (res) => res.status === 201 || res.status === 409,
    'contention conflict has stable code': (res) => res.status !== 409 || res.json('error_code') === 'PAYER_VERSION_CONFLICT',
  });

  sleep(0.05);
}
