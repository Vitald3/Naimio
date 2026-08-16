"use client";

import type { ElementType } from "react";

type AuthBootstrapLoaderProps = {
  as?: ElementType;
  className?: string;
};

export function AuthBootstrapLoader({
  as: Component = "main",
  className = "",
}: AuthBootstrapLoaderProps) {
  return (
    <Component
      className={`auth-bootstrap-screen ${className}`.trim()}
      aria-live="polite"
      aria-busy="true"
    >
      <div
        className="auth-bootstrap-spinner"
        role="status"
        aria-label="Проверяем авторизацию"
      />
    </Component>
  );
}
