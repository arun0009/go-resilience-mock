import http from 'k6/http';
import { check, sleep, group } from 'k6';
// Embedded scenarios
const scenariosData = [
    {
        "path": "/api/resiliency/delay-fixed",
        "method": "GET",
        "responses": [
            {
                "status": 200,
                "delay": 100000000,
                "body": "{\"type\": \"fixed-delay\"}"
            }
        ]
    },
    {
        "path": "/api/resiliency/delay-jitter",
        "method": "GET",
        "responses": [
            {
                "status": 200,
                "delayRange": "50ms-150ms",
                "body": "{\"type\": \"jitter-delay\"}"
            }
        ]
    },
    {
        "path": "/api/resiliency/chaos",
        "method": "GET",
        "responses": [
            {
                "status": 500,
                "probability": 0.2,
                "body": "{\"error\": \"chaos-failure\"}"
            },
            {
                "status": 200,
                "body": "{\"status\": \"success\"}"
            }
        ]
    },
    {
        "path": "/api/resiliency/circuit-breaker",
        "method": "GET",
        "circuitBreaker": {
            "failureThreshold": 5,
            "successThreshold": 2,
            "timeout": 2000000000
        },
        "responses": [
            {
                "status": 500,
                "body": "{\"error\": \"failure\"}"
            }
        ]
    }
];

export const options = {
    stages: [
        { duration: '5s', target: 5 },   // Warm up
        { duration: '10s', target: 20 }, // Load
        { duration: '5s', target: 0 },   // Ramp down
    ],
    thresholds: {
        'checks': ['rate>0.9'], // 90% of checks must pass
        'http_req_duration': ['p(95)<1000'], // 95% < 1s
    },
};

const BASE_URL = 'http://localhost:8080';

// Setup: Inject scenarios from file + functional ones
export function setup() {
    // 1. Inject File-based Scenarios
    const resFile = http.post(`${BASE_URL}/scenario`, JSON.stringify(scenariosData), {
        headers: { 'Content-Type': 'application/json' },
    });
    check(resFile, { 'file scenarios added': (r) => r.status === 200 });

    // 2. Inject Functional Scenarios
    const functionalScenarios = [
        {
            path: "/api/k6/match",
            method: "GET",
            matches: { query: { type: "admin" } },
            responses: [{ status: 200, body: "admin-response" }]
        },
        {
            path: "/api/k6/match",
            method: "GET",
            responses: [{ status: 200, body: "guest-response" }]
        },
        {
            path: "/api/k6/dynamic/{id}",
            method: "GET",
            responses: [{ status: 200, body: "User {{.Request.PathVars.id}}" }]
        }
    ];

    const resFunc = http.post(`${BASE_URL}/scenario`, JSON.stringify(functionalScenarios), {
        headers: { 'Content-Type': 'application/json' },
    });
    check(resFunc, { 'functional scenarios added': (r) => r.status === 200 });
}

export default function () {

    group('Functional - Core', function () {
        // 1. Health
        const resHealth = http.get(`${BASE_URL}/health`);
        check(resHealth, { 'health is 200': (r) => r.status === 200 });

        // 2. Echo JSON
        const payload = JSON.stringify({ hello: "k6" });
        const resEcho = http.post(`${BASE_URL}/echo`, payload, {
            headers: { 'Content-Type': 'application/json' },
        });
        check(resEcho, {
            'echo status 200': (r) => r.status === 200,
            'echo body correct': (r) => r.body.includes('hello'),
        });

        // 3. History
        const resHistory = http.get(`${BASE_URL}/history`);
        check(resHistory, { 'history is 200': (r) => r.status === 200 });
    });

    group('Functional - Advanced', function () {
        // 4. Matching Rules
        const resAdmin = http.get(`${BASE_URL}/api/k6/match?type=admin`);
        check(resAdmin, { 'match admin': (r) => r.body.includes('admin-response') });

        const resGuest = http.get(`${BASE_URL}/api/k6/match`);
        check(resGuest, { 'match guest': (r) => r.body.includes('guest-response') });

        // 5. Dynamic Paths
        const userId = Math.floor(Math.random() * 1000);
        const resDyn = http.get(`${BASE_URL}/api/k6/dynamic/${userId}`);
        check(resDyn, { 'dynamic path var': (r) => r.body.includes(`User ${userId}`) });
    });

    group('Resiliency', function () {
        // 6. Jitter
        const resJitter = http.get(`${BASE_URL}/api/resiliency/delay-jitter`);
        check(resJitter, { 'jitter 200': (r) => r.status === 200 });

        // 7. Chaos
        const resChaos = http.get(`${BASE_URL}/api/resiliency/chaos`);
        check(resChaos, {
            'chaos handled': (r) => r.status === 200 || r.status === 500,
        });

        // 8. Circuit Breaker
        const resCB = http.get(`${BASE_URL}/api/resiliency/circuit-breaker`);
        check(resCB, {
            'cb handled': (r) => r.status === 500 || r.status === 503,
        });
    });

    group('Stress Testing', function () {
        // 9. CPU Stress (Short duration to avoid stalling k6)
        const resCPU = http.get(`${BASE_URL}/api/stress/cpu/10ms`);
        check(resCPU, { 'cpu stress 200': (r) => r.status === 200 });

        // 10. Memory Stress
        const resMem = http.get(`${BASE_URL}/api/stress/mem/1MB`);
        check(resMem, { 'mem stress 200': (r) => r.status === 200 });
    });

    sleep(1);
}
