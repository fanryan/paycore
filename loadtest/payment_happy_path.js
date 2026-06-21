import http from 'k6/http';
import { check, sleep } from 'k6';

export const options = {
  vus: Number(__ENV.PAYCORE_LOADTEST_VUS || 5),
  duration: __ENV.PAYCORE_LOADTEST_DURATION || '30s',
  thresholds: {
    http_req_failed: ['rate<0.05'],
    http_req_duration: ['p(95)<500'],
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
      ...extra,
    },
  };
}

export default function () {
  const merchantID = uniqueID('merchant');
  const payerID = uniqueID('payer');

  const merchantResponse = http.post(
    `${baseURL}/merchants`,
    JSON.stringify({
      id: merchantID,
      name: `Load Test Merchant ${merchantID}`,
      settlement_currency: 'USD',
    }),
    jsonHeaders(),
  );

  check(merchantResponse, {
    'merchant created': (response) => response.status === 201,
  });

  const payerResponse = http.post(
    `${baseURL}/payers`,
    JSON.stringify({
      id: payerID,
      available_balance_minor: 10000,
      currency: 'USD',
    }),
    jsonHeaders(),
  );

  check(payerResponse, {
    'payer created': (response) => response.status === 201,
  });

  const authorizeResponse = http.post(
    `${baseURL}/payments/authorize`,
    JSON.stringify({
      merchant_id: merchantID,
      payer_id: payerID,
      amount: 4000,
      currency: 'USD',
    }),
    jsonHeaders({
      'Idempotency-Key': uniqueID('authorize-key'),
    }),
  );

  const authorized = check(authorizeResponse, {
    'payment authorized': (response) => response.status === 201,
    'authorization returned payment id': (response) => Boolean(response.json('payment_id')),
  });

  if (!authorized) {
    sleep(1);
    return;
  }

  const paymentID = authorizeResponse.json('payment_id');
  const captureResponse = http.post(
    `${baseURL}/payments/${paymentID}/capture`,
    JSON.stringify({}),
    jsonHeaders({
      'Idempotency-Key': uniqueID('capture-key'),
    }),
  );

  check(captureResponse, {
    'payment captured': (response) => response.status === 200,
    'capture status is captured': (response) => response.json('status') === 'CAPTURED',
  });

  sleep(1);
}
