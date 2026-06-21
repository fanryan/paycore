import http from 'k6/http';
import { check, sleep } from 'k6';

export const options = {
  vus: Number(__ENV.PAYCORE_LOADTEST_VUS || 10),
  duration: __ENV.PAYCORE_LOADTEST_DURATION || '30s',
  thresholds: {
    http_req_failed: ['rate<0.05'],
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
      'X-Forwarded-For': '203.0.113.30',
      ...extra,
    },
  };
}

export default function () {
  const merchantID = uniqueID('backlog-merchant');
  const payerID = uniqueID('backlog-payer');

  const merchantResponse = http.post(
    `${baseURL}/merchants`,
    JSON.stringify({
      id: merchantID,
      name: `Backlog Merchant ${merchantID}`,
      settlement_currency: 'USD',
    }),
    jsonHeaders(),
  );

  const payerResponse = http.post(
    `${baseURL}/payers`,
    JSON.stringify({
      id: payerID,
      available_balance_minor: 10000,
      currency: 'USD',
    }),
    jsonHeaders(),
  );

  const setupSucceeded = check(merchantResponse, {
    'backlog merchant created': (response) => response.status === 201,
  }) && check(payerResponse, {
    'backlog payer created': (response) => response.status === 201,
  });

  if (!setupSucceeded) {
    sleep(0.1);
    return;
  }

  const authorizeResponse = http.post(
    `${baseURL}/payments/authorize`,
    JSON.stringify({
      merchant_id: merchantID,
      payer_id: payerID,
      amount: 4000,
      currency: 'USD',
    }),
    jsonHeaders({
      'Idempotency-Key': uniqueID('backlog-authorize-key'),
    }),
  );

  const authorized = check(authorizeResponse, {
    'backlog payment authorized': (response) => response.status === 201,
    'backlog authorization returned payment id': (response) => Boolean(response.json('payment_id')),
  });

  if (!authorized) {
    sleep(0.1);
    return;
  }

  const paymentID = authorizeResponse.json('payment_id');
  const captureResponse = http.post(
    `${baseURL}/payments/${paymentID}/capture`,
    JSON.stringify({}),
    jsonHeaders({
      'Idempotency-Key': uniqueID('backlog-capture-key'),
    }),
  );

  check(captureResponse, {
    'backlog payment captured': (response) => response.status === 200,
    'backlog capture status is captured': (response) => response.json('status') === 'CAPTURED',
  });

  sleep(0.1);
}
