"use client";

/**
 * Root error boundary. This replaces the entire document (including the root
 * layout and its globals.css import) when a render error escapes the layout,
 * so it must render its own <html>/<body> and cannot rely on stylesheet
 * classes — styles are inlined intentionally.
 */
export default function GlobalError({
  error: _error,
  reset,
}: {
  error: Error & { digest?: string };
  reset: () => void;
}) {
  return (
    <html lang="ru">
      <body
        style={{
          margin: 0,
          background: "#fff",
          color: "#13261d",
          font: '15px/1.55 -apple-system,BlinkMacSystemFont,"Segoe UI",Inter,Arial,sans-serif',
        }}
      >
        <main
          data-error-digest={_error.digest}
          style={{
            maxWidth: 560,
            margin: "0 auto",
            padding: "120px 24px",
            textAlign: "center",
          }}
        >
          <h1 style={{ fontSize: 32, letterSpacing: "-1px", margin: "0 0 12px" }}>
            Произошла ошибка
          </h1>
          <p style={{ color: "#53645b", margin: "0 0 20px" }}>
            Приложение временно недоступно. Обновите страницу или вернитесь
            немного позже.
          </p>
          <button
            onClick={() => reset()}
            style={{
              padding: "11px 18px",
              border: "1px solid #15956a",
              background: "#15956a",
              color: "#fff",
              borderRadius: 999,
              fontWeight: 800,
              cursor: "pointer",
            }}
          >
            Повторить
          </button>
        </main>
      </body>
    </html>
  );
}
