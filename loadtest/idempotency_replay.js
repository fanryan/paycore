import http from 'k6/http';
import { check, sleep } from 'k6';

export const options = {
  vus: Number(__ENV.PAYCORE_LOADTEST_VUS || 3),
  duration: __ENV.PAYCORE_LOADTEST_DURATION || '30s',
  thresholds: {
    http_req_failed: ['rate<0.25'],
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

function createMerchant(merchantID) {
  return http.post(
    `${baseURL}/merchants`,
    JSON.stringify({
      id: merchantID,
      name: `Replay Merchant ${merchantID}`,
      settlement_currency: 'USD',
    }),
    jsonHeaders(),
  );
}

function createPayer(payerID) {
  return http.post(
    `${baseURL}/payers`,
    JSON.stringify({
      id: payerID,
      available_balance_minor: 10000,
      currency: 'USD',
    }),
    jsonHeaders(),
  );
}

function authorize(body, key) {
  return http.post(
    `${baseURL}/payments/authorize`,
    JSON.stringify(body),
    jsonHeaders({
      'Idempotency-Key': key,
    }),
  );
}

export default function () {
  const merchantID = uniqueID('merchant');
  const payerID = uniqueID('payer');
  const idempotencyKey = uniqueID('authorize-replay-key');

  const merchantResponse = createMerchant(merchantID);
  const payerResponse = createPayer(payerID);

  const setupSucceeded = check(merchantResponse, {
    'merchant created': (response) => response.status === 201,
  }) && check(payerResponse, {
    'payer created': (response) => response.status === 201,
  });

  if (!setupSucceeded) {
    sleep(1);
    return;
  }

  const requestBody = {
    merchant_id: merchantID,
    payer_id: payerID,
    amount: 4000,
    currency: 'USD',
  };

  const firstResponse = authorize(requestBody, idempotencyKey);
  const firstPaymentID = firstResponse.json('payment_id');

  const firstSucceeded = check(firstResponse, {
    'first authorization created': (response) => response.status === 201,
    'first authorization returned payment id': () => Boolean(firstPaymentID),
  });

  if (!firstSucceeded) {
    sleep(1);
    return;
  }

  const replayResponse = authorize(requestBody, idempotencyKey);
  check(replayResponse, {
    'same key same payload replays created response': (response) => response.status === 201,
    'replay returns same payment id': (response) => response.json('payment_id') === firstPaymentID,
  });

  const conflictResponse = authorize(
    {
      ...requestBody,
      amount: 5000,
    },
    idempotencyKey,
  );

  check(conflictResponse, {
    'same key different payload conflicts': (response) => response.status === 409,
    'conflict returns idempotency error code': (response) => response.json('error_code') === 'IDEMPOTENCY_KEY_CONFLICT',
  });

  sleep(1);
}
