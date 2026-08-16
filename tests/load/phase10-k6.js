import http from "k6/http";
import { check, sleep } from "k6";
import { Counter } from "k6/metrics";

const base = __ENV.BASE_URL || "http://127.0.0.1:8088";
const profiles = {
  smoke: [{ duration: "20s", target: 2 }],
  baseline: [{ duration: "30s", target: 10 }, { duration: "60s", target: 25 }, { duration: "20s", target: 0 }],
  vu100: [{ duration: "1m", target: 100 }, { duration: "3m", target: 100 }, { duration: "30s", target: 0 }],
  vu1000: [{ duration: "3m", target: 1000 }, { duration: "10m", target: 1000 }, { duration: "1m", target: 0 }],
};
export const options = {
	stages: profiles[__ENV.LOAD_PROFILE || "smoke"],
	summaryTrendStats: ["min", "med", "avg", "p(90)", "p(95)", "p(99)", "max"],
  thresholds: {
    http_req_failed: ["rate<0.01"],
    http_req_duration: ["p(95)<1000", "p(99)<2500"],
    "load_failures{status:429}": ["count==0"],
    "load_failures{status:500}": ["count==0"],
    "load_failures{status:502}": ["count==0"],
    "load_failures{status:503}": ["count==0"],
  },
};
const failures = new Counter("load_failures");
const publicPaths = ["/", "/categories", "/freelancers", "/projects", "/services", "/vacancies", "/education", "/blog", "/pro"];
const apiPaths = ["/api/v1/categories", "/api/v1/freelancers", "/api/v1/projects", "/api/v1/services", "/api/v1/vacancies", "/api/v1/blog"];
export default function () {
  const path = Math.random() < 0.65 ? publicPaths[Math.floor(Math.random() * publicPaths.length)] : apiPaths[Math.floor(Math.random() * apiPaths.length)];
  const requestClass = path.startsWith("/api/") ? "public_api" : "public_web";
  const response = http.get(base + path, { tags: { scenario: "catalog_read", route: path, request_class: requestClass } });
  if (response.status < 200 || response.status >= 400) failures.add(1, { scenario: "catalog_read", route: path, request_class: requestClass, status: String(response.status) });
  check(response, { "response is successful": r => r.status >= 200 && r.status < 400 });
  sleep(0.5 + Math.random());
}
