"use client";
import { useSiteSettings } from "./site-settings";

export default function NaimioLogo({ compact = false }: { compact?: boolean }) {
  const settings = useSiteSettings();
  return (
    <span
      className={`naimio-logo${compact ? " naimio-logo--compact" : ""}`}
      aria-label={settings.project_name}
    >
      <span className="naimio-logo__mark" aria-hidden="true">
        <svg viewBox="0 0 64 64">
          <defs>
            <linearGradient
              id="naimio-logo-bg"
              x1="8"
              y1="6"
              x2="56"
              y2="60"
              gradientUnits="userSpaceOnUse"
            >
              <stop stopColor="#18a777" />
              <stop offset=".58" stopColor="#0d7452" />
              <stop offset="1" stopColor="#123d2c" />
            </linearGradient>
          </defs>
          <rect width="64" height="64" rx="18" fill="url(#naimio-logo-bg)" />
          <path
            fill="#fff"
            d="M13 44V20c0-3.9 3.1-7 7-7h4.1c2.8 0 5.3 1.7 6.4 4.2l2.4 5.8 2.4-5.8c1.1-2.5 3.6-4.2 6.4-4.2H48v31h-8V26.2l-4.2 10h-5.9l-4.2-10V44H13Z"
          />
          <circle cx="47" cy="48" r="5" fill="#b7ef93" />
        </svg>
      </span>
      {!compact ? (
        <span className="naimio-logo__word">{settings.project_name}</span>
      ) : null}
    </span>
  );
}
